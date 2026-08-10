package main

import (
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
