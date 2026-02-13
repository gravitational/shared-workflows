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
	"io"
	"os"
	"testing"
	"time"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/record"
	"github.com/gravitational/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type passThroughEncoder struct {
	w io.Writer
}

func (e *passThroughEncoder) Encode(v any) error {
	b, ok := v.([]byte)
	if !ok {
		return trace.BadParameter("passThroughEncoder expects []byte")
	}
	_, err := e.w.Write(b)
	return trace.Wrap(err)
}

// factory matching EncoderFactory(io.WriteCloser) Encoder
func passThroughEncoderFactory(w io.Writer) Encoder {
	return &passThroughEncoder{w: w}
}

func TestNew_WriterOutputs(t *testing.T) {
	tmpFile := t.TempDir() + "/file.txt"

	tests := []struct {
		name        string
		path        string
		wantSinkKey string
	}{
		{
			name:        "stdout path",
			path:        "-",
			wantSinkKey: "stdout",
		},
		{
			name:        "empty path defaults to stdout",
			path:        "",
			wantSinkKey: "stdout",
		},
		{
			name:        "/dev/null",
			path:        "/dev/null",
			wantSinkKey: "/dev/null",
		},
		{
			name:        "local file",
			path:        tmpFile,
			wantSinkKey: tmpFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := New(t.Context(), tt.path, nil, passThroughEncoderFactory)
			require.NoError(t, err)
			require.NotNil(t, w)

			assert.Equal(t, tt.wantSinkKey, w.SinkKey())

			data := []byte("hello world")
			err = w.Write(data)
			require.NoError(t, err)
			require.NoError(t, w.Close())

			// If it's a file, verify content
			if tt.path != "-" && tt.path != "" && tt.path != "/dev/null" {
				b, err := os.ReadFile(tt.path)
				require.NoError(t, err)
				assert.Equal(t, "hello world", string(b))
			}
		})
	}
}

func TestRenderJinjaPathFromMeta(t *testing.T) {
	ts := time.Date(2026, 1, 16, 15, 4, 5, 0, time.UTC)
	meta := &record.Meta{
		MetaID:              "abc123",
		RecordSchemaVersion: "v1",
		CanonicalMeta: record.CanonicalMeta{
			RepositoryName: "repo/test",
		},
		Timestamp: ts.Format(time.RFC3339),
	}

	template := "/out/{{REPOSITORY}}/{{YEAR}}/{{MONTH}}/{{DAY}}/{{TIMESTAMP}}/{{ID}}_{{META_VERSION}}.json"
	path := renderJinjaPathFromMeta(template, meta)

	assert.NotContains(t, path, "{{REPOSITORY}}")
	assert.NotContains(t, path, "{{YEAR}}")
	assert.NotContains(t, path, "{{MONTH}}")
	assert.NotContains(t, path, "{{DAY}}")
	assert.NotContains(t, path, "{{TIMESTAMP}}")
	assert.NotContains(t, path, "{{ID}}")
	assert.NotContains(t, path, "{{META_VERSION}}")
}

func TestRenderJinjaPathFromMeta_NilMeta(t *testing.T) {
	template := "/foo/{{ID}}.json"
	path := renderJinjaPathFromMeta(template, nil)
	assert.Equal(t, template, path)
}

func TestRenderJinjaPathFromMeta_EmptyTemplate(t *testing.T) {
	path := renderJinjaPathFromMeta("", &record.Meta{MetaID: "foo"})
	assert.Equal(t, "", path)
}
