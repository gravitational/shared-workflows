// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package partition

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gravitational/trace"
)

// bucket represents one inclusive range selection for a query
type bucket struct {
	year      int
	month     time.Month
	firstDay  int
	lastDay   int
	fullMonth bool
}

// bucketsFromRange splits an inclusive day range into per cursor buckets suitable for Athena partitions.
// It is the responsiblity of the caller to ensure the locale has been set correctly.
func bucketsFromRange(from, to time.Time) []bucket {
	var out []bucket

	cursor := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, from.Location())
	for ; !cursor.After(to); cursor = cursor.AddDate(0, 1, 0) {
		monthEnd := cursor.AddDate(0, 1, -1)

		first, last := cursor, monthEnd
		if from.After(first) {
			first = from
		}
		if to.Before(last) {
			last = to
		}

		out = append(out, bucket{
			year:      cursor.Year(),
			month:     cursor.Month(),
			firstDay:  first.Day(),
			lastDay:   last.Day(),
			fullMonth: first.Equal(cursor) && last.Equal(monthEnd),
		})
	}

	return out
}

var aliasPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// day truncates t to nearest day.
func day(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// PeriodPredicate takes the alias for table and a time span [from, to] and produces a SQL predicate
// using the parition keys year=/month=/day= that the JSONL table uses.
func PeriodPredicate(alias string, from, to time.Time) (string, error) {
	if !aliasPattern.MatchString(alias) {
		return "", trace.BadParameter("invalid table alias %q", alias)
	}

	// Convert to UTC
	from, to = day(from.In(time.UTC)), day(to.In(time.UTC))
	if from.After(to) {
		return "", trace.BadParameter("invalid UTC time period %q -> %q",
			from.Format(time.DateOnly), to.Format(time.DateOnly))
	}

	var clauses []string
	for _, b := range bucketsFromRange(from, to) {
		var sb strings.Builder
		fmt.Fprintf(&sb, "%s.year = '%04d' AND %s.month = '%02d'",
			alias, b.year, alias, int(b.month))

		switch {
		case b.fullMonth:
		case b.firstDay == b.lastDay:
			fmt.Fprintf(&sb, " AND %s.day = '%02d'", alias, b.firstDay)
		default:
			fmt.Fprintf(&sb, " AND %s.day BETWEEN '%02d' AND '%02d'",
				alias, b.firstDay, b.lastDay)
		}

		clauses = append(clauses, "("+sb.String()+")")
	}

	switch len(clauses) {
	case 0:
		return "", trace.BadParameter("no valid clauses produced (this is a bug)")
	case 1:
		return clauses[0], nil
	default:
		return "(" + strings.Join(clauses, "\n       OR ") + ")", nil
	}
}
