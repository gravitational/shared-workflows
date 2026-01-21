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

package dispatch

import (
	"sync"
	"testing"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWriter struct {
	mu       sync.Mutex
	records  []any
	sink     string
	failNext bool
	closed   bool
}

func (m *mockWriter) Write(r any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.failNext {
		return trace.BadParameter("fail write")
	}

	m.records = append(m.records, r)
	return nil
}

func (m *mockWriter) SinkKey() string {
	return m.sink
}

func (m *mockWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func TestDispatcher_WriteAndClose(t *testing.T) {
	type MyRecord struct {
		Value string
	}

	// Prepare writers
	writerA := &mockWriter{sink: "A"}
	writerB := &mockWriter{sink: "B"}

	disp, err := New(t.Context(),
		WithWriter(MyRecord{}, writerA),
		WithWriter(MyRecord{}, writerB), // two writers for same type
	)
	require.NoError(t, err)

	rec1 := MyRecord{"first"}
	rec2 := MyRecord{"second"}

	// Write two records
	require.NoError(t, disp.Write(rec1))
	require.NoError(t, disp.Write(rec2))

	// Allow writers to process
	assert.NoError(t, disp.Close())

	// Assert all records received
	assert.Equal(t, []any{rec1, rec2}, writerA.records)
	assert.Equal(t, []any{rec1, rec2}, writerB.records)

	// All writers closed and flushed
	assert.True(t, writerA.closed)
	assert.True(t, writerB.closed)
}

func TestDispatcher_UnregisteredTypeFails(t *testing.T) {
	type UnknownRecord struct{ ID int }

	disp, err := New(t.Context())
	require.NoError(t, err)

	rec := UnknownRecord{42}
	err = disp.Write(rec)
	require.ErrorContains(t, err, "no writer registered")

	// Close to flush
	require.NoError(t, disp.Close())

}

func TestDispatcher_DedupWriters(t *testing.T) {
	type R struct{ Val string }

	writer := &mockWriter{sink: "same"}

	disp, err := New(t.Context(),
		WithWriter(R{}, writer),
		WithWriter(R{}, writer), // duplicate writer for same type
	)
	require.NoError(t, err)

	rec := R{"x"}
	require.NoError(t, disp.Write(rec))
	require.NoError(t, disp.Close())

	// Writer should only receive record once per Write call
	assert.Equal(t, []any{rec}, writer.records)
	assert.True(t, writer.closed)
}

func TestDispatcher_WriteFailureOnFlush(t *testing.T) {
	type R struct{ V string }

	writer := &mockWriter{sink: "fail", failNext: true}
	disp, err := New(t.Context(), WithWriter(R{}, writer))
	require.NoError(t, err)

	_ = disp.Write(R{"oops"})
	err = disp.Close()
	require.Error(t, err)
	assert.ErrorContains(t, err, "fail write")
}

func TestDispatcher_WriteFailure(t *testing.T) {
	type R struct{ V string }

	writer := &mockWriter{sink: "fail", failNext: true}
	disp, err := New(t.Context(), WithWriter(R{}, writer))
	require.NoError(t, err)
	t.Cleanup(func() { _ = disp.Close() })

	for range 512 {
		_ = disp.Write(R{"oops"})
	}
	err = disp.Write(R{"oops"})
	require.Error(t, err)
	assert.ErrorContains(t, err, "fail write")
}

func TestDispatcher_NoWriters(t *testing.T) {
	type MyRecord struct {
		Value string
	}

	disp, err := New(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = disp.Close() })
	assert.ErrorContains(t, disp.Write(MyRecord{"rec"}), "no writer registered")
}
