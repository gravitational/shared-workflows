package ttleventcache_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gravitational/shared-workflows/tools/approval-service/internal/ttleventcache"
)

var (
	eventIDForTest           = "event-id-for-test"
	eventIDInProgressForTest = "event-id-in-progress-for-test"
)

// TestTryAddConcurrency ensures that under a burst of concurrent TryAdds for the same key
// exactly one caller wins (returns true) and others are rejected within the TTL
func TestTryAddConcurrency(t *testing.T) {
	// Arrange
	ttl := 200 * time.Millisecond
	ec, cleanupFunc, err := ttleventcache.MakeTTLCache(ttl)
	if err != nil {
		t.Fatalf("MakeTTLCache error: %v", err)
	}
	defer func() {
		_ = cleanupFunc
	}()

	const goroutines = 500
	var wg sync.WaitGroup

	var wins int32
	var mu sync.Mutex

	// Act
	for i := 0; i < goroutines; i++ {
		wg.Go(func() {
			if ec.TryAdd(eventIDForTest) {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	// Assert
	if wins != 1 {
		t.Fatalf("expected exactly 1 TryAdd winner, got %d", wins)
	}
}

// TestTryAddDebounceAndCooldown verifies that
// 1- The first caller for a given eventID wins and sets the TTL
// 2- concurrent callers during that TTL are rejected (debounced)
// 3- after TTL expires, TryAdd succeeds again (cooldown expired)
func TestTryAddDebounceAndCooldown(t *testing.T) {
	ttl := 100 * time.Millisecond
	ec, cleanupFunc, err := ttleventcache.MakeTTLCache(ttl)
	if err != nil {
		t.Fatalf("MakeTTLCache error: %v", err)
	}
	defer func() { _ = cleanupFunc() }()

	// first caller wins and sets the TTL
	if !ec.TryAdd(eventIDInProgressForTest) {
		t.Fatalf("expected first TryAdd to succeed")
	}

	done := make(chan bool, 1)
	go func() {
		done <- ec.TryAdd(eventIDInProgressForTest)
	}()
	addResult := <-done
	if addResult {
		t.Fatalf("expected concurrent TryAdd to be rejected while cooldown is active")
	}

	// after TTL elapses, TryAdd should succeed again
	time.Sleep(ttl + 20*time.Millisecond)
	if !ec.TryAdd(eventIDInProgressForTest) {
		t.Fatalf("expected TryAdd to succeed after cooldown expired")
	}
}

// TestCleanerEvictsExpiredEvents verifies that the background cleaner removes expired items after ttl + some time
// uses TryAdd to insert and then waits for eviction
func TestCleanerEvictsExpiredEvents(t *testing.T) {
	ttl := 100 * time.Millisecond
	cleanupTicker := 50 * time.Millisecond
	ec, cleanupFunc, err := ttleventcache.MakeTTLCache(ttl, ttleventcache.WithCleanupInterval(cleanupTicker))
	if err != nil {
		t.Fatalf("MakeTTLCache error: %v", err)
	}
	defer func() {
		_ = cleanupFunc()
	}()

	ok := ec.TryAdd(eventIDForTest)
	if !ok {
		t.Fatalf("expected initial TryAdd to succeed")
	}

	// entry should be present immediately
	if got := ec.Len(); got != 1 {
		t.Fatalf("expected 1 TryAdd to contain an event, got %d", got)
	}

	// wait for TTL + cleanup interval to ensure cleaner gets a chance to run
	time.Sleep(ttl + cleanupTicker + 20*time.Millisecond)

	if got := ec.Len(); got != 0 {
		t.Fatalf("expected cache to be empty after ttl+cleanup, got %d", got)
	}
}

// TestStopPreventsAdds ensures Stop prevents further additions and that Stop returns
func TestStopPreventsAdds(t *testing.T) {
	ttl := 100 * time.Millisecond
	ec, cleanupFunc, err := ttleventcache.MakeTTLCache(ttl)
	if err != nil {
		t.Fatalf("MakeTTLCache error: %v", err)
	}
	defer func() {
		_ = cleanupFunc()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := ec.Stop(ctx); err != nil {
		t.Fatalf("Stop error: %v", err)
	}

	// after stop, TryAdd should return false
	if ec.TryAdd(eventIDForTest) {
		t.Fatalf("expected TryAdd to fail after Stop")
	}
}
