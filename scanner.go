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

// scanResultMsg is sent when directory scanning completes.
type scanResultMsg struct {
	path       string
	entries    []FileEntry
	totalSize  int64
	totalFiles int
	totalDirs  int
	dirModTime time.Time
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

// batchLineCountMsg is sent when batch "count all" completes.
type batchLineCountMsg struct {
	Counts map[string]int // name → lineCount
}

// scanUpToDateMsg signals that a smart refresh found no changes.
type scanUpToDateMsg struct {
	path string
}

// --- Commands ---

// scanDirectory performs a full directory listing with sizes computed upfront.
// Directory sizes are computed in parallel using bounded concurrency.
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

		// Separate dirs and files
		type dirInfo2 struct {
			index int
			name  string
		}
		fileEntries := make([]os.DirEntry, 0, len(dirEntries))
		dirEntryIndices := make([]dirInfo2, 0, len(dirEntries)/4+1)

		for _, de := range dirEntries {
			if de.IsDir() || de.Type()&os.ModeSymlink != 0 {
				name := de.Name()
				isHidden := strings.HasPrefix(name, ".")
				isSymlink := de.Type()&os.ModeSymlink != 0
				isDir := de.IsDir()

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
					idx := len(entries)
					entries = append(entries, FileEntry{
						Name:      name,
						IsDir:     true,
						IsHidden:  isHidden,
						IsSymlink: isSymlink,
						ModTime:   modTime,
					})
					dirEntryIndices = append(dirEntryIndices, dirInfo2{index: idx, name: name})
				} else {
					fileEntries = append(fileEntries, de)
				}
			} else {
				fileEntries = append(fileEntries, de)
			}
		}

		// Compute directory sizes in parallel
		if len(dirEntryIndices) > 0 {
			type dirResult struct {
				index      int
				size       int64
				childFiles int
				childDirs  int
			}
			results := make([]dirResult, len(dirEntryIndices))
			var wg sync.WaitGroup
			sem := make(chan struct{}, minInt(runtime.NumCPU(), 16))

			for ri, di := range dirEntryIndices {
				wg.Add(1)
				go func(resultIdx int, info dirInfo2) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					dirPath := filepath.Join(absPath, info.name)
					// Use os.ReadDir + manual recursion instead of filepath.WalkDir
					// to reduce syscall overhead (one getdirentries per dir vs Lstat per entry)
					size, files, dirs := dirSizeRecursive(dirPath)
					results[resultIdx] = dirResult{index: info.index, size: size, childFiles: files, childDirs: dirs}
				}(ri, di)
			}
			wg.Wait()

			// Apply results back to entries
			for _, r := range results {
				entries[r.index].Size = r.size
				entries[r.index].ChildFiles = r.childFiles
				entries[r.index].ChildDirs = r.childDirs
				totalSize += r.size
			}
		}

		// Stat files — parallel if large directory
		if len(fileEntries) > 20 {
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

		// Compute final percentages
		if totalSize > 0 {
			for i := range entries {
				entries[i].Percentage = float64(entries[i].Size) / float64(totalSize) * 100
			}
		}

		SortBySize(entries)

		return scanResultMsg{
			path:       absPath,
			entries:    entries,
			totalSize:  totalSize,
			totalFiles: totalFiles,
			totalDirs:  totalDirs,
			dirModTime: dirModTime,
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

// dirSizeRecursive computes the total size, file count, and subdirectory count
// of a directory using os.ReadDir + manual recursion. This is more efficient than
// filepath.WalkDir because os.ReadDir uses a single getdirentries syscall per
// directory, and we only call Info() on files (not dirs) since we only need file sizes.
func dirSizeRecursive(path string) (size int64, files int, dirs int) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs++
			s, f, d := dirSizeRecursive(filepath.Join(path, e.Name()))
			size += s
			files += f
			dirs += d
		} else {
			files++
			if info, err := e.Info(); err == nil {
				size += info.Size()
			}
		}
	}
	return size, files, dirs
}
