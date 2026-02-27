# dirgo

A fast, interactive terminal directory analyzer built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea). Visualize disk usage, explore directories, and identify space hogs — all from your terminal.

![dirgo screenshot](https://raw.githubusercontent.com/mohsinkaleem/dirgo/main/.github/screenshot.png)

## Features

- **Instant directory listing** — files appear immediately; directory sizes compute in the background
- **Proportional size bars** — color-coded percentage bars for quick visual scanning
- **Efficient directory scanning** — uses `os.ReadDir` + manual recursion to minimize syscalls; parallel stat with bounded concurrency
- **Smart refresh** — checks directory modtime before rescanning; skips unchanged directories
- **LRU cache** — bounded in-memory cache (100 entries) with disk persistence across sessions (respects `XDG_CACHE_HOME`)
- **Line counting** — automatic line count for the selected text file; batch count all with `s`
- **Hex view** — built-in hex dump for binary files (`xxd` on macOS, `hexdump` fallback on Linux)
- **Large file protection** — prevents accidentally opening very large blob files
- **Fuzzy search** — filter entries in real time with subsequence matching
- **Symlink detection** — symlinks shown with `→` / `⇢` indicators
- **Move to trash** — safely delete files/directories with `d`
- **Cross-platform** — works on macOS, Linux, and Windows (Quick Look, file open, and cache paths adapt per OS)
- **CPU profiling** — built-in `--profile` flag for performance analysis

## Install

### Go install

```bash
go install github.com/mohsinkaleem/dirgo@latest
```

### From source

```bash
git clone https://github.com/mohsinkaleem/dirgo.git
cd dirgo
make build
```

### Homebrew (coming soon)

```bash
brew install mohsinkaleem/tap/dirgo
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
| `f` | Cycle filter (all → dirs only → files only) |
| `s` | Count lines for all files |
| `c` | cd to path |
| `x` | Hex view (binary files) |
| `d` | Move to trash |
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

## Requirements

- Go 1.21+
- macOS / Linux / Windows

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

[MIT](LICENSE)
