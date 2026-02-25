package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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

	// Scan progress: shared with scanner goroutine
	scanProg      *ScanProgress
	scanProgFiles int64 // snapshot for display
	scanProgDirs  int64
	scanProgSize  int64

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

	// Error
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
	_ = cache.LoadFromDisk() // best-effort load

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
		scanProg:      &ScanProgress{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(scanDirectory(m.path, m.scanProg), m.spinner.Tick)
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
		m.loading = false
		m.fromCache = false
		m.scanProg = nil
		m.scanProgFiles = 0
		m.scanProgDirs = 0
		m.scanProgSize = 0
		m.path = msg.path
		m.entries = msg.entries
		m.totalSize = msg.totalSize
		m.totalFiles = msg.totalFiles
		m.totalDirs = msg.totalDirs

		// Compute deep totals (immediate + recursive children)
		m.deepTotalFiles = int64(msg.totalFiles)
		m.deepTotalDirs = int64(msg.totalDirs)
		for _, e := range msg.entries {
			if e.IsDir {
				m.deepTotalFiles += int64(e.ChildFiles)
				m.deepTotalDirs += int64(e.ChildDirs)
			}
		}

		m.cursor = 0
		m.offset = 0
		m.applyFilter()

		// Restore cursor to remembered entry (e.g., when navigating up to parent)
		if m.pendingCursorEntry != "" {
			for i, e := range m.filtered {
				if e.Name == m.pendingCursorEntry {
					m.cursor = i
					m.ensureVisible()
					break
				}
			}
			m.pendingCursorEntry = ""
		}

		// Trigger line count for the selected entry
		if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
			e := m.filtered[m.cursor]
			if !e.IsDir && !e.IsBinary {
				cmds = append(cmds, countLinesCmd(m.path, e.Name))
			}
		}

		return m, tea.Batch(cmds...)

	case scanUpToDateMsg:
		// Smart refresh: nothing changed
		m.loading = false
		return m, nil

	case scanErrorMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case trashResultMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		// Remove deleted entry locally (avoid expensive full rescan)
		for i, e := range m.entries {
			if e.Name == msg.name {
				m.entries = append(m.entries[:i], m.entries[i+1:]...)
				break
			}
		}
		// Update totals
		m.totalSize -= msg.size
		if m.totalSize < 0 {
			m.totalSize = 0
		}
		if msg.isDir {
			m.totalDirs--
		} else {
			m.totalFiles--
		}
		// Recompute percentages
		if m.totalSize > 0 {
			for i := range m.entries {
				m.entries[i].Percentage = float64(m.entries[i].Size) / float64(m.totalSize) * 100
			}
		} else {
			for i := range m.entries {
				m.entries[i].Percentage = 0
			}
		}
		m.computeDeepTotals()
		m.applyFilter()
		// Adjust cursor to stay in bounds
		if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureVisible()
		// Invalidate stale cache for this directory
		m.cache.Delete(m.path)
		return m, m.lineCountForSelected()

	case lineCountMsg:
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
		// Batch line count completed (L key)
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
			}
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
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
				m.applyFilter()
				m.cursor = 0
				m.offset = 0
				return m, nil
			case msg.Type == tea.KeyEnter:
				m.searchMode = false
				m.searchInput.Blur()
				return m, nil
			case key.Matches(msg, m.keys.Up):
				m = m.moveCursor(-1)
				return m, m.lineCountForSelected()
			case key.Matches(msg, m.keys.Down):
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
			// Save cache to disk on quit
			_ = m.cache.SaveToDisk()
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
			pageSize := m.height - 5
			if pageSize < 1 {
				pageSize = 1
			}
			m = m.moveCursor(-pageSize)
			return m, m.lineCountForSelected()

		case key.Matches(msg, m.keys.PageDown):
			pageSize := m.height - 5
			if pageSize < 1 {
				pageSize = 1
			}
			m = m.moveCursor(pageSize)
			return m, m.lineCountForSelected()

		case key.Matches(msg, m.keys.Refresh):
			// Smart refresh: check modtime before full rescan
			m.err = nil
			m.scanProg = &ScanProgress{}
			if cached, ok := m.cache.Get(m.path); ok {
				m.loading = true
				return m, tea.Batch(smartRefreshCmd(m.path, cached, m.scanProg), m.spinner.Tick)
			}
			m.loading = true
			return m, tea.Batch(scanDirectory(m.path, m.scanProg), m.spinner.Tick)

		case key.Matches(msg, m.keys.TopView):
			m.topMode = !m.topMode
			m.applyFilter()
			m.cursor = 0
			m.offset = 0
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
			m.applyFilter()
			m.cursor = 0
			m.offset = 0
			return m, nil

		case key.Matches(msg, m.keys.DirOnly):
			// Cycle: all → dirs only → files only → all
			m.viewFilter = (m.viewFilter + 1) % 3
			m.applyFilter()
			m.cursor = 0
			m.offset = 0
			return m, nil

		case key.Matches(msg, m.keys.Delete):
			if len(m.filtered) > 0 {
				entry := m.filtered[m.cursor]
				fullPath := filepath.Join(m.path, entry.Name)
				return m, trashCmd(fullPath, entry.Name, entry.Size, entry.IsDir)
			}
			return m, nil

		case key.Matches(msg, m.keys.CountAll):
			// Batch count lines for all visible entries
			return m, countAllLinesCmd(m.filtered, m.path)

		case key.Matches(msg, m.keys.Help):
			m.helpMode = true
			return m, nil

		case key.Matches(msg, m.keys.Escape):
			if m.topMode {
				m.topMode = false
				m.applyFilter()
			}
			return m, nil
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
	hdrLines := headerLineCount(m)
	m.viewBuf.WriteString(renderHeader(m))
	m.viewBuf.WriteString("\n")

	// Separator (cached in Update on WindowSizeMsg)
	m.viewBuf.WriteString(m.cachedSep)
	m.viewBuf.WriteString("\n")

	// Reserved: header (hdrLines) + sep (1) + footer sep (1) + footer (1) + padding (1) = 4 + hdrLines
	listHeight := m.height - 4 - hdrLines
	if listHeight < 1 {
		listHeight = 1
	}

	if m.loading {
		spinnerView := m.spinner.View() + " Scanning..."
		if m.scanProgFiles > 0 || m.scanProgDirs > 0 {
			spinnerView += fmt.Sprintf("\n\n  %d files · %d dirs · %s scanned",
				m.scanProgFiles, m.scanProgDirs, formatSize(m.scanProgSize))
		}
		lines := strings.Count(spinnerView, "\n") + 1
		padTop := (listHeight - lines) / 2
		if padTop < 0 {
			padTop = 0
		}
		for i := 0; i < padTop; i++ {
			m.viewBuf.WriteString("\n")
		}
		m.viewBuf.WriteString(lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(spinnerView))
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

func (m *Model) ensureVisible() {
	listHeight := m.height - 5
	if listHeight < 1 {
		listHeight = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+listHeight {
		m.offset = m.cursor - listHeight + 1
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

	// Remember which directory we came from so parent highlights it
	childName := filepath.Base(m.path)

	// Save current cursor position for this directory
	if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
		m.rememberCursor(m.path, m.filtered[m.cursor].Name)
	}

	m.path = parent
	m.err = nil
	m.searchInput.SetValue("")
	m.searchMode = false
	if cached, ok := m.cache.Get(parent); ok {
		m.loading = false
		m.fromCache = true
		m.entries = cached.entries
		m.totalSize = cached.totalSize
		m.totalFiles = cached.totalFiles
		m.totalDirs = cached.totalDirs
		m.computeDeepTotals()
		m.cursor = 0
		m.offset = 0
		m.applyFilter()
		// Highlight the directory we navigated up from
		for i, e := range m.filtered {
			if e.Name == childName {
				m.cursor = i
				m.ensureVisible()
				break
			}
		}
		return m, m.lineCountForSelected()
	}
	// For async scan, remember to restore cursor when results arrive
	m.pendingCursorEntry = childName
	m.loading = true
	m.scanProg = &ScanProgress{}
	return m, tea.Batch(scanDirectory(parent, m.scanProg), m.spinner.Tick)
}

func (m Model) navigateIn() (Model, tea.Cmd) {
	if len(m.filtered) == 0 {
		return m, nil
	}
	entry := m.filtered[m.cursor]

	// For files, open with default application
	if !entry.IsDir {
		filePath := filepath.Join(m.path, entry.Name)
		openPath(filePath)
		return m, nil
	}

	// Save current cursor position for this directory
	m.rememberCursor(m.path, entry.Name)

	target := filepath.Join(m.path, entry.Name)
	m.path = target
	m.err = nil
	m.searchInput.SetValue("")
	m.searchMode = false

	// Check if we have a remembered cursor position for this directory
	pendingEntry := m.cursorHistory[target]

	if cached, ok := m.cache.Get(target); ok {
		m.loading = false
		m.fromCache = true
		m.entries = cached.entries
		m.totalSize = cached.totalSize
		m.totalFiles = cached.totalFiles
		m.totalDirs = cached.totalDirs
		m.computeDeepTotals()
		m.cursor = 0
		m.offset = 0
		m.applyFilter()
		// Restore cursor to previously selected entry
		if pendingEntry != "" {
			for i, e := range m.filtered {
				if e.Name == pendingEntry {
					m.cursor = i
					m.ensureVisible()
					break
				}
			}
		}
		return m, m.lineCountForSelected()
	}
	if pendingEntry != "" {
		m.pendingCursorEntry = pendingEntry
	}
	m.loading = true
	m.scanProg = &ScanProgress{}
	return m, tea.Batch(scanDirectory(target, m.scanProg), m.spinner.Tick)
}

func (m Model) navigateTo(target string) (Model, tea.Cmd) {
	// Expand ~ to home directory
	if strings.HasPrefix(target, "~") {
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

	// Verify it exists and is a directory
	info, err := os.Stat(target)
	if err != nil {
		m.err = fmt.Errorf("cannot navigate: %w", err)
		return m, nil
	}
	if !info.IsDir() {
		m.err = fmt.Errorf("not a directory: %s", target)
		return m, nil
	}

	// Save current cursor position
	if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
		m.rememberCursor(m.path, m.filtered[m.cursor].Name)
	}

	m.path = target
	m.err = nil
	m.searchInput.SetValue("")
	m.searchMode = false

	if cached, ok := m.cache.Get(target); ok {
		m.loading = false
		m.fromCache = true
		m.entries = cached.entries
		m.totalSize = cached.totalSize
		m.totalFiles = cached.totalFiles
		m.totalDirs = cached.totalDirs
		m.computeDeepTotals()
		m.cursor = 0
		m.offset = 0
		m.applyFilter()
		return m, m.lineCountForSelected()
	}
	m.loading = true
	m.scanProg = &ScanProgress{}
	return m, tea.Batch(scanDirectory(target, m.scanProg), m.spinner.Tick)
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
	if len(m.filtered) == 0 {
		return m, nil
	}
	entry := m.filtered[m.cursor]
	targetPath := filepath.Join(m.path, entry.Name)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("qlmanage", "-p", targetPath)
	case "linux":
		// Try common previewers in order of preference
		for _, opener := range []string{"xdg-open", "gnome-open", "less"} {
			if _, err := exec.LookPath(opener); err == nil {
				cmd = exec.Command(opener, targetPath)
				break
			}
		}
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", targetPath)
	}

	if cmd == nil {
		return m, nil
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err == nil {
		defer devNull.Close()
		cmd.Stdout = devNull
		cmd.Stderr = devNull
		if err := cmd.Start(); err != nil {
			m.err = fmt.Errorf("Quick Look failed: %w", err)
		}
	}
	return m, nil
}

func (m Model) lineCountForSelected() tea.Cmd {
	if len(m.filtered) == 0 {
		return nil
	}
	e := m.filtered[m.cursor]
	if e.IsDir || e.IsBinary || e.LineCount > 0 {
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
		cmd = exec.Command("cmd", "/c", "start", "", path)
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

// trashResultMsg is returned after a trash operation.
type trashResultMsg struct {
	err   error
	name  string // entry name that was deleted
	size  int64  // size of deleted entry
	isDir bool   // whether it was a directory
}

// trashCmd moves the given path to the system trash asynchronously.
func trashCmd(path, name string, size int64, isDir bool) tea.Cmd {
	return func() tea.Msg {
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
		default:
			// Generic fallback: move to ~/.Trash
			home, herr := os.UserHomeDir()
			if herr != nil {
				err = fmt.Errorf("trash: cannot find home dir: %w", herr)
				break
			}
			dest := filepath.Join(home, ".Trash", filepath.Base(path))
			// Avoid overwriting existing items in Trash
			if _, statErr := os.Stat(dest); statErr == nil {
				dest = dest + ".1"
			}
			err = os.Rename(path, dest)
		}
		return trashResultMsg{err: err, name: name, size: size, isDir: isDir}
	}
}
