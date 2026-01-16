package writer

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			wantSinkKey: "null",
		},
		{
			name:        "local file",
			path:        tmpFile,
			wantSinkKey: tmpFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := New(tt.path, nil)
			require.NoError(t, err)
			require.NotNil(t, w)

			assert.Equal(t, tt.wantSinkKey, w.SinkKey())

			data := []byte("hello world")
			n, err := w.Write(data)
			require.NoError(t, err)
			assert.Equal(t, len(data), n)
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
		Common: record.Common{
			ID:                  "abc123",
			RecordSchemaVersion: "v1",
		},
		CanonicalMeta: record.CanonicalMeta{

			Repository: "repo/test",
			Workflow:   "ci",
			Job:        "build",
			SHA:        "deadbeef",
		},
		Timestamp: ts.Format(time.RFC3339),
	}

	template := "/out/{{REPOSITORY}}/{{YEAR}}/{{MONTH}}/{{DAY}}/{{TIMESTAMP}}/{{ID}}_{{WORKFLOW}}_{{SHA}}_{{JOB}}_{{META_VERSION}}.json"
	path := renderJinjaPathFromMeta(template, meta)

	assert.Contains(t, path, "repo%2Ftest")
	assert.Contains(t, path, "2026/01/16")
	assert.Contains(t, path, "20260116T150405Z")
	assert.Contains(t, path, "abc123")
	assert.Contains(t, path, "ci")
	assert.Contains(t, path, "deadbeef")
	assert.Contains(t, path, "build")
	assert.Contains(t, path, "v1")
}

func TestRenderJinjaPathFromMeta_NilMeta(t *testing.T) {
	template := "/foo/{{ID}}.json"
	path := renderJinjaPathFromMeta(template, nil)
	assert.Equal(t, template, path)
}

func TestRenderJinjaPathFromMeta_EmptyTemplate(t *testing.T) {
	path := renderJinjaPathFromMeta("", &record.Meta{Common: record.Common{ID: "foo"}})
	assert.Equal(t, "", path)
}

func TestFileWriter_Close(t *testing.T) {
	var buf bytes.Buffer
	w := &fileWriter{WriteCloser: nopCloser{&buf}, sink: "buffer"}
	n, err := w.Write([]byte("data"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	require.NoError(t, w.Close())
	assert.Equal(t, "data", buf.String())
	assert.Equal(t, "buffer", w.SinkKey())
}
