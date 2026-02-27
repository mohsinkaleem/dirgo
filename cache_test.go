package main

import (
	"testing"
	"time"
)

func TestLRUCachePutGet(t *testing.T) {
	c := newLRUCache(3)
	c.Put("/a", scanResultMsg{path: "/a", totalFiles: 1})
	c.Put("/b", scanResultMsg{path: "/b", totalFiles: 2})
	c.Put("/c", scanResultMsg{path: "/c", totalFiles: 3})

	if c.Len() != 3 {
		t.Fatalf("expected 3, got %d", c.Len())
	}

	v, ok := c.Get("/b")
	if !ok || v.totalFiles != 2 {
		t.Errorf("expected /b with totalFiles=2, got ok=%v totalFiles=%d", ok, v.totalFiles)
	}
}

func TestLRUCacheEviction(t *testing.T) {
	c := newLRUCache(2)
	c.Put("/a", scanResultMsg{path: "/a"})
	c.Put("/b", scanResultMsg{path: "/b"})
	c.Put("/c", scanResultMsg{path: "/c"}) // should evict /a

	if _, ok := c.Get("/a"); ok {
		t.Error("/a should have been evicted")
	}
	if _, ok := c.Get("/b"); !ok {
		t.Error("/b should still be present")
	}
	if _, ok := c.Get("/c"); !ok {
		t.Error("/c should still be present")
	}
}

func TestLRUCacheLRUOrder(t *testing.T) {
	c := newLRUCache(2)
	c.Put("/a", scanResultMsg{path: "/a"})
	c.Put("/b", scanResultMsg{path: "/b"})

	// Access /a to make it most recently used
	c.Get("/a")

	// Adding /c should evict /b (least recently used)
	c.Put("/c", scanResultMsg{path: "/c"})

	if _, ok := c.Get("/b"); ok {
		t.Error("/b should have been evicted")
	}
	if _, ok := c.Get("/a"); !ok {
		t.Error("/a should still be present")
	}
}

func TestLRUCacheDelete(t *testing.T) {
	c := newLRUCache(5)
	c.Put("/a", scanResultMsg{path: "/a"})
	c.Delete("/a")

	if _, ok := c.Get("/a"); ok {
		t.Error("/a should have been deleted")
	}
	if c.Len() != 0 {
		t.Errorf("expected length 0, got %d", c.Len())
	}
}

func TestLRUCacheUpdate(t *testing.T) {
	c := newLRUCache(5)
	c.Put("/a", scanResultMsg{path: "/a", totalFiles: 1})
	c.Put("/a", scanResultMsg{path: "/a", totalFiles: 99})

	v, _ := c.Get("/a")
	if v.totalFiles != 99 {
		t.Errorf("expected updated totalFiles=99, got %d", v.totalFiles)
	}
	if c.Len() != 1 {
		t.Errorf("expected length 1, got %d", c.Len())
	}
}

func TestLRUCacheDiskRoundtrip(t *testing.T) {
	// Use a temp directory to avoid corrupting the user's actual cache
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	c := newLRUCache(10)
	c.Put("/test/dir1", scanResultMsg{
		path:       "/test/dir1",
		totalFiles: 42,
		dirModTime: time.Now(),
		entries: []FileEntry{
			{Name: "file.go", Size: 1024, IsDir: false},
		},
	})

	if err := c.SaveToDisk(); err != nil {
		t.Fatalf("SaveToDisk: %v", err)
	}

	c2 := newLRUCache(10)
	if err := c2.LoadFromDisk(); err != nil {
		t.Fatalf("LoadFromDisk: %v", err)
	}

	v, ok := c2.Get("/test/dir1")
	if !ok {
		t.Fatal("expected to find /test/dir1 after load")
	}
	if v.totalFiles != 42 {
		t.Errorf("expected totalFiles=42, got %d", v.totalFiles)
	}
	if len(v.entries) != 1 || v.entries[0].Name != "file.go" {
		t.Error("entries not preserved through disk roundtrip")
	}
}

func TestFilterEntries(t *testing.T) {
	entries := []FileEntry{
		{Name: "visible.go", IsDir: false},
		{Name: ".hidden", IsDir: false, IsHidden: true},
		{Name: "src", IsDir: true},
		{Name: ".git", IsDir: true, IsHidden: true},
	}

	// No hidden, no dir-only
	f := FilterEntries(entries, false, FilterAll, "")
	if len(f) != 2 {
		t.Errorf("expected 2, got %d", len(f))
	}

	// Show hidden
	f = FilterEntries(entries, true, FilterAll, "")
	if len(f) != 4 {
		t.Errorf("expected 4, got %d", len(f))
	}

	// Dir only
	f = FilterEntries(entries, false, FilterDirsOnly, "")
	if len(f) != 1 {
		t.Errorf("expected 1 dir, got %d", len(f))
	}

	// Search filter
	f = FilterEntries(entries, true, FilterAll, "git")
	if len(f) != 1 || f[0].Name != ".git" {
		t.Errorf("expected .git match, got %v", f)
	}
}

func TestSortBySize(t *testing.T) {
	entries := []FileEntry{
		{Name: "small", Size: 100},
		{Name: "big", Size: 10000},
		{Name: "medium", Size: 1000},
	}
	SortBySize(entries)
	if entries[0].Name != "big" || entries[2].Name != "small" {
		t.Errorf("sort order wrong: %v", entries)
	}
}
