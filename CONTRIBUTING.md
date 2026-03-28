# Contributing to dirgo

Thanks for your interest in contributing! Here's how to get started.

## Getting Started

1. **Fork** the repository
2. **Clone** your fork:
   ```bash
   git clone https://github.com/<your-username>/dirgo.git
   cd dirgo
   ```
3. **Build** the project:
   ```bash
   make build
   ```

## Development

### Prerequisites

- Go 1.21+
- Make

### Common Commands

```bash
make build         # Build the binary
make test          # Run tests with race detection
make bench         # Run benchmarks
make profile-cpu   # CPU profiling
make profile-mem   # Memory profiling
```

### Project Structure

| File | Purpose |
|---|---|
| `main.go` | Entry point, flags, Bubble Tea setup |
| `model.go` | App state, Update loop, message handling |
| `scanner.go` | Directory scanning, parallel stat |
| `cache.go` | LRU cache with disk persistence |
| `entry.go` | FileEntry model, sorting, filtering |
| `render.go` | Row rendering, header/footer, help |
| `keys.go` | Key bindings |
| `styles.go` | Lipgloss styles |
| `utils.go` | Formatting, line counting, helpers |

## Making Changes

1. Create a feature branch:
   ```bash
   git checkout -b feature/your-feature
   ```
2. Make your changes
3. Run tests:
   ```bash
   make test
   ```
4. Commit with a clear message:
   ```bash
   git commit -m "feat: add your feature description"
   ```
5. Push and open a Pull Request:
   ```bash
   git push origin feature/your-feature
   ```

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` — new feature
- `fix:` — bug fix
- `docs:` — documentation only
- `test:` — adding or updating tests
- `refactor:` — code change that neither fixes a bug nor adds a feature
- `ci:` — CI/CD changes
- `chore:` — maintenance tasks

## Code Style

- Run `go vet ./...` before submitting
- Follow standard Go conventions (`gofmt`)
- Keep functions focused and small
- Add tests for new functionality

## Reporting Issues

- Use [GitHub Issues](https://github.com/mohsinkaleem/dirgo/issues)
- Include your OS, Go version, and steps to reproduce

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
