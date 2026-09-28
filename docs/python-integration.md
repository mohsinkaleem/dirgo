# Python Integration & Release Workflow

This document explains how dirgo (a Go binary) is distributed via PyPI so users can install it with `pip install dirgo` or `uv tool install dirgo`, and how releases are managed going forward.

---

## How It Works

dirgo is written in Go. The Python package on PyPI is a **thin wrapper** — it does not contain the compiled binary. Instead, on first run it downloads the correct pre-built binary from GitHub Releases for the user's OS and architecture.

### Package Structure

```
dirgo_python/
├── __init__.py    # Declares __version__ (e.g. "1.2.1")
└── _cli.py        # Entry point: downloads + runs the Go binary
```

### Entry Point

`pyproject.toml` declares a console script:

```toml
[project.scripts]
dirgo = "dirgo_python._cli:main"
```

When a user runs `dirgo`, Python calls `dirgo_python._cli:main()`.

### Binary Download Flow (`_cli.py`)

1. **Detect platform** — maps `platform.system()` and `platform.machine()` to goreleaser's naming convention:
   - OS: `darwin`, `linux`, `windows`
   - Arch: `amd64`, `arm64`

2. **Check cache** — looks for the binary at `dirgo_python/_bin/<version>/dirgo` (or `dirgo.exe` on Windows). If it exists, skip download. The path is per version because pip and uv only remove files they installed: an unversioned cache would keep running the old binary after an upgrade.

3. **Download** — constructs the URL from the version in `__init__.py`:
   ```
   https://github.com/mohsinkaleem/dirgo/releases/download/v{version}/dirgo_{version}_{os}_{arch}.tar.gz
   ```
   Windows archives use `.zip` instead of `.tar.gz`.

4. **Verify and extract** — checks the archive's SHA-256 against the release's `checksums.txt`, extracts the `dirgo` binary, writes it to a temp file and renames it into place (so an interrupted run never leaves a truncated binary), then removes binaries cached for other versions.

5. **Execute** — runs the binary with `subprocess.call()`, passing through all CLI arguments and the exit code.

### Supported Platforms

| OS      | Arch  | Archive Format |
|---------|-------|---------------|
| macOS   | amd64 | tar.gz        |
| macOS   | arm64 | tar.gz        |
| Linux   | amd64 | tar.gz        |
| Linux   | arm64 | tar.gz        |
| Windows | amd64 | zip           |
| Windows | arm64 | zip           |

---

## Release Process

### Step-by-Step: Cutting a New Release

1. **Update versions** in two files:
   - `dirgo_python/__init__.py` → `__version__ = "X.Y.Z"`
   - `pyproject.toml` → `version = "X.Y.Z"`

2. **Commit and push** to `main`:
   ```bash
   git add -A
   git commit -m "chore: bump version to X.Y.Z"
   git push origin main
   ```

3. **Tag and push**:
   ```bash
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

4. The tag push triggers the **Release** workflow → goreleaser builds binaries and creates a GitHub Release.

5. The Release workflow completing triggers the **Publish to PyPI** workflow → builds the Python sdist/wheel and publishes to PyPI.

6. Users can now run:
   ```bash
   pip install --upgrade dirgo
   uv tool install dirgo
   ```

### What Gets Published to PyPI

The PyPI package contains **only the Python wrapper code** (~3 KB), not the Go binary. The binary is downloaded on first use from GitHub Releases. This means:
- The PyPI package is tiny and platform-independent (`py3-none-any`)
- The Go binary version is pinned to the `__version__` in the Python package
- Upgrading the pip package automatically picks up the new binary on next run

---

## GitHub Actions Workflows

### 1. CI (`ci.yml`)

**Trigger:** Push to `main`, pull requests to `main`

Runs Go build, tests (with race detection), and vet across a matrix:
- OS: ubuntu, macOS, Windows
- Go: 1.24, 1.25

### 2. Release (`release.yml`)

**Trigger:** Tag push matching `v*`

Runs [goreleaser](https://goreleaser.com/) which:
- Builds `dirgo` for linux/darwin/windows × amd64/arm64 (CGO disabled)
- Creates `.tar.gz` archives (`.zip` for Windows)
- Generates checksums
- Creates a GitHub Release with all artifacts
- Pushes the Homebrew cask to `mohsinkaleem/homebrew-tap` (skipped for pre-releases)

**Secrets required:**
- `GITHUB_TOKEN` — automatic, used for the release itself
- `HOMEBREW_TAP_GITHUB_TOKEN` — PAT with `repo` scope, used to push the Homebrew cask to the tap repo

### 3. Publish to PyPI (`pypi.yml`)

**Trigger:** Completion of the Release workflow, or manual dispatch

Builds the Python sdist + wheel with `python -m build` and publishes to PyPI using [Trusted Publishing](https://docs.pypi.org/trusted-publishers/) (OIDC, no API tokens needed in secrets).

**PyPI setup:**
- Trusted Publisher is configured on pypi.org for `mohsinkaleem/dirgo`, workflow `pypi.yml`
- The workflow uses `id-token: write` permission for OIDC authentication

---

## Secrets Summary

| Secret | Where | Purpose |
|--------|-------|---------|
| `GITHUB_TOKEN` | Auto-provided | GitHub Release creation, artifact upload |
| `HOMEBREW_TAP_GITHUB_TOKEN` | Repo secret | Push Homebrew cask to `homebrew-tap` repo |
| *(none for PyPI)* | OIDC | Trusted Publishing handles PyPI auth |

---

## Troubleshooting

**PyPI workflow didn't trigger after a release?**
The `workflow_run` event only fires when the Release workflow completes successfully. If the release failed, fix the issue and either re-tag or manually trigger: `gh workflow run pypi.yml`.

**Binary download fails at install time?**
The GitHub Release must contain the binary archives. Verify at `https://github.com/mohsinkaleem/dirgo/releases/tag/vX.Y.Z`.

**Version mismatch?**
Ensure `__version__` in `dirgo_python/__init__.py`, `version` in `pyproject.toml`, and the git tag all match.
