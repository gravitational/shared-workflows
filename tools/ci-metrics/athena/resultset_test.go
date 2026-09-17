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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRow is the decode target for these tests.
type testRow struct {
	Name  string
	Execs int64
	Score float64
	Err   string
}

// collect drains a scanner, failing on the first error.
func collect[T any](t *testing.T, s *Scanner[T]) []T {
	t.Helper()

	var out []T
	for v, err := range s.Scan() {
		require.NoError(t, err)
		out = append(out, v)
	}
	return out
}

// scanErr drains a scanner and returns the error it reported
func scanErr[T any](s *Scanner[T]) ([]T, error) {
	var out []T
	for v, err := range s.Scan() {
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, nil
}

func TestScanDecodesTypedValues(t *testing.T) {
	t.Parallel()

	rs := &Result{
		Columns: []string{"test_name", "execs", "flake_score"},
		Rows: []Row{
			{aws.String("TestFoo"), aws.String("42"), aws.String("0.8712")},
		},
	}

	rows := collect(t, NewScanner[testRow](rs).
		Str("test_name", func(r *testRow, v string) { r.Name = v }).
		Int64("execs", func(r *testRow, v int64) { r.Execs = v }).
		Float64("flake_score", func(r *testRow, v float64) { r.Score = v }))

	require.Len(t, rows, 1)
	assert.Equal(t, "TestFoo", rows[0].Name)
	assert.Equal(t, int64(42), rows[0].Execs)
	assert.InDelta(t, 0.8712, rows[0].Score, 1e-9)
}

func TestScanEmptyResultYieldsNothing(t *testing.T) {
	t.Parallel()

	rows, err := scanErr(NewScanner[testRow](&Result{}).
		Str("test_name", func(r *testRow, v string) { r.Name = v }))

	require.NoError(t, err)
	assert.Empty(t, rows)
}

func TestScanErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		scanner   func() *Scanner[testRow]
		assertErr require.ErrorAssertionFunc
	}{
		"missing column": {
			scanner: func() *Scanner[testRow] {
				rs := &Result{Columns: []string{"a"}, Rows: []Row{{aws.String("1")}}}
				return NewScanner[testRow](rs).
					Str("nope", func(r *testRow, v string) { r.Name = v })
			},
			assertErr: func(t require.TestingT, err error, _ ...any) {
				require.True(t, trace.IsNotFound(err))
			},
		},
		"duplicate columns": {
			scanner: func() *Scanner[testRow] {
				rs := &Result{Columns: []string{"a", "a"}, Rows: []Row{{aws.String("1")}}}
				return NewScanner[testRow](rs).
					Str("a", func(r *testRow, v string) { r.Name = v })
			},
			assertErr: func(t require.TestingT, err error, _ ...any) {
				require.True(t, trace.IsBadParameter(err))
			},
		},
		"short row": {
			scanner: func() *Scanner[testRow] {
				rs := &Result{Columns: []string{"a", "b"}, Rows: []Row{{aws.String("1")}}}
				return NewScanner[testRow](rs).
					Str("b", func(r *testRow, v string) { r.Name = v })
			},
			assertErr: func(t require.TestingT, err error, _ ...any) {
				require.True(t, trace.IsBadParameter(err))
			},
		},
		"not an integer": {
			scanner: func() *Scanner[testRow] {
				rs := &Result{Columns: []string{"a"}, Rows: []Row{{aws.String("honestly_im_an_integer")}}}
				return NewScanner[testRow](rs).
					Int64("a", func(r *testRow, v int64) { r.Execs = v })
			},
			assertErr: require.Error,
		},
		"not a number": {
			scanner: func() *Scanner[testRow] {
				rs := &Result{Columns: []string{"a"}, Rows: []Row{{aws.String("dont_listen")}}}
				return NewScanner[testRow](rs).
					Float64("a", func(r *testRow, v float64) { r.Score = v })
			},
			assertErr: require.Error,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := scanErr(tt.scanner())
			tt.assertErr(t, err)
		})
	}
}

func TestScanKeepsTheFirstDeclarationError(t *testing.T) {
	t.Parallel()

	rs := &Result{Columns: []string{"a"}, Rows: []Row{{aws.String("1")}}}
	s := NewScanner[testRow](rs).
		Str("first_bad", func(r *testRow, v string) { r.Name = v }).
		Str("second_bad", func(r *testRow, v string) { r.Name = v })

	require.Error(t, s.Err())
	assert.Contains(t, s.Err().Error(), "first_bad")
	assert.NotContains(t, s.Err().Error(), "second_bad")
}

func TestScanStopsAtTheFirstBadRow(t *testing.T) {
	t.Parallel()

	rs := &Result{
		Columns: []string{"a"},
		Rows: []Row{
			{aws.String("7")},
			{aws.String("banana")},
			{aws.String("9")},
		},
	}

	rows, err := scanErr(NewScanner[testRow](rs).
		Int64("a", func(r *testRow, v int64) { r.Execs = v }))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "row 1")
	require.Len(t, rows, 1)
	assert.Equal(t, int64(7), rows[0].Execs)
}
