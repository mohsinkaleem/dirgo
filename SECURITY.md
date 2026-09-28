# Security Policy

## Supported Versions

Security fixes are released for the latest version of dirgo only. Please upgrade before reporting:

```bash
brew upgrade mohsinkaleem/tap/dirgo   # Homebrew
pip install --upgrade dirgo           # pip
uv tool upgrade dirgo                 # uv
```

## Reporting a Vulnerability

**Please do not open a public issue for security problems.**

Report them privately through GitHub instead: [open a security advisory](https://github.com/mohsinkaleem/dirgo/security/advisories/new) (Security tab → *Report a vulnerability*). Only the maintainer can see it, and the fix is coordinated in the same thread.

Include:

- the dirgo version (`dirgo --version`) and how you installed it
- your OS and terminal
- steps or a file/directory layout that reproduces the problem
- what an attacker could achieve

dirgo runs external commands for some actions (opening files, Quick Look, hex view, move to trash), so reports about file names or paths that escape those commands are especially welcome.
