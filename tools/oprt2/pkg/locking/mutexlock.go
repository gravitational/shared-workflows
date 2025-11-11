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
	"fmt"
)

// Keyed mutex allows for locking and unlocking specific "keys". It handles key
// cleanup to prevent unbounded memory growth.
// Individual mutexes are stored on the heap.
type MutexMap[T comparable] struct {
	mapLock *ContextLock
	locks   map[T]struct {
		mutex    *ContextLock
		refCount uint
	}
}

func NewMutexMap[T comparable]() *MutexMap[T] {
	return &MutexMap[T]{
		mapLock: NewContextLock(),
		locks: map[T]struct {
			mutex    *ContextLock
			refCount uint
		}{},
	}
}

func (mm *MutexMap[T]) Lock(ctx context.Context, key T) error {
	if err := mm.mapLock.Lock(ctx); err != nil {
		return err
	}

	itemMutex, ok := mm.locks[key]
	if !ok {
		itemMutex = struct {
			mutex    *ContextLock
			refCount uint
		}{
			mutex: NewContextLock(),
		}
	}

	itemMutex.refCount++
	mm.locks[key] = itemMutex

	// This must not be deferred or the entire map will be locked until Lock() returns
	mm.mapLock.Unlock()
	return itemMutex.mutex.Lock(ctx)
}

func (mm *MutexMap[T]) Unlock(ctx context.Context, key T) error {
	if err := mm.mapLock.Lock(ctx); err != nil {
		return err
	}
	// This must be deferred to ensure that the lock is released upon panic
	defer mm.mapLock.Unlock()

	itemMutex, ok := mm.locks[key]
	if !ok {
		panic(fmt.Sprintf("key %v not found in mutext map", key))
	}

	switch itemMutex.refCount {
	case 0:
		// This shouldn't be hit but should be covered anyway in case of a bug
		panic(fmt.Sprintf("key %v not currently locked", key))
	case 1:
		// Prune the map to prevent it from going infinitely with every call to this function
		// The only remaining reference to theitem  mutex at this point should be the one in
		// the current context, which should be cleaned up once GC runs.
		delete(mm.locks, key)
	default:
		itemMutex.refCount--
		mm.locks[key] = itemMutex
	}

	return nil
}

func (mm *MutexMap[T]) Close(ctx context.Context) error {
	// Ensure all locks are released prior to closing the instance.
	if err := mm.mapLock.Lock(ctx); err != nil {
		return err
	}

	for _, item := range mm.locks {
		if err := item.mutex.Close(ctx); err != nil {
			return err
		}
	}

	return nil
}
