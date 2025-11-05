package eventcache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// EventCache is a thread-safe dedupe cache.
// It supports two usage patterns:
// - TryAdd: "debounce-on-arrival" (first arrival wins, subsequent arrivals within the TTL window are rejected
// - TryStart/Finish: "in-progress exclusion + post-finish cooldown"
type EventCache struct {
	mu              sync.Mutex
	items           map[string]time.Time // expiry time for an item; zero time reserved for in-progress marker
	ttl             time.Duration
	cleanupInterval time.Duration
	stop            chan struct{}
	done            chan struct{}
	closed          bool
}

// Option configures EventCache construction
type Option func(*EventCache) error

// WithCleanupInterval allows overriding the tick interval (used by janitor for cleanup) via option.
// Validates duration > 0.
func WithCleanupInterval(interval time.Duration) Option {
	return func(c *EventCache) error {
		if interval <= 0 {
			return fmt.Errorf("invalid interval")
		}
		c.cleanupInterval = interval
		return nil
	}
}

// MakeEventCache creates a new EventCache.
// ttl: how long to prevent re-processing after an arrival or after finish().
// Use options for additional configuration (e.g., WithCleanupInterval).
func MakeEventCache(ttl time.Duration, opts ...Option) (*EventCache, func() error, error) {
	// set a logical default
	if ttl <= 0 {
		ttl = 15 * time.Second
	}

	// default tick: ttl/4 for moderate TTLs, minimum 1s
	defaultCleanupInterval := func(ttl time.Duration) time.Duration {
		if ttl > 4*time.Second {
			return ttl / 4
		}
		return 1 * time.Second
	}

	c := &EventCache{
		items:           make(map[string]time.Time),
		ttl:             ttl,
		cleanupInterval: defaultCleanupInterval(ttl),
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, nil, err
		}
	}

	cleanup := func() error {
		_ = c.Stop(context.Background())
		return nil
	}

	// start evictor
	go c.evictor()
	return c, cleanup, nil
}

// TryAdd implements debounce-on-arrival semantics.
// Returns true if the caller should process the event; false if unexpired entry exists.
func (c *EventCache) TryAdd(eventID string) bool {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return false
	}

	if expiry, ok := c.items[eventID]; ok {
		// in-progress (zero time) -> reject
		if expiry.IsZero() {
			return false
		}

		// unexpired -> reject
		if now.Before(expiry) {
			return false
		}

		// expired -> delete and allow insertion
		delete(c.items, eventID)
	}

	c.items[eventID] = now.Add(c.ttl)
	return true
}

// TryStart provides in-progress exclusion semantics.
// Returns (true, finish) for the caller that won. Caller must call finish() when done.
// finish will set cooldown expiry = now + ttl
func (c *EventCache) TryStart(eventID string) (bool, func()) {
	now := time.Now()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false, func() {}
	}

	if expiry, ok := c.items[eventID]; ok {
		// in-progress -> reject
		if expiry.IsZero() {
			c.mu.Unlock()
			return false, func() {}
		}

		// unexpired -> reject
		if now.Before(expiry) {
			c.mu.Unlock()
			return false, func() {}
		}

		// expired -> remove and proceed
		delete(c.items, eventID)
	}

	// mark in-progress / in-process with zero-time
	c.items[eventID] = time.Time{}
	c.mu.Unlock()

	finishOnce := sync.Once{}
	finish := func() {
		finishOnce.Do(func() {
			c.mu.Lock()
			c.items[eventID] = now.Add(c.ttl)
			c.mu.Unlock()
		})
	}

	return true, finish
}

// Stop stops the cleaner (key evictor) and prevents further additions. It blocks until the cleaner exits or ctx is done.
func (c *EventCache) Stop(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	// signal cleaner
	close(c.stop)
	c.mu.Unlock()

	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Len returns the number of entries currently in the cache. Useful for tests/metrics.
func (c *EventCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *EventCache) evictor() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()
	defer close(c.done)

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			c.mu.Lock()
			for k, v := range c.items {
				if v.IsZero() {
					// in progress: keep until finish sets expiry
					continue
				}
				if now.After(v) {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		case <-c.stop:
			return
		}
	}
}
