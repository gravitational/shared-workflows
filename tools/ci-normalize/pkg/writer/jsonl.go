package writer

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/gravitational/trace"
)

type JSONLWriter struct {
	out       io.Writer
	mu        sync.Mutex
	bufWriter *bufio.Writer
}

func NewJSONLWriter(path string) (*JSONLWriter, error) {
	var w io.Writer
	if path == "" || path == "-" {
		w = os.Stdout
	} else {
		f, err := os.Create(path)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		w = f
	}

	return &JSONLWriter{
		out:       w,
		bufWriter: bufio.NewWriter(w),
	}, nil
}

func (w *JSONLWriter) Write(record any) error {
	data, err := json.Marshal(record)
	if err != nil {
		return trace.Wrap(err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.bufWriter.Write(append(data, '\n'))
	return trace.Wrap(err)
}

func (w *JSONLWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.bufWriter != nil {
		if err := w.bufWriter.Flush(); err != nil {
			return trace.Wrap(err)
		}
	}

	if f, ok := w.out.(*os.File); ok && f != os.Stdout && f != os.Stderr {
		if err := f.Close(); err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}
