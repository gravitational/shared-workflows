// Copyright 2026 Gravitational, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dispatch

import (
	"context"
	"slices"
	"sync"

	"github.com/gravitational/shared-workflows/tools/ci-normalize/record"
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

		for {
			select {
			case <-ctx.Done():
				bw.err = ctx.Err()
				return

			case r, ok := <-bw.ch:
				if !ok {
					return
				}
				if err := w.Write(r); err != nil {
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
	ctx context.Context

	suiteWriters []*bufferedWriter
	testWriters  []*bufferedWriter
	metaWriters  []*bufferedWriter

	// mu protects below:
	mu sync.Mutex
	// bySink map of sink keys to writers
	bySink map[string]*bufferedWriter
}

var _ record.Writer = (*Dispatcher)(nil)

type Option func(*Dispatcher) error

// New creates a new [Dispatcher] with the given writers.
func New(ctx context.Context, suiteWriters, testWriters, metaWriters []RecordWriter) (*Dispatcher, error) {
	d := &Dispatcher{
		ctx:    ctx,
		bySink: make(map[string]*bufferedWriter),
	}

	for _, opt := range opts {
		if err := opt(d); err != nil {
			return nil, err
		}
	}

	if len(d.suiteWriters) == 0 &&
		len(d.testWriters) == 0 &&
		len(d.metaWriters) == 0 {
		return nil, trace.BadParameter("no writers registered")
	}

	return d, nil
}

// WithSuiteWriter registers a RecordWriter for suite records.
// Example:
//
//	disp, err := New(context.Background(),
//		WithSuiteWriter(writerA),
//		WithSuiteWriter(writerB),
//	)
func WithSuiteWriter(w RecordWriter) Option {
	return func(d *Dispatcher) error {
		sw := d.getBufferedWriter(w)
		if slices.Contains(d.suiteWriters, sw) {
			return trace.BadParameter(
				"duplicate suite writer for sink %q",
				w.SinkKey(),
			)
		}
		d.suiteWriters = append(d.suiteWriters, sw)
		return nil
	}
}

func WithTestcaseWriter(w RecordWriter) Option {
	return func(d *Dispatcher) error {
		sw := d.getBufferedWriter(w)
		if slices.Contains(d.testWriters, sw) {
			return trace.BadParameter(
				"duplicate testcase writer for sink %q",
				w.SinkKey(),
			)
		}
		d.testWriters = append(d.testWriters, sw)
		return nil
	}
}

func WithMetaWriter(w RecordWriter) Option {
	return func(d *Dispatcher) error {
		sw := d.getBufferedWriter(w)
		if slices.Contains(d.metaWriters, sw) {
			return trace.BadParameter(
				"duplicate meta writer for sink %q",
				w.SinkKey(),
			)
		}
		d.metaWriters = append(d.metaWriters, sw)
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

func (d *Dispatcher) WriteSuite(r *record.Suite) error {
	return writeAll(d.ctx, d.suiteWriters, r)
}

func (d *Dispatcher) WriteTestcase(r *record.Testcase) error {
	return writeAll(d.ctx, d.testWriters, r)
}

func (d *Dispatcher) WriteMeta(r *record.Meta) error {
	return writeAll(d.ctx, d.metaWriters, r)
}

func writeAll(ctx context.Context, writers []*bufferedWriter, record any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if len(writers) == 0 {
		return trace.BadParameter("no writers registered")
	}

	var errs []error
	for _, bw := range writers {
		if err := bw.Write(record); err != nil {
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

	for _, bw := range d.suiteWriters {
		seen[bw] = struct{}{}
	}
	for _, bw := range d.testWriters {
		seen[bw] = struct{}{}
	}
	for _, bw := range d.metaWriters {
		seen[bw] = struct{}{}
	}

	for bw := range seen {
		bw := bw
		g.Go(func() error {
			return bw.Close()
		})
	}

	select {
	case <-d.ctx.Done():
		return d.ctx.Err()
	default:
		return g.Wait()
	}
}
