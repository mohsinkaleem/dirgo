# v1.2.1

## What's New

### Install
- **Homebrew now installs dirgo as a cask**: GoReleaser has deprecated generated Homebrew formulas, so releases now publish `Casks/dirgo.rb` to the tap. The install command is unchanged (`brew install mohsinkaleem/tap/dirgo`). If you installed dirgo from Homebrew before v1.2.1, switch to the cask once (Homebrew 6+ won't do it automatically because the new cask isn't trusted yet):

  ```bash
  brew uninstall --formula --force dirgo
  brew install --cask mohsinkaleem/tap/dirgo
  ```
- **`go install` builds report their version**: `dirgo --version` printed `dev` for binaries built with `go install ...@vX.Y.Z`. It now falls back to the module version Go records at build time.

### Bug Fixes
- **pip / uv upgrades now pick up the new binary**: the downloaded binary was cached at a fixed path that pip and uv don't remove, so after `pip install --upgrade dirgo` the old version kept running. Binaries are now cached per version and older ones are cleaned up.
- **Downloaded binaries are verified**: the pip/uv wrapper now checks the archive against the release's `checksums.txt` before installing it, and writes the binary atomically so an interrupted first run can't leave a truncated file behind.

### Project
- Added a security policy (private vulnerability reporting), a code of conduct, and issue / pull request templates.
- The architecture guide and interactive tour are now published at https://mohsinkaleem.github.io/dirgo/.
- CI now validates the GoReleaser config on every push, and the release workflow runs the tests before publishing.

## Install

```bash
brew install mohsinkaleem/tap/dirgo
```

```bash
pip install dirgo
```

```bash
go install github.com/mohsinkaleem/dirgo@v1.2.1
```
