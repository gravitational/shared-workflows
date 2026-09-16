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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestPredicate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		alias    string
		from, to time.Time
		want     string
	}{
		{
			name:  "single day",
			alias: "t",
			from:  date(1978, time.December, 5),
			to:    date(1978, time.December, 5),
			want:  "(t.year = '1978' AND t.month = '12' AND t.day = '05')",
		},
		{
			name:  "range within one month",
			alias: "t",
			from:  date(1978, time.December, 5),
			to:    date(1978, time.December, 24),
			want:  "(t.year = '1978' AND t.month = '12' AND t.day BETWEEN '05' AND '24')",
		},
		{
			name:  "crosses a new year boundary",
			alias: "t",
			from:  date(1978, time.December, 5),
			to:    date(1979, time.January, 1),
			want: "((t.year = '1978' AND t.month = '12' AND t.day BETWEEN '05' AND '31')\n" +
				"       OR (t.year = '1979' AND t.month = '01' AND t.day = '01'))",
		},
		{
			name:  "single day either side of a new year",
			alias: "t",
			from:  date(2025, time.December, 31),
			to:    date(2026, time.January, 1),
			want: "((t.year = '2025' AND t.month = '12' AND t.day = '31')\n" +
				"       OR (t.year = '2026' AND t.month = '01' AND t.day = '01'))",
		},
		{
			name:  "whole month drops the day predicate",
			alias: "t",
			from:  date(1978, time.December, 1),
			to:    date(1978, time.December, 31),
			want:  "(t.year = '1978' AND t.month = '12')",
		},
		{
			name:  "a month missing its last day is not whole",
			alias: "t",
			from:  date(1978, time.December, 1),
			to:    date(1978, time.December, 30),
			want:  "(t.year = '1978' AND t.month = '12' AND t.day BETWEEN '01' AND '30')",
		},
		{
			name:  "full months within a range drop day predicate",
			alias: "p",
			from:  date(1978, time.July, 30),
			to:    date(1978, time.September, 2),
			want: "((p.year = '1978' AND p.month = '07' AND p.day BETWEEN '30' AND '31')\n" +
				"       OR (p.year = '1978' AND p.month = '08')\n" +
				"       OR (p.year = '1978' AND p.month = '09' AND p.day BETWEEN '01' AND '02'))",
		},
		{
			name:  "february in a leap year is a whole month",
			alias: "t",
			from:  date(2028, time.February, 1),
			to:    date(2028, time.February, 29),
			want:  "(t.year = '2028' AND t.month = '02')",
		},
		{
			name:  "february 1-28 in a leap year is not a whole month",
			alias: "t",
			from:  date(2028, time.February, 1),
			to:    date(2028, time.February, 28),
			want:  "(t.year = '2028' AND t.month = '02' AND t.day BETWEEN '01' AND '28')",
		},
		{
			name:  "times of day are truncated",
			alias: "t",
			from:  time.Date(1978, time.December, 14, 23, 59, 59, 0, time.UTC),
			to:    time.Date(1978, time.December, 14, 0, 0, 1, 0, time.UTC),
			want:  "(t.year = '1978' AND t.month = '12' AND t.day = '14')",
		},
		{
			name:  "non-UTC input is converted, not just truncated",
			alias: "t",
			// 1978-12-15T01:00 at UTC+2 is 1978-12-14T23:00Z, the day before.
			from: time.Date(1978, time.December, 15, 1, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60)),
			to:   date(1978, time.December, 14),
			want: "(t.year = '1978' AND t.month = '12' AND t.day = '14')",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := PeriodPredicate(tc.alias, tc.from, tc.to)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPredicateErrors(t *testing.T) {
	t.Parallel()

	t.Run("reversed range", func(t *testing.T) {
		_, err := PeriodPredicate("t", date(1978, time.December, 11), date(1978, time.December, 1))
		assert.Error(t, err)
	})

	t.Run("reversed across a year", func(t *testing.T) {
		_, err := PeriodPredicate("t", date(2025, time.December, 31), date(1978, time.January, 1))
		assert.Error(t, err)
	})

	t.Run("invalid alias", func(t *testing.T) {
		for _, alias := range []string{"", "1t", "t; DROP TABLE bobby tables", "t.u", "t-u"} {
			_, err := PeriodPredicate(alias, date(1978, time.December, 1), date(1978, time.December, 1))
			assert.Error(t, err, "alias %q should be rejected", alias)
		}
	})
}
