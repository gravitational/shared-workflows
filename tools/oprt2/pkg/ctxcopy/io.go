/*
 *  Copyright 2025 Gravitational, Inc
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package ctxcopy

import (
	"context"
	"io"
)

// Implement Reader, Writer, ReaderFrom, WriterFrom interfaces via wrappers that are context cancellable.

// Readers

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func newContextReader(ctx context.Context, reader io.Reader) *contextReader {
	return &contextReader{
		ctx:    ctx,
		reader: reader,
	}
}

var _ io.Reader = &contextReader{}

// Read satisfies the [io.Read] interface.
func (cr *contextReader) Read(p []byte) (int, error) {
	select {
	case <-cr.ctx.Done():
		return 0, cr.ctx.Err()
	default:
	}

	return cr.reader.Read(p)
}

type readerFromReader interface {
	io.Reader
	io.ReaderFrom
}

type contextReaderFrom struct {
	*contextReader
	reader io.ReaderFrom
}

var _ io.Reader = &contextReaderFrom{}
var _ io.ReaderFrom = &contextReaderFrom{}

func newContextReaderFrom(ctx context.Context, reader readerFromReader) *contextReaderFrom {
	return &contextReaderFrom{
		contextReader: newContextReader(ctx, reader),
		reader:        reader,
	}
}

// ReadFrom satisfies the [io.ReaderFrom] interface
func (crf *contextReaderFrom) ReadFrom(r io.Reader) (int64, error) {
	select {
	case <-crf.ctx.Done():
		return 0, crf.ctx.Err()
	default:
	}

	return crf.reader.ReadFrom(r)
}

// Writers

type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func newContextWriter(ctx context.Context, writer io.Writer) *contextWriter {
	return &contextWriter{
		ctx:    ctx,
		writer: writer,
	}
}

var _ io.Writer = &contextWriter{}

// Write satisfies the [io.Write] interface.
func (cw *contextWriter) Write(p []byte) (int, error) {
	select {
	case <-cw.ctx.Done():
		return 0, cw.ctx.Err()
	default:
	}

	return cw.writer.Write(p)
}

type writerToWriter interface {
	io.Writer
	io.WriterTo
}

type contextWriterTo struct {
	*contextWriter
	writer io.WriterTo
}

var _ io.Writer = &contextWriterTo{}
var _ io.WriterTo = &contextWriterTo{}

func newContextWriterTo(ctx context.Context, writer writerToWriter) *contextWriterTo {
	return &contextWriterTo{
		contextWriter: newContextWriter(ctx, writer),
		writer:        writer,
	}
}

// WriteTo satisfies the [io.WriterTo] interface.
func (cwt *contextWriterTo) WriteTo(w io.Writer) (int64, error) {
	select {
	case <-cwt.ctx.Done():
		return 0, cwt.ctx.Err()
	default:
	}

	return cwt.writer.WriteTo(w)
}
