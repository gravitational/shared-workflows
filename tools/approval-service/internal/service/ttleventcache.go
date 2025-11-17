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
	"sync"
	"time"
)

// cacheEntry stores ID and expiry time for an item in the expiry list
type cacheEntry struct {
	id     string
	expiry time.Time
}

// TTLEventCache is a thread-safe TTL-based event cache.
// It uses "evict-on-write" semantics: expired entries are cleaned up during calls to TryAdd.
//
// The cache prevents re-processing of the string-based key for the configured time-to-live (ttl). While an entry
// is in the cache, TryAdd will return false.
type TTLEventCache struct {
	mu         sync.Mutex
	cache      map[string]time.Time // stores key -> expiry time for O(1) lookups
	expiryList []cacheEntry         // stores (key, expiry time), sorted by expiry time
	ttl        time.Duration
}

// NewTTLEventCache creates and starts a TTL-based in memory cache.
// ttl specifies the time-to-live for each entry.
// If ttl is zero or negative, a default of 15 seconds will be used.
func NewTTLEventCache(ttl time.Duration) *TTLEventCache {
	// set a logical default
	if ttl <= 0 {
		ttl = 15 * time.Second
	}

	c := &TTLEventCache{
		cache: make(map[string]time.Time),
		ttl:   ttl,
	}

	return c
}

// TryAdd attempts to add an entry to the cache.
//
// Returns true if the ID was added, or false if the ID is already in the cache and has not expired.
//
// Calls to TryAdd will prune any expired entries from the cache before performing the check.
func (c *TTLEventCache) TryAdd(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.evictExpired(now)

	if _, found := c.cache[id]; found {
		return false
	}

	// add new entry
	expiry := now.Add(c.ttl)
	c.cache[id] = expiry
	c.expiryList = append(c.expiryList, cacheEntry{id, expiry})

	return true
}

// evictExpired removes expired entries from the cache.
// This method MUST be called with the mutex c.mu held.
func (c *TTLEventCache) evictExpired(now time.Time) {
	// fast path: no entries to expire
	if len(c.expiryList) == 0 {
		return
	}

	// fast path: oldest entry is not expired, so nothing is. The expiry slice is guaranteed to be sorted by expiry time
	if c.expiryList[0].expiry.After(now) {
		return
	}

	var i int
	for i < len(c.expiryList) {
		if c.expiryList[i].expiry.After(now) {
			break // found first non-expired item at index i
		}

		// this item (c.expiryList[i]) is expired, delete it
		delete(c.cache, c.expiryList[i].id)
		i++
	}

	// reslice to remove expired items from the expiry list
	if i == len(c.expiryList) {
		c.expiryList = nil // items were expired, reset slice
	} else {
		c.expiryList = c.expiryList[i:] // keep items from i onwards
	}
}

// Len returns the number of entries currently in the cache and expiry list. Useful for tests/metrics.
func (c *TTLEventCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.cache)
}
