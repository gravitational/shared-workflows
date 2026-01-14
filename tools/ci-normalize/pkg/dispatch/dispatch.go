package dispatch

import (
	"fmt"
	"reflect"
	"sync"

	"golang.org/x/sync/errgroup"
)

// RecordWriter accepts records and writes them to a sink.
// Implemented by the encoded writer adapter.
type RecordWriter interface {
	Write(any) error
	Close() error
}

type safeWriter struct {
	w    RecordWriter
	ch   chan any
	err  chan error
	once sync.Once
}

func newSafeWriter(w RecordWriter) *safeWriter {
	safe := &safeWriter{
		w:   w,
		ch:  make(chan any, 256),
		err: make(chan error, 1),
	}

	go func() {
		for r := range safe.ch {
			if err := w.Write(r); err != nil {
				safe.err <- err
				return
			}
		}

		safe.err <- w.Close()
	}()

	return safe
}

// Dispatcher routes records to writers based on record type.
// It is safe for concurrent use.
type Dispatcher struct {
	byType map[reflect.Type][]*safeWriter
	def    []*safeWriter // default writers if type not registered
}

// Option configures Dispatcher.
type Option func(*Dispatcher) error

// New creates a new Dispatcher.
func New(opts ...Option) (*Dispatcher, error) {
	mw := &Dispatcher{
		byType: make(map[reflect.Type][]*safeWriter),
	}

	for _, opt := range opts {
		if err := opt(mw); err != nil {
			return nil, err
		}
	}

	return mw, nil
}

// WithWriter registers a writer for a specific record type.
func WithWriter(recordPrototype any, w RecordWriter) Option {
	return func(mw *Dispatcher) error {
		t := reflect.TypeOf(recordPrototype)
		safe := newSafeWriter(w)
		mw.byType[t] = append(mw.byType[t], safe)
		return nil
	}
}

// WithDefaultWriter registers a writer for unmatched record types.
func WithDefaultWriter(w RecordWriter) Option {
	return func(mw *Dispatcher) error {
		mw.def = append(mw.def, newSafeWriter(w))
		return nil
	}
}

// Write routes a record to its writer(s).
func (mw *Dispatcher) Write(record any) error {
	t := reflect.TypeOf(record)

	writers := mw.byType[t]
	if len(writers) == 0 {
		writers = mw.def
	}

	if len(writers) == 0 {
		return fmt.Errorf("no writer registered for record type %v", t)
	}

	for _, l := range writers {
		select {
		case l.ch <- record:
		case err := <-l.err:
			return err
		}
	}

	return nil
}

// Close gracefully shuts down all writers concurrently.
// Blocks until all queued records are written.
func (mw *Dispatcher) Close() error {
	var g errgroup.Group

	seen := map[*safeWriter]struct{}{}

	for _, loops := range mw.byType {
		for _, l := range loops {
			seen[l] = struct{}{}
		}
	}

	for _, l := range mw.def {
		seen[l] = struct{}{}
	}

	for l := range seen {
		l := l // capture for closure
		g.Go(func() error {
			l.once.Do(func() {
				close(l.ch)
			})
			return <-l.err
		})
	}

	return g.Wait()
}
