package cache

import (
	"sync"
	"time"
)

// Cache is a generic interface for caching operations
// Implementations can be in-memory, Redis, or any other backing store
type Cache interface {
	// Get retrieves a value from the cache
	// Returns the value and true if found, nil and false otherwise
	Get(key string) (interface{}, bool)

	// Set stores a value in the cache with the specified TTL
	Set(key string, value interface{}, ttl time.Duration)

	// GetOrSet atomically gets a value or computes and caches it if not found
	// The compute function is only called if the key is not in cache
	GetOrSet(key string, ttl time.Duration, compute func() (interface{}, error)) (interface{}, error)

	// Delete removes a specific key from the cache
	Delete(key string)

	// Clear removes all items from the cache
	Clear()

	// Size returns the number of items currently in the cache
	Size() int

	// Stop gracefully shuts down the cache (e.g., stops cleanup goroutines)
	Stop()
}

// cacheItem represents a single cached value with expiration
type cacheItem struct {
	value      interface{}
	expiration time.Time
}

// isExpired checks if the cache item has expired
func (item *cacheItem) isExpired() bool {
	return time.Now().After(item.expiration)
}

// InMemoryCache is a thread-safe in-memory cache implementation
type InMemoryCache struct {
	items           map[string]*cacheItem
	mu              sync.RWMutex
	cleanupInterval time.Duration
	stopCleanup     chan bool
}

// NewInMemoryCache creates a new in-memory cache with automatic cleanup
// cleanupInterval determines how often expired items are removed
func NewInMemoryCache(cleanupInterval time.Duration) *InMemoryCache {
	cache := &InMemoryCache{
		items:           make(map[string]*cacheItem),
		cleanupInterval: cleanupInterval,
		stopCleanup:     make(chan bool),
	}

	// Start background cleanup goroutine
	go cache.startCleanup()

	return cache
}

// Get retrieves a value from the cache
func (c *InMemoryCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, found := c.items[key]
	if !found {
		return nil, false
	}

	// Check if item has expired
	if item.isExpired() {
		return nil, false
	}

	return item.value, true
}

// Set stores a value in the cache with the specified TTL
func (c *InMemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiration := time.Now().Add(ttl)
	c.items[key] = &cacheItem{
		value:      value,
		expiration: expiration,
	}
}

// GetOrSet atomically gets a value or computes and caches it if not found
// This is useful for avoiding cache stampede and duplicate computation
func (c *InMemoryCache) GetOrSet(key string, ttl time.Duration, compute func() (interface{}, error)) (interface{}, error) {
	// First, try to get with read lock
	c.mu.RLock()
	item, found := c.items[key]
	if found && !item.isExpired() {
		c.mu.RUnlock()
		return item.value, nil
	}
	c.mu.RUnlock()

	// Not found or expired, acquire write lock to compute
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have computed it)
	item, found = c.items[key]
	if found && !item.isExpired() {
		return item.value, nil
	}

	// Compute the value
	value, err := compute()
	if err != nil {
		return nil, err
	}

	// Store in cache
	expiration := time.Now().Add(ttl)
	c.items[key] = &cacheItem{
		value:      value,
		expiration: expiration,
	}

	return value, nil
}

// Delete removes a specific key from the cache
func (c *InMemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}

// Clear removes all items from the cache
func (c *InMemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheItem)
}

// Size returns the number of items currently in the cache
// Note: This includes expired items that haven't been cleaned up yet
func (c *InMemoryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.items)
}

// Stop gracefully shuts down the cache and stops the cleanup goroutine
func (c *InMemoryCache) Stop() {
	c.stopCleanup <- true
}

// startCleanup runs a background goroutine that periodically removes expired items
func (c *InMemoryCache) startCleanup() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanup removes all expired items from the cache
func (c *InMemoryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if now.After(item.expiration) {
			delete(c.items, key)
		}
	}
}

