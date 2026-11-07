package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// Pastikan implementasi memenuhi interface Cache
var _ Cache = (*MemoryCache)(nil)
var _ Cache = (*RedisCache)(nil)

func TestGenerateCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		url1     string
		url2     string
		expected bool // true if keys should match, false if they should differ
	}{
		{
			name:     "identical URLs",
			url1:     "https://example.com/article",
			url2:     "https://example.com/article",
			expected: true,
		},
		{
			name:     "case sensitivity in host",
			url1:     "https://EXAMPLE.COM/article",
			url2:     "https://example.com/article",
			expected: true,
		},
		{
			name:     "fragment ignored",
			url1:     "https://example.com/article#section-1",
			url2:     "https://example.com/article",
			expected: true,
		},
		{
			name:     "query params sorted",
			url1:     "https://example.com/search?b=2&a=1",
			url2:     "https://example.com/search?a=1&b=2",
			expected: true,
		},
		{
			name:     "trailing slash on root normalized",
			url1:     "https://example.com/",
			url2:     "https://example.com",
			expected: true,
		},
		{
			name:     "default port 443 normalized",
			url1:     "https://example.com:443/article",
			url2:     "https://example.com/article",
			expected: true,
		},
		{
			name:     "default port 80 normalized",
			url1:     "http://example.com:80/article",
			url2:     "http://example.com/article",
			expected: true,
		},
		{
			name:     "different paths should differ",
			url1:     "https://example.com/article-1",
			url2:     "https://example.com/article-2",
			expected: false,
		},
		{
			name:     "different schemes should differ",
			url1:     "http://example.com/article",
			url2:     "https://example.com/article",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k1 := GenerateCacheKey(tt.url1)
			k2 := GenerateCacheKey(tt.url2)

			// Format check: "reader:cache:<64-char-hex>"
			if len(k1) != 13+64 || !matchesPrefix(k1, "reader:cache:") {
				t.Fatalf("k1 invalid format: %s", k1)
			}
			if len(k2) != 13+64 || !matchesPrefix(k2, "reader:cache:") {
				t.Fatalf("k2 invalid format: %s", k2)
			}

			if (k1 == k2) != tt.expected {
				t.Errorf("GenerateCacheKey() comparison mismatch for [%s] vs [%s]: got match=%v, expected=%v", tt.url1, tt.url2, k1 == k2, tt.expected)
			}
		})
	}
}

func matchesPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func TestMemoryCache_BasicGetSet(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryCache()
	defer cache.Close()

	// Get on empty cache
	val, found, err := cache.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found || val != "" {
		t.Fatalf("expected miss, got found=%v val=%s", found, val)
	}

	// Set & Get
	err = cache.Set(ctx, "key1", "val1", 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected set error: %v", err)
	}

	val, found, err = cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	if !found || val != "val1" {
		t.Fatalf("expected hit 'val1', got found=%v val=%s", found, val)
	}

	// Update existing key
	err = cache.Set(ctx, "key1", "val1_updated", 1*time.Minute)
	if err != nil {
		t.Fatalf("unexpected set error: %v", err)
	}
	val, found, err = cache.Get(ctx, "key1")
	if !found || val != "val1_updated" {
		t.Fatalf("expected hit 'val1_updated', got found=%v val=%s", found, val)
	}
}

func TestMemoryCache_TTL(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryCache()
	defer cache.Close()

	err := cache.Set(ctx, "ttlKey", "ttlVal", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected set error: %v", err)
	}

	// Immediate get
	val, found, err := cache.Get(ctx, "ttlKey")
	if err != nil || !found || val != "ttlVal" {
		t.Fatalf("expected immediate hit, got found=%v val=%s err=%v", found, val, err)
	}

	// Sleep until expired
	time.Sleep(70 * time.Millisecond)

	val, found, err = cache.Get(ctx, "ttlKey")
	if err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	if found || val != "" {
		t.Fatalf("expected item to be expired, got found=%v val=%s", found, val)
	}
}

func TestMemoryCache_LRUEviction(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryCacheWithOptions(MemoryCacheOptions{
		Capacity:        2,
		CleanupInterval: 1 * time.Minute,
	})
	defer cache.Close()

	_ = cache.Set(ctx, "k1", "v1", 0)
	_ = cache.Set(ctx, "k2", "v2", 0)

	// Access k1 to make it most recently used (k2 is least recently used)
	_, found, _ := cache.Get(ctx, "k1")
	if !found {
		t.Fatalf("expected k1 to be found")
	}

	// Insert k3; k2 should be evicted
	_ = cache.Set(ctx, "k3", "v3", 0)

	// k2 should be missing
	_, found, _ = cache.Get(ctx, "k2")
	if found {
		t.Fatalf("expected k2 to be evicted")
	}

	// k1 and k3 should exist
	val1, found1, _ := cache.Get(ctx, "k1")
	if !found1 || val1 != "v1" {
		t.Fatalf("expected k1 to still exist")
	}

	val3, found3, _ := cache.Get(ctx, "k3")
	if !found3 || val3 != "v3" {
		t.Fatalf("expected k3 to exist")
	}
}

func TestMemoryCache_PeriodicCleanup(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryCacheWithOptions(MemoryCacheOptions{
		Capacity:        100,
		CleanupInterval: 25 * time.Millisecond,
	})
	defer cache.Close()

	_ = cache.Set(ctx, "short1", "val1", 20*time.Millisecond)
	_ = cache.Set(ctx, "short2", "val2", 20*time.Millisecond)

	time.Sleep(60 * time.Millisecond)

	// Background cleaner should have removed expired items from internal map
	cache.mu.RLock()
	count := len(cache.items)
	cache.mu.RUnlock()

	if count != 0 {
		t.Fatalf("expected 0 items after background cleanup, got %d", count)
	}
}

func TestMemoryCache_ConcurrentRace(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryCacheWithOptions(MemoryCacheOptions{
		Capacity:        20,
		CleanupInterval: 10 * time.Millisecond,
	})
	defer cache.Close()

	var wg sync.WaitGroup
	workers := 50
	iterations := 100

	for i := 0; i < workers; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				key := fmt.Sprintf("key-%d", (workerID+j)%30)
				val := fmt.Sprintf("val-%d-%d", workerID, j)
				_ = cache.Set(ctx, key, val, 100*time.Millisecond)
				_, _, _ = cache.Get(ctx, key)
			}
		}()
	}

	wg.Wait()
}

func TestMemoryCache_ContextCanceled(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := cache.Get(ctx, "key")
	if err == nil {
		t.Fatalf("expected error on canceled context for Get, got nil")
	}

	err = cache.Set(ctx, "key", "val", time.Minute)
	if err == nil {
		t.Fatalf("expected error on canceled context for Set, got nil")
	}
}

func TestRedisCache_Integration(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	redisURL := "redis://" + s.Addr()
	rCache, err := NewRedisCache(redisURL)
	if err != nil {
		t.Fatalf("failed to create redis cache: %v", err)
	}
	defer rCache.Close()

	// Get miss
	val, found, err := rCache.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error on redis miss: %v", err)
	}
	if found || val != "" {
		t.Fatalf("expected redis miss, got found=%v val=%s", found, val)
	}

	// Set & Get
	err = rCache.Set(ctx, "redis_k1", "redis_v1", 1*time.Minute)
	if err != nil {
		t.Fatalf("failed to set redis key: %v", err)
	}

	val, found, err = rCache.Get(ctx, "redis_k1")
	if err != nil || !found || val != "redis_v1" {
		t.Fatalf("expected hit on redis, got found=%v val=%s err=%v", found, val, err)
	}

	// Fast forward time in miniredis for TTL verification
	s.FastForward(2 * time.Minute)

	val, found, err = rCache.Get(ctx, "redis_k1")
	if err != nil {
		t.Fatalf("unexpected error after TTL: %v", err)
	}
	if found || val != "" {
		t.Fatalf("expected redis key to be expired, got found=%v val=%s", found, val)
	}
}
