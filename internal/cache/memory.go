package cache

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// memItem merepresentasikan payload item dalam memory cache
type memItem struct {
	key       string
	value     string
	expiresAt time.Time
}

// MemoryCache mengimplementasikan Cache menggunakan sync.RWMutex dan LRU container/list
type MemoryCache struct {
	mu         sync.RWMutex
	items      map[string]*list.Element
	evictList  *list.List
	capacity   int
	stopClean  chan struct{}
	cleanTimer *time.Ticker
	closeOnce  sync.Once
}

// MemoryCacheOptions mendefinisikan opsi inisialisasi MemoryCache
type MemoryCacheOptions struct {
	Capacity        int           // Default: 10000 item
	CleanupInterval time.Duration // Default: 1 menit
}

// NewMemoryCache menginisialisasi MemoryCache dengan opsi default
func NewMemoryCache() *MemoryCache {
	return NewMemoryCacheWithOptions(MemoryCacheOptions{
		Capacity:        10000,
		CleanupInterval: 1 * time.Minute,
	})
}

// NewMemoryCacheWithOptions menginisialisasi MemoryCache dengan konfigurasi kustom
func NewMemoryCacheWithOptions(opts MemoryCacheOptions) *MemoryCache {
	capacity := opts.Capacity
	if capacity <= 0 {
		capacity = 10000
	}

	cleanupInterval := opts.CleanupInterval
	if cleanupInterval <= 0 {
		cleanupInterval = 1 * time.Minute
	}

	c := &MemoryCache{
		items:      make(map[string]*list.Element),
		evictList:  list.New(),
		capacity:   capacity,
		stopClean:  make(chan struct{}),
		cleanTimer: time.NewTicker(cleanupInterval),
	}

	go c.cleanupLoop()

	return c
}

func (c *MemoryCache) cleanupLoop() {
	for {
		select {
		case <-c.cleanTimer.C:
			c.cleanupExpired()
		case <-c.stopClean:
			return
		}
	}
}

func (c *MemoryCache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, elem := range c.items {
		item := elem.Value.(*memItem)
		if !item.expiresAt.IsZero() && now.After(item.expiresAt) {
			c.evictList.Remove(elem)
			delete(c.items, key)
		}
	}
}

// Get mengambil nilai string berdasarkan key.
func (c *MemoryCache) Get(ctx context.Context, key string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, exists := c.items[key]
	if !exists {
		return "", false, nil
	}

	item := elem.Value.(*memItem)
	if !item.expiresAt.IsZero() && time.Now().After(item.expiresAt) {
		c.evictList.Remove(elem)
		delete(c.items, key)
		return "", false, nil
	}

	c.evictList.MoveToFront(elem)
	return item.value, true, nil
}

// Set menyimpan key-value dengan durasi time-to-live (TTL).
func (c *MemoryCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		item := elem.Value.(*memItem)
		item.value = value
		item.expiresAt = expiresAt
		c.evictList.MoveToFront(elem)
		return nil
	}

	if c.capacity > 0 && len(c.items) >= c.capacity {
		oldest := c.evictList.Back()
		if oldest != nil {
			oldestItem := oldest.Value.(*memItem)
			c.evictList.Remove(oldest)
			delete(c.items, oldestItem.key)
		}
	}

	item := &memItem{
		key:       key,
		value:     value,
		expiresAt: expiresAt,
	}
	elem := c.evictList.PushFront(item)
	c.items[key] = elem

	return nil
}

// Close menghentikan background cleaner goroutine dan membersihkan cache.
func (c *MemoryCache) Close() error {
	c.closeOnce.Do(func() {
		c.cleanTimer.Stop()
		close(c.stopClean)

		c.mu.Lock()
		defer c.mu.Unlock()
		c.items = make(map[string]*list.Element)
		c.evictList.Init()
	})
	return nil
}
