package writer

import (
	"io"
	"os"
	"strings"
)

// RecordWriter accepts records and writes them to a sink.
// Implemented by the encoded writer adapter.
type KeyedWriter interface {
	io.WriteCloser
	SinkKey() string
}

func New(path string) (KeyedWriter, error) {
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
			return nil, err
		}
		return &fileWriter{
			WriteCloser: f,
			sink:        path,
		}, nil
	}
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
