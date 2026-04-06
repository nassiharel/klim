# Contributing to clim

Thank you for your interest in contributing! This document covers the development workflow.

## Prerequisites

- [Go](https://go.dev/dl/) 1.25+ (see `.go-version`)
- [golangci-lint](https://golangci-lint.run/welcome/install/) v2+
- [GoReleaser](https://goreleaser.com/install/) v2+ (for testing releases locally)

## Quick Start

```bash
git clone https://github.com/nassiharel/clim.git
cd clim
make build          # compile to bin/clim
make test           # run tests with -race
make lint           # run golangci-lint
```

## Development Commands

| Command          | Description                          | CI Equivalent       |
| ---------------- | ------------------------------------ | ------------------- |
| `make build`     | Build binary to `bin/clim`           | test job (go build) |
| `make test`      | Run tests with race detector         | test job            |
| `make lint`      | Run golangci-lint                    | lint job            |
| `make tidy`      | Check go.mod tidiness                | tidy job            |
| `make vulncheck` | Check for known Go vulnerabilities   | govulncheck job     |
| `make cover`     | Generate HTML coverage report        | —                   |
| `make run`       | Build and run clim                   | —                   |
| `make clean`     | Remove build artifacts               | —                   |
| `make all`       | lint + test + build (default target) | —                   |

## Pull Request Checklist

- [ ] `make lint` passes with no issues
- [ ] `make test` passes on your platform
- [ ] `make tidy` shows no diff
- [ ] New code includes tests where appropriate
- [ ] Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, etc.)

## Project Structure

```
cmd/clim/          → Entry point (main.go)
internal/
  build/           → Version info (ldflags injection)
  cli/             → Cobra commands (root, list, version, tools, export, import)
  detector/        → Binary version detection (PE resources on Windows)
  finder/          → PATH scanning and tool discovery
  pkgmgr/          → Package manager integration (brew, winget, apt, etc.)
  registry/        → Tool definitions, version comparison, marketplace YAML
  tui/             → Bubbletea interactive UI (model, view, commands, styles)
```

See [AGENTS.md](AGENTS.md) for detailed architecture documentation.

## Version Injection

When building from source, version information is injected via ldflags:

```bash
go build -trimpath -ldflags "\
  -s -w \
  -X github.com/nassiharel/clim/internal/build.Version=1.0.0 \
  -X github.com/nassiharel/clim/internal/build.Commit=$(git rev-parse --short HEAD) \
  -X github.com/nassiharel/clim/internal/build.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o clim ./cmd/clim
```

When installed via `go install`, version info is automatically read from the Go module metadata — no ldflags needed.

## Releases

Releases are automated via [GoReleaser](https://goreleaser.com/) and GitHub Actions. To create a release:

1. Tag the commit: `git tag v1.x.x`
2. Push the tag: `git push origin v1.x.x`
3. GitHub Actions builds binaries, Linux packages (deb/rpm), updates Homebrew tap, and creates the GitHub Release.

To test the release pipeline locally:

```bash
goreleaser release --snapshot --clean
```
