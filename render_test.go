package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
)

func makeTestModel(entryCount int) Model {
	entries := make([]FileEntry, entryCount)
	var totalSize int64
	for i := range entries {
		sz := int64((entryCount - i) * 1024)
		entries[i] = FileEntry{
			Name:       "file_" + padLeft(string(rune('a'+i%26)), 2) + ".go",
			Size:       sz,
			Percentage: float64(sz) / float64(entryCount*1024) * 100,
			LineCount:  (i + 1) * 42,
		}
		totalSize += sz
	}
	// Add a directory entry
	if entryCount > 0 {
		entries[0].IsDir = true
		entries[0].ChildFiles = 50
		entries[0].ChildDirs = 3
		entries[0].LineCount = 0
	}

	return Model{
		width:       120,
		height:      40,
		entries:     entries,
		filtered:    append([]FileEntry(nil), entries...),
		totalSize:   totalSize,
		totalFiles:  entryCount - 1,
		totalDirs:   1,
		path:        "/home/user/project",
		keys:        DefaultKeyMap(),
		viewBuf:     &strings.Builder{},
		searchInput: textinput.New(),
	}
}

func TestRenderScanning(t *testing.T) {
	m := makeTestModel(3)
	m.loading = true
	m.spinner = spinner.New()
	m.path = "/home/user/project"
	m.scanProgFiles = 400850
	m.scanProgDirs = 41140
	m.scanProgSize = 33071895347
	m.scanElapsed = 12400 * time.Millisecond

	got := renderScanning(m)
	for _, want := range []string{"Scanning", "/home/user/project", "400,850", "41,140", "30.8 GB", "12s elapsed", "32.3k files/s"} {
		if !strings.Contains(got, want) {
			t.Errorf("scanning view missing %q:\n%s", want, got)
		}
	}

	// Before any counter moves, only the title and path are shown.
	m.scanProgFiles, m.scanProgDirs, m.scanProgSize, m.scanElapsed = 0, 0, 0, 0
	early := renderScanning(m)
	if strings.Contains(early, "files") || strings.Contains(early, "elapsed") {
		t.Errorf("expected minimal early view, got:\n%s", early)
	}

	// A slow scan with no counters yet still shows a ticking clock.
	m.scanElapsed = 3 * time.Second
	if slow := renderScanning(m); !strings.Contains(slow, "3s elapsed") {
		t.Errorf("expected elapsed time on a slow scan, got:\n%s", slow)
	}
}

func BenchmarkRenderRow(b *testing.B) {
	m := makeTestModel(20)
	entry := m.entries[1] // a file entry
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderRow(m, 1, entry, false)
	}
}

func BenchmarkRenderRowSelected(b *testing.B) {
	m := makeTestModel(20)
	entry := m.entries[1]
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderRow(m, 1, entry, true)
	}
}

func BenchmarkRenderHeader(b *testing.B) {
	m := makeTestModel(20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderHeader(m)
	}
}

func BenchmarkRenderFooter(b *testing.B) {
	m := makeTestModel(20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderFooter(m)
	}
}

func BenchmarkModelView(b *testing.B) {
	m := makeTestModel(100)
	m.filtered = m.entries[:40] // simulate visible entries
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.View()
	}
}

func BenchmarkApplyFilter(b *testing.B) {
	entries := make([]FileEntry, 10000)
	for i := range entries {
		entries[i] = FileEntry{
			Name: "file_" + string(rune('a'+i%26)) + "_test.go",
			Size: int64(i * 100),
		}
	}
	m := Model{
		entries:     entries,
		filtered:    make([]FileEntry, 0, len(entries)),
		searchInput: textinput.New(),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.applyFilter()
	}
}

func BenchmarkApplyFilterWithSearch(b *testing.B) {
	entries := make([]FileEntry, 10000)
	for i := range entries {
		entries[i] = FileEntry{
			Name: "file_" + string(rune('a'+i%26)) + "_test.go",
			Size: int64(i * 100),
		}
	}
	m := Model{
		entries:     entries,
		filtered:    make([]FileEntry, 0, len(entries)),
		searchInput: textinput.New(),
	}
	m.searchInput.SetValue("test")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.applyFilter()
	}
}
