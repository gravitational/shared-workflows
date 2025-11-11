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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMutexMap(t *testing.T) {
	mm := NewMutexMap[string]()
	t.Cleanup(func() { assert.NoError(t, mm.Close(context.TODO())) })

	assert.NotNil(t, mm.mapLock)
	assert.NotNil(t, mm.locks)
}

func TestMapLockUnlock(t *testing.T) {
	keyA := "key A"
	keyB := "key B"

	mm := NewMutexMap[string]()
	t.Cleanup(func() { assert.NoError(t, mm.Close(context.TODO())) })

	err := mm.Lock(t.Context(), keyA)
	require.NoError(t, err)
	assert.Equal(t, uint(1), mm.locks[keyA].refCount)

	err = mm.Lock(t.Context(), keyB)
	require.NoError(t, err)
	assert.Equal(t, uint(1), mm.locks[keyB].refCount)

	err = mm.Unlock(t.Context(), keyB)
	require.NoError(t, err)

	assert.Panics(t, func() { _ = mm.Unlock(t.Context(), keyB) })

	err = mm.Unlock(t.Context(), keyA)
	require.NoError(t, err)

	assert.Len(t, mm.locks, 0)
}
