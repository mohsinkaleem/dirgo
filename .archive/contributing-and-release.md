# Contributing, Releasing & Homebrew — dirgo

A complete reference for how to develop, release, and distribute dirgo.

---

## Table of Contents

1. [Project structure quick-ref](#1-project-structure-quick-ref)
2. [Adding a feature](#2-adding-a-feature)
3. [Pushing to GitHub](#3-pushing-to-github)
4. [CI — what runs and when](#4-ci--what-runs-and-when)
5. [Making a release](#5-making-a-release)
6. [Where released binaries come from](#6-where-released-binaries-come-from)
7. [Homebrew — how it works](#7-homebrew--how-it-works)
8. [Customising the Homebrew formula](#8-customising-the-homebrew-formula)
9. [One-time setup checklist](#9-one-time-setup-checklist)

---

## 1. Project structure quick-ref

```
dirgo/
├── main.go          # Entry point, CLI flags, Tea program setup
├── model.go         # All app state, Update() and View() logic
├── scanner.go       # Directory scanning, line counting, progress
├── cache.go         # In-session LRU cache (no disk persistence)
├── entry.go         # FileEntry struct, filtering, sorting, fuzzy match
├── render.go        # Header, row, footer, help overlay rendering
├── styles.go        # Lipgloss color palette and style objects
├── keys.go          # Key bindings (KeyMap)
├── utils.go         # formatSize, countLines, barString, helpers
├── *_test.go        # Tests alongside each source file
├── .goreleaser.yml  # Build + archive + Homebrew formula config
├── .github/
│   └── workflows/
│       ├── ci.yml       # Runs on every push/PR to main
│       └── release.yml  # Runs when a v* tag is pushed
└── docs/
    └── contributing-and-release.md  # ← you are here
```

---

## 2. Adding a feature

### Typical change locations

| What you're adding | Where to edit |
|---|---|
| New key binding | `keys.go` — add to `KeyMap` struct and `DefaultKeyMap()` |
| New UI action / state | `model.go` — add state to `Model`, handle in `Update()` |
| New background operation | `scanner.go` — write a `tea.Cmd` returning a message type |
| New display column / style | `render.go` + `styles.go` |
| New utility function | `utils.go` (with test in `utils_test.go`) |
| New entry metadata | `entry.go` — add field to `FileEntry`, populate in `scanner.go` |

### Step-by-step

```bash
# 1. Work on a branch (optional but recommended for bigger features)
git checkout -b feat/my-feature

# 2. Make your changes
# 3. Run tests locally before committing
go test -race ./...
go vet ./...
go build ./...    # quick sanity check

# 4. Run the app
go run . [path]

# 5. Commit (use conventional commit prefixes — they affect changelog)
git commit -m "feat: describe what it does"
```

### Conventional commit prefixes

The GoReleaser changelog filters these prefixes from release notes — use them consistently:

| Prefix | Meaning | Appears in changelog? |
|---|---|---|
| `feat:` | New feature | Yes |
| `fix:` | Bug fix | Yes |
| `docs:` | Documentation only | No |
| `ci:` | CI/workflow changes | No |
| `chore:` | Misc maintenance | No |
| `test:` | Test changes only | No |

---

## 3. Pushing to GitHub

```bash
# Push your branch (triggers CI — see section 4)
git push origin feat/my-feature

# Or push directly to main for small changes
git push origin main
```

The remote is `https://github.com/mohsinkaleem/dirgo.git`.

**Never push a `v*` tag unless you intend to cut a release** — pushing a tag triggers the GoReleaser release workflow immediately.

---

## 4. CI — what runs and when

### CI workflow (`.github/workflows/ci.yml`)

**Triggers:** every push to `main`, every pull request targeting `main`.

**Matrix:** 6 combinations — 3 OSes × 2 Go versions:

| OS | Go versions |
|---|---|
| ubuntu-latest | 1.24, 1.25 |
| macos-latest | 1.24, 1.25 |
| windows-latest | 1.24, 1.25 |

**Steps per combination:**
1. `go build ./...` — ensures the project compiles
2. `go test -race ./...` — runs all tests with race detector
3. `go vet ./...` — static analysis

**What it does NOT do:** create a release, build distributable binaries, or touch the Homebrew tap. CI is purely a quality gate.

### Release workflow (`.github/workflows/release.yml`)

**Trigger:** pushing any tag that matches `v*` (e.g. `v1.2.0`).

**What it does:**
1. Checks out the full git history (`fetch-depth: 0` — needed for GoReleaser changelog)
2. Sets up Go (stable version)
3. Runs GoReleaser, which:
   - Builds 6 binaries (see section 6)
   - Packages archives
   - Creates the GitHub Release with all assets
   - Generates and pushes the Homebrew formula to `mohsinkaleem/homebrew-tap`

---

## 5. Making a release

### Prerequisites (one-time — see section 9)
- `HOMEBREW_TAP_GITHUB_TOKEN` secret added to the dirgo repo settings

### Release steps

```bash
# 1. Make sure main is clean and tests pass
git checkout main
git pull origin main
go test -race ./...

# 2. Decide the version (semver)
#    v1.0.0 — first stable
#    v1.1.0 — new features
#    v1.1.1 — bug fix only

# 3. Create and push an annotated tag
git tag v1.2.0 -m "v1.2.0: short description of this release"
git push origin v1.2.0
```

That's it. The release workflow fires automatically within ~10 seconds.

### Monitoring the release

```bash
# Watch workflow runs
gh run list --limit 5

# Watch a specific run live
gh run watch <run-id>

# Verify the release was published
gh release view v1.2.0
```

### If the release workflow fails

- Binaries still uploaded? → GoReleaser uploads assets before pushing the Homebrew formula. Check `gh release view` — if assets are there, the only likely failure is the Homebrew push.
- Homebrew push failed (403)? → The `HOMEBREW_TAP_GITHUB_TOKEN` secret is missing or expired. See section 9.
- Build failed? → GoReleaser will print the Go compiler error. Fix the code, delete the tag (`git push origin :refs/tags/v1.2.0`), and re-tag.

---

## 6. Where released binaries come from

GoReleaser (`.goreleaser.yml`) builds 6 static binaries in CI using cross-compilation:

| OS | Arch | Archive format |
|---|---|---|
| linux | amd64 | `.tar.gz` |
| linux | arm64 | `.tar.gz` |
| darwin (macOS) | amd64 (Intel) | `.tar.gz` |
| darwin (macOS) | arm64 (Apple Silicon) | `.tar.gz` |
| windows | amd64 | `.zip` |
| windows | arm64 | `.zip` |

Each archive contains the `dirgo` (or `dirgo.exe`) binary.  
`CGO_ENABLED=0` is set so the binaries are fully static — no system libs required.  
The version string is injected at compile time via `-X main.version={{.Version}}`.

All archives plus `checksums.txt` are attached to the GitHub Release at:  
`https://github.com/mohsinkaleem/dirgo/releases/tag/v<version>`

---

## 7. Homebrew — how it works

### The two repos involved

| Repo | Purpose |
|---|---|
| `mohsinkaleem/dirgo` | Source code, CI, releases |
| `mohsinkaleem/homebrew-tap` | Hosts the Homebrew formula (`dirgo.rb`) |

### What `brew install mohsinkaleem/tap/dirgo` does

1. Homebrew fetches `dirgo.rb` from `mohsinkaleem/homebrew-tap`
2. The formula selects the correct archive URL based on OS and CPU arch
3. Homebrew downloads and verifies the `.tar.gz` SHA256 against the formula
4. It extracts the binary and symlinks it into `/opt/homebrew/bin/dirgo` (macOS/Linux)
5. From that point `dirgo` is available system-wide

### How the formula gets updated on each release

GoReleaser automatically:
1. Generates a new `dirgo.rb` with the correct version, URLs, and SHA256 checksums
2. Pushes (commits) the updated file to `mohsinkaleem/homebrew-tap` using the `HOMEBREW_TAP_GITHUB_TOKEN`

So after a release, `brew upgrade dirgo` (or `brew install mohsinkaleem/tap/dirgo`) will get the new version without any manual work.

### Current formula (for reference)

```ruby
class Dirgo < Formula
  desc "Fast, interactive terminal directory analyzer"
  homepage "https://github.com/mohsinkaleem/dirgo"
  license "MIT"

  on_macos do
    on_intel do
      url "...darwin_amd64.tar.gz"
      sha256 "..."
    end
    on_arm do
      url "...darwin_arm64.tar.gz"
      sha256 "..."
    end
  end

  on_linux do
    on_intel do
      url "...linux_amd64.tar.gz"
      sha256 "..."
    end
    on_arm do
      url "...linux_arm64.tar.gz"
      sha256 "..."
    end
  end

  def install
    bin.install "dirgo"
  end

  test do
    assert_match "dirgo", shell_output("#{bin}/dirgo --version")
  end
end
```

Windows is not distributed via Homebrew (Homebrew is macOS/Linux only). Windows users download the `.zip` from the GitHub Release page directly.

---

## 8. Customising the Homebrew formula

All customisation lives in `.goreleaser.yml` under the `brews:` key. GoReleaser regenerates and pushes `dirgo.rb` on every release from this config.

### Things you can change

**Formula description / homepage**
```yaml
brews:
  - description: "Your updated description"
    homepage: "https://github.com/mohsinkaleem/dirgo"
```

**Post-install shell completion** (example: zsh)
```yaml
    install: |
      bin.install "dirgo"
      # If you ship a completions file in the archive:
      zsh_completion.install "completions/dirgo.zsh" => "_dirgo"
```

**Dependencies** (if you ever need a runtime dep)
```yaml
    dependencies:
      - name: less
        os: linux
```

**`brew test` block** — the test runs after install to sanity-check:
```yaml
    test: |
      assert_match "dirgo #{version}", shell_output("#{bin}/dirgo --version")
```

**Caveats shown to the user after install**
```yaml
    caveats: |
      Run `dirgo [path]` to browse a directory interactively.
```

**Tap repository** — if you ever rename the tap:
```yaml
    repository:
      owner: mohsinkaleem
      name: homebrew-tap          # repo name
      branch: main                # optional, defaults to main
      token: "{{ .Env.HOMEBREW_TAP_GITHUB_TOKEN }}"
```

After changing `.goreleaser.yml`, the updated formula will be pushed automatically on the next release — no manual edits to `dirgo.rb` needed.

### Manually updating the formula (e.g. to patch without a new release)

```bash
# Clone the tap
git clone https://github.com/mohsinkaleem/homebrew-tap.git
cd homebrew-tap

# Edit dirgo.rb directly, then commit and push
git add dirgo.rb
git commit -m "fix: patch formula"
git push origin main
```

---

## 9. One-time setup checklist

Items that only need to be done once (or when rotating credentials):

### `HOMEBREW_TAP_GITHUB_TOKEN` secret

GoReleaser needs permission to push to the *separate* `mohsinkaleem/homebrew-tap` repo. The default `GITHUB_TOKEN` in Actions is scoped only to the current repo, so a PAT is required.

**Steps:**
1. Go to GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens
2. Create a token scoped to `mohsinkaleem/homebrew-tap` with **Contents: Read & Write**
3. Go to `github.com/mohsinkaleem/dirgo` → Settings → Secrets and variables → Actions
4. Create a secret named exactly `HOMEBREW_TAP_GITHUB_TOKEN` and paste the token

Once set, every `v*` tag push will auto-update the formula.

### Local goreleaser (for dry-run testing)

```bash
# Install goreleaser locally
brew install goreleaser

# Dry-run a release without publishing anything
goreleaser release --snapshot --clean
# Outputs to ./dist/ — lets you inspect archives before tagging
```

---

## Quick reference — day-to-day

```bash
# Develop
go test -race ./...        # run tests
go run . [path]            # run the app
go vet ./...               # lint

# Push (triggers CI only)
git push origin main

# Release
git tag v1.x.y -m "v1.x.y: description"
git push origin v1.x.y    # triggers GoReleaser

# Monitor
gh run list --limit 5
gh release view v1.x.y

# Homebrew install (users)
brew tap mohsinkaleem/tap
brew install dirgo

# Or in one step:
brew install mohsinkaleem/tap/dirgo
```
