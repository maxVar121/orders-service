// cache/cache.go
package cache

import (
	"encoding/gob"
	"os"
	"sync"
	"time"

	"orders-service/model"
)

// Item represents a cached order with an optional expiration time
type Item struct {
	Order      model.Order
	Expiration int64 // Unix time in nanoseconds
}

// IsExpired checks if the item has passed its expiration time
func (item Item) IsExpired() bool {
	if item.Expiration == 0 {
		return false
	}
	return time.Now().UnixNano() > item.Expiration
}

const (
	NoExpiration     = -1 * time.Second
	DefaultTTL       = 10 * time.Minute // Default time-to-live for cached orders
	gcInterval       = 30 * time.Second // GC runs every 30 seconds
)

// Cache is a thread-safe in-memory cache for orders with TTL and persistence
type Cache struct {
	items        map[string]Item
	mu           sync.RWMutex
	gcInterval   time.Duration
	stopGC       chan bool
	cacheFile    string
}

// gcLoop runs periodic cleanup of expired items in the background
func (c *Cache) gcLoop() {
	ticker := time.NewTicker(c.gcInterval)
	for {
		select {
		case <-ticker.C:
			c.DeleteExpired()
		case <-c.stopGC:
			ticker.Stop()
			return
		}
	}
}

// delete removes an item from the map (caller must hold lock)
func (c *Cache) delete(k string) {
	delete(c.items, k)
}

// DeleteExpired removes all expired items from the cache
func (c *Cache) DeleteExpired() {
	now := time.Now().UnixNano()
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, v := range c.items {
		if v.Expiration > 0 && now > v.Expiration {
			c.delete(k)
		}
	}
}

// New creates a new in-memory cache with GC and file persistence support
func New(cacheFile string) *Cache {
	if cacheFile == "" {
		cacheFile = "order_cache.gob"
	}

	cache := &Cache{
		items:      make(map[string]Item),
		gcInterval: gcInterval,
		stopGC:     make(chan bool),
		cacheFile:  cacheFile,
	}

	go cache.gcLoop()

	return cache
}

// Set adds an order to the cache with optional TTL
func (c *Cache) Set(order model.Order, d time.Duration) {
	var e int64

	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[order.OrderUID] = Item{
		Order: order,
		Expiration: e,
	}
}

// Get retrieves an order from the cache if it exists and is not expired
func (c *Cache) Get(orderUID string) (model.Order, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, found := c.items[orderUID]
	if !found {
		return model.Order{}, false
	}
	return item.Order, true
}

// DeleteExpired removes all expired items from the cache
func (c *Cache) Delete(orderUID string) {
	c.mu.Lock()
	delete(c.items, orderUID)
	c.mu.Unlock()
}

// SaveToFile safely dumps the current cache state to a file for persistence
func (c *Cache) SaveToFile() error {
	c.mu.RLock()
	items := make(map[string]Item, len(c.items))
	for k, v := range c.items {
		items[k] = v
	}
	c.mu.RUnlock()

	file, err := os.Create(c.cacheFile)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	return encoder.Encode(items)
}

func (c *Cache) Stop() {
	c.stopGC <- true
}

// LoadFromFile restores the cache from a persisted file if it exists
func (c *Cache) LoadFromFile() error {
	file, err := os.Open(c.cacheFile)
	if err != nil {
		return err // file may not exist on first run
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)

	var items map[string]Item

	if err := decoder.Decode(&items); err != nil {
		return err
	}

	c.mu.Lock()
	c.items = items
	c.mu.Unlock()

	return nil
}