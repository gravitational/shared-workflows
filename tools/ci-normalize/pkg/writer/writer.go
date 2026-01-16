package writer

import (
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/record"
	"github.com/gravitational/trace"
)

// RecordWriter accepts records and writes them to a sink.
// Implemented by the encoded writer adapter.
type KeyedWriter interface {
	io.WriteCloser
	SinkKey() string
}

func New(path string, metadata *record.Meta) (KeyedWriter, error) {
	path = renderJinjaPathFromMeta(path, metadata)

	switch path {
	case "-", "":
		return &fileWriter{
			WriteCloser: nopCloser{os.Stdout},
			sink:        "stdout",
		}, nil

	case "/dev/null":
		return &fileWriter{
			WriteCloser: nopCloser{io.Discard},
			sink:        "null",
		}, nil

	default:
		if strings.HasPrefix(path, "s3://") {
			return newS3Writer(path)
		}

		f, err := os.Create(path)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		return &fileWriter{
			WriteCloser: f,
			sink:        path,
		}, nil
	}
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

type fileWriter struct {
	io.WriteCloser
	sink string
}

func (w *fileWriter) SinkKey() string {
	return w.sink
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
