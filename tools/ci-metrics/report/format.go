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

package report

import (
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
)

// Int formats a count with thousands separators.
func Int(n int64) string {
	return humanize.Comma(n)
}

// Pct formats a percentage that is already on a 0-100 scale.
func Pct(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64) + "%"
}

// Day formats a date without its time.
func Day(t time.Time) string {
	return t.Format(time.DateOnly)
}

// Window formats an inclusive date range.
func Window(from, to time.Time) string {
	return Day(from) + " .. " + Day(to)
}

// Bytes formats a byte count in IEC units.
func Bytes(n int64) string {
	if n < 0 {
		n = 0
	}
	return humanize.IBytes(uint64(n))
}
