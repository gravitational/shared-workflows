package dispatch

import (
	"reflect"
	"sync"

	"github.com/gravitational/trace"
	"golang.org/x/sync/errgroup"
)

type RecordWriter interface {
	Write(any) error
	SinkKey() string
	Close() error
}

type safeWriter struct {
	w    RecordWriter
	ch   chan any
	once sync.Once
	done chan struct{} // closed when writer goroutine exits
	err  error         // last error (nil if finished successfully)

}

func (sw *safeWriter) Write(record any) error {
	select {
	case <-sw.done:
		return sw.err
	case sw.ch <- record:
		return nil
	}
}

func (sw *safeWriter) Close() error {
	sw.once.Do(func() {
		close(sw.ch)
	})
	<-sw.done
	return sw.err
}

func newSafeWriter(w RecordWriter) *safeWriter {
	safe := &safeWriter{
		w:  w,
		ch: make(chan any, 256),
		// err: make(chan error, 1),
		done: make(chan struct{}),
	}

	go func() {
		var err error
		for r := range safe.ch {
			if err = w.Write(r); err != nil {
				break
			}
		}
		safe.err = err
		w.Close()
		close(safe.done)
	}()

	return safe
}

// Dispatcher routes records to writers based on record type.
// It is safe for concurrent use.
type Dispatcher struct {
	byType map[reflect.Type][]*safeWriter
	def    []*safeWriter // default writers if type not registered

	mu     sync.Mutex             // Protects below:
	bySink map[string]*safeWriter // Output format is a global flag, dedup on destination for interleaved files.
}

type Option func(*Dispatcher) error

func New(opts ...Option) (*Dispatcher, error) {
	d := &Dispatcher{
		byType: make(map[reflect.Type][]*safeWriter),
		bySink: make(map[string]*safeWriter),
	}

	for _, opt := range opts {
		if err := opt(d); err != nil {
			return nil, err
		}
	}

	return d, nil
}

func WithWriter(recordPrototype any, w RecordWriter) Option {
	return func(d *Dispatcher) error {
		t := reflect.TypeOf(recordPrototype)
		sw := d.getSafeWriter(w)
		d.byType[t] = append(d.byType[t], sw)
		return nil
	}
}

func WithDefaultWriter(w RecordWriter) Option {
	return func(d *Dispatcher) error {
		d.def = append(d.def, d.getSafeWriter(w))
		return nil
	}
}

func (d *Dispatcher) getSafeWriter(w RecordWriter) *safeWriter {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := w.SinkKey()

	if sw, ok := d.bySink[key]; ok {
		return sw
	}

	sw := newSafeWriter(w)
	d.bySink[key] = sw
	return sw
}

func (d *Dispatcher) Write(record any) error {
	t := reflect.TypeOf(record)

	writers := d.byType[t]
	if len(writers) == 0 {
		writers = d.def
	}

	if len(writers) == 0 {
		return trace.BadParameter("no writer registered for record type %v", t)
	}

	var errs []error
	seen := map[*safeWriter]struct{}{}
	for _, sw := range writers {
		if _, ok := seen[sw]; ok {
			continue
		}
		seen[sw] = struct{}{}
		if err := sw.Write(record); err != nil {
			errs = append(errs, err)
		}
	}

	return trace.NewAggregate(errs...)
}

// Close gracefully shuts down all writers concurrently.
// Blocks until all queued records are written.
func (d *Dispatcher) Close() error {
	var g errgroup.Group
	seen := map[*safeWriter]struct{}{}

	for _, ws := range d.byType {
		for _, sw := range ws {
			seen[sw] = struct{}{}
		}
	}
	for _, sw := range d.def {
		seen[sw] = struct{}{}
	}

	for sw := range seen {
		sw := sw // capture
		g.Go(func() error {
			return sw.Close()
		})
	}

	return g.Wait()
}
