package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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

// scanErrorMsg is sent when a directory scan fails. An empty path means the
// error did not come from a directory scan and always applies.
type scanErrorMsg struct {
	path string
	err  error
}

// lineCountMsg is sent when line counting for a single file completes.
type lineCountMsg struct {
	dir   string
	name  string
	lines int
}

// batchLineCountMsg is sent when batch "count all" completes.
type batchLineCountMsg struct {
	Dir    string
	Counts map[string]int // name → lineCount
}

// scanUpToDateMsg signals that a smart refresh found no changes.
type scanUpToDateMsg struct {
	path string
}

// ScanProgress holds live progress counters updated by the scanner goroutine.
// Read via atomic loads from the UI goroutine (spinner tick). Start is written
// once before the scan goroutine launches and is read-only thereafter.
type ScanProgress struct {
	Files atomic.Int64
	Dirs  atomic.Int64
	Size  atomic.Int64
	Start time.Time
}

func newScanProgress() *ScanProgress {
	return &ScanProgress{Start: time.Now()}
}

// --- Commands ---

// scanDirectory performs a full directory listing with sizes computed upfront.
// Directory sizes are computed in parallel using bounded concurrency.
// If prog is non-nil, progress counters are updated as the scan proceeds.
func scanDirectory(path string, prog *ScanProgress) tea.Cmd {
	return func() tea.Msg {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return scanErrorMsg{path: path, err: err}
		}

		// Stat the directory itself for modtime
		dirInfo, err := os.Stat(absPath)
		if err != nil {
			return scanErrorMsg{path: absPath, err: err}
		}
		dirModTime := dirInfo.ModTime()

		dirEntries, err := os.ReadDir(absPath)
		if err != nil {
			return scanErrorMsg{path: absPath, err: err}
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

			parallelFor(len(dirEntryIndices), minInt(runtime.NumCPU(), 16), func(ri int) {
				info := dirEntryIndices[ri]
				dirPath := filepath.Join(absPath, info.name)
				// Use os.ReadDir + manual recursion instead of filepath.WalkDir
				// to reduce syscall overhead (one getdirentries per dir vs Lstat per entry)
				size, files, dirs := dirSizeRecursive(dirPath, prog)
				results[ri] = dirResult{index: info.index, size: size, childFiles: files, childDirs: dirs}
			})

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
			parallelFor(len(fileEntries), runtime.NumCPU(), func(i int) {
				d := fileEntries[i]
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
				if prog != nil {
					prog.Files.Add(1)
					prog.Size.Add(e.Size)
				}
				results[i] = e
			})
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
func smartRefreshCmd(path string, cached scanResultMsg, prog *ScanProgress) tea.Cmd {
	return func() tea.Msg {
		info, err := os.Stat(path)
		if err != nil {
			return scanDirectory(path, prog)() // fallback to full scan
		}
		if info.ModTime().Equal(cached.dirModTime) {
			return scanUpToDateMsg{path: path}
		}
		return scanDirectory(path, prog)() // directory modified, full rescan
	}
}

// countLinesCmd returns a tea.Cmd to count lines for a specific file.
func countLinesCmd(dir, name string) tea.Cmd {
	return func() tea.Msg {
		path := filepath.Join(dir, name)
		lines, _, _ := countLines(path, 10*1024*1024) // 10MB max
		return lineCountMsg{dir: dir, name: name, lines: lines}
	}
}

// countAllLinesCmd returns a tea.Cmd that counts lines for all non-binary,
// non-directory entries. Uses bounded concurrency.
func countAllLinesCmd(entries []FileEntry, dir string) tea.Cmd {
	return func() tea.Msg {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir || e.IsBinary {
				continue
			}
			names = append(names, e.Name)
		}

		counts := make(map[string]int, len(names))
		var mu sync.Mutex
		parallelFor(len(names), runtime.NumCPU(), func(i int) {
			name := names[i]
			lines, isBin, _ := countLines(filepath.Join(dir, name), 10*1024*1024)
			if !isBin && lines > 0 {
				mu.Lock()
				counts[name] = lines
				mu.Unlock()
			}
		})

		return batchLineCountMsg{Dir: dir, Counts: counts}
	}
}

// parallelFor runs fn(i) for every i in [0, n) using at most workers goroutines.
// Unlike a goroutine-per-item fan-out, this keeps memory flat on directories
// with hundreds of thousands of entries.
func parallelFor(n, workers int, fn func(i int)) {
	if n <= 0 {
		return
	}
	if workers < 1 {
		workers = 1
	}
	if workers > n {
		workers = n
	}
	var next atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				fn(i)
			}
		}()
	}
	wg.Wait()
}

// dirSizeRecursive computes the total size, file count, and subdirectory count
// of a directory using os.ReadDir + manual recursion. This is more efficient than
// filepath.WalkDir because os.ReadDir uses a single getdirentries syscall per
// directory, and we only call Info() on files (not dirs) since we only need file sizes.
func dirSizeRecursive(path string, prog *ScanProgress) (size int64, files int, dirs int) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, 0, 0
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs++
			if prog != nil {
				prog.Dirs.Add(1)
			}
			s, f, d := dirSizeRecursive(filepath.Join(path, e.Name()), prog)
			size += s
			files += f
			dirs += d
		} else {
			files++
			if info, err := e.Info(); err == nil {
				size += info.Size()
				if prog != nil {
					prog.Files.Add(1)
					prog.Size.Add(info.Size())
				}
			}
		}
	}
	return size, files, dirs
}
