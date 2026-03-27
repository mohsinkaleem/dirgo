package main

import (
	"container/list"
	"sync"
)

// lruCache is a bounded in-memory LRU cache for scan results.
type lruCache struct {
	maxEntries int
	ll         *list.List
	items      map[string]*list.Element
	mu         sync.Mutex
}

type cacheItem struct {
	key   string
	value scanResultMsg
}

func newLRUCache(maxEntries int) *lruCache {
	return &lruCache{
		maxEntries: maxEntries,
		ll:         list.New(),
		items:      make(map[string]*list.Element),
	}
}

// Get retrieves a cached scan result. Returns (result, true) on hit.
func (c *lruCache) Get(key string) (scanResultMsg, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*cacheItem).value, true
	}
	return scanResultMsg{}, false
}

// Put stores a scan result in the cache, evicting the LRU entry if over capacity.
func (c *lruCache) Put(key string, val scanResultMsg) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		el.Value.(*cacheItem).value = val
		return
	}
	el := c.ll.PushFront(&cacheItem{key: key, value: val})
	c.items[key] = el
	if c.ll.Len() > c.maxEntries {
		c.evictOldest()
	}
}

// Delete removes a cached entry.
func (c *lruCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.Remove(el)
		delete(c.items, key)
	}
}

// Len returns the number of cached entries.
func (c *lruCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

func (c *lruCache) evictOldest() {
	el := c.ll.Back()
	if el != nil {
		c.ll.Remove(el)
		item := el.Value.(*cacheItem)
		delete(c.items, item.key)
	}
}


