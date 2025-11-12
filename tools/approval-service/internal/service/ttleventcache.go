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
	"context"
	"fmt"
	"sync"
	"time"
)

// TTLEventCache is a thread-safe dedupe cache.
// - TryAdd: "debounce-on-arrival" (first arrival wins, subsequent arrivals within the TTL window are rejected
type TTLEventCache struct {
	mu              sync.Mutex
	items           map[string]time.Time // expiry time for an item; zero time reserved for in-progress marker
	ttl             time.Duration
	cleanupInterval time.Duration
	stop            chan struct{}
	done            chan struct{}
	closed          bool
}

// Option configures TTLEventCache construction
type Option func(*TTLEventCache) error

// WithCleanupInterval allows overriding the tick interval (used by janitor for cleanup) via option.
// Validates duration > 0.
func WithCleanupInterval(interval time.Duration) Option {
	return func(c *TTLEventCache) error {
		if interval <= 0 {
			return fmt.Errorf("invalid interval")
		}
		c.cleanupInterval = interval
		return nil
	}
}

// NewTTLEventCache creates and starts a TTL-based in memory cache.
//
// The cache prevents re-processing of the string based key for the configured time-to-live (ttl).
// The cache stores the key with an expiration of now + ttl; while that expiration is in the cache, TryAdd for the
// same key will return false.
//
// Eviction and Cleanup
//   - A background evictor goroutine runs using a cleanup interval chose by default as max(ttl/4, 1s).
//     The evictor removes entries whose expiration time is past the tick time.
//
// Lifecycle and concurrency
//   - the cache is safe for concurrent use from multiple goroutines
//   - Callers must call Stop(ctx) to shut down the background evictor and to prevent further additions. Stop blocks
//     until the evictor exits or the provided context is done.
//
// Use options for additional configuration (e.g., WithCleanupInterval).
func NewTTLEventCache(ttl time.Duration, opts ...Option) (*TTLEventCache, error) {
	// set a logical default
	if ttl <= 0 {
		ttl = 15 * time.Second
	}

	c := &TTLEventCache{
		items:           make(map[string]time.Time),
		ttl:             ttl,
		cleanupInterval: max(ttl/4, 1*time.Second), // default tick: ttl/4 for moderate TTLs, minimum 1s
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	// start evictor
	go c.evictor()
	return c, nil
}

// TryAdd implements debounce-on-arrival semantics.
// Returns true if the entry is not in the cache, or it has expired; false otherwise.
func (c *TTLEventCache) TryAdd(eventID string) bool {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return false
	}

	if expiry, ok := c.items[eventID]; ok {
		// unexpired -> reject
		if now.Before(expiry) {
			return false
		}
	}

	c.items[eventID] = now.Add(c.ttl)
	return true
}

// Stop stops the cleaner (key evictor) and prevents further additions. It blocks until the cleaner exits or ctx is done.
func (c *TTLEventCache) Stop(ctx context.Context) error {
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
func (c *TTLEventCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// evictor runs in a background goroutine and periodically removes expired entries from the cache.
func (c *TTLEventCache) evictor() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()
	defer close(c.done)

	for {
		select {
		case now := <-ticker.C:
			c.mu.Lock()
			for k, v := range c.items {
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
