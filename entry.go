package main

import (
	"sort"
	"time"
)

// FileEntry represents a file or directory with its metadata.
type FileEntry struct {
	Name       string
	Size       int64
	IsDir      bool
	IsHidden   bool
	IsBinary   bool
	IsSymlink  bool
	LineCount  int // 0 if unknown/binary/dir
	Percentage float64
	ChildFiles int // only for dirs
	ChildDirs  int // only for dirs
	ModTime    time.Time
}

// SortBySize sorts entries by size descending.
func SortBySize(entries []FileEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Size > entries[j].Size
	})
}

// FilterEntries returns a filtered slice based on visibility and directory-only settings.
func FilterEntries(entries []FileEntry, showHidden, dirOnly bool, search string) []FileEntry {
	return filterEntriesInto(make([]FileEntry, 0, len(entries)), entries, showHidden, dirOnly, search)
}

// filterEntriesInto appends filtered entries into dst, allowing callers to reuse slices.
func filterEntriesInto(dst []FileEntry, entries []FileEntry, showHidden, dirOnly bool, search string) []FileEntry {
	for _, e := range entries {
		if !showHidden && e.IsHidden {
			continue
		}
		if dirOnly && !e.IsDir {
			continue
		}
		if search != "" && !fuzzyMatch(e.Name, search) {
			continue
		}
		dst = append(dst, e)
	}
	return dst
}

// toLower returns the ASCII-lowered byte (only for A-Z).
func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + 32
	}
	return b
}

// fuzzyMatch performs a case-insensitive subsequence match.
// E.g. "mgo" matches "model.go", "rdm" matches "README.md".
func fuzzyMatch(name, pattern string) bool {
	pi := 0
	for ni := 0; ni < len(name) && pi < len(pattern); ni++ {
		if toLower(name[ni]) == toLower(pattern[pi]) {
			pi++
		}
	}
	return pi == len(pattern)
}
