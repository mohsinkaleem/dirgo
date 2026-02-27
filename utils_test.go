package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Functional tests ---

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}
	for _, tt := range tests {
		got := formatSize(tt.input)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestShortenPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		input string
		want  string
	}{
		{home + "/Documents", "~/Documents"},
		{"/usr/local/bin", "/usr/local/bin"},
		{home, "~"},
	}
	for _, tt := range tests {
		got := shortenPath(tt.input)
		if got != tt.want {
			t.Errorf("shortenPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		s   string
		max int
		exp string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hell…"},
		{"hi", 2, "hi"},
		{"", 5, ""},
		{"abcdef", 0, ""},
	}
	for _, tt := range tests {
		got := truncateStr(tt.s, tt.max)
		if got != tt.exp {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.exp)
		}
	}
}

func TestCountLines(t *testing.T) {
	dir := t.TempDir()

	// Text file with 5 lines
	textFile := filepath.Join(dir, "text.txt")
	os.WriteFile(textFile, []byte("a\nb\nc\nd\ne\n"), 0o644)
	lines, isBin, err := countLines(textFile, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if isBin {
		t.Error("text file detected as binary")
	}
	if lines != 5 {
		t.Errorf("expected 5 lines, got %d", lines)
	}

	// Binary file
	binFile := filepath.Join(dir, "binary.bin")
	data := make([]byte, 100)
	data[50] = 0 // null byte
	os.WriteFile(binFile, data, 0o644)
	lines, isBin, _ = countLines(binFile, 1024*1024)
	if !isBin {
		t.Error("binary file not detected")
	}
	if lines != 0 {
		t.Errorf("expected 0 lines for binary, got %d", lines)
	}

	// Empty file
	emptyFile := filepath.Join(dir, "empty.txt")
	os.WriteFile(emptyFile, []byte{}, 0o644)
	lines, isBin, _ = countLines(emptyFile, 1024*1024)
	if isBin {
		t.Error("empty file detected as binary")
	}
	if lines != 0 {
		t.Errorf("expected 0 lines for empty file, got %d", lines)
	}

	// File over maxSize should return 0
	bigFile := filepath.Join(dir, "big.txt")
	os.WriteFile(bigFile, []byte(strings.Repeat("line\n", 100)), 0o644)
	lines, _, _ = countLines(bigFile, 10) // maxSize = 10 bytes
	if lines != 0 {
		t.Errorf("expected 0 for oversized file, got %d", lines)
	}
}

func TestIsBinaryContent(t *testing.T) {
	if isBinaryContent([]byte("hello world")) {
		t.Error("text should not be binary")
	}
	if !isBinaryContent([]byte{0x48, 0x65, 0x00, 0x6c}) {
		t.Error("data with null byte should be binary")
	}
}

func TestIsBinaryExt(t *testing.T) {
	binExts := []string{".png", ".jpg", ".zip", ".exe", ".pdf", ".wasm"}
	for _, ext := range binExts {
		if !isBinaryExt("file" + ext) {
			t.Errorf("expected %s to be binary", ext)
		}
	}
	textExts := []string{".go", ".txt", ".md", ".json", ".yaml"}
	for _, ext := range textExts {
		if isBinaryExt("file" + ext) {
			t.Errorf("expected %s to be text", ext)
		}
	}
}

func TestBarString(t *testing.T) {
	bar := barString(50.0, 10)
	runes := []rune(bar)
	if len(runes) != 10 {
		t.Errorf("expected 10 runes, got %d", len(runes))
	}
	filled := strings.Count(bar, "█")
	if filled != 5 {
		t.Errorf("expected 5 filled, got %d for 50%% bar width 10", filled)
	}
}

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		name, pattern string
		want          bool
	}{
		{"README.md", "read", true},
		{"main.go", "MAIN", true},
		{"scanner.go", "xyz", false},
		{"test", "testing", false},
		// Subsequence matching (new behavior)
		{"model.go", "mgo", true},
		{"README.md", "rdm", true},
		{"scanner_test.go", "stg", true},
		{"abcdef", "ace", true},
		{"abcdef", "afc", false},
		{"", "a", false},
		{"a", "", true},
	}
	for _, tt := range tests {
		got := fuzzyMatch(tt.name, tt.pattern)
		if got != tt.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.name, tt.pattern, got, tt.want)
		}
	}
}

func TestPadLeftRight(t *testing.T) {
	if got := padLeft("hi", 5); got != "   hi" {
		t.Errorf("padLeft: got %q", got)
	}
	if got := padRight("hi", 5); got != "hi   " {
		t.Errorf("padRight: got %q", got)
	}
	// Already wide enough
	if got := padLeft("hello", 3); got != "hello" {
		t.Errorf("padLeft overflow: got %q", got)
	}
}

// --- Benchmarks ---

func BenchmarkCountLines(b *testing.B) {
	dir := b.TempDir()
	// Create a ~1MB text file
	data := strings.Repeat("this is a line of text for benchmarking\n", 25000)
	path := filepath.Join(dir, "bench.txt")
	os.WriteFile(path, []byte(data), 0o644)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		countLines(path, 10*1024*1024)
	}
}

func BenchmarkCountLinesBinary(b *testing.B) {
	dir := b.TempDir()
	data := make([]byte, 1024)
	data[100] = 0 // null byte triggers early exit
	path := filepath.Join(dir, "binary.bin")
	os.WriteFile(path, data, 0o644)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		countLines(path, 10*1024*1024)
	}
}

func BenchmarkFormatSize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		formatSize(1234567890)
	}
}

func BenchmarkBarString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		barString(42.5, 20)
	}
}

func BenchmarkFuzzyMatch(b *testing.B) {
	for i := 0; i < b.N; i++ {
		fuzzyMatch("some_long_filename_for_testing.go", "test")
	}
}
