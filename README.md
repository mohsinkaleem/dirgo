# dirgo

A fast, interactive terminal directory analyzer built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea). Visualize disk usage, explore directories, and identify space hogs — all from your terminal.

## Features

- **Instant directory listing** — files appear immediately; directory sizes compute in the background
- **Proportional size bars** — color-coded percentage bars for quick visual scanning
- **Efficient directory scanning** — uses `os.ReadDir` + manual recursion to minimize syscalls; parallel stat with bounded concurrency
- **Smart refresh** — checks directory modtime before rescanning; skips unchanged directories
- **LRU cache** — bounded in-memory cache (100 entries) with disk persistence across sessions (respects `XDG_CACHE_HOME`)
- **Line counting** — automatic line count for the selected text file; batch count all with `s`
- **Fuzzy search** — filter entries in real time with subsequence matching
- **Symlink detection** — symlinks shown with `→` / `⇢` indicators
- **Cross-platform** — works on macOS, Linux, and Windows (Quick Look, file open, and cache paths adapt per OS)
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

# Print version
dirgo --version

# Enable CPU profiling
dirgo --profile /path/to/dir
```

## Keybindings

| Key | Action |
|---|---|
| `↑` / `k` | Move cursor up |
| `↓` / `j` | Move cursor down |
| `←` / `Backspace` | Go to parent directory |
| `→` / `l` / `Enter` | Open selected directory / file |
| `Space` | Quick Look preview (macOS `qlmanage`, Linux `xdg-open`, Windows `start`) |
| `g` | Jump to top |
| `G` | Jump to bottom |
| `PgUp` / `Ctrl+U` | Page up |
| `PgDn` / `Ctrl+D` | Page down |
| `r` | Smart refresh (skips if unchanged) |
| `t` | Toggle top 10 view |
| `o` | Open in Finder / file manager |
| `/` | Search / filter |
| `Esc` | Cancel search / close help |
| `h` | Toggle hidden files |
| `d` | Toggle directory-only view |
| `s` | Count lines for all files |
| `?` | Help |
| `q` / `Ctrl+C` | Quit |

## Architecture

```
main.go        Entry point, --profile/--version flags, Bubble Tea program setup
model.go       Application state, Update loop, message handling
scanner.go     Directory scanning with os.ReadDir + manual recursion, bounded concurrency
cache.go       LRU cache with bounded eviction + gob disk persistence (XDG-aware)
entry.go       FileEntry data model, sorting, filtering, fuzzy match
render.go      Row rendering, header/footer, help overlay
keys.go        Key bindings
styles.go      Lipgloss color and style definitions (pre-defined bar color styles)
utils.go       Formatting, line counting (bytes.Count + sync.Pool), helpers
```

### Scanning Pipeline

1. `scanDirectory()` calls `os.ReadDir` to read the directory in a single syscall, immediately stats files, and separates directories from files.
2. Directory sizes are computed in parallel using `dirSizeRecursive()` — a manual recursive function using `os.ReadDir` that avoids the overhead of `filepath.WalkDir`. Bounded concurrency is enforced via a semaphore (CPU count, max 16).
3. File stat is parallelised for directories with 20+ files to leverage multi-core CPUs.

### Caching

- **In-memory**: LRU cache holding up to 100 directory scan results. Accessed on navigation; updated on scan completion.
- **On-disk**: Top 50 LRU entries serialized to `$XDG_CACHE_HOME/dirgo/cache.gob` (or `~/.cache/dirgo/cache.gob` if `XDG_CACHE_HOME` is unset) on quit. Entries older than 24 hours are discarded on load.

### Smart Refresh

Pressing `r` compares the directory's current modtime against the cached value. If unchanged, the rescan is skipped entirely (~microseconds). If changed, a full rescan is triggered.

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

# Build cross-platform release binaries
make release
```

### Benchmark Results (Apple M4 Pro)

| Benchmark | Time | Allocs | Bytes/op |
|---|---|---|---|
| ScanDirectory (100 files, 10 subdirs) | ~1.2 ms | 1,159 | 138 KB |
| ScanDeepDir (5 levels) | ~800 µs | 343 | 40 KB |
| CountLines (1 MB text) | ~103 µs | 6 | 535 B |
| FormatSize | ~30 ns | 1 | 8 B |
| BarString | ~66 ns | 1 | 80 B |
| FuzzyMatch | ~18 ns | 0 | 0 B |
| RenderRow | ~11 µs | 99 | 2.4 KB |
| RenderHeader | ~9 µs | 78 | 3.4 KB |
| ApplyFilter (10k entries) | ~35 µs | 0 | 0 B |
| Full View (40 visible rows) | ~465 µs | 3,971 | 152 KB |

### Cross-Platform Binaries

```bash
make VERSION=v1.0.0 release
```

| Target | Binary Size |
|---|---|
| darwin/arm64 | ~3.9 MB |
| darwin/amd64 | ~4.1 MB |
| linux/amd64 | ~4.1 MB |
| windows/amd64 | ~4.3 MB |

## Requirements

- Go 1.21+
- macOS / Linux / Windows

## License

MIT
