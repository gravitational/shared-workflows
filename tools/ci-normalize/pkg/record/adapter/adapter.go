package adapter

import (
	"io"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/encoder"
)

// Writer accepts records and writes them via an Encoder.
type Adapter struct {
	enc encoder.Encoder
	out io.WriteCloser
}

func New(enc encoder.Encoder, out io.WriteCloser) *Adapter {
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
