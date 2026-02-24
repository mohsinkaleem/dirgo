package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Message types ---

// scanResultMsg is sent when Phase 1 (quick directory listing) completes.
type scanResultMsg struct {
	path        string
	entries     []FileEntry
	totalSize   int64
	totalFiles  int
	totalDirs   int
	dirModTime  time.Time
	pendingDirs []string // directory names still needing size computation
}

// scanErrorMsg is sent when a directory scan fails.
type scanErrorMsg struct {
	err error
}

// lineCountMsg is sent when line counting for a single file completes.
type lineCountMsg struct {
	name  string
	lines int
}

// dirSizeMsg is sent when a background directory size computation finishes (Phase 2).
type dirSizeMsg struct {
	Path       string // base directory (parent)
	Name       string // entry name
	Size       int64
	ChildFiles int
	ChildDirs  int
}

// batchLineCountMsg is sent when batch "count all" completes.
type batchLineCountMsg struct {
	Counts map[string]int // name → lineCount
}

// scanUpToDateMsg signals that a smart refresh found no changes.
type scanUpToDateMsg struct {
	path string
}

// --- Concurrency control ---

// scanSem limits concurrent directory walks. Sized to CPU count (capped at 16).
var scanSem = make(chan struct{}, minInt(runtime.NumCPU(), 16))

// --- Commands ---

// scanDirectory performs Phase 1: instant directory listing.
// Files are stat'd immediately; directories are marked SizeComputing.
// Returns scanResultMsg with pendingDirs for Phase 2.
func scanDirectory(path string) tea.Cmd {
	return func() tea.Msg {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return scanErrorMsg{err: err}
		}

		// Stat the directory itself for modtime
		dirInfo, err := os.Stat(absPath)
		if err != nil {
			return scanErrorMsg{err: err}
		}
		dirModTime := dirInfo.ModTime()

		dirEntries, err := os.ReadDir(absPath)
		if err != nil {
			return scanErrorMsg{err: err}
		}

		entries := make([]FileEntry, 0, len(dirEntries))
		var totalSize int64
		var totalFiles, totalDirs int
		var pendingDirs []string

		// Batched file stats: for dirs with > 100 files, parallelize Info() calls
		type fileResult struct {
			entry FileEntry
		}

		// Separate dirs and files
		var fileEntries []os.DirEntry
		for _, de := range dirEntries {
			if de.IsDir() || de.Type()&os.ModeSymlink != 0 {
				// Handle directory or symlink-to-directory
				name := de.Name()
				isHidden := strings.HasPrefix(name, ".")
				isSymlink := de.Type()&os.ModeSymlink != 0
				isDir := de.IsDir()

				// For symlinks, check if target is a directory
				if isSymlink && !isDir {
					target, err := os.Stat(filepath.Join(absPath, name))
					if err == nil && target.IsDir() {
						isDir = true
					}
				}

				if isDir {
					totalDirs++
					var modTime time.Time
					info, err := de.Info()
					if err == nil {
						modTime = info.ModTime()
					}
					entries = append(entries, FileEntry{
						Name:          name,
						IsDir:         true,
						IsHidden:      isHidden,
						IsSymlink:     isSymlink,
						SizeComputing: true,
						ModTime:       modTime,
					})
					pendingDirs = append(pendingDirs, name)
				} else {
					// Symlink to file
					fileEntries = append(fileEntries, de)
				}
			} else {
				fileEntries = append(fileEntries, de)
			}
		}

		// Stat files — parallel if large directory
		if len(fileEntries) > 100 {
			results := make([]FileEntry, len(fileEntries))
			var wg sync.WaitGroup
			sem := make(chan struct{}, runtime.NumCPU())
			for idx, de := range fileEntries {
				wg.Add(1)
				go func(i int, d os.DirEntry) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					name := d.Name()
					e := FileEntry{
						Name:      name,
						IsHidden:  strings.HasPrefix(name, "."),
						IsBinary:  isBinaryExt(name),
						IsSymlink: d.Type()&os.ModeSymlink != 0,
					}
					info, err := d.Info()
					if err == nil {
						e.Size = info.Size()
						e.ModTime = info.ModTime()
					}
					results[i] = e
				}(idx, de)
			}
			wg.Wait()
			for _, e := range results {
				totalFiles++
				totalSize += e.Size
				entries = append(entries, e)
			}
		} else {
			for _, de := range fileEntries {
				totalFiles++
				name := de.Name()
				e := FileEntry{
					Name:      name,
					IsHidden:  strings.HasPrefix(name, "."),
					IsBinary:  isBinaryExt(name),
					IsSymlink: de.Type()&os.ModeSymlink != 0,
				}
				info, err := de.Info()
				if err == nil {
					e.Size = info.Size()
					e.ModTime = info.ModTime()
				}
				totalSize += e.Size
				entries = append(entries, e)
			}
		}

		// Compute initial percentages (directories are 0 until Phase 2 completes)
		if totalSize > 0 {
			for i := range entries {
				entries[i].Percentage = float64(entries[i].Size) / float64(totalSize) * 100
			}
		}

		SortBySize(entries)

		return scanResultMsg{
			path:        absPath,
			entries:     entries,
			totalSize:   totalSize,
			totalFiles:  totalFiles,
			totalDirs:   totalDirs,
			dirModTime:  dirModTime,
			pendingDirs: pendingDirs,
		}
	}
}

// computeDirSizeCmd returns a tea.Cmd that computes the total size of a single
// subdirectory. It acquires a slot from the global semaphore to bound concurrency.
func computeDirSizeCmd(basePath, name string) tea.Cmd {
	return func() tea.Msg {
		scanSem <- struct{}{}
		defer func() { <-scanSem }()

		dirPath := filepath.Join(basePath, name)
		var size int64
		var files, dirs int

		filepath.WalkDir(dirPath, func(_ string, d os.DirEntry, err error) error {
			if err != nil {
				return filepath.SkipDir
			}
			if d.IsDir() {
				dirs++
				return nil
			}
			files++
			info, err := d.Info()
			if err == nil {
				size += info.Size()
			}
			return nil
		})
		// dirs includes the root dir itself
		if dirs > 0 {
			dirs--
		}

		return dirSizeMsg{
			Path:       basePath,
			Name:       name,
			Size:       size,
			ChildFiles: files,
			ChildDirs:  dirs,
		}
	}
}

// smartRefreshCmd checks if a directory has changed before triggering a full rescan.
func smartRefreshCmd(path string, cached scanResultMsg) tea.Cmd {
	return func() tea.Msg {
		info, err := os.Stat(path)
		if err != nil {
			return scanDirectory(path)() // fallback to full scan
		}
		if info.ModTime().Equal(cached.dirModTime) {
			return scanUpToDateMsg{path: path}
		}
		return scanDirectory(path)() // directory modified, full rescan
	}
}

// countLinesCmd returns a tea.Cmd to count lines for a specific file.
func countLinesCmd(dir, name string) tea.Cmd {
	return func() tea.Msg {
		path := filepath.Join(dir, name)
		lines, _, _ := countLines(path, 10*1024*1024) // 10MB max
		return lineCountMsg{name: name, lines: lines}
	}
}

// countAllLinesCmd returns a tea.Cmd that counts lines for all non-binary,
// non-directory entries. Uses bounded concurrency.
func countAllLinesCmd(entries []FileEntry, dir string) tea.Cmd {
	return func() tea.Msg {
		counts := make(map[string]int)
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, runtime.NumCPU())

		for _, e := range entries {
			if e.IsDir || e.IsBinary {
				continue
			}
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				path := filepath.Join(dir, name)
				lines, isBin, _ := countLines(path, 10*1024*1024)
				if !isBin && lines > 0 {
					mu.Lock()
					counts[name] = lines
					mu.Unlock()
				}
			}(e.Name)
		}
		wg.Wait()

		return batchLineCountMsg{Counts: counts}
	}
}
