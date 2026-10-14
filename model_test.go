package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestStaleScanResultDoesNotClobberCurrentDir(t *testing.T) {
	m := makeTestModel(3)
	m.cache = newLRUCache(10)
	m.loading = true
	current := m.path

	stale := scanResultMsg{
		path:       "/some/other/dir",
		entries:    []FileEntry{{Name: "ghost.txt", Size: 1}},
		totalSize:  1,
		totalFiles: 1,
	}
	updated, _ := m.Update(stale)
	got := updated.(Model)

	if got.path != current {
		t.Errorf("path changed to %q, want %q", got.path, current)
	}
	if !got.loading {
		t.Error("loading was cleared by a result for another directory")
	}
	if len(got.entries) != 3 {
		t.Errorf("entries replaced by stale result: got %d, want 3", len(got.entries))
	}
	// The stale result should still be cached for fast back-navigation.
	if _, ok := got.cache.Get("/some/other/dir"); !ok {
		t.Error("stale result was not cached")
	}
}

func TestStaleLineCountIsIgnored(t *testing.T) {
	m := makeTestModel(3)
	before := m.entries[1].LineCount

	updated, _ := m.Update(lineCountMsg{dir: "/elsewhere", name: m.entries[1].Name, lines: 999})
	if got := updated.(Model).entries[1].LineCount; got != before {
		t.Errorf("line count from another directory applied: got %d, want %d", got, before)
	}
}

func TestEnsureVisibleClampsOffset(t *testing.T) {
	m := makeTestModel(50)
	m.cursor = 49
	m.ensureVisible()

	h := m.listHeight()
	if m.offset != len(m.filtered)-h {
		t.Errorf("offset = %d, want %d", m.offset, len(m.filtered)-h)
	}

	// Shrinking the list must pull the offset back into range.
	m.filtered = m.filtered[:5]
	m.cursor = 0
	m.ensureVisible()
	if m.offset != 0 {
		t.Errorf("offset = %d after list shrank, want 0", m.offset)
	}
}

func TestEscapeClearsSearchFilter(t *testing.T) {
	m := makeTestModel(10)
	m.searchInput = textinput.New()
	m.searchInput.SetValue("zzzzz")
	m.applyFilter()
	if len(m.filtered) != 0 {
		t.Fatalf("expected filter to match nothing, got %d entries", len(m.filtered))
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := updated.(Model)
	if got.searchInput.Value() != "" {
		t.Errorf("search value = %q, want empty", got.searchInput.Value())
	}
	if len(got.filtered) != 10 {
		t.Errorf("filtered = %d entries after Esc, want 10", len(got.filtered))
	}
}

// scannedModel returns a model showing dir after a real scan.
func scannedModel(t *testing.T, dir string) Model {
	t.Helper()
	m := NewModel(dir)
	m.width, m.height = 120, 40
	return send(m, scanDirectory(dir, nil, nil)())
}

func send(m Model, msg tea.Msg) Model {
	updated, _ := m.Update(msg)
	return updated.(Model)
}

// openEntry selects name and opens it, completing any scan that starts.
func openEntry(t *testing.T, m Model, name string) Model {
	t.Helper()
	if !m.selectName(name) {
		t.Fatalf("%s is not listed in %s", name, m.path)
	}
	m, _ = m.navigateIn()
	if m.loading {
		m = send(m, scanDirectory(m.path, nil, nil)())
	}
	return m
}

func entryNamed(entries []FileEntry, name string) (FileEntry, bool) {
	for _, e := range entries {
		if e.Name == name {
			return e, true
		}
	}
	return FileEntry{}, false
}

func TestTrashThenReturnUsesCache(t *testing.T) {
	root := makeTempDir(t, 3, 2, 4)
	m := scannedModel(t, root)
	before := m.totalSize
	victim, _ := entryNamed(m.entries, "file_0000.txt")

	m = send(m, trashResultMsg{dir: root, entry: victim})
	if _, ok := entryNamed(m.entries, victim.Name); ok {
		t.Fatal("trashed entry still listed")
	}
	if m.totalSize != before-victim.Size || m.totalFiles != 2 {
		t.Errorf("totals = %d bytes / %d files, want %d / 2", m.totalSize, m.totalFiles, before-victim.Size)
	}

	m = openEntry(t, m, "subdir_0000")
	m, _ = m.navigateUp()
	if m.loading {
		t.Fatal("returning to the directory after a delete rescanned it")
	}
	if _, ok := entryNamed(m.entries, victim.Name); ok {
		t.Error("trashed entry is back after returning")
	}
	if got := m.selectedName(); got != "subdir_0000" {
		t.Errorf("selected %q after going back, want subdir_0000", got)
	}
}

func TestTrashInChildShrinksCachedParent(t *testing.T) {
	root := makeTempDir(t, 3, 2, 4)
	m := scannedModel(t, root)
	rootSize := m.totalSize
	sub, _ := entryNamed(m.entries, "subdir_0000")

	m = openEntry(t, m, "subdir_0000")
	victim, _ := entryNamed(m.entries, "file_0001.txt")
	m = send(m, trashResultMsg{dir: m.path, entry: victim})

	m, _ = m.navigateUp()
	if m.loading {
		t.Fatal("parent was rescanned")
	}
	got, _ := entryNamed(m.entries, "subdir_0000")
	if got.Size != sub.Size-victim.Size || got.ChildFiles != sub.ChildFiles-1 {
		t.Errorf("parent row = %d bytes / %d files, want %d / %d", got.Size, got.ChildFiles, sub.Size-victim.Size, sub.ChildFiles-1)
	}
	if m.totalSize != rootSize-victim.Size {
		t.Errorf("parent total = %d, want %d", m.totalSize, rootSize-victim.Size)
	}
	if m.deepTotalFiles != 3+2*4-1 {
		t.Errorf("deep file total = %d, want %d", m.deepTotalFiles, 3+2*4-1)
	}
}

// A trash result can land after the user has already moved on; it must update
// the directory the entry was in, not whichever directory is on screen.
func TestTrashResultAfterNavigatingAway(t *testing.T) {
	root := makeTempDir(t, 3, 2, 4)
	m := scannedModel(t, root)
	rootFiles := m.totalFiles
	sub, _ := entryNamed(m.entries, "subdir_0000")

	m = openEntry(t, m, "subdir_0000")
	subPath := m.path
	m.selectName("file_0002.txt")
	victim, _ := m.selected()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cmd == nil {
		t.Fatal("delete key produced no command")
	}
	m, _ = updated.(Model).navigateUp()

	m = send(m, trashResultMsg{dir: subPath, entry: victim})
	if m.loading || m.totalFiles != rootFiles {
		t.Fatalf("parent listing was disturbed: loading=%v files=%d", m.loading, m.totalFiles)
	}
	if got, _ := entryNamed(m.entries, "subdir_0000"); got.Size != sub.Size-victim.Size {
		t.Errorf("subdir row = %d bytes, want %d", got.Size, sub.Size-victim.Size)
	}
	cached, _ := m.cache.Get(subPath)
	if _, ok := entryNamed(cached.entries, victim.Name); ok {
		t.Error("trashed entry still in the subdirectory's cached listing")
	}
	if len(m.trashing) != 0 {
		t.Error("trash still marked in flight")
	}
}

func TestTrashedDirLeavesNoCachedListings(t *testing.T) {
	root := makeTempDir(t, 1, 2, 2)
	m := scannedModel(t, root)
	m = openEntry(t, m, "subdir_0000")
	sub := m.path
	m, _ = m.navigateUp()

	victim, _ := entryNamed(m.entries, "subdir_0000")
	m = send(m, trashResultMsg{dir: root, entry: victim})
	if _, ok := m.cache.Get(sub); ok {
		t.Error("trashed directory's listing is still cached")
	}
	if _, ok := m.cache.Get(root); !ok {
		t.Error("parent listing was dropped too")
	}
}

func TestFailedTrashKeepsListing(t *testing.T) {
	m := makeTestModel(5)
	m.cache = newLRUCache(10)
	victim := m.filtered[1]
	m = send(m, trashResultMsg{dir: m.path, entry: victim, err: errors.New("trash failed: nope")})

	if m.err != nil {
		t.Fatalf("action error replaced the listing: %v", m.err)
	}
	view := m.View()
	if !strings.Contains(view, victim.Name) || !strings.Contains(view, "trash failed: nope") {
		t.Errorf("want listing plus footer error, got:\n%s", view)
	}
	// The message clears on the next key.
	m = send(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.status != "" {
		t.Errorf("status %q survived a key press", m.status)
	}
}

// While a scan is showing, the old directory's rows are not on screen and
// must not be acted on: they would resolve against the new path.
func TestNoRowActionsWhileScanning(t *testing.T) {
	root := makeTempDir(t, 3, 2, 4)
	m := scannedModel(t, root)
	m.selectName("subdir_0000")
	m, _ = m.navigateIn() // uncached: starts a scan
	if !m.loading {
		t.Fatal("expected a scan")
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}); cmd != nil {
		t.Error("delete is live while scanning")
	}
	if next, _ := m.navigateIn(); next.path != m.path {
		t.Errorf("navigated into %s while scanning", next.path)
	}
}

func TestRevisitDuringScanJoinsIt(t *testing.T) {
	root := makeTempDir(t, 3, 2, 4)
	m := scannedModel(t, root)
	m.selectName("subdir_0000")
	m, _ = m.navigateIn()
	first := m.scanProg

	m, _ = m.navigateUp() // cached
	m.selectName("subdir_0000")
	m, _ = m.navigateIn()
	if m.scanProg != first {
		t.Error("re-entering a directory mid-scan started a second scan")
	}
	m = send(m, scanDirectory(m.path, first, nil)())
	if m.loading || len(m.scans) != 0 {
		t.Errorf("scan not finished: loading=%v in flight=%d", m.loading, len(m.scans))
	}
}

func TestRefreshKeepsSelection(t *testing.T) {
	root := makeTempDir(t, 3, 2, 4)
	m := scannedModel(t, root)
	m.selectName("file_0002.txt")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(Model)
	if cmd == nil || !m.loading {
		t.Fatal("refresh did not start")
	}
	// A changed directory gets a full rescan.
	m = send(m, scanDirectory(root, nil, nil)())
	if got := m.selectedName(); got != "file_0002.txt" {
		t.Errorf("selected %q after refresh, want file_0002.txt", got)
	}
}

func TestSearchAcceptsVimKeys(t *testing.T) {
	m := makeTestModel(5)
	m.searchMode = true
	m.searchInput.Focus()
	for _, r := range "jk" {
		m = send(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got := m.searchInput.Value(); got != "jk" {
		t.Errorf("search query = %q, want \"jk\"", got)
	}
}

func TestGoingUpReusesChildTotals(t *testing.T) {
	root := makeTempDir(t, 2, 2, 3)
	child := filepath.Join(root, "subdir_0001")
	m := scannedModel(t, child)

	known := m.knownSubtrees(root)
	want := subtreeTotals{size: m.totalSize, files: 3}
	if known["subdir_0001"] != want {
		t.Fatalf("known subtree = %+v, want %+v", known["subdir_0001"], want)
	}

	// Planting fake totals proves the parent scan used them instead of walking.
	fake := subtreeTotals{size: 12345, files: 7, dirs: 2}
	res := scanDirectory(root, nil, map[string]subtreeTotals{"subdir_0001": fake})().(scanResultMsg)
	got, _ := entryNamed(res.entries, "subdir_0001")
	if got.Size != fake.size || got.ChildFiles != fake.files || got.ChildDirs != fake.dirs {
		t.Errorf("reused entry = %d/%d/%d, want %+v", got.Size, got.ChildFiles, got.ChildDirs, fake)
	}
}

func TestNavigateToBadPathKeepsListing(t *testing.T) {
	m := makeTestModel(5)
	m, _ = m.navigateTo("/definitely/not/here")
	if m.err != nil || m.status == "" || len(m.filtered) != 5 {
		t.Errorf("err=%v status=%q rows=%d; want footer error and listing intact", m.err, m.status, len(m.filtered))
	}
}
