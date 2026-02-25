package main

import (
	"container/list"
	"encoding/gob"
	"os"
	"path/filepath"
	"sync"
	"time"
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

// --- Disk persistence ---

// serializedEntry is the gob-friendly representation of a cached scan result.
type serializedEntry struct {
	Path       string
	Entries    []FileEntry
	TotalSize  int64
	TotalFiles int
	TotalDirs  int
	DirModTime time.Time
	CachedAt   time.Time
}

type serializedCache struct {
	Version int
	Items   []serializedEntry
}

const (
	cacheVersion = 1
	cacheDir     = ".cache/dirgo"
	cacheFile    = "cache.gob"
	cacheMaxAge  = 24 * time.Hour
	diskCacheMax = 50
)

func cacheFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, cacheDir, cacheFile)
}

// SaveToDisk serializes the top N LRU entries to disk.
func (c *lruCache) SaveToDisk() error {
	path := cacheFilePath()
	if path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	c.mu.Lock()
	var items []serializedEntry
	count := 0
	for el := c.ll.Front(); el != nil && count < diskCacheMax; el = el.Next() {
		ci := el.Value.(*cacheItem)
		items = append(items, serializedEntry{
			Path:       ci.value.path,
			Entries:    ci.value.entries,
			TotalSize:  ci.value.totalSize,
			TotalFiles: ci.value.totalFiles,
			TotalDirs:  ci.value.totalDirs,
			DirModTime: ci.value.dirModTime,
			CachedAt:   time.Now(),
		})
		count++
	}
	c.mu.Unlock()

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return gob.NewEncoder(f).Encode(serializedCache{Version: cacheVersion, Items: items})
}

// LoadFromDisk loads cached scan results from disk, discarding stale entries.
func (c *lruCache) LoadFromDisk() error {
	path := cacheFilePath()
	if path == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	var sc serializedCache
	if err := gob.NewDecoder(f).Decode(&sc); err != nil {
		return nil // corrupted cache, just ignore
	}

	// Skip incompatible cache versions
	if sc.Version != cacheVersion {
		return nil
	}

	now := time.Now()
	// Load in reverse order so that the first item ends up at the front (most recent)
	for i := len(sc.Items) - 1; i >= 0; i-- {
		item := sc.Items[i]
		if now.Sub(item.CachedAt) > cacheMaxAge {
			continue // stale
		}
		c.Put(item.Path, scanResultMsg{
			path:       item.Path,
			entries:    item.Entries,
			totalSize:  item.TotalSize,
			totalFiles: item.TotalFiles,
			totalDirs:  item.TotalDirs,
			dirModTime: item.DirModTime,
		})
	}
	return nil
}
