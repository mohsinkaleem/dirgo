package main

import (
	"sort"
	"time"
)

// FileEntry represents a file or directory with its metadata.
type FileEntry struct {
	Name          string
	Size          int64
	IsDir         bool
	IsHidden      bool
	IsBinary      bool
	IsSymlink     bool
	LineCount     int // 0 if unknown/binary/dir
	Percentage    float64
	ChildFiles    int // only for dirs
	ChildDirs     int // only for dirs
	ModTime       time.Time
	SizeComputing bool // true while background size computation is in progress
}

// SortBySize sorts entries by size descending.
func SortBySize(entries []FileEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Size > entries[j].Size
	})
}

// FilterEntries returns a filtered slice based on visibility and directory-only settings.
func FilterEntries(entries []FileEntry, showHidden, dirOnly bool, search string) []FileEntry {
	result := make([]FileEntry, 0, len(entries))
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
		result = append(result, e)
	}
	return result
}

// fuzzyMatch does a simple case-insensitive substring match.
func fuzzyMatch(name, pattern string) bool {
	nl := len(name)
	pl := len(pattern)
	if pl > nl {
		return false
	}
	for i := 0; i <= nl-pl; i++ {
		match := true
		for j := 0; j < pl; j++ {
			nc := name[i+j]
			pc := pattern[j]
			if nc >= 'A' && nc <= 'Z' {
				nc += 32
			}
			if pc >= 'A' && pc <= 'Z' {
				pc += 32
			}
			if nc != pc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
