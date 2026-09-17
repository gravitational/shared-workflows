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

package reporter

import (
	"context"
	"strings"
	"testing"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gravitational/shared-workflows/tools/ci-metrics/report"
)

// mockReporter is a reporter that also implements the optional
// [Preflighter]. See [mockPlainReporter] for one that does not.
type mockReporter struct {
	mock.Mock
}

var (
	_ Reporter    = (*mockReporter)(nil)
	_ Preflighter = (*mockReporter)(nil)
)

func (m *mockReporter) Name() string {
	return m.Called().String(0)
}

func (m *mockReporter) Report(ctx context.Context, doc *report.Document) error {
	return m.Called(ctx, doc).Error(0)
}

func (m *mockReporter) Preflight(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *mockReporter) Close() error {
	return m.Called().Error(0)
}

// mockPlainReporter is a [Reporter] with no [Reporter.Preflight]
type mockPlainReporter struct {
	mock.Mock
}

var _ Reporter = (*mockPlainReporter)(nil)

func (m *mockPlainReporter) Name() string {
	return m.Called().String(0)
}

func (m *mockPlainReporter) Report(ctx context.Context, doc *report.Document) error {
	return m.Called(ctx, doc).Error(0)
}

func (m *mockPlainReporter) Close() error {
	return m.Called().Error(0)
}

func newMockReporter(name string) *mockReporter {
	m := &mockReporter{}
	m.On("Name").Maybe().Return(name)
	return m
}

func TestMultiReportsToEveryReporter(t *testing.T) {
	t.Parallel()

	a := newMockReporter("a")
	a.On("Report", mock.Anything, mock.Anything).Return(nil).Once()
	b := newMockReporter("b")
	b.On("Report", mock.Anything, mock.Anything).Return(nil).Once()

	m := Multi{a, b}

	require.NoError(t, m.Report(t.Context(), &report.Document{}))
	a.AssertExpectations(t)
	b.AssertExpectations(t)
}

func TestMultiContinuesPastAFailure(t *testing.T) {
	t.Parallel()

	failing := newMockReporter("slack")
	failing.On("Report", mock.Anything, mock.Anything).
		Return(trace.Errorf("something terrible happened")).Once()
	healthy := newMockReporter("gha")
	healthy.On("Report", mock.Anything, mock.Anything).Return(nil).Once()

	m := Multi{failing, healthy}

	err := m.Report(t.Context(), &report.Document{})
	require.Error(t, err)

	healthy.AssertNumberOfCalls(t, "Report", 1)
	assert.ErrorContains(t, err, "slack")
	assert.ErrorContains(t, err, "something terrible happened")
}

func TestMultiAggregatesEveryFailure(t *testing.T) {
	t.Parallel()

	a := newMockReporter("a")
	a.On("Report", mock.Anything, mock.Anything).Return(trace.Errorf("first boom")).Once()
	b := newMockReporter("b")
	b.On("Report", mock.Anything, mock.Anything).Return(trace.Errorf("second boom")).Once()

	m := Multi{a, b}

	err := m.Report(t.Context(), &report.Document{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "first boom")
	assert.Contains(t, err.Error(), "second boom")
}

func TestMultiPreflightSkipsReportersWithout(t *testing.T) {
	t.Parallel()

	withPreflight := newMockReporter("slack")
	withPreflight.On("Preflight", mock.Anything).Return(nil).Once()

	without := &mockPlainReporter{}
	without.On("Name").Maybe().Return("stdout")

	m := Multi{withPreflight, without}
	require.NoError(t, m.Preflight(t.Context()))
	withPreflight.AssertExpectations(t)
}

func TestMultiPreflightReportsFailures(t *testing.T) {
	t.Parallel()

	bad := newMockReporter("slack")
	bad.On("Preflight", mock.Anything).Return(trace.Errorf("prelight has boomed")).Once()
	good := newMockReporter("gha")
	good.On("Preflight", mock.Anything).Return(nil).Maybe()

	m := Multi{bad, good}

	err := m.Preflight(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "slack")
	assert.Contains(t, err.Error(), "prelight has boomed")
}

func TestMultiClosesEveryReporter(t *testing.T) {
	t.Parallel()

	a := newMockReporter("a")
	a.On("Close").Return(trace.Errorf("close failed")).Once()
	b := newMockReporter("b")
	b.On("Close").Return(nil).Once()

	m := Multi{a, b}

	err := m.Close()
	require.Error(t, err)
	a.AssertNumberOfCalls(t, "Close", 1)
	b.AssertNumberOfCalls(t, "Close", 1)
}

func TestMulti(t *testing.T) {
	t.Parallel()

	t.Run("name is constructed correctly", func(t *testing.T) {
		a := &mockReporter{}
		a.On("Name").Return("gha")
		b := &mockReporter{}
		b.On("Name").Return("ci-health")
		m := Multi{a, b}
		assert.Equal(t, "gha,ci-health", m.Name())
	})

	t.Run("empty multi reporter is a noop", func(t *testing.T) {
		var m Multi
		ctx := t.Context()
		assert.NoError(t, m.Report(ctx, &report.Document{}))
		assert.NoError(t, m.Preflight(ctx))
		assert.NoError(t, m.Close())
		assert.Equal(t, "", m.Name())
	})
	t.Run("bad configs are rejected", func(t *testing.T) {
		_, err := New("foo", report.ReporterConfig{Type: "bar"}, nil)
		require.True(t, trace.IsBadParameter(err))

		_, err = New("foo", report.ReporterConfig{Type: ""}, nil)
		require.True(t, trace.IsBadParameter(err))

		var buf strings.Builder
		r, err := New("console", report.ReporterConfig{Type: TypeStdout, MaxRows: 5}, &buf)
		require.NoError(t, err)
		assert.Equal(t, "console", r.Name())
	})
}
