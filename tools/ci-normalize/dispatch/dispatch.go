package dispatch

import (
	"context"
	"reflect"
	"sync"

	"github.com/gravitational/trace"
	"golang.org/x/sync/errgroup"
)

// RecordWriter defines an interface for writing records to a sink.
type RecordWriter interface {
	Write(any) error
	SinkKey() string
	Close() error
}

// bufferedWriter wraps a RecordWriter with a buffered channel.
// Each bufferedWriter starts a goroutine that reads from the channel
// and writes to the underlying RecordWriter.
type bufferedWriter struct {
	ctx  context.Context
	w    RecordWriter
	ch   chan any
	once sync.Once
	done chan struct{} // closed when writer goroutine exits
	err  error         // last error (nil if finished successfully)

}

func (sw *bufferedWriter) Write(record any) error {
	select {
	case <-sw.ctx.Done():
		return sw.ctx.Err()
	case <-sw.done:
		return sw.err
	case sw.ch <- record:
		return nil
	}
}

func (sw *bufferedWriter) Close() error {
	sw.once.Do(func() {
		close(sw.ch)
	})

	select {
	case <-sw.done:
		return sw.err
	case <-sw.ctx.Done():
		return sw.ctx.Err()
	}
}

func newBufferedWriter(ctx context.Context, w RecordWriter) *bufferedWriter {
	bw := &bufferedWriter{
		ctx:  ctx,
		w:    w,
		ch:   make(chan any, 256),
		done: make(chan struct{}),
	}

	go func() {
		defer close(bw.done)
		defer func() {
			// Clean up path, ignore the error from underlying writer.
			_ = w.Close()
		}()
		var err error

		for {
			select {
			case <-ctx.Done():
				err = ctx.Err()
				bw.err = err
				return

			case r, ok := <-bw.ch:
				if !ok {
					return
				}
				if err = w.Write(r); err != nil {
					bw.err = err
					return
				}
			}
		}
	}()

	return bw
}

// Dispatcher routes records to appropriate writers based on their type.
// It supports multiple writers for the same record type.
// Writers are deduplicated based on their SinkKey.
type Dispatcher struct {
	ctx    context.Context
	byType map[reflect.Type][]*bufferedWriter

	// mu protects below:
	mu sync.Mutex
	// bySink map of sink keys to writers
	bySink map[string]*bufferedWriter
}

type Option func(*Dispatcher) error

// New creates a new [Dispatcher] with the given options.
func New(ctx context.Context, opts ...Option) (*Dispatcher, error) {
	d := &Dispatcher{
		ctx:    ctx,
		byType: make(map[reflect.Type][]*bufferedWriter),
		bySink: make(map[string]*bufferedWriter),
	}

	for _, opt := range opts {
		if err := opt(d); err != nil {
			return nil, err
		}
	}

	return d, nil
}

// WithWriter registers a RecordWriter for the given record type.
// Example:
//
//	disp, err := New(t.Context(),
//		WithWriter(FooRecord{}, writerA),
//		WithWriter(BarRecord{}, writerB),
//	)
func WithWriter(recordPrototype any, w RecordWriter) Option {
	return func(d *Dispatcher) error {
		t := reflect.TypeOf(recordPrototype)
		sw := d.getBufferedWriter(w)
		d.byType[t] = append(d.byType[t], sw)
		return nil
	}
}

func (d *Dispatcher) getBufferedWriter(w RecordWriter) *bufferedWriter {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := w.SinkKey()

	if sw, ok := d.bySink[key]; ok {
		return sw
	}

	sw := newBufferedWriter(d.ctx, w)
	d.bySink[key] = sw
	return sw
}

func (d *Dispatcher) Write(record any) error {
	select {
	case <-d.ctx.Done():
		return d.ctx.Err()
	default:
	}

	t := reflect.TypeOf(record)

	writers := d.byType[t]
	if len(writers) == 0 {
		return trace.BadParameter("no writer registered for record type %v", t)
	}

	var errs []error
	seen := map[*bufferedWriter]struct{}{}
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
	seen := map[*bufferedWriter]struct{}{}

	for _, ws := range d.byType {
		for _, sw := range ws {
			seen[sw] = struct{}{}
		}
	}

	for sw := range seen {
		sw := sw // capture
		g.Go(func() error {
			return sw.Close()
		})
	}

	select {
	case <-d.ctx.Done():
		return d.ctx.Err()
	default:
		return g.Wait()
	}
}
