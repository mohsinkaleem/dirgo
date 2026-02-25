package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// formatSize converts bytes to a human-readable string.
func formatSize(b int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)
	switch {
	case b >= TB:
		return strconv.FormatFloat(float64(b)/float64(TB), 'f', 1, 64) + " TB"
	case b >= GB:
		return strconv.FormatFloat(float64(b)/float64(GB), 'f', 1, 64) + " GB"
	case b >= MB:
		return strconv.FormatFloat(float64(b)/float64(MB), 'f', 1, 64) + " MB"
	case b >= KB:
		return strconv.FormatFloat(float64(b)/float64(KB), 'f', 1, 64) + " KB"
	default:
		return strconv.FormatInt(b, 10) + " B"
	}
}

// shortenPath replaces the home directory prefix with ~.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// truncateStr truncates a string to max length with ellipsis.
func truncateStr(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

// bufPool reuses read buffers for line counting to avoid per-call allocations.
var bufPool = sync.Pool{
	New: func() interface{} { return make([]byte, 64*1024) },
}

// isBinaryContent checks whether a byte slice looks like binary data.
// Only checks the first 512 bytes (sufficient for detection, matches HTTP sniffing).
func isBinaryContent(data []byte) bool {
	check := data
	if len(check) > 512 {
		check = check[:512]
	}
	return bytes.IndexByte(check, 0) >= 0
}

// countLines counts newlines using chunked reads + bytes.Count.
// Returns (lineCount, isBinary, error).
// The first read chunk doubles as the binary check — no seek needed.
func countLines(path string, maxSize int64) (int, bool, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return 0, false, err
	}
	if info.Size() > maxSize {
		return 0, false, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer f.Close()

	buf := bufPool.Get().([]byte)
	defer bufPool.Put(buf)

	// First chunk: binary check + start counting
	n, err := f.Read(buf)
	if n > 0 && isBinaryContent(buf[:n]) {
		return 0, true, nil
	}

	count := 0
	if n > 0 {
		count = bytes.Count(buf[:n], []byte{'\n'})
	}
	totalRead := int64(n)

	for totalRead < maxSize && err == nil {
		n, err = f.Read(buf)
		if n > 0 {
			count += bytes.Count(buf[:n], []byte{'\n'})
			totalRead += int64(n)
		}
	}
	return count, false, nil
}

// isBinaryExt returns true if the file extension suggests a binary file.
func isBinaryExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".ico", ".webp", ".svg",
		".mp3", ".mp4", ".wav", ".avi", ".mov", ".mkv", ".flac", ".ogg",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z", ".rar", ".zst",
		".exe", ".dll", ".so", ".dylib", ".bin", ".o", ".a",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".ttf", ".otf", ".woff", ".woff2", ".eot",
		".pyc", ".pyo", ".class", ".wasm",
		".db", ".sqlite", ".sqlite3":
		return true
	}
	return false
}

// barString generates a proportional bar using block characters.
func barString(percentage float64, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	filled := int(percentage / 100.0 * float64(maxWidth))
	if filled < 0 {
		filled = 0
	}
	if filled > maxWidth {
		filled = maxWidth
	}
	var b strings.Builder
	b.Grow(maxWidth*3 + (maxWidth - filled)) // █ is 3 bytes UTF-8
	for i := 0; i < filled; i++ {
		b.WriteRune('█')
	}
	for i := filled; i < maxWidth; i++ {
		b.WriteByte(' ')
	}
	return b.String()
}

// padRight pads a string to the given width with spaces.
func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}

// padLeft pads a string to the given width with leading spaces.
func padLeft(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(runes)) + s
}

// max returns the larger of two ints.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min returns the smaller of two ints.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
