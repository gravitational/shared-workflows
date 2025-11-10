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

package locking

import (
	"context"
	"sync"
)

// Loosely based on https://h12.io/article/go-pattern-context-aware-lock

// ContextLock is a mutex lock with context cancellation support.
type ContextLock struct {
	// This makes linters/go vet warn about copying types that contain a context lock
	_ sync.Mutex

	lock      chan struct{}
	closeLock func()
}

func NewContextLock() *ContextLock {
	cl := &ContextLock{
		// The lock is held when the buffered channel is full
		lock: make(chan struct{}, 1),
	}
	cl.closeLock = sync.OnceFunc(func() { close(cl.lock) })

	return cl
}

// Attempts to aquire the lock until the context expires. If the context expires prior
// to aquiring the lock, the context cancellation error is returned.
func (cl *ContextLock) Lock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case cl.lock <- struct{}{}:
		return nil
	}
}

func (cl *ContextLock) Unlock() {
	<-cl.lock
}

func (cl *ContextLock) CloseCtx(ctx context.Context) error {
	// Aquire the lock to make sure nothing else breaks
	if err := cl.Lock(ctx); err != nil {
		return err
	}

	cl.closeLock()
	return nil
}

func (cl *ContextLock) Close() {
	_ = cl.CloseCtx(context.Background())
}
