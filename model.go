package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model holds the entire application state.
type Model struct {
	// Directory state
	path       string
	entries    []FileEntry // all entries (unfiltered)
	filtered   []FileEntry // entries after filter/search
	totalSize  int64
	totalFiles int
	totalDirs  int

	// Deep totals (including all subdirectories recursively)
	deepTotalFiles int64
	deepTotalDirs  int64

	// Navigation
	cursor int
	offset int

	// Scan cache: LRU with bounded size
	cache *lruCache

	// In-flight scans by path. Re-entering a directory while its scan is
	// still running joins that scan instead of starting a second one.
	scans map[string]*ScanProgress

	// Full paths with a trash operation in flight, so a repeated 'd' on the
	// same row is ignored.
	trashing map[string]bool

	// One-line footer message for action results; cleared on the next key.
	status    string
	statusErr bool

	// Scan progress: shared with scanner goroutine
	scanProg      *ScanProgress
	scanProgFiles int64 // snapshot for display
	scanProgDirs  int64
	scanProgSize  int64
	scanElapsed   time.Duration

	// View
	width  int
	height int

	// Reusable string builder for rendering
	viewBuf *strings.Builder

	// Cached separator string (rebuilt only when width changes)
	cachedSep      string
	cachedSepWidth int

	// Cursor history: remembers selected entry name per directory path
	cursorHistory      map[string]string
	pendingCursorEntry string

	// Modes
	loading    bool
	showHidden bool
	viewFilter ViewFilter
	topMode    bool
	helpMode   bool
	searchMode bool
	gotoMode   bool

	// Stale cache indicator: true when viewing cached (not freshly scanned) data
	fromCache bool

	// Components
	spinner     spinner.Model
	searchInput textinput.Model
	gotoInput   textinput.Model
	keys        KeyMap

	// Error from scanning the current directory (replaces the listing)
	err error
}

// NewModel creates an initial model for the given path.
func NewModel(path string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.CharLimit = 64
	ti.Width = 30

	gi := textinput.New()
	gi.Placeholder = "path (e.g. ~/Downloads, /tmp)..."
	gi.CharLimit = 256
	gi.Width = 50

	cache := newLRUCache(100)
	prog := newScanProgress()

	return Model{
		path:          path,
		loading:       true,
		showHidden:    true,
		keys:          DefaultKeyMap(),
		spinner:       s,
		searchInput:   ti,
		gotoInput:     gi,
		cursorHistory: make(map[string]string),
		cache:         cache,
		viewBuf:       &strings.Builder{},
		scanProg:      prog,
		scans:         map[string]*ScanProgress{path: prog},
		trashing:      make(map[string]bool),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(scanDirectory(m.path, m.scanProg, nil), m.spinner.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Rebuild separator cache (View has value receiver, so caching there is lost)
		m.cachedSep = lipgloss.NewStyle().Foreground(colorDimmer).Width(m.width).Render(strings.Repeat("─", m.width))
		m.cachedSepWidth = m.width
		// A shorter window can leave the cursor below the visible rows.
		m.ensureVisible()
		return m, nil

	case tea.MouseMsg:
		if !m.loading && !m.helpMode {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m = m.moveCursor(-3)
				return m, m.lineCountForSelected()
			case tea.MouseButtonWheelDown:
				m = m.moveCursor(3)
				return m, m.lineCountForSelected()
			}
		}
		return m, nil

	case scanResultMsg:
		// Phase 1 complete — populate entries immediately
		m.cache.Put(msg.path, msg)
		delete(m.scans, msg.path)
		if msg.path != m.path {
			// Result for a directory we already navigated away from; keep it cached only.
			return m, nil
		}
		selectName := m.pendingCursorEntry
		m.pendingCursorEntry = ""
		m.leaveScanView()
		m.fromCache = false
		m.err = nil
		m.setListing(msg)
		m.cursor = 0
		m.offset = 0
		// Restore cursor to remembered entry (e.g., when navigating up to parent)
		m.selectName(selectName)
		return m, m.lineCountForSelected()

	case scanUpToDateMsg:
		// Smart refresh: nothing changed
		delete(m.scans, msg.path)
		if msg.path != m.path {
			return m, nil
		}
		m.pendingCursorEntry = ""
		m.leaveScanView()
		m.fromCache = false // modtime check confirmed the listing is current
		return m, nil

	case scanErrorMsg:
		delete(m.scans, msg.path)
		if msg.path != m.path {
			return m, nil
		}
		m.pendingCursorEntry = ""
		m.leaveScanView()
		m.err = msg.err
		// Nothing can be listed, so drop any rows left over from before:
		// they would otherwise stay selectable (and deletable) behind the error.
		m.entries = nil
		m.applyFilter()
		m.cursor = 0
		m.offset = 0
		return m, nil

	case statusMsg:
		m.setStatus(msg.text, msg.isErr)
		return m, nil

	case trashResultMsg:
		delete(m.trashing, filepath.Join(msg.dir, msg.entry.Name))
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		m.setStatus("Moved "+msg.entry.Name+" to Trash", false)
		// Patch the listings in place instead of invalidating them, so going
		// back to this directory or any parent never needs a rescan.
		m.forgetEntry(msg.dir, msg.entry, msg.dirModTime)
		return m, m.lineCountForSelected()

	case lineCountMsg:
		if msg.dir != m.path {
			return m, nil
		}
		// Update line count for matching entry
		for i := range m.entries {
			if m.entries[i].Name == msg.name {
				m.entries[i].LineCount = msg.lines
				break
			}
		}
		for i := range m.filtered {
			if m.filtered[i].Name == msg.name {
				m.filtered[i].LineCount = msg.lines
				break
			}
		}
		// Write-back into cache
		if cached, ok := m.cache.Get(m.path); ok {
			for i := range cached.entries {
				if cached.entries[i].Name == msg.name {
					cached.entries[i].LineCount = msg.lines
					m.cache.Put(m.path, cached)
					break
				}
			}
		}
		return m, nil

	case batchLineCountMsg:
		// Batch line count completed (s key)
		if msg.Dir != m.path {
			return m, nil
		}
		for i := range m.entries {
			if c, ok := msg.Counts[m.entries[i].Name]; ok {
				m.entries[i].LineCount = c
			}
		}
		for i := range m.filtered {
			if c, ok := msg.Counts[m.filtered[i].Name]; ok {
				m.filtered[i].LineCount = c
			}
		}
		// Update cache
		if cached, ok := m.cache.Get(m.path); ok {
			for i := range cached.entries {
				if c, ok := msg.Counts[cached.entries[i].Name]; ok {
					cached.entries[i].LineCount = c
				}
			}
			m.cache.Put(m.path, cached)
		}
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			// Read scan progress for display
			if m.scanProg != nil {
				m.scanProgFiles = m.scanProg.Files.Load()
				m.scanProgDirs = m.scanProg.Dirs.Load()
				m.scanProgSize = m.scanProg.Size.Load()
				m.scanElapsed = time.Since(m.scanProg.Start)
			}
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		m.status = ""

		// If in goto mode, handle text input first
		if m.gotoMode {
			switch {
			case key.Matches(msg, m.keys.Escape):
				m.gotoMode = false
				m.gotoInput.SetValue("")
				m.gotoInput.Blur()
				return m, nil
			case msg.Type == tea.KeyEnter:
				m.gotoMode = false
				m.gotoInput.Blur()
				target := m.gotoInput.Value()
				m.gotoInput.SetValue("")
				if target == "" {
					return m, nil
				}
				return m.navigateTo(target)
			default:
				var cmd tea.Cmd
				m.gotoInput, cmd = m.gotoInput.Update(msg)
				return m, cmd
			}
		}

		// If in search mode, handle text input first
		if m.searchMode {
			switch {
			case key.Matches(msg, m.keys.Escape):
				m.searchMode = false
				m.searchInput.SetValue("")
				m.searchInput.Blur()
				m.refilter()
				return m, nil
			case msg.Type == tea.KeyEnter:
				m.searchMode = false
				m.searchInput.Blur()
				return m, nil
			// Arrow keys only: the k/j aliases must stay typeable in the query.
			case msg.Type == tea.KeyUp:
				m = m.moveCursor(-1)
				return m, m.lineCountForSelected()
			case msg.Type == tea.KeyDown:
				m = m.moveCursor(1)
				return m, m.lineCountForSelected()
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				m.applyFilter()
				m.cursor = 0
				m.offset = 0
				return m, cmd
			}
		}

		// If in help mode, any key closes it
		if m.helpMode {
			if key.Matches(msg, m.keys.Escape) || key.Matches(msg, m.keys.Help) || msg.Type == tea.KeyEnter {
				m.helpMode = false
			}
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Up):
			m = m.moveCursor(-1)
			return m, m.lineCountForSelected()

		case key.Matches(msg, m.keys.Down):
			m = m.moveCursor(1)
			return m, m.lineCountForSelected()

		case key.Matches(msg, m.keys.Top):
			m.cursor = 0
			m.offset = 0
			return m, m.lineCountForSelected()

		case key.Matches(msg, m.keys.Bottom):
			if len(m.filtered) > 0 {
				m.cursor = len(m.filtered) - 1
				m.ensureVisible()
			}
			return m, m.lineCountForSelected()

		case key.Matches(msg, m.keys.Left):
			return m.navigateUp()

		case key.Matches(msg, m.keys.Right):
			return m.navigateIn()

		case key.Matches(msg, m.keys.QuickLook):
			return m.quickLook()

		case key.Matches(msg, m.keys.PageUp):
			m = m.moveCursor(-m.listHeight())
			return m, m.lineCountForSelected()

		case key.Matches(msg, m.keys.PageDown):
			m = m.moveCursor(m.listHeight())
			return m, m.lineCountForSelected()

		case key.Matches(msg, m.keys.Refresh):
			if m.loading {
				return m, nil // a scan of this directory is already running
			}
			// Smart refresh: check modtime before full rescan. Explicit refreshes
			// never reuse cached subtrees, so they always reflect the disk.
			m.err = nil
			m.pendingCursorEntry = m.selectedName()
			path := m.path
			cached, ok := m.cache.Get(path)
			return m, m.startScan(path, func(prog *ScanProgress) tea.Cmd {
				if ok {
					return smartRefreshCmd(path, cached, prog)
				}
				return scanDirectory(path, prog, nil)
			})

		case key.Matches(msg, m.keys.TopView):
			m.topMode = !m.topMode
			m.refilter()
			return m, nil

		case key.Matches(msg, m.keys.Open):
			openPath(m.path)
			return m, nil

		case key.Matches(msg, m.keys.Search):
			m.searchMode = true
			m.searchInput.Focus()
			return m, textinput.Blink

		case key.Matches(msg, m.keys.GoTo):
			m.gotoMode = true
			m.gotoInput.SetValue("")
			m.gotoInput.Focus()
			return m, textinput.Blink

		case key.Matches(msg, m.keys.Hidden):
			m.showHidden = !m.showHidden
			m.refilter()
			return m, nil

		case key.Matches(msg, m.keys.DirOnly):
			// Cycle: all → dirs only → files only → all
			m.viewFilter = (m.viewFilter + 1) % 3
			m.refilter()
			return m, nil

		case key.Matches(msg, m.keys.Delete):
			entry, ok := m.selected()
			if !ok {
				return m, nil
			}
			fullPath := filepath.Join(m.path, entry.Name)
			if m.trashing[fullPath] {
				return m, nil // already on its way to the Trash
			}
			m.trashing[fullPath] = true
			m.setStatus("Moving "+entry.Name+" to Trash…", false)
			return m, trashCmd(m.path, entry)

		case key.Matches(msg, m.keys.CountAll):
			if m.loading {
				return m, nil
			}
			// Batch count lines for all visible entries
			return m, countAllLinesCmd(m.filtered, m.path)

		case key.Matches(msg, m.keys.HexView):
			return m.hexView()

		case key.Matches(msg, m.keys.Help):
			m.helpMode = true
			return m, nil

		case key.Matches(msg, m.keys.Escape):
			// Clear whichever narrowing is active; a leftover search filter is
			// otherwise unreachable once search mode has been closed with Enter.
			switch {
			case m.searchInput.Value() != "":
				m.searchInput.SetValue("")
			case m.topMode:
				m.topMode = false
			default:
				return m, nil
			}
			m.refilter()
			return m, m.lineCountForSelected()
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Help overlay
	if m.helpMode {
		return renderHelp(m)
	}

	m.viewBuf.Reset()

	// Header (1 or 2 lines depending on path length)
	m.viewBuf.WriteString(renderHeader(m))
	m.viewBuf.WriteString("\n")

	// Separator (cached in Update on WindowSizeMsg)
	m.viewBuf.WriteString(m.cachedSep)
	m.viewBuf.WriteString("\n")

	listHeight := m.listHeight()

	if m.loading {
		block := renderScanning(m)
		lines := strings.Count(block, "\n") + 1
		padTop := (listHeight - lines) / 2
		if padTop < 0 {
			padTop = 0
		}
		for i := 0; i < padTop; i++ {
			m.viewBuf.WriteString("\n")
		}
		m.viewBuf.WriteString(lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(block))
		m.viewBuf.WriteString("\n")
		for i := padTop + lines; i < listHeight; i++ {
			m.viewBuf.WriteString("\n")
		}
	} else if m.err != nil {
		padTop := listHeight / 2
		for i := 0; i < padTop; i++ {
			m.viewBuf.WriteString("\n")
		}
		m.viewBuf.WriteString(errorStyle.Render("Error: " + m.err.Error()))
		m.viewBuf.WriteString("\n")
		for i := padTop + 1; i < listHeight; i++ {
			m.viewBuf.WriteString("\n")
		}
	} else if len(m.filtered) == 0 {
		padTop := listHeight / 2
		for i := 0; i < padTop; i++ {
			m.viewBuf.WriteString("\n")
		}
		msg := "  Empty directory"
		if m.searchInput.Value() != "" {
			msg = "(no matches)"
		}
		m.viewBuf.WriteString(lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Foreground(colorDim).Render(msg))
		m.viewBuf.WriteString("\n")
		for i := padTop + 1; i < listHeight; i++ {
			m.viewBuf.WriteString("\n")
		}
	} else {
		// Render visible rows
		visibleEnd := minInt(m.offset+listHeight, len(m.filtered))
		rendered := 0
		for i := m.offset; i < visibleEnd; i++ {
			selected := i == m.cursor
			m.viewBuf.WriteString(renderRow(m, i, m.filtered[i], selected))
			m.viewBuf.WriteString("\n")
			rendered++
		}
		// Fill remaining space
		for i := rendered; i < listHeight; i++ {
			m.viewBuf.WriteString("\n")
		}
	}

	// Footer separator
	m.viewBuf.WriteString(m.cachedSep)
	m.viewBuf.WriteString("\n")

	// Footer
	m.viewBuf.WriteString(renderFooter(m))

	return m.viewBuf.String()
}

// --- helpers ---

// startScan shows the scanning view for path and returns the commands that
// drive it; scan builds the command from a fresh progress block. A scan
// already running for path is joined instead of repeated.
func (m *Model) startScan(path string, scan func(*ScanProgress) tea.Cmd) tea.Cmd {
	m.leaveScanView() // zero the counters from any previous scan
	m.loading = true
	if prog, ok := m.scans[path]; ok {
		m.scanProg = prog
		return m.spinner.Tick
	}
	m.scanProg = newScanProgress()
	m.scans[path] = m.scanProg
	return tea.Batch(scan(m.scanProg), m.spinner.Tick)
}

// leaveScanView hides the scanning view and clears its progress display.
func (m *Model) leaveScanView() {
	m.loading = false
	m.scanProg = nil
	m.scanProgFiles = 0
	m.scanProgDirs = 0
	m.scanProgSize = 0
	m.scanElapsed = 0
}

func (m *Model) setStatus(text string, isErr bool) {
	m.status = text
	m.statusErr = isErr
}

// selected returns the entry under the cursor. Nothing is selectable while a
// scan is showing, because the rows behind it are not on screen.
func (m Model) selected() (FileEntry, bool) {
	if m.loading || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return FileEntry{}, false
	}
	return m.filtered[m.cursor], true
}

func (m Model) selectedName() string {
	e, _ := m.selected()
	return e.Name
}

// selectName moves the cursor to the named entry, reporting whether it is listed.
func (m *Model) selectName(name string) bool {
	if name == "" {
		return false
	}
	for i, e := range m.filtered {
		if e.Name == name {
			m.cursor = i
			m.ensureVisible()
			return true
		}
	}
	return false
}

// clampCursor pulls the cursor back into the list after it shrank.
func (m *Model) clampCursor() {
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.ensureVisible()
}

// refilter re-applies the view filters, keeping the selected entry if it is
// still visible and going to the top otherwise.
func (m *Model) refilter() {
	name := m.selectedName()
	m.applyFilter()
	m.cursor = 0
	m.offset = 0
	m.selectName(name)
}

// setListing shows res as the current directory's contents. The caller
// positions the cursor.
func (m *Model) setListing(res scanResultMsg) {
	m.entries = res.entries
	m.totalSize = res.totalSize
	m.totalFiles = res.totalFiles
	m.totalDirs = res.totalDirs
	m.computeDeepTotals()
	m.applyFilter()
}

func (m Model) moveCursor(delta int) Model {
	if len(m.filtered) == 0 {
		return m
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	m.ensureVisible()
	return m
}

// listHeight returns the number of rows available for the entry list.
// Reserved: header (1-2) + sep (1) + footer sep (1) + footer (1) + padding (1).
func (m Model) listHeight() int {
	h := m.height - 4 - headerLineCount(m)
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) ensureVisible() {
	listHeight := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+listHeight {
		m.offset = m.cursor - listHeight + 1
	}
	if maxOffset := len(m.filtered) - listHeight; m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *Model) applyFilter() {
	search := m.searchInput.Value()
	// Reuse underlying array to reduce GC pressure
	m.filtered = filterEntriesInto(m.filtered[:0], m.entries, m.showHidden, m.viewFilter, search)
	if m.topMode && len(m.filtered) > 10 {
		m.filtered = m.filtered[:10]
	}
}

// maxCursorHistory caps the cursorHistory map to prevent unbounded growth.
const maxCursorHistory = 500

// rememberCursor saves the current cursor position for a directory path,
// evicting a random entry if the map exceeds maxCursorHistory.
func (m *Model) rememberCursor(path, name string) {
	if len(m.cursorHistory) >= maxCursorHistory {
		// Evict one arbitrary entry (Go map iteration is random)
		for k := range m.cursorHistory {
			delete(m.cursorHistory, k)
			break
		}
	}
	m.cursorHistory[path] = name
}

func (m Model) navigateUp() (Model, tea.Cmd) {
	parent := filepath.Dir(m.path)
	if parent == m.path {
		return m, nil // already at root
	}
	// Highlight the directory we navigated up from
	return m.enterDir(parent, filepath.Base(m.path))
}

func (m Model) navigateIn() (Model, tea.Cmd) {
	entry, ok := m.selected()
	if !ok {
		return m, nil
	}

	// For files, open with default application
	if !entry.IsDir {
		// Block opening very large binary files to avoid freezing the system
		const maxOpenSize = 100 * 1024 * 1024 // 100 MB
		if entry.IsBinary && entry.Size > maxOpenSize {
			m.setStatus(fmt.Sprintf("file too large to open (%s) — use 'x' for hex view", formatSize(entry.Size)), true)
			return m, nil
		}
		filePath := filepath.Join(m.path, entry.Name)
		openPath(filePath)
		return m, nil
	}

	target := filepath.Join(m.path, entry.Name)
	return m.enterDir(target, m.cursorHistory[target])
}

func (m Model) navigateTo(target string) (Model, tea.Cmd) {
	target = strings.TrimSpace(target)

	// Expand ~ to home directory (~user forms are left alone)
	if target == "~" || strings.HasPrefix(target, "~/") || strings.HasPrefix(target, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err == nil {
			target = filepath.Join(home, target[1:])
		}
	}

	// Resolve relative paths
	if !filepath.IsAbs(target) {
		target = filepath.Join(m.path, target)
	}

	// Clean the path
	target = filepath.Clean(target)

	// Verify it exists and is a directory. Failures go to the footer: the
	// current listing is still valid.
	info, err := os.Stat(target)
	if err != nil {
		m.setStatus(fmt.Sprintf("cannot navigate: %v", err), true)
		return m, nil
	}
	if !info.IsDir() {
		m.setStatus("not a directory: "+target, true)
		return m, nil
	}

	return m.enterDir(target, m.cursorHistory[target])
}

// enterDir shows dir — straight from the cache when possible, otherwise by
// scanning it — and highlights selectName once it is listed.
func (m Model) enterDir(dir, selectName string) (Model, tea.Cmd) {
	// Save current cursor position so coming back restores it
	if name := m.selectedName(); name != "" {
		m.rememberCursor(m.path, name)
	}

	m.path = dir
	m.err = nil
	m.searchInput.SetValue("")
	m.searchMode = false
	m.pendingCursorEntry = ""
	m.cursor = 0
	m.offset = 0

	if cached, ok := m.cache.Get(dir); ok {
		m.leaveScanView()
		m.fromCache = true
		m.setListing(cached)
		m.selectName(selectName)
		return m, m.lineCountForSelected()
	}

	// Drop the old directory's rows so no key acts on them while scanning.
	m.entries = nil
	m.applyFilter()
	m.fromCache = false
	// For async scan, remember to restore cursor when results arrive
	m.pendingCursorEntry = selectName
	known := m.knownSubtrees(dir)
	return m, m.startScan(dir, func(prog *ScanProgress) tea.Cmd {
		return scanDirectory(dir, prog, known)
	})
}

// knownSubtrees collects cached totals for dir's immediate subdirectories, so
// scanning dir does not walk them again. The usual case is going up into a
// parent that is not cached yet: the child just left is already sized.
func (m *Model) knownSubtrees(dir string) map[string]subtreeTotals {
	var known map[string]subtreeTotals
	m.cache.Each(func(path string, res scanResultMsg) {
		if path == dir || filepath.Dir(path) != dir {
			return
		}
		if known == nil {
			known = make(map[string]subtreeTotals)
		}
		known[filepath.Base(path)] = subtreeOf(res)
	})
	return known
}

// listing returns the newest listing of dir: the one on screen if dir is the
// current directory, otherwise the cached one.
func (m *Model) listing(dir string) (scanResultMsg, bool) {
	if dir == m.path && !m.loading && m.err == nil {
		res, _ := m.cache.Get(dir) // keeps dirModTime
		res.path = dir
		res.entries = m.entries
		res.totalSize = m.totalSize
		res.totalFiles = m.totalFiles
		res.totalDirs = m.totalDirs
		return res, true
	}
	return m.cache.Get(dir)
}

// storeListing caches res and, when it is the directory on screen, shows it
// with the cursor on the same entry — or the same row if that entry is gone.
func (m *Model) storeListing(res scanResultMsg) {
	m.cache.Put(res.path, res)
	if res.path != m.path || m.loading || m.err != nil {
		return
	}
	name := m.selectedName()
	m.setListing(res)
	if !m.selectName(name) {
		m.clampCursor()
	}
}

// forgetEntry updates every known listing after entry was trashed from dir:
// dir loses the row, and each ancestor's row on the way down to dir shrinks by
// what was removed. Nothing is rescanned, so returning to dir or any parent is
// instant and shows the new sizes. dirModTime is dir's modtime after the
// removal, which keeps smart refresh from treating our own delete as a change.
func (m *Model) forgetEntry(dir string, entry FileEntry, dirModTime time.Time) {
	if res, ok := m.listing(dir); ok {
		if updated, removed, ok := withoutEntry(res, entry.Name); ok {
			entry = removed
			if !dirModTime.IsZero() {
				updated.dirModTime = dirModTime
			}
			m.storeListing(updated)
		}
	}

	// The trashed path no longer exists; a listing cached under it would
	// otherwise be shown for anything recreated there later.
	if entry.IsDir {
		m.cache.DeleteTree(filepath.Join(dir, entry.Name))
	}

	gone := subtreeTotals{size: entry.Size, files: 1}
	if entry.IsDir && !entry.IsSymlink {
		gone = subtreeTotals{size: entry.Size, files: entry.ChildFiles, dirs: entry.ChildDirs + 1}
	}
	for child, parent := dir, filepath.Dir(dir); parent != child; child, parent = parent, filepath.Dir(parent) {
		// Parents never count what lies behind a symlink, so stop there.
		if fi, err := os.Lstat(child); err != nil || fi.Mode()&os.ModeSymlink != 0 {
			return
		}
		if res, ok := m.listing(parent); ok {
			if updated, ok := shrinkEntry(res, filepath.Base(child), gone); ok {
				m.storeListing(updated)
			}
		}
	}
}

// withoutEntry returns res minus the named entry, plus the entry removed. The
// entries slice is copied: the old one is shared with the cache and the model.
func withoutEntry(res scanResultMsg, name string) (scanResultMsg, FileEntry, bool) {
	i := slices.IndexFunc(res.entries, func(e FileEntry) bool { return e.Name == name })
	if i < 0 {
		return res, FileEntry{}, false
	}
	removed := res.entries[i]
	res.entries = slices.Delete(slices.Clone(res.entries), i, i+1)
	res.totalSize = max(0, res.totalSize-removed.Size)
	if removed.IsDir {
		res.totalDirs--
	} else {
		res.totalFiles--
	}
	setPercentages(res.entries, res.totalSize)
	return res, removed, true
}

// shrinkEntry returns res with gone subtracted from the named subdirectory,
// re-sorted since that directory's size changed.
func shrinkEntry(res scanResultMsg, name string, gone subtreeTotals) (scanResultMsg, bool) {
	i := slices.IndexFunc(res.entries, func(e FileEntry) bool {
		return e.Name == name && e.IsDir && !e.IsSymlink
	})
	if i < 0 {
		return res, false
	}
	res.entries = slices.Clone(res.entries)
	e := &res.entries[i]
	e.Size = max(0, e.Size-gone.size)
	e.ChildFiles = max(0, e.ChildFiles-gone.files)
	e.ChildDirs = max(0, e.ChildDirs-gone.dirs)
	res.totalSize = max(0, res.totalSize-gone.size)
	setPercentages(res.entries, res.totalSize)
	SortBySize(res.entries)
	return res, true
}

func (m *Model) computeDeepTotals() {
	m.deepTotalFiles = int64(m.totalFiles)
	m.deepTotalDirs = int64(m.totalDirs)
	for _, e := range m.entries {
		if e.IsDir {
			m.deepTotalFiles += int64(e.ChildFiles)
			m.deepTotalDirs += int64(e.ChildDirs)
		}
	}
}

func (m Model) quickLook() (Model, tea.Cmd) {
	entry, ok := m.selected()
	if !ok {
		return m, nil
	}
	targetPath := filepath.Join(m.path, entry.Name)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("qlmanage", "-p", targetPath)
	case "linux":
		// Try common previewers in order of preference
		for _, opener := range []string{"xdg-open", "gnome-open"} {
			if _, err := exec.LookPath(opener); err == nil {
				cmd = exec.Command(opener, targetPath)
				break
			}
		}
	case "windows":
		// explorer.exe launches the default handler without going through cmd.exe,
		// which would interpret &, |, ^ and %VAR% in the path.
		cmd = exec.Command("explorer", targetPath)
	}

	if cmd == nil {
		return m, nil
	}

	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		m.setStatus(fmt.Sprintf("Quick Look failed: %v", err), true)
	} else {
		go cmd.Wait() // reap child process to avoid zombies
	}
	return m, nil
}

// maxHexViewSize is the maximum file size (50 MB) for hex view to prevent
// loading extremely large files into the pager.
const maxHexViewSize = 50 * 1024 * 1024

func (m Model) hexView() (Model, tea.Cmd) {
	entry, ok := m.selected()
	if !ok || entry.IsDir {
		return m, nil
	}
	if entry.Size > maxHexViewSize {
		m.setStatus(fmt.Sprintf("file too large for hex view (%s, max %s)", formatSize(entry.Size), formatSize(maxHexViewSize)), true)
		return m, nil
	}

	targetPath := filepath.Join(m.path, entry.Name)

	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		// Windows: use PowerShell's Format-Hex. Single quotes are escaped by doubling,
		// and -LiteralPath stops PowerShell from glob-expanding [] in the path.
		c = exec.Command("powershell", "-NoProfile", "-Command",
			fmt.Sprintf("Format-Hex -LiteralPath '%s' | more", strings.ReplaceAll(targetPath, "'", "''")))
	} else {
		// Find hex dump tool
		var hexCmd string
		for _, tool := range []string{"xxd", "hexdump"} {
			if _, err := exec.LookPath(tool); err == nil {
				hexCmd = tool
				break
			}
		}
		if hexCmd == "" {
			m.setStatus("no hex viewer found (install xxd or hexdump)", true)
			return m, nil
		}

		// Find pager
		pager := "less"
		if p := os.Getenv("PAGER"); p != "" {
			pager = p
		}

		// Use shell pipe: xxd file | less. The path must be shell-quoted —
		// %q leaves $ and ` live inside sh double quotes.
		shellCmd := fmt.Sprintf("%s %s | %s", hexCmd, shellQuote(targetPath), pager)
		c = exec.Command("sh", "-c", shellCmd)
	}

	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return statusMsg{text: fmt.Sprintf("hex view failed: %v", err), isErr: true}
		}
		return nil
	})
}

func (m Model) lineCountForSelected() tea.Cmd {
	e, ok := m.selected()
	if !ok || e.IsDir || e.IsBinary || e.LineCount > 0 {
		return nil
	}
	return countLinesCmd(m.path, e.Name)
}

// openPath opens a file or directory with the OS default handler.
func openPath(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		// explorer.exe launches the default handler without going through cmd.exe,
		// which would interpret &, |, ^ and %VAR% in the path.
		cmd = exec.Command("explorer", path)
	default: // linux, freebsd, etc.
		// Try openers in order; some minimal Linux installs lack xdg-open
		for _, opener := range []string{"xdg-open", "sensible-open", "gnome-open"} {
			if _, err := exec.LookPath(opener); err == nil {
				cmd = exec.Command(opener, path)
				break
			}
		}
		if cmd == nil {
			return // No opener available
		}
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err == nil {
		go cmd.Wait() // reap child process to avoid zombies
	}
}

// statusMsg carries the outcome of a background action to the footer.
type statusMsg struct {
	text  string
	isErr bool
}

// trashResultMsg is returned after a trash operation. It names the directory
// the entry was in: by the time it arrives the user may be somewhere else.
type trashResultMsg struct {
	err        error
	dir        string    // directory the entry was in
	entry      FileEntry // the entry as listed when it was trashed
	dirModTime time.Time // dir's modtime after the removal; zero if unknown
}

// trashCmd moves entry (in dir) to the system trash asynchronously.
func trashCmd(dir string, entry FileEntry) tea.Cmd {
	return func() tea.Msg {
		path := filepath.Join(dir, entry.Name)
		var err error
		switch runtime.GOOS {
		case "darwin":
			// Use AppleScript so Finder properly manages the Trash.
			// Escape backslashes and double quotes to prevent injection.
			escaped := strings.ReplaceAll(path, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `"`, `\"`)
			script := fmt.Sprintf(`tell application "Finder" to delete POSIX file "%s"`, escaped)
			out, e := exec.Command("osascript", "-e", script).CombinedOutput()
			if e != nil {
				err = fmt.Errorf("trash failed: %s", strings.TrimSpace(string(out)))
			}
		case "linux":
			err = trashLinux(path)
		case "windows":
			err = trashWindows(path)
		default:
			err = fmt.Errorf("trash not supported on %s", runtime.GOOS)
		}
		msg := trashResultMsg{err: err, dir: dir, entry: entry}
		if err == nil {
			if info, statErr := os.Stat(dir); statErr == nil {
				msg.dirModTime = info.ModTime()
			}
		}
		return msg
	}
}

// trashLinux moves a file to trash following the XDG Trash specification.
// Tries desktop-native commands first, then falls back to manual XDG implementation.
func trashLinux(path string) error {
	// Try gio trash (GNOME/GTK desktops)
	if gioPath, err := exec.LookPath("gio"); err == nil {
		if out, err := exec.Command(gioPath, "trash", path).CombinedOutput(); err == nil {
			return nil
		} else {
			_ = out // gio failed, try next
		}
	}

	// Try trash-put (trash-cli package)
	if trashPut, err := exec.LookPath("trash-put"); err == nil {
		if out, err := exec.Command(trashPut, path).CombinedOutput(); err == nil {
			return nil
		} else {
			_ = out // trash-put failed, try next
		}
	}

	// Manual XDG Trash spec fallback: ~/.local/share/Trash/{files,info}
	trashDir := os.Getenv("XDG_DATA_HOME")
	if trashDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("trash: cannot find home dir: %w", err)
		}
		trashDir = filepath.Join(home, ".local", "share")
	}
	trashDir = filepath.Join(trashDir, "Trash")

	filesDir := filepath.Join(trashDir, "files")
	infoDir := filepath.Join(trashDir, "info")
	if err := os.MkdirAll(filesDir, 0o700); err != nil {
		return fmt.Errorf("trash: cannot create trash dir: %w", err)
	}
	if err := os.MkdirAll(infoDir, 0o700); err != nil {
		return fmt.Errorf("trash: cannot create trash info dir: %w", err)
	}

	baseName := filepath.Base(path)
	dest := filepath.Join(filesDir, baseName)
	infoFile := filepath.Join(infoDir, baseName+".trashinfo")

	// Handle name collisions with incrementing suffix
	for i := 1; ; i++ {
		if _, err := os.Stat(dest); os.IsNotExist(err) {
			break
		}
		dest = filepath.Join(filesDir, fmt.Sprintf("%s.%d", baseName, i))
		infoFile = filepath.Join(infoDir, fmt.Sprintf("%s.%d.trashinfo", baseName, i))
		if i > 1000 {
			return fmt.Errorf("trash: too many collisions for %s", baseName)
		}
	}

	// Write .trashinfo file (XDG spec)
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	trashInfo := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n",
		absPath, time.Now().Format("2006-01-02T15:04:05"))
	if err := os.WriteFile(infoFile, []byte(trashInfo), 0o600); err != nil {
		return fmt.Errorf("trash: cannot write info file: %w", err)
	}

	// Move file to trash; os.Rename fails across filesystems, so fall back to copy+delete
	if err := os.Rename(path, dest); err != nil {
		// Cross-device fallback: copy then remove original
		if cpErr := copyPath(path, dest); cpErr != nil {
			os.Remove(infoFile) // clean up info file
			return fmt.Errorf("trash: move failed: %w", cpErr)
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("trash: copied but failed to remove original: %w", err)
		}
	}
	return nil
}

// trashWindows moves a file to the recycle bin using PowerShell.
func trashWindows(path string) error {
	// Use the .NET Shell API via PowerShell to move to recycle bin
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("trash: cannot resolve path: %w", err)
	}
	script := fmt.Sprintf(
		`Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile('%s', 'OnlyErrorDialogs', 'SendToRecycleBin')`,
		strings.ReplaceAll(absPath, "'", "''"),
	)
	out, e := exec.Command("powershell", "-NoProfile", "-Command", script).CombinedOutput()
	if e != nil {
		return fmt.Errorf("trash failed: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// copyPath copies a file or directory tree from src to dst.
func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst, info.Mode())
	}
	return copyFile(src, dst, info.Mode())
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	buf := make([]byte, 256*1024)
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}
	return nil
}

func copyDir(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(dst, mode); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		info, err := e.Info()
		if err != nil {
			return err
		}
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath, info.Mode()); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath, info.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}
