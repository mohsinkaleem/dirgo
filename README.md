# dirgo

A fast, interactive terminal directory analyzer built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea). Visualize disk usage, explore directories, and identify space hogs — all from your terminal.

## Features

- **Instant directory listing** — files appear immediately; directory sizes compute in the background
- **Proportional size bars** — color-coded percentage bars for quick visual scanning
- **Incremental scanning** — Phase 1 lists entries instantly, Phase 2 computes directory sizes concurrently
- **Smart refresh** — checks directory modtime before rescanning; skips unchanged directories
- **LRU cache** — bounded in-memory cache (100 entries) with disk persistence across sessions
- **Line counting** — automatic line count for the selected text file; batch count all with `L`
- **Fuzzy search** — filter entries in real time
- **Symlink detection** — symlinks shown with `→` / `⇢` indicators
- **CPU profiling** — built-in `--profile` flag for performance analysis

## Install

```bash
go install github.com/m0k099s/dirgo@latest
```

Or build from source:

```bash
git clone https://github.com/m0k099s/dirgo.git
cd dirgo
make build
```

## Usage

```bash
# Analyze current directory
dirgo

# Analyze a specific path
dirgo ~/Documents

# Enable CPU profiling
dirgo --profile /path/to/dir
```

## Keybindings

| Key | Action |
|---|---|
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `←` / `Backspace` | Go to parent directory |
| `→` / `l` / `Enter` | Open selected directory |
| `g` | Jump to top |
| `G` | Jump to bottom |
| `r` | Smart refresh (skips if unchanged) |
| `t` | Toggle top 10 view |
| `o` | Open in Finder (macOS) |
| `/` | Search / filter |
| `h` | Toggle hidden files |
| `d` | Toggle directory-only view |
| `L` | Count lines for all files |
| `?` | Help |
| `q` / `Ctrl+C` | Quit |

## Architecture

```
main.go        Entry point, --profile flag, Bubble Tea program setup
model.go       Application state, Update loop, message handling
scanner.go     Two-phase directory scanning, background size computation
cache.go       LRU cache with bounded eviction + gob disk persistence
entry.go       FileEntry data model, sorting, filtering
render.go      Row rendering, header/footer, help overlay
keys.go        Key bindings
styles.go      Lipgloss color and style definitions
utils.go       Formatting, line counting (bytes.Count + sync.Pool), helpers
```

### Scanning Pipeline

1. **Phase 1** — `scanDirectory()` calls `os.ReadDir`, stats files immediately, marks directories as `SizeComputing`. Returns a `scanResultMsg` within milliseconds.
2. **Phase 2** — For each pending directory, `computeDirSizeCmd()` runs `filepath.WalkDir` with bounded concurrency (semaphore sized to CPU count, max 16). Each completion sends a `dirSizeMsg` that triggers re-sort and percentage recalculation.

### Caching

- **In-memory**: LRU cache holding up to 100 directory scan results. Accessed on navigation; updated on scan completion.
- **On-disk**: Top 50 LRU entries serialized to `~/.cache/dirgo/cache.gob` on quit. Entries older than 24 hours are discarded on load.

### Smart Refresh

Pressing `r` compares the directory's current modtime against the cached value. If unchanged, the rescan is skipped entirely (~microseconds). If changed, a full Phase 1 + Phase 2 scan is triggered.

## Development

```bash
# Run tests
make test

# Run benchmarks
make bench

# CPU profile a benchmark
make profile-cpu

# Memory profile
make profile-mem
```

### Benchmark Results (Apple M4 Pro)

| Benchmark | Time | Allocs |
|---|---|---|
| ScanDirectory (100 files, 10 subdirs) | ~920 µs | 584 |
| ScanLargeDir (1000 files) | ~5.4 ms | 7,156 |
| ScanDeepDir (5 levels) | ~153 µs | 79 |
| CountLines (1 MB text) | ~111 µs | 6 |
| CountLines (binary, early exit) | ~59 µs | 6 |
| FormatSize | ~55 ns | 2 |
| FuzzyMatch | ~20 ns | 0 |

## Requirements

- Go 1.21+
- macOS / Linux (the `o` key uses `open` on macOS)

## License

MIT
