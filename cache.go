package main

import (
	"container/list"
	"path/filepath"
	"strings"
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

// DeleteTree removes the cached listings of root and of every path below it.
func (c *lruCache) DeleteTree(root string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := root + string(filepath.Separator)
	for key, el := range c.items {
		if key == root || strings.HasPrefix(key, prefix) {
			c.ll.Remove(el)
			delete(c.items, key)
		}
	}
}

// Each calls fn for every cached entry without changing recency. fn must not
// call back into the cache.
func (c *lruCache) Each(fn func(key string, val scanResultMsg)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for el := c.ll.Front(); el != nil; el = el.Next() {
		item := el.Value.(*cacheItem)
		fn(item.key, item.value)
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
