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
