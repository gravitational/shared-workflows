package adapter

import (
	"github.com/gravitational/shared-workflows/tools/ci-normalize/dispatch"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/writer"
)

type Encoder interface {
	Encode(any) error
}

// Adapter adapts a writer.KeyedWriter and encoder.Encoder to implement dispatch.RecordWriter.
type Adapter struct {
	enc Encoder
	out writer.KeyedWriter
}

// New creates a new Adapter.
func New(enc Encoder, out writer.KeyedWriter) *Adapter {
	return &Adapter{
		enc: enc,
		out: out,
	}
}

// Write encodes and writes the record.
func (w *Adapter) Write(record any) error {
	return w.enc.Encode(record)
}

// Close closes the underlying writer.
func (w *Adapter) Close() error {
	return w.out.Close()
}

// SinkKey returns the sink key of the underlying writer.
func (w *Adapter) SinkKey() string {
	return w.out.SinkKey()
}

var _ dispatch.RecordWriter = (*Adapter)(nil)
