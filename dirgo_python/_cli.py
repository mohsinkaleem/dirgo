"""Download and run the dirgo Go binary for the current platform."""

import os
import platform
import stat
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
_BIN_DIR = Path(__file__).parent / "_bin"


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


def _download_url():
    os_name, arch = _platform_key()
    ext = "zip" if os_name == "windows" else "tar.gz"
    version = __version__
    filename = f"dirgo_{version}_{os_name}_{arch}.{ext}"
    return f"https://github.com/{GITHUB_REPO}/releases/download/v{version}/{filename}"


def _ensure_binary():
    binary = _binary_path()
    if binary.exists():
        return binary

    url = _download_url()
    print(f"dirgo: downloading {url} ...", file=sys.stderr)

    with urlopen(url) as resp:  # noqa: S310 — URL is hardcoded to GitHub
        data = resp.read()

    _BIN_DIR.mkdir(parents=True, exist_ok=True)
    os_name, _ = _platform_key()

    if os_name == "windows":
        with zipfile.ZipFile(BytesIO(data)) as zf:
            for member in zf.namelist():
                if member.endswith("dirgo.exe"):
                    binary.write_bytes(zf.read(member))
                    break
    else:
        with tarfile.open(fileobj=BytesIO(data), mode="r:gz") as tf:
            for member in tf.getmembers():
                if member.name.endswith("dirgo"):
                    f = tf.extractfile(member)
                    if f:
                        binary.write_bytes(f.read())
                    break

    if not binary.exists():
        sys.exit("dirgo: failed to extract binary from archive")

    binary.chmod(binary.stat().st_mode | stat.S_IEXEC | stat.S_IXGRP | stat.S_IXOTH)
    return binary


def main():
    binary = _ensure_binary()
    raise SystemExit(subprocess.call([str(binary)] + sys.argv[1:]))


if __name__ == "__main__":
    main()
