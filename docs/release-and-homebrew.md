# Release binaries and Homebrew install (`brew install dirgo`)

This document describes how to:

1. Build and upload binary release assets to GitHub.
2. Publish a Homebrew tap so users can install with `brew install dirgo`.

## Prerequisites

- GitHub repo: `mohsinkaleem/dirgo`
- `gh` CLI authenticated (`gh auth status`)
- Go installed locally
- Homebrew installed (for local formula testing)

## 1) Build release binaries

From the repo root:

```bash
VERSION=v0.1.0 make release
```

This creates binaries in `dist/`:

- `dirgo-darwin-arm64`
- `dirgo-darwin-amd64`
- `dirgo-linux-amd64`
- `dirgo-linux-arm64`
- `dirgo-windows-amd64.exe`

## 2) Package archives (recommended for Homebrew)

Homebrew formulas are easiest to maintain with `.tar.gz` assets per OS/arch.

```bash
VERSION=v0.1.0
rm -rf release && mkdir -p release

cp dist/dirgo-darwin-arm64 release/dirgo
tar -C release -czf release/dirgo_${VERSION#v}_darwin_arm64.tar.gz dirgo

cp dist/dirgo-darwin-amd64 release/dirgo
tar -C release -czf release/dirgo_${VERSION#v}_darwin_amd64.tar.gz dirgo

cp dist/dirgo-linux-amd64 release/dirgo
tar -C release -czf release/dirgo_${VERSION#v}_linux_amd64.tar.gz dirgo

cp dist/dirgo-linux-arm64 release/dirgo
tar -C release -czf release/dirgo_${VERSION#v}_linux_arm64.tar.gz dirgo

cp dist/dirgo-windows-amd64.exe release/dirgo.exe
tar -C release -czf release/dirgo_${VERSION#v}_windows_amd64.tar.gz dirgo.exe
```

Generate checksums:

```bash
shasum -a 256 release/*.tar.gz > release/checksums.txt
```

## 3) Create GitHub release and upload assets

Create a tag and push it:

```bash
git tag -a "$VERSION" -m "Release $VERSION"
git push origin "$VERSION"
```

Create release and upload archives:

```bash
gh release create "$VERSION" release/*.tar.gz release/checksums.txt \
  --title "$VERSION" \
  --notes "See checksums.txt for SHA256 checksums."
```

## 4) Publish a Homebrew tap

Create a separate tap repo named `homebrew-tap`:

- `https://github.com/mohsinkaleem/homebrew-tap`

Inside that repo, create `Formula/dirgo.rb` (name must match the install command):

```ruby
class Dirgo < Formula
  desc "Fast interactive terminal directory analyzer"
  homepage "https://github.com/mohsinkaleem/dirgo"
  version "0.1.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/mohsinkaleem/dirgo/releases/download/v#{version}/dirgo_#{version}_darwin_arm64.tar.gz"
      sha256 "REPLACE_DARWIN_ARM64_SHA256"
    else
      url "https://github.com/mohsinkaleem/dirgo/releases/download/v#{version}/dirgo_#{version}_darwin_amd64.tar.gz"
      sha256 "REPLACE_DARWIN_AMD64_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/mohsinkaleem/dirgo/releases/download/v#{version}/dirgo_#{version}_linux_arm64.tar.gz"
      sha256 "REPLACE_LINUX_ARM64_SHA256"
    else
      url "https://github.com/mohsinkaleem/dirgo/releases/download/v#{version}/dirgo_#{version}_linux_amd64.tar.gz"
      sha256 "REPLACE_LINUX_AMD64_SHA256"
    end
  end

  def install
    bin.install "dirgo"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/dirgo --version")
  end
end
```

## 5) User install commands

After formula is merged to `homebrew-tap`:

```bash
brew tap mohsinkaleem/tap
brew install dirgo
```

Optional upgrade flow:

```bash
brew update
brew upgrade dirgo
```

## 6) Release checklist for each new version

1. Bump `VERSION` and run `make release`.
2. Build `.tar.gz` assets and `checksums.txt`.
3. Publish GitHub tag + release assets.
4. Update `Formula/dirgo.rb`:
   - `version`
   - asset URLs
   - SHA256 values
5. Merge formula update in `homebrew-tap`.
6. Validate locally:
   - `brew untap mohsinkaleem/tap || true`
   - `brew tap mohsinkaleem/tap`
   - `brew reinstall dirgo`
   - `dirgo --version`
