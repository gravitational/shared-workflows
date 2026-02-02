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

package writer

import (
	"context"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/dispatch"
	"github.com/gravitational/shared-workflows/tools/ci-normalize/record"
	"github.com/gravitational/trace"
)

// Encoder defines an interface for encoding records.
type Encoder interface {
	Encode(any) error
}

// EncoderFactory is a factory function for creating Encoders.
type EncoderFactory func(io.Writer) Encoder

// encodedWriter implements [dispatch.RecordWriter].
// It wraps an [Encoder] and the underlying [io.WriteCloser].
type encodedWriter struct {
	enc    Encoder
	writer io.WriteCloser
	key    string
}

func (w *encodedWriter) Write(record any) error { return w.enc.Encode(record) }
func (w *encodedWriter) Close() error           { return w.writer.Close() }
func (w *encodedWriter) SinkKey() string        { return w.key }

func New(ctx context.Context, path string, metadata *record.Meta, encFactory EncoderFactory) (dispatch.RecordWriter, error) {
	path = renderJinjaPathFromMeta(path, metadata)

	var raw io.WriteCloser
	key := path

	switch path {
	case "-", "":
		raw = nopCloser{os.Stdout}
		key = "stdout"
	case "/dev/null":
		raw = nopCloser{io.Discard}
	default:
		if strings.HasPrefix(path, "s3://") {
			w, err := newS3Writer(ctx, path)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			raw = w
		} else {
			f, err := os.Create(path)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			raw = f
		}
	}

	return &encodedWriter{
		// Wrap the raw writer with the provided encoder
		enc:    encFactory(raw),
		writer: raw,
		key:    key,
	}, nil
}

func renderJinjaPathFromMeta(template string, meta *record.Meta) string {
	if template == "" || meta == nil {
		return template
	}

	ts := time.Now().UTC()
	if meta.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339, meta.Timestamp); err == nil {
			ts = t.UTC()
		}
	}

	path := template

	replacements := map[string]string{
		"REPOSITORY":   meta.RepositoryName,
		"YEAR":         ts.Format("2006"),
		"MONTH":        ts.Format("01"),
		"DAY":          ts.Format("02"),
		"TIMESTAMP":    ts.Format("20060102T150405Z"),
		"ID":           meta.ID,
		"META_VERSION": meta.RecordSchemaVersion,
	}

	for k, v := range replacements {
		placeholder := "{{" + k + "}}"
		path = strings.ReplaceAll(path, placeholder, url.PathEscape(v))
	}

	return path
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
