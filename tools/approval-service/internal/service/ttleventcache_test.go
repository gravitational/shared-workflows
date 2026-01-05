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

package service

import (
	"fmt"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func checkInternalState(t *testing.T, ec *TTLEventCache, expectedMapLen, expectedListLen int) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if len(ec.cache) != expectedMapLen {
		t.Errorf("len(c.cache) = %d; want %d", len(ec.cache), expectedMapLen)
	}
	if len(ec.expiryList) != expectedListLen {
		t.Errorf("len(c.expiryList) = %d; want %d", len(ec.expiryList), expectedListLen)
	}
}

func TestTTLEventCache_WithInvalidTTL_ShouldSetDefaultValue(t *testing.T) {
	defaultTTL := 15 * time.Second

	ecWithZero := NewTTLEventCache(0)
	if ecWithZero.ttl != defaultTTL {
		t.Errorf("NewTTLEventCache(0) did not set default TTL: got: %v; want: %v", ecWithZero.ttl, defaultTTL)
	}

	ecNegative := NewTTLEventCache(-5 * time.Second)
	if ecNegative.ttl != defaultTTL {
		t.Errorf("NewTTLEventCache(-5) did not set default TTL: got: %v; want: %v", ecNegative.ttl, defaultTTL)
	}
}

func TestTTLEventCache_ValidTTL_ShouldOverrideDefault(t *testing.T) {
	// Arrange
	oneSecondTTlForTest := 1 * time.Second

	// Act
	ecPositive := NewTTLEventCache(oneSecondTTlForTest)

	// Assert
	if ecPositive.ttl != oneSecondTTlForTest {
		t.Errorf("NewTTLEventCache(1) did not set correct TTL: got: %v; want: %v", ecPositive.ttl, oneSecondTTlForTest)
	}
}

func TestTTLEventCache_TryAdd_HappyPath(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Arrange
		keyForTest := "key-for-test"
		ec := NewTTLEventCache(15 * time.Second)

		// Act
		isSuccessfulFirstTryAdd := ec.TryAdd(keyForTest)

		// Assert
		if !isSuccessfulFirstTryAdd {
			t.Errorf("TryAdd(key) returned false on first add, expected true")
		}
		checkInternalState(t, ec, 1, 1)
	})
}

func TestTTLEventCache_TryAdd_BasicRejectDuplicates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ec := NewTTLEventCache(15 * time.Second)
		keyForTest := "key-for-test"

		isSuccessfulFirstTryAdd := ec.TryAdd(keyForTest)
		if !isSuccessfulFirstTryAdd {
			t.Errorf("TryAdd(key) returned false on first add, expected true")
		}
		checkInternalState(t, ec, 1, 1)

		isSuccessfulSecondTryAdd := ec.TryAdd(keyForTest)
		if isSuccessfulSecondTryAdd {
			t.Errorf("TryAdd(keyForTest) returned true on duplicate add, expected false")
		}
		checkInternalState(t, ec, 1, 1)
	})
}

func TestTTLEventCache_TryAdd_Concurrency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ttl := 15 * time.Second
		ec := NewTTLEventCache(ttl)

		const (
			distinctKeys   = 5
			requestsPerKey = 200
		)

		var successfulAdds int32

		// Schedule one small step per logical request. The synctest harness runs these
		// steps under a deterministic scheduler so inter-leavings are reproducible.
		for i := 0; i < distinctKeys; i++ {
			key := fmt.Sprintf("key-%d", i)
			for j := 0; j < requestsPerKey; j++ {
				if ec.TryAdd(key) {
					atomic.AddInt32(&successfulAdds, 1)
				}
			}
		}

		// After all scheduled steps run (all requests within the TTL), exactly one
		// successful add per key is expected.
		got := int(atomic.LoadInt32(&successfulAdds))
		if got != distinctKeys {
			t.Fatalf("expected %d successful adds (one per key), got %d", distinctKeys, got)
		}
	})

}

func TestTTLEventCache_TryAdd_AddAfterExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ttl := 15 * time.Second
		ec := NewTTLEventCache(ttl)
		keyABCForTest := "ABC"
		key123ForTest := "123"

		isSuccessfulKeyOneFirstTry := ec.TryAdd(keyABCForTest)
		if !isSuccessfulKeyOneFirstTry {
			t.Errorf("TryAdd(keyOne) returned false on first add, expected true")
		}

		// wait for TTL to pass (virtual sleep)
		time.Sleep(ttl + 10*time.Millisecond)

		// try to add a different key to trigger eviction
		isSuccessfulKeyTwoFirstTry := ec.TryAdd(key123ForTest)
		if !isSuccessfulKeyTwoFirstTry {
			t.Errorf("TryAdd(keyTwo) returned false, expected true")
		}

		// internal state check: keyForTest should be gone
		ec.mu.Lock()
		if _, found := ec.cache[keyABCForTest]; found {
			t.Errorf("cache map still contains expired key after eviction")
		}

		// the expiry list should only contain "keyTwo"
		if len(ec.expiryList) != 1 || ec.expiryList[0].id != key123ForTest {
			t.Errorf("expiry list was not pruned correctly, keyOne should have been evicted")
		}
		ec.mu.Unlock()

		checkInternalState(t, ec, 1, 1)

		// try adding original key (should succeed)
		isSuccessfulRetry := ec.TryAdd(keyABCForTest)
		if !isSuccessfulRetry {
			t.Errorf("TryAdd returned false when trying to insert key after after ttl expired, expected true")
		}

		checkInternalState(t, ec, 2, 2)

	})
}

func TestTTLEventCache_EvictionLogic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ttl := 100 * time.Millisecond
		ec := NewTTLEventCache(ttl)

		// 1. Add "A" at T=0
		if !ec.TryAdd("A") {
			t.Fatal("TryAdd(A) failed")
		} // "A" expires at T=100ms

		// 2. Wait 50ms, Add "B" at T=50ms
		time.Sleep(50 * time.Millisecond)
		if !ec.TryAdd("B") {
			t.Fatal("TryAdd(B) failed")
		} // "B" expires at T=150ms

		checkInternalState(t, ec, 2, 2)

		// 2a. Sleep 49ms (T = 99ms) and verify both A and B are present
		time.Sleep(49 * time.Millisecond)
		if ec.Len() != 2 {
			t.Fatalf("After 99ms: Len() is %d, expected 2 (A, B)", ec.Len())
		}
		ec.mu.Lock()
		if _, ok := ec.cache["A"]; !ok {
			t.Error("after 99ms: cache does not contain key 'A' (expected present)")
		}
		if _, ok := ec.cache["B"]; !ok {
			t.Error("after 99ms: cache does not contain key 'B' (expected present)")
		}
		ec.mu.Unlock()

		// 3. Wait 2ms, time is now T=111ms
		// "A" (T=100ms) is expired.
		// "B" (T=150ms) is NOT expired.
		time.Sleep(1 * time.Millisecond)

		// 4. Add "C", which triggers expire()
		if !ec.TryAdd("C") {
			t.Fatal("TryAdd(C) failed")
		} // "C" expires at T=210ms

		// 5. Check state. "A" should be gone. "B" and "C" should remain.
		if ec.Len() != 2 {
			t.Errorf("Len() is %d, expected 2 (B, C)", ec.Len())
		}

		ec.mu.Lock()
		if _, ok := ec.cache["A"]; ok {
			t.Error("cache map still contains expired key 'A'")
		}
		if _, ok := ec.cache["B"]; !ok {
			t.Error("cache map does not contain key 'B'")
		}
		if _, ok := ec.cache["C"]; !ok {
			t.Error("cache map does not contain key 'C'")
		}
		if len(ec.expiryList) != 2 || ec.expiryList[0].id != "B" || ec.expiryList[1].id != "C" {
			t.Errorf("expiry list not pruned correctly: got %v, want [B, C]", ec.expiryList)
		}
		ec.mu.Unlock()

		// 6. Add "A" again, should be fine
		if !ec.TryAdd("A") {
			t.Fatal("TryAdd(A) again failed")
		}
		if ec.Len() != 3 {
			t.Errorf("Len() is %d, expected 3 (B, C, A)", ec.Len())
		}
	})
}

func TestTTLEventCache_EvictAll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ttl := 50 * time.Millisecond
		ec := NewTTLEventCache(ttl)

		if !ec.TryAdd("A") {
			t.Fatal("TryAdd(A) failed")
		}
		if !ec.TryAdd("B") {
			t.Fatal("TryAdd(B) failed")
		}
		checkInternalState(t, ec, 2, 2)

		// Wait for all to expire
		time.Sleep(ttl + 10*time.Millisecond)

		// Add "C", triggering eviction of "A" and "B"
		if !ec.TryAdd("C") {
			t.Fatal("TryAdd(C) failed")
		}

		// Check state. Only "C" should remain.
		if ec.Len() != 1 {
			t.Errorf("Len() is %d, expected 1 (C)", ec.Len())
		}
		ec.mu.Lock()
		if _, ok := ec.cache["A"]; ok {
			t.Error("cache map still contains expired key 'A'")
		}
		if _, ok := ec.cache["B"]; ok {
			t.Error("cache map still contains expired key 'B'")
		}
		if _, ok := ec.cache["C"]; !ok {
			t.Error("cache map does not contain key 'C'")
		}
		if len(ec.expiryList) != 1 || ec.expiryList[0].id != "C" {
			t.Errorf("expiry list not pruned correctly: got %v, want [C]", ec.expiryList)
		}
		ec.mu.Unlock()
	})
}
