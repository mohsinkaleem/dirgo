"""Download and run the dirgo Go binary for the current platform."""

import contextlib
import hashlib
import os
import platform
import shutil
import subprocess
import sys
import tarfile
import tempfile
import zipfile
from io import BytesIO
from pathlib import Path
from urllib.request import urlopen

from dirgo_python import __version__

GITHUB_REPO = "mohsinkaleem/dirgo"
# pip and uv only remove the files they installed, so the downloaded binary
# survives an upgrade. Keeping it under a per-version directory makes an
# upgraded package fetch its own binary instead of running the old one.
_BIN_ROOT = Path(__file__).parent / "_bin"
_BIN_DIR = _BIN_ROOT / __version__


def _platform_key():
    system = platform.system().lower()
    machine = platform.machine().lower()

    os_map = {"darwin": "darwin", "linux": "linux", "windows": "windows"}
    arch_map = {
        "x86_64": "amd64",
        "amd64": "amd64",
        "arm64": "arm64",
        "aarch64": "arm64",
    }

    os_name = os_map.get(system)
    arch = arch_map.get(machine)
    if not os_name or not arch:
        sys.exit(f"dirgo: unsupported platform {system}/{machine}")
    return os_name, arch


def _binary_path():
    os_name, _ = _platform_key()
    name = "dirgo.exe" if os_name == "windows" else "dirgo"
    return _BIN_DIR / name


def _archive_name():
    os_name, arch = _platform_key()
    ext = "zip" if os_name == "windows" else "tar.gz"
    return f"dirgo_{__version__}_{os_name}_{arch}.{ext}"


def _release_url(filename):
    return f"https://github.com/{GITHUB_REPO}/releases/download/v{__version__}/{filename}"


def _fetch(url):
    with urlopen(url) as resp:  # noqa: S310 — URL is hardcoded to GitHub
        return resp.read()


def _expected_sha256(archive):
    for line in _fetch(_release_url("checksums.txt")).decode().splitlines():
        parts = line.split()
        if len(parts) == 2 and parts[1] == archive:
            return parts[0]
    sys.exit(f"dirgo: {archive} is not listed in checksums.txt")


def _extract(data, archive, name):
    if archive.endswith(".zip"):
        with zipfile.ZipFile(BytesIO(data)) as zf:
            for member in zf.namelist():
                if os.path.basename(member) == name:
                    return zf.read(member)
    else:
        with tarfile.open(fileobj=BytesIO(data), mode="r:gz") as tf:
            for member in tf.getmembers():
                if member.isfile() and os.path.basename(member.name) == name:
                    return tf.extractfile(member).read()
    sys.exit("dirgo: failed to extract binary from archive")


def _remove_old_binaries():
    # Best effort: an old binary may still be running (and locked on Windows).
    for entry in _BIN_ROOT.iterdir():
        if entry == _BIN_DIR:
            continue
        if entry.is_dir():
            shutil.rmtree(entry, ignore_errors=True)
        else:
            with contextlib.suppress(OSError):
                entry.unlink()


def _ensure_binary():
    binary = _binary_path()
    if binary.exists():
        return binary

    archive = _archive_name()
    url = _release_url(archive)
    print(f"dirgo: downloading {url} ...", file=sys.stderr)
    data = _fetch(url)

    if hashlib.sha256(data).hexdigest() != _expected_sha256(archive):
        sys.exit(f"dirgo: checksum mismatch for {archive}")

    content = _extract(data, archive, binary.name)

    # Write to a temp file and rename it into place so an interrupted run
    # never leaves a truncated binary that later runs would pick up.
    _BIN_DIR.mkdir(parents=True, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=_BIN_DIR)
    try:
        with os.fdopen(fd, "wb") as f:
            f.write(content)
        os.chmod(tmp, 0o755)
        os.replace(tmp, binary)
    except BaseException:
        with contextlib.suppress(OSError):
            os.unlink(tmp)
        raise

    _remove_old_binaries()
    return binary


def main():
    binary = _ensure_binary()
    raise SystemExit(subprocess.call([str(binary)] + sys.argv[1:]))


if __name__ == "__main__":
    main()
