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

// Copy is a contextual version of [io.Copy]. If the context is cancelled, calls to
// read and write functions will error, stopping the copy.
// This retains all properties of [io.Copy], including support for [io.WriterTo] and
// [io.ReaderFrom].
func Copy(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	if dstWriterTo, ok := dst.(writerToWriter); ok {
		dst = newContextWriterTo(ctx, dstWriterTo)
	} else {
		dst = newContextWriter(ctx, dst)
	}

	if srcReaderFrom, ok := src.(readerFromReader); ok {
		src = newContextReaderFrom(ctx, srcReaderFrom)
	} else {
		src = newContextReader(ctx, src)
	}

	return io.Copy(dst, src)
}

// copyConcurrently copies all context from src to dst without blocking. When the copy is complete
// (or it fails), an error will be sent and the channel will close.
// This calls [Copy], so it inherits all properties of [Copy].
// The returned channel will be closed when the internal goroutine exits, so it should not be closed
// by the caller.
func CopyConcurrently(ctx context.Context, dst io.Writer, src io.Reader) <-chan error {
	done := make(chan error, 1)

	go func() {
		defer close(done)

		_, err := Copy(ctx, dst, src)
		done <- err
	}()

	return done
}
