# v1.3.0

## What's New

### Faster scans
- **Big folders no longer scan on one thread.** Each top-level subfolder used to get its own worker, so one huge folder (`node_modules`, `~/Library`, a Go module cache) did all the work while the rest sat idle. Workers now share the walk folder by folder: warm-cache scans of trees with one dominant child are about 3.5× faster (`~/go`, 145k files: 4.2 s → 1.2 s; a 640k-file `~/Documents`: 15.5 s → 4.5 s).
- **Going up reuses what's already scanned.** Opening a parent for the first time no longer re-walks the folder you just came from.

### Bug Fixes
- **Going back after deleting no longer rescans.** Deleting with `d` discarded the folder's cached listing, so returning to it — or deleting and then pressing `←` before Finder finished — rescanned the whole tree. Deletes now update the cached sizes of the folder and every parent in place.
- **Keys no longer act on rows you can't see.** While the scanning screen or an error was showing, `d` and `→` used the previous folder's rows against the new path, which could trash a same-named file in the wrong folder.
- **Errors no longer hide the listing.** A failed delete, a bad `c` path, or a refused open now shows in the footer and clears on the next key.
- **`j` and `k` can be typed in search** instead of moving the cursor.
- **Refresh keeps your place**, and so do the hidden / filter / top-10 toggles and `Esc`.
- Re-entering a folder while it is still scanning joins that scan instead of starting another.
- Fixed a data race in count-all-lines (`s`), the cursor going off-screen after a resize, `~` shortening of paths like `/Users/alex` when home is `/Users/al`, and the footer wrapping (and pushing the header off screen) in narrow windows.

### Behaviour change
- **Symlinked folders are sized as the link itself.** They can still be opened, but are no longer followed when computing sizes — the same as `du` and `ncdu`. Previously their targets were counted twice (on macOS `/etc` is a link into `/private/etc`), and a link to `/` walked the whole disk.

### Install
- New `curl | sh` install script: `curl -fsSL https://raw.githubusercontent.com/mohsinkaleem/dirgo/main/scripts/install.sh | sh`

## Install

```bash
brew install mohsinkaleem/tap/dirgo
```

```bash
pip install dirgo
```

```bash
go install github.com/mohsinkaleem/dirgo@v1.3.0
```
