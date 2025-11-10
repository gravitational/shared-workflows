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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewContextLock(t *testing.T) {
	lock := NewContextLock()
	assert.NotNil(t, lock.lock)
	assert.NotNil(t, lock.closeLock)
	t.Cleanup(func() { assert.NoError(t, lock.Close(context.TODO())) })
}

func TestLock(t *testing.T) {
	lock := NewContextLock()
	t.Cleanup(func() { assert.NoError(t, lock.Close(context.TODO())) })

	err := lock.Lock(t.Context())
	require.NoError(t, err)
	defer lock.Unlock()

	secondLockCtx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	// This should fail
	err = lock.Lock(secondLockCtx)
	require.Error(t, err)
}

func TestUnlock(t *testing.T) {
	lock := NewContextLock()
	t.Cleanup(func() { assert.NoError(t, lock.Close(context.TODO())) })

	err := lock.Lock(t.Context())
	require.NoError(t, err)

	assert.NotPanics(t, lock.Unlock)
	assert.Panics(t, lock.Unlock)
}

func TestClose(t *testing.T) {

	t.Run("close unlocked lock", func(t *testing.T) {
		lock := NewContextLock()
		assert.NoError(t, lock.Close(t.Context()))
	})

	t.Run("close locked lock", func(t *testing.T) {
		closeCtx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		cancel()

		lock := NewContextLock()
		assert.Error(t, lock.Close(closeCtx))
	})
}
