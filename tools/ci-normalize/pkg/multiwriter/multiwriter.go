package multiwriter

import (
	"fmt"
	"reflect"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/writer"
	"github.com/gravitational/trace"
)

type recordWrapper struct {
	record any
	done   chan error
}

type fileWriter struct {
	ch   chan recordWrapper
	done chan struct{}
}

type Option func(*MultiWriter) error

type MultiWriter struct {
	writers     map[string]*fileWriter  // destination to writer
	typeMap     map[reflect.Type]string // record type to destination
	defaultPath string                  // path for default writer
}

func New(opts ...Option) (*MultiWriter, error) {
	mw := &MultiWriter{
		writers: make(map[string]*fileWriter),
		typeMap: make(map[reflect.Type]string),
	}

	for _, opt := range opts {
		if err := opt(mw); err != nil {
			return nil, trace.Wrap(err)
		}
	}

	if mw.defaultPath == "" {
		w, err := writer.New("jsonl", "-")
		if err != nil {
			return nil, trace.Wrap(err)
		}
		mw.defaultPath = "-"
		mw.writers[mw.defaultPath] = newFileWriter(w)
	}

	return mw, nil
}

// WithWriter adds a type-specific writer to MultiWriter
func WithWriter(recordPrototype any, path string, format string) Option {
	return func(mw *MultiWriter) error {
		t := reflect.TypeOf(recordPrototype)
		if _, ok := mw.writers[path]; !ok {
			w, err := writer.New(format, path)
			if err != nil {
				return err
			}
			mw.writers[path] = newFileWriter(w)
		}

		mw.typeMap[t] = path
		return nil
	}
}

// WithDefaultWriter sets the default writer used for unmatched types
func WithDefaultWriter(path string, format string) Option {
	return func(mw *MultiWriter) error {
		if _, ok := mw.writers[path]; !ok {
			w, err := writer.New(format, path)
			if err != nil {
				return err
			}
			mw.writers[path] = newFileWriter(w)
		}
		mw.defaultPath = path
		return nil
	}
}

func newFileWriter(w writer.Writer) *fileWriter {
	fw := &fileWriter{
		ch:   make(chan recordWrapper, 100),
		done: make(chan struct{}),
	}

	go func() {
		for rw := range fw.ch {
			err := w.Write(rw.record)
			if rw.done != nil {
				rw.done <- err
			}
		}
		w.Close()
		close(fw.done)
	}()

	return fw
}

func (mw *MultiWriter) Write(record any) error {
	t := reflect.TypeOf(record)
	path, ok := mw.typeMap[t]
	if !ok {
		path = mw.defaultPath
	}

	fw, ok := mw.writers[path]
	if !ok {
		return fmt.Errorf("no writer found for path %q", path)
	}

	fw.ch <- recordWrapper{record: record}
	return nil
}

func (mw *MultiWriter) Close() error {
	for _, fw := range mw.writers {
		close(fw.ch)
		<-fw.done
	}
	return nil
}
