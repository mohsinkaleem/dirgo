# v1.2.0

## What's New

### Scanning View
- **Reworked progress display**: shows the directory being scanned, thousands-separated counts, elapsed time, and live throughput (`files/s`) instead of a bare `Scanning...` line.
- **Progress on flat directories**: counters previously only advanced while recursing into subdirectories, so a directory with many files and no subdirectories sat at `0 files · 0 dirs` for the entire scan. Top-level file stats now report progress too.
- **No frozen display**: elapsed time appears after 500ms even when no counter has moved yet, so a slow scan is always distinguishable from a hung one.
- **Stale counters cleared**: starting a new scan no longer briefly shows the previous scan's numbers.

### Security
- **Fixed shell injection in hex view (`x`)**: the path was interpolated with `%q` into `sh -c`, which leaves `$` and backticks live inside double quotes. A file named `$(...)` could execute code. Paths are now single-quoted.
- **Fixed PowerShell injection in hex view on Windows**: single quotes in paths are now escaped, and `-LiteralPath` prevents `[]` glob expansion.
- **Removed `cmd.exe` from file opening on Windows**: `cmd /c start` interpreted `&`, `|`, `^` and `%VAR%` in filenames. Now uses `explorer` directly.

### Bug Fixes
- **Fixed navigation being reverted by in-flight scans**: a scan completing after you navigated away would overwrite the current directory. Async results are now tagged with their path and discarded when they no longer match.
- **Fixed scrolling off-by-one**: the scroll viewport assumed a fixed header height and drifted whenever the header wrapped to two lines on narrow terminals.
- **Fixed scroll offset when the list shrinks**: the viewport could be left scrolled past the end of a filtered list.
- **Fixed an unreachable search filter**: applying a search with `Enter` left the filter active with no way to clear it. `Esc` now clears it, and an active filter is shown in the header.
- **Fixed line counts landing on the wrong file**: results were matched by name only, so identically named files in different directories could be crossed.
- **Fixed `.svg` treated as binary**: SVG is XML, so line counting now works on it.
- **Fixed CPU profile not being written**: `--profile` runs that exited via an error path skipped the pprof flush.

### Performance
- **Bounded scanner concurrency**: the worker semaphore was acquired inside each goroutine, so a directory with 200k entries spawned 200k goroutines before any throttling took effect. Replaced with a fixed worker pool.

### Docs
- Corrected the README and CONTRIBUTING claims of on-disk cache persistence — the cache is in-memory and per-session.
- Removed a dead link to a relocated maintainer doc.
- Published the architecture walkthrough and interactive tour under `docs/`.

## Install

```bash
brew install mohsinkaleem/tap/dirgo
```

```bash
pip install dirgo
```

```bash
go install github.com/mohsinkaleem/dirgo@v1.2.0
```
