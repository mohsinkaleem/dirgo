package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func makeTempDir(t testing.TB, files, subdirs, subFiles int) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < files; i++ {
		name := filepath.Join(dir, fmt.Sprintf("file_%04d.txt", i))
		os.WriteFile(name, []byte("hello world\nline 2\n"), 0o644)
	}
	for i := 0; i < subdirs; i++ {
		sub := filepath.Join(dir, fmt.Sprintf("subdir_%04d", i))
		os.Mkdir(sub, 0o755)
		for j := 0; j < subFiles; j++ {
			name := filepath.Join(sub, fmt.Sprintf("file_%04d.txt", j))
			os.WriteFile(name, []byte("content\n"), 0o644)
		}
	}
	return dir
}

func makeDeepDir(t testing.TB, depth, filesPerLevel int) string {
	t.Helper()
	root := t.TempDir()
	cur := root
	for d := 0; d < depth; d++ {
		for f := 0; f < filesPerLevel; f++ {
			os.WriteFile(filepath.Join(cur, fmt.Sprintf("f%d.txt", f)), []byte("x\n"), 0o644)
		}
		next := filepath.Join(cur, fmt.Sprintf("level_%d", d))
		os.Mkdir(next, 0o755)
		cur = next
	}
	return root
}

func TestScanDirectory(t *testing.T) {
	dir := makeTempDir(t, 5, 3, 2)
	cmd := scanDirectory(dir, nil)
	msg := cmd()

	result, ok := msg.(scanResultMsg)
	if !ok {
		t.Fatalf("expected scanResultMsg, got %T", msg)
	}
	if result.totalFiles != 5 {
		t.Errorf("expected 5 files, got %d", result.totalFiles)
	}
	if result.totalDirs != 3 {
		t.Errorf("expected 3 dirs, got %d", result.totalDirs)
	}
	if len(result.entries) != 8 {
		t.Errorf("expected 8 entries, got %d", len(result.entries))
	}
	// Verify directory sizes were computed upfront
	for _, e := range result.entries {
		if e.IsDir && e.Size <= 0 {
			t.Errorf("expected dir %s to have computed size, got %d", e.Name, e.Size)
		}
	}
}

func TestDirSizeComputed(t *testing.T) {
	dir := makeTempDir(t, 0, 1, 10)
	cmd := scanDirectory(dir, nil)
	msg := cmd()

	result, ok := msg.(scanResultMsg)
	if !ok {
		t.Fatalf("expected scanResultMsg, got %T", msg)
	}

	// Find the subdirectory entry and verify its size was computed
	found := false
	for _, e := range result.entries {
		if e.IsDir && e.Name == "subdir_0000" {
			found = true
			if e.ChildFiles != 10 {
				t.Errorf("expected 10 child files, got %d", e.ChildFiles)
			}
			if e.Size <= 0 {
				t.Error("expected positive size")
			}
			break
		}
	}
	if !found {
		t.Error("subdir_0000 not found in entries")
	}
}

func TestSmartRefreshUnchanged(t *testing.T) {
	dir := makeTempDir(t, 3, 0, 0)
	scanMsg := scanDirectory(dir, nil)().(scanResultMsg)

	cmd := smartRefreshCmd(dir, scanMsg, nil)
	msg := cmd()

	if _, ok := msg.(scanUpToDateMsg); !ok {
		t.Fatalf("expected scanUpToDateMsg, got %T", msg)
	}
}

func TestSmartRefreshChanged(t *testing.T) {
	dir := makeTempDir(t, 3, 0, 0)
	scanMsg := scanDirectory(dir, nil)().(scanResultMsg)

	os.WriteFile(filepath.Join(dir, "new_file.txt"), []byte("new"), 0o644)

	cmd := smartRefreshCmd(dir, scanMsg, nil)
	msg := cmd()

	if _, ok := msg.(scanResultMsg); !ok {
		t.Fatalf("expected scanResultMsg after change, got %T", msg)
	}
}

func TestCountAllLines(t *testing.T) {
	dir := makeTempDir(t, 5, 0, 0)
	entries := make([]FileEntry, 5)
	for i := 0; i < 5; i++ {
		entries[i] = FileEntry{
			Name: fmt.Sprintf("file_%04d.txt", i),
		}
	}

	cmd := countAllLinesCmd(entries, dir)
	msg := cmd()

	result, ok := msg.(batchLineCountMsg)
	if !ok {
		t.Fatalf("expected batchLineCountMsg, got %T", msg)
	}
	if len(result.Counts) != 5 {
		t.Errorf("expected 5 line counts, got %d", len(result.Counts))
	}
	for name, count := range result.Counts {
		if count != 2 {
			t.Errorf("file %s: expected 2 lines, got %d", name, count)
		}
	}
}

func BenchmarkScanDirectory(b *testing.B) {
	dir := makeTempDir(b, 100, 10, 5)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scanDirectory(dir, nil)()
	}
}

func BenchmarkScanLargeDir(b *testing.B) {
	dir := makeTempDir(b, 1000, 0, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scanDirectory(dir, nil)()
	}
}

func BenchmarkScanDeepDir(b *testing.B) {
	dir := makeDeepDir(b, 5, 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scanDirectory(dir, nil)()
	}
}
