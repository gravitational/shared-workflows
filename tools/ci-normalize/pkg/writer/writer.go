package writer

import (
	"io"
	"os"
	"strings"
)

func New(path string) (io.WriteCloser, error) {
	switch path {
	case "-", "":
		return nopCloser{os.Stdout}, nil

	case "/dev/null":
		return &nopCloser{io.Discard}, nil

	default:
		if strings.HasPrefix(path, "s3://") {
			return newS3Writer(path)
		}
		return os.Create(path)
	}
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
