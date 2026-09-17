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

package athena

import (
	"iter"
	"strconv"

	"github.com/gravitational/trace"
)

// Row is one row of a [Result], positionally aligned with [Result.Columns].
type Row []*string

// Scanner decodes the rows of a [Result] into values of type T.
//
// Build one with [NewScanner], chain a call per column, then range over
// [Scanner.Scan]:
//
//	scanner := athena.NewScanner[flakyRow](result).
//		Str("test_name", func(r *flakyRow, v string) { r.TestName = v }).
//		Int64("execs", func(r *flakyRow, v int64) { r.Execs = v })
//
//	for row, err := range scanner.Scan() {
//		if err != nil {
//			return trace.Wrap(err)
//		}
//		out = append(out, row)
//	}
type Scanner[T any] struct {
	result *Result

	// cols maps column name to position, resolved once from the result.
	cols map[string]int

	fields []scanField[T]

	// err is the first declaration failure.
	err error
}

// scanField is one declared column and the decoder that fills it.
type scanField[T any] struct {
	column string
	index  int
	decode func(*T, *string) error
}

// NewScanner builds a [Scanner] over the rows of result.
func NewScanner[T any](result *Result) *Scanner[T] {
	s := &Scanner[T]{
		result: result,
		cols:   make(map[string]int, len(result.Columns)),
	}

	for i, name := range result.Columns {
		if _, dup := s.cols[name]; dup {
			s.err = trace.BadParameter("result has duplicate column %q", name)
			return s
		}
		s.cols[name] = i
	}

	// Since we know all rows up front just do a quick scan to make sure we have no malformed rows.
	for _, row := range result.Rows {
		if len(row) != len(result.Columns) {
			s.err = trace.BadParameter("result contains invalid rows with length:%d, column count:%d", len(row), len(result.Columns))
			return s
		}
	}

	return s
}

// Err returns the first declaration failure, if any, so a caller can fail
// before iterating. [Scanner.Scan] reports the same error, so checking this is
// optional.
func (s *Scanner[T]) Err() error {
	return s.err
}

func (s *Scanner[T]) field(col string, decode func(*T, *string) error) *Scanner[T] {
	if s.err != nil {
		return s
	}

	i, ok := s.cols[col]
	if !ok {
		s.err = trace.NotFound("no column %q in result", col)
		return s
	}

	s.fields = append(s.fields, scanField[T]{column: col, index: i, decode: decode})
	return s
}

// Str declares a string column. A NULL decodes as the empty string
func (s *Scanner[T]) Str(col string, set func(*T, string)) *Scanner[T] {
	return s.field(col, func(t *T, cell *string) error {
		if cell == nil {
			set(t, "")
			return nil
		}
		set(t, *cell)
		return nil
	})
}

// Int64 declares an integer column. A NULL decodes as 0.
func (s *Scanner[T]) Int64(col string, set func(*T, int64)) *Scanner[T] {
	return s.field(col, func(t *T, cell *string) error {
		if cell == nil {
			set(t, 0)
			return nil
		}

		n, err := strconv.ParseInt(*cell, 10, 64)
		if err != nil {
			return trace.Wrap(err, "column %q is not an integer", col)
		}
		set(t, n)
		return nil
	})
}

// Float64 declares a floating point column. A NULL decodes as 0.
func (s *Scanner[T]) Float64(col string, set func(*T, float64)) *Scanner[T] {
	return s.field(col, func(t *T, cell *string) error {
		if cell == nil {
			set(t, 0)
			return nil
		}

		f, err := strconv.ParseFloat(*cell, 64)
		if err != nil {
			return trace.Wrap(err, "column %q is not a number", col)
		}
		set(t, f)
		return nil
	})
}

// Scan iterates the rows, yielding a decoded T for each.
func (s *Scanner[T]) Scan() iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		if len(s.result.Rows) == 0 {
			return
		}

		if s.err != nil {
			yield(*new(T), s.err)
			return
		}

		for i, row := range s.result.Rows {
			var v T
			if err := s.decodeRow(&v, row); err != nil {
				yield(*new(T), trace.Wrap(err, "decoding row %d", i))
				return
			}

			if !yield(v, nil) {
				return
			}
		}
	}
}

// decodeRow fills v from one row.
func (s *Scanner[T]) decodeRow(v *T, row Row) error {
	if len(row) != len(s.fields) {
		return trace.BadParameter("row len(%d) mismatch with column len(%d)!", len(row), len(s.fields))
	}

	for _, f := range s.fields {
		if err := f.decode(v, row[f.index]); err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}
