## What's New

### Bug Fixes
- **Fixed delete (d) on Linux**: Implemented proper XDG Trash specification support with `gio trash` and `trash-put` fallbacks. Creates `.trashinfo` metadata files per the spec.
- **Fixed cross-filesystem trash**: Falls back to copy+delete when `os.Rename` fails across mount points.
- **Fixed trash collision handling**: Uses incrementing suffix instead of naive single `.1` append.
- **Fixed zombie process leak in Quick Look**: Added proper child process reaping.
- **Fixed file handle race in Quick Look**: Removed premature devNull close.

### Cross-Platform Improvements
- **Windows trash**: Uses PowerShell RecycleBin API (`Microsoft.VisualBasic.FileIO`) instead of broken `~/.Trash` fallback.
- **Windows hex view**: Uses `Format-Hex | more` instead of `sh -c` which doesn't exist on Windows.
- **Linux trash**: Tries desktop-native tools (`gio trash`, `trash-put`) before manual XDG Trash fallback.

### Binaries
| Platform | Binary |
|----------|--------|
| macOS (Apple Silicon) | `dirgo-darwin-arm64` |
| macOS (Intel) | `dirgo-darwin-amd64` |
| Linux (x86_64) | `dirgo-linux-amd64` |
| Linux (ARM64) | `dirgo-linux-arm64` |
| Windows (x86_64) | `dirgo-windows-amd64.exe` |
