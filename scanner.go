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

// subtreeTotals is the recursive size and entry counts of one directory tree,
// counted the way sizeTrees counts them: symlinks are leaves, never followed.
type subtreeTotals struct {
	size  int64
	files int
	dirs  int
}

// subtreeOf derives a directory's recursive totals from its own scan result,
// so a parent scan can reuse a child that was already scanned.
func subtreeOf(res scanResultMsg) subtreeTotals {
	t := subtreeTotals{size: res.totalSize}
	for _, e := range res.entries {
		if e.IsDir && !e.IsSymlink {
			t.files += e.ChildFiles
			t.dirs += e.ChildDirs + 1
		} else {
			t.files++
		}
	}
	return t
}

// scanDirectory performs a full directory listing with sizes computed upfront.
// Directory sizes are computed in parallel using bounded concurrency.
// If prog is non-nil, progress counters are updated as the scan proceeds.
// known holds already-computed totals for subdirectories (by name); those are
// reused instead of being walked again.
func scanDirectory(path string, prog *ScanProgress, known map[string]subtreeTotals) tea.Cmd {
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

		// Separate dirs and files. Symlinks to directories are listed as
		// directories so they can be opened, but are sized as the link itself:
		// following them would double-count their target (e.g. /etc and
		// /private/etc on macOS) or walk an entire disk via a link to /.
		fileEntries := make([]os.DirEntry, 0, len(dirEntries))
		var walkIdx []int // indices into entries of directories to walk
		var walkPaths []string

		for _, de := range dirEntries {
			name := de.Name()
			isSymlink := de.Type()&os.ModeSymlink != 0
			if de.IsDir() {
				totalDirs++
				var modTime time.Time
				if info, err := de.Info(); err == nil {
					modTime = info.ModTime()
				}
				e := FileEntry{
					Name:     name,
					IsDir:    true,
					IsHidden: strings.HasPrefix(name, "."),
					ModTime:  modTime,
				}
				if t, ok := known[name]; ok {
					e.Size, e.ChildFiles, e.ChildDirs = t.size, t.files, t.dirs
					totalSize += t.size
					if prog != nil {
						prog.Files.Add(int64(t.files))
						prog.Dirs.Add(int64(t.dirs))
						prog.Size.Add(t.size)
					}
				} else {
					walkIdx = append(walkIdx, len(entries))
					walkPaths = append(walkPaths, filepath.Join(absPath, name))
				}
				entries = append(entries, e)
				continue
			}
			if isSymlink {
				if target, err := os.Stat(filepath.Join(absPath, name)); err == nil && target.IsDir() {
					totalDirs++
					e := FileEntry{
						Name:      name,
						IsDir:     true,
						IsHidden:  strings.HasPrefix(name, "."),
						IsSymlink: true,
					}
					if info, err := de.Info(); err == nil {
						e.Size = info.Size()
						e.ModTime = info.ModTime()
					}
					totalSize += e.Size
					entries = append(entries, e)
					continue
				}
			}
			fileEntries = append(fileEntries, de)
		}

		// Compute directory sizes in parallel
		if len(walkPaths) > 0 {
			results := sizeTrees(walkPaths, scanWorkers(), prog)
			for i, r := range results {
				e := &entries[walkIdx[i]]
				e.Size, e.ChildFiles, e.ChildDirs = r.size, r.files, r.dirs
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

		setPercentages(entries, totalSize)
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

// setPercentages sets each entry's share of total.
func setPercentages(entries []FileEntry, total int64) {
	for i := range entries {
		if total > 0 {
			entries[i].Percentage = float64(entries[i].Size) / float64(total) * 100
		} else {
			entries[i].Percentage = 0
		}
	}
}

// smartRefreshCmd checks if a directory has changed before triggering a full rescan.
func smartRefreshCmd(path string, cached scanResultMsg, prog *ScanProgress) tea.Cmd {
	return func() tea.Msg {
		info, err := os.Stat(path)
		if err != nil {
			return scanDirectory(path, prog, nil)() // fallback to full scan
		}
		if info.ModTime().Equal(cached.dirModTime) {
			return scanUpToDateMsg{path: path}
		}
		return scanDirectory(path, prog, nil)() // directory modified, full rescan
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
	// Collect names now, not in the command: entries is usually the model's
	// filtered slice, whose array the UI goroutine reuses on the next filter.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir || e.IsBinary {
			continue
		}
		names = append(names, e.Name)
	}
	return func() tea.Msg {
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

// scanWorkers is the number of goroutines walking directory trees, capped so
// slow or network disks are not flooded. Measured on an SSD, going wider than
// this gains nothing.
func scanWorkers() int {
	return minInt(runtime.NumCPU(), 16)
}

// dirJob is one directory waiting to be read, tagged with the index of the
// top-level tree whose totals it adds to.
type dirJob struct {
	path string
	root int
}

// dirQueue is a LIFO of directories shared by the sizeTrees workers. Taking
// the newest job first walks depth-first, which keeps the queue short.
type dirQueue struct {
	mu      sync.Mutex
	cond    *sync.Cond
	stack   []dirJob
	pending int // jobs queued or being processed
}

func newDirQueue(jobs []dirJob) *dirQueue {
	q := &dirQueue{stack: jobs, pending: len(jobs)}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// pop returns the next directory to read, blocking while other workers may
// still queue more. It reports false once every job is finished.
func (q *dirQueue) pop() (dirJob, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.stack) == 0 {
		if q.pending == 0 {
			return dirJob{}, false
		}
		q.cond.Wait()
	}
	last := len(q.stack) - 1
	job := q.stack[last]
	q.stack[last] = dirJob{}
	q.stack = q.stack[:last]
	return job, true
}

// finish marks one job done and queues the subdirectories it found.
func (q *dirQueue) finish(children []dirJob) {
	q.mu.Lock()
	q.stack = append(q.stack, children...)
	q.pending += len(children) - 1
	wake := len(children) > 0 || q.pending == 0
	q.mu.Unlock()
	if wake {
		q.cond.Broadcast()
	}
}

// sizeTrees computes the recursive size, file count and subdirectory count of
// each root. Workers share one queue of directories rather than taking a root
// each, so a single huge child (node_modules, ~/Library, a module cache) is
// spread over every worker instead of leaving the others idle.
//
// Directories are read with File.ReadDir, which skips the name sort that
// os.ReadDir does, and Info (an lstat) is only called for non-directories.
// Symlinks are counted as files and never followed.
func sizeTrees(roots []string, workers int, prog *ScanProgress) []subtreeTotals {
	type acc struct{ size, files, dirs atomic.Int64 }
	accs := make([]acc, len(roots))

	jobs := make([]dirJob, len(roots))
	for i, r := range roots {
		jobs[i] = dirJob{path: r, root: i}
	}
	q := newDirQueue(jobs)

	var wg sync.WaitGroup
	for w := 0; w < maxInt(1, workers); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var children []dirJob
			for {
				job, ok := q.pop()
				if !ok {
					return
				}
				children = children[:0]
				var size, files int64
				for _, e := range readDirUnsorted(job.path) {
					if e.IsDir() {
						children = append(children, dirJob{path: job.path + string(os.PathSeparator) + e.Name(), root: job.root})
						continue
					}
					files++
					if info, err := e.Info(); err == nil {
						size += info.Size()
					}
				}
				// One atomic add per directory, not per file: the counters
				// are shared by every worker.
				a := &accs[job.root]
				a.size.Add(size)
				a.files.Add(files)
				a.dirs.Add(int64(len(children)))
				if prog != nil {
					prog.Files.Add(files)
					prog.Dirs.Add(int64(len(children)))
					prog.Size.Add(size)
				}
				q.finish(children)
			}
		}()
	}
	wg.Wait()

	out := make([]subtreeTotals, len(roots))
	for i := range accs {
		out[i] = subtreeTotals{
			size:  accs[i].size.Load(),
			files: int(accs[i].files.Load()),
			dirs:  int(accs[i].dirs.Load()),
		}
	}
	return out
}

// readDirUnsorted lists a directory in on-disk order. Errors yield whatever
// entries were read before the failure, matching how unreadable directories
// were always counted (as empty).
func readDirUnsorted(path string) []os.DirEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	entries, _ := f.ReadDir(-1)
	return entries
}
