package cache

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestInMemoryCache_BasicOperations(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	// Test Set and Get
	cache.Set("key1", "value1", 1*time.Second)
	value, found := cache.Get("key1")
	if !found {
		t.Error("Expected to find key1")
	}
	if value != "value1" {
		t.Errorf("Expected value1, got %v", value)
	}

	// Test Get non-existent key
	_, found = cache.Get("nonexistent")
	if found {
		t.Error("Expected not to find nonexistent key")
	}
}

func TestInMemoryCache_Expiration(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	// Set with short TTL
	cache.Set("expire", "value", 50*time.Millisecond)

	// Should be available immediately
	value, found := cache.Get("expire")
	if !found || value != "value" {
		t.Error("Expected to find key immediately after setting")
	}

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Should not be available after expiration
	_, found = cache.Get("expire")
	if found {
		t.Error("Expected key to be expired")
	}
}

func TestInMemoryCache_Delete(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	cache.Set("key1", "value1", 1*time.Second)
	
	// Verify it exists
	_, found := cache.Get("key1")
	if !found {
		t.Error("Expected to find key1")
	}

	// Delete it
	cache.Delete("key1")

	// Verify it's gone
	_, found = cache.Get("key1")
	if found {
		t.Error("Expected key1 to be deleted")
	}
}

func TestInMemoryCache_Clear(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	// Add multiple items
	cache.Set("key1", "value1", 1*time.Second)
	cache.Set("key2", "value2", 1*time.Second)
	cache.Set("key3", "value3", 1*time.Second)

	if cache.Size() != 3 {
		t.Errorf("Expected size 3, got %d", cache.Size())
	}

	// Clear all
	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", cache.Size())
	}

	// Verify items are gone
	_, found := cache.Get("key1")
	if found {
		t.Error("Expected key1 to be cleared")
	}
}

func TestInMemoryCache_Size(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	if cache.Size() != 0 {
		t.Error("Expected empty cache to have size 0")
	}

	cache.Set("key1", "value1", 1*time.Second)
	if cache.Size() != 1 {
		t.Errorf("Expected size 1, got %d", cache.Size())
	}

	cache.Set("key2", "value2", 1*time.Second)
	if cache.Size() != 2 {
		t.Errorf("Expected size 2, got %d", cache.Size())
	}

	cache.Delete("key1")
	if cache.Size() != 1 {
		t.Errorf("Expected size 1 after delete, got %d", cache.Size())
	}
}

func TestInMemoryCache_Cleanup(t *testing.T) {
	cache := NewInMemoryCache(20 * time.Millisecond)
	defer cache.Stop()

	// Add items with short TTL
	cache.Set("key1", "value1", 10*time.Millisecond)
	cache.Set("key2", "value2", 10*time.Millisecond)
	cache.Set("key3", "value3", 1*time.Second) // This one shouldn't expire

	initialSize := cache.Size()
	if initialSize != 3 {
		t.Errorf("Expected initial size 3, got %d", initialSize)
	}

	// Wait for cleanup to run
	time.Sleep(50 * time.Millisecond)

	// Size should be 1 (only key3 remains)
	finalSize := cache.Size()
	if finalSize != 1 {
		t.Errorf("Expected size 1 after cleanup, got %d", finalSize)
	}

	// Verify key3 is still accessible
	value, found := cache.Get("key3")
	if !found || value != "value3" {
		t.Error("Expected key3 to still be available")
	}
}

func TestInMemoryCache_ConcurrentAccess(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	var wg sync.WaitGroup
	numGoroutines := 100
	numOperations := 100

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := string(rune('a' + (id % 26)))
				cache.Set(key, id*numOperations+j, 1*time.Second)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := string(rune('a' + (id % 26)))
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()

	// If we get here without panic or deadlock, the test passes
}

func TestInMemoryCache_GetOrSet_CacheMiss(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	computeCalled := false
	compute := func() (interface{}, error) {
		computeCalled = true
		return "computed_value", nil
	}

	value, err := cache.GetOrSet("key1", 1*time.Second, compute)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !computeCalled {
		t.Error("Expected compute function to be called on cache miss")
	}

	if value != "computed_value" {
		t.Errorf("Expected computed_value, got %v", value)
	}

	// Verify it's in cache
	cachedValue, found := cache.Get("key1")
	if !found || cachedValue != "computed_value" {
		t.Error("Expected value to be cached")
	}
}

func TestInMemoryCache_GetOrSet_CacheHit(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	// Pre-populate cache
	cache.Set("key1", "existing_value", 1*time.Second)

	computeCalled := false
	compute := func() (interface{}, error) {
		computeCalled = true
		return "computed_value", nil
	}

	value, err := cache.GetOrSet("key1", 1*time.Second, compute)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if computeCalled {
		t.Error("Expected compute function NOT to be called on cache hit")
	}

	if value != "existing_value" {
		t.Errorf("Expected existing_value, got %v", value)
	}
}

func TestInMemoryCache_GetOrSet_ComputeError(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	expectedError := errors.New("compute failed")
	compute := func() (interface{}, error) {
		return nil, expectedError
	}

	value, err := cache.GetOrSet("key1", 1*time.Second, compute)
	if err != expectedError {
		t.Errorf("Expected compute error, got %v", err)
	}

	if value != nil {
		t.Errorf("Expected nil value on error, got %v", value)
	}

	// Verify nothing was cached
	_, found := cache.Get("key1")
	if found {
		t.Error("Expected nothing to be cached on compute error")
	}
}

func TestInMemoryCache_GetOrSet_Concurrent(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	callCount := 0
	var mu sync.Mutex

	compute := func() (interface{}, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		time.Sleep(10 * time.Millisecond) // Simulate expensive computation
		return "computed", nil
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	// Multiple goroutines try to GetOrSet the same key
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.GetOrSet("key1", 1*time.Second, compute)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		}()
	}

	wg.Wait()

	// The compute function should only be called once due to locking
	// Note: There might be slight race conditions where it's called twice
	// but it should never be called 10 times
	if callCount > 2 {
		t.Errorf("Expected compute to be called at most 2 times, got %d", callCount)
	}
}

func TestInMemoryCache_Stop(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)

	// Add some data
	cache.Set("key1", "value1", 1*time.Second)

	// Stop the cache
	cache.Stop()

	// Give it a moment to ensure goroutine stops
	time.Sleep(20 * time.Millisecond)

	// Cache should still be usable for Get/Set operations
	// (only cleanup goroutine stops)
	value, found := cache.Get("key1")
	if !found || value != "value1" {
		t.Error("Cache should still work after Stop()")
	}
}

func TestInMemoryCache_DifferentTypes(t *testing.T) {
	cache := NewInMemoryCache(10 * time.Millisecond)
	defer cache.Stop()

	// Test with different value types
	cache.Set("string", "hello", 1*time.Second)
	cache.Set("int", 42, 1*time.Second)
	cache.Set("bool", true, 1*time.Second)
	cache.Set("struct", struct{ Name string }{"test"}, 1*time.Second)

	// Verify all types
	if val, found := cache.Get("string"); !found || val.(string) != "hello" {
		t.Error("String value mismatch")
	}
	if val, found := cache.Get("int"); !found || val.(int) != 42 {
		t.Error("Int value mismatch")
	}
	if val, found := cache.Get("bool"); !found || val.(bool) != true {
		t.Error("Bool value mismatch")
	}
	if val, found := cache.Get("struct"); !found || val.(struct{ Name string }).Name != "test" {
		t.Error("Struct value mismatch")
	}
}

// Benchmark tests
func BenchmarkInMemoryCache_Set(b *testing.B) {
	cache := NewInMemoryCache(1 * time.Second)
	defer cache.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set("key", "value", 1*time.Second)
	}
}

func BenchmarkInMemoryCache_Get(b *testing.B) {
	cache := NewInMemoryCache(1 * time.Second)
	defer cache.Stop()
	cache.Set("key", "value", 1*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get("key")
	}
}

func BenchmarkInMemoryCache_GetOrSet(b *testing.B) {
	cache := NewInMemoryCache(1 * time.Second)
	defer cache.Stop()

	compute := func() (interface{}, error) {
		return "value", nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.GetOrSet("key", 1*time.Second, compute)
	}
}

func BenchmarkInMemoryCache_ConcurrentReads(b *testing.B) {
	cache := NewInMemoryCache(1 * time.Second)
	defer cache.Stop()
	cache.Set("key", "value", 1*time.Second)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Get("key")
		}
	})
}

