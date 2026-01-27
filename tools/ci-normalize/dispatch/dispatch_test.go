package dispatch

import (
	"sync"
	"testing"

	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/record"
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
	writerA := &mockWriter{sink: "A"}
	writerB := &mockWriter{sink: "B"}

	disp, err := New(
		t.Context(),
		[]RecordWriter{writerA, writerB},
		[]RecordWriter{writerA},
		[]RecordWriter{writerA},
	)
	require.NoError(t, err)

	rec1 := &record.Suite{}
	rec2 := &record.Suite{}

	require.NoError(t, disp.WriteSuite(rec1))
	require.NoError(t, disp.WriteSuite(rec2))

	require.NoError(t, disp.Close())

	assert.Equal(t, []any{rec1, rec2}, writerA.records)
	assert.Equal(t, []any{rec1, rec2}, writerB.records)

	assert.True(t, writerA.closed)
	assert.True(t, writerB.closed)
}

func TestDispatcher_UnregisteredTypeFails(t *testing.T) {
	writer := &mockWriter{sink: "a"}

	disp, err := New(
		t.Context(),
		[]RecordWriter{writer},
		[]RecordWriter{writer},
		[]RecordWriter{},
	)
	require.ErrorContains(t, err, "no writers ")
	require.ErrorContains(t, err, "meta")
	require.Nil(t, disp)
}

func TestDispatcher_SameTypeDifferentSink(t *testing.T) {
	writerA := &mockWriter{sink: "a"}
	writerB := &mockWriter{sink: "b"}

	disp, err := New(
		t.Context(),
		[]RecordWriter{writerA},
		[]RecordWriter{writerA},
		[]RecordWriter{writerA, writerB},
	)
	require.NoError(t, err)

	rec := &record.Meta{}
	require.NoError(t, disp.WriteMeta(rec))
	require.NoError(t, disp.Close())

	assert.Equal(t, []any{rec}, writerA.records)
	assert.Equal(t, []any{rec}, writerB.records)
	assert.True(t, writerA.closed)
	assert.True(t, writerB.closed)
}

func TestDispatcher_DedupWritersSameSink(t *testing.T) {
	writer := &mockWriter{sink: "same"}
	unique := []RecordWriter{writer}
	duplicated := []RecordWriter{writer, writer}

	disp, err := New(t.Context(), unique, duplicated, unique)

	require.Error(t, err)
	require.Nil(t, disp)
	require.ErrorContains(t, err, "duplicate writer")
	require.ErrorContains(t, err, "testcase")
}

func TestDispatcher_WriteFailureOnFlush(t *testing.T) {
	writers := []RecordWriter{&mockWriter{sink: "fail", failNext: true}}

	disp, err := New(t.Context(), writers, writers, writers)
	require.NoError(t, err)

	_ = disp.WriteSuite(&record.Suite{})
	err = disp.Close()
	require.Error(t, err)
	assert.ErrorContains(t, err, "fail write")
}

func TestDispatcher_WriteFailure(t *testing.T) {
	writers := []RecordWriter{&mockWriter{sink: "fail", failNext: true}}

	disp, err := New(t.Context(), writers, writers, writers)
	require.NoError(t, err)
	t.Cleanup(func() { _ = disp.Close() })

	for range 512 {
		_ = disp.WriteSuite(&record.Suite{})
	}

	err = disp.WriteSuite(&record.Suite{})
	require.Error(t, err)
	assert.ErrorContains(t, err, "fail write")
}
