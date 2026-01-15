package adapter

import (
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/dispatch"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/encoder"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/writer"
)

// Writer accepts records and writes them via an Encoder.
type Adapter struct {
	enc encoder.Encoder
	out writer.KeyedWriter
}

func New(enc encoder.Encoder, out writer.KeyedWriter) *Adapter {
	return &Adapter{
		enc: enc,
		out: out,
	}
}

func (w *Adapter) Write(record any) error {
	return w.enc.Encode(record)
}

func (w *Adapter) Close() error {
	return w.out.Close()
}

func (w *Adapter) SinkKey() string {
	return w.out.SinkKey()
}

var _ dispatch.RecordWriter = (*Adapter)(nil)
