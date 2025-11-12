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

package service_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/service"
)

var (
	eventIDForTest           = "event-id-for-test"
	eventIDInProgressForTest = "event-id-in-progress-for-test"
)

// TestTryAddConcurrency ensures that under a burst of concurrent TryAdds for the same key
// exactly one caller wins (returns true) and others are rejected within the TTL
func TestTryAddConcurrency(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Arrange
		ttl := 200 * time.Millisecond
		ec, err := service.NewTTLEventCache(ttl)
		if err != nil {
			t.Fatalf("MakeTTLCache error: %v", err)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := ec.Stop(ctx); err != nil {
				t.Fatalf("Failed to stop cache: %v", err)
			}
		}()

		const goroutines = 500
		var wg sync.WaitGroup
		wg.Add(goroutines)

		var wins int32
		start := make(chan struct{})

		// Act
		for i := 0; i < goroutines; i++ {
			go func() {
				defer wg.Done()
				<-start
				if ec.TryAdd(eventIDForTest) {
					atomic.AddInt32(&wins, 1)
				}
			}()
		}

		// release all goroutines simultaneously
		close(start)
		wg.Wait()

		// Assert
		if got := atomic.LoadInt32(&wins); got != 1 {
			t.Errorf("expected exactly 1 TryAdd winner, got %d", got)
		}
	})
}

// TestTryAddDebounceAndCooldown verifies that
// 1- The first caller for a given eventID wins and sets the TTL
// 2- concurrent callers during that TTL are rejected (debounced)
// 3- after TTL expires, TryAdd succeeds again (cooldown expired)
func TestTryAddDebounceAndCooldown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ttl := 10 * time.Second
		cleanupTicker := 2 * time.Second

		ec, err := service.NewTTLEventCache(ttl, service.WithCleanupInterval(cleanupTicker))
		if err != nil {
			t.Fatalf("MakeTTLCache error: %v", err)
		}

		// ensure evictor is stopped
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := ec.Stop(ctx); err != nil {
				t.Fatalf("Failed to stop cache: %v", err)
			}
		}()

		// first caller wins and sets the TTL
		if !ec.TryAdd(eventIDInProgressForTest) {
			t.Fatalf("expected first TryAdd to succeed")
		}

		// concurrent event should be rejected immediately
		resultChan := make(chan bool, 1)
		go func() { resultChan <- ec.TryAdd(eventIDInProgressForTest) }()

		if secondTryAddResult := <-resultChan; secondTryAddResult {
			t.Fatalf("expected concurrent TryAdd to be rejected while ttl has not expired")
		}

		// virtual sleep inside synctest bubble for TTL + cleanupTicker + small buffer
		// and then advance the time in bubble so timers / tickers run immediately
		time.Sleep(ttl + cleanupTicker + 20*time.Millisecond)
		synctest.Wait()

		// Assert
		if !ec.TryAdd(eventIDInProgressForTest) {
			t.Fatalf("expected first TryAdd to succeed for the same key after TTL has expired")
		}
	})
}

// TestCleanerEvictsExpiredEvents verifies that the background cleaner removes expired items after ttl + some time
// uses TryAdd to insert and then waits for eviction
func TestCleanerEvictsExpiredEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ttl := 15 * time.Second
		cleanupTicker := 5 * time.Second

		ec, err := service.NewTTLEventCache(ttl, service.WithCleanupInterval(cleanupTicker))
		if err != nil {
			t.Fatalf("MakeTTLCache error: %v", err)
		}

		// ensure evictor is stopped
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := ec.Stop(ctx); err != nil {
				t.Fatalf("Failed to stop cache: %v", err)
			}
		}()

		ok := ec.TryAdd(eventIDForTest)
		if !ok {
			t.Fatalf("expected initial TryAdd to succeed")
		}

		if got := ec.Len(); got != 1 {
			t.Fatalf("expected 1 TryAdd to contain an event, got %d", got)
		}

		// virtual sleep inside synctest bubble for TTL + cleanupTicker + small buffer
		// and then advance the time in bubble so timers / tickers run immediately
		time.Sleep(ttl + cleanupTicker + 20*time.Millisecond)
		synctest.Wait()

		if got := ec.Len(); got != 0 {
			t.Fatalf("expected cache to be empty after ttl+cleanup, got %d", got)
		}

	})

}

// // TestStopPreventsAdds ensures Stop prevents further additions and that Stop returns
func TestStopPreventsAdds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ttl := 100 * time.Millisecond
		cleanupTicker := 50 * time.Millisecond

		ec, err := service.NewTTLEventCache(ttl, service.WithCleanupInterval(cleanupTicker))
		if err != nil {
			t.Fatalf("MakeTTLCache error: %v", err)
		}

		// ensure evictor is stopped
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if err := ec.Stop(ctx); err != nil {
				t.Fatalf("Failed to stop cache: %v", err)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		if err := ec.Stop(ctx); err != nil {
			t.Fatalf("Failed to stop cache: %v", err)
		}

		// after stop, TryAdd should return false
		if ec.TryAdd(eventIDForTest) {
			t.Fatalf("expected TryAdd to fail after Stop")
		}
	})

}
