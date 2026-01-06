package multiwriter

import (
	"reflect"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/pkg/writer"
)

type Option func(*MultiWriter) error

type MultiWriter struct {
	writers map[reflect.Type]writer.Writer
}

type defaultWriterSentinel struct{}

func New(format string, opts ...Option) (*MultiWriter, error) {
	mw := &MultiWriter{
		writers: make(map[reflect.Type]writer.Writer),
	}

	for _, opt := range opts {
		if err := opt(mw); err != nil {
			return nil, err
		}
	}

	// Any unmatched writes go to default
	w, err := writer.New(format, "-")
	if err != nil {
		return nil, err
	}
	mw.writers[reflect.TypeOf(defaultWriterSentinel{})] = w

	return mw, nil
}

func WithWriter(recordPrototype any, path string, format string) Option {
	return func(mw *MultiWriter) error {
		w, err := writer.New(format, path)
		if err != nil {
			return err
		}
		t := reflect.TypeOf(recordPrototype)
		mw.writers[t] = w
		return nil
	}
}

// Write routes the record to the correct writer
func (mw *MultiWriter) Write(record any) error {
	t := reflect.TypeOf(record)

	if w, ok := mw.writers[t]; ok {
		return w.Write(record)
	}

	if w, ok := mw.writers[reflect.TypeOf(defaultWriterSentinel{})]; ok {
		return w.Write(record)
	}

	return nil
}

func (mw *MultiWriter) Close() error {
	var err error
	for _, w := range mw.writers {
		if e := w.Close(); e != nil {
			err = e
		}
	}
	return err
}
