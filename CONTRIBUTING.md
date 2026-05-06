# Contributing to klim

Thank you for your interest in contributing! This document covers the development workflow.

## Code of Conduct

Please be respectful and constructive in all interactions. We are committed to providing a welcoming and inclusive experience for everyone.

## First-Time Contributors

New to klim? Here's how to get started:

1. Look for issues labeled [`good first issue`](https://github.com/nassiharel/klim/labels/good%20first%20issue) — these are scoped tasks that are great for newcomers.
2. Read through this guide and [AGENTS.md](AGENTS.md) to understand the project structure.
3. Fork the repo, create a branch, make your change, and open a PR.

Not sure where to start? Open a [Discussion](https://github.com/nassiharel/klim/discussions) and we'll help you find something.

## Prerequisites

- [Go](https://go.dev/dl/) 1.25+ (see `.go-version`)
- [golangci-lint](https://golangci-lint.run/welcome/install/) v2+
- [GoReleaser](https://goreleaser.com/install/) v2+ (for testing releases locally)

## Quick Start

```bash
git clone https://github.com/nassiharel/klim.git
cd klim
make build          # compile to bin/klim
make test           # run tests with -race
make lint           # run golangci-lint
```

## Development Commands

| Command          | Description                          | CI Equivalent       |
| ---------------- | ------------------------------------ | ------------------- |
| `make build`     | Build binary to `bin/klim`           | test job (go build) |
| `make test`      | Run tests with race detector         | test job            |
| `make lint`      | Run golangci-lint                    | lint job            |
| `make tidy`      | Check go.mod tidiness                | tidy job            |
| `make vulncheck` | Check for known Go vulnerabilities   | govulncheck job     |
| `make cover`     | Generate HTML coverage report        | —                   |
| `make run`       | Build and run klim                   | —                   |
| `make clean`     | Remove build artifacts               | —                   |
| `make all`       | lint + test + build (default target) | —                   |

## Pull Request Checklist

- [ ] `make lint` passes with no issues
- [ ] `make test` passes on your platform
- [ ] `make tidy` shows no diff
- [ ] New code includes tests where appropriate
- [ ] Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, etc.)

## Adding a Tool to the Marketplace

1. Create `marketplace/tools/mytool.yaml` (see existing files for the schema).
2. Run `make marketplace-validate` to check your definition.
3. Open a PR — CI validates automatically and publishes on merge.

## Project Structure

```
cmd/klim/          → Entry point (main.go)
internal/
  build/           → Version info (ldflags injection)
  cli/             → Cobra commands (root, list, version, tools, export, import, update)
  detector/        → Binary version detection (Go buildinfo, PE resources on Windows)
  finder/          → PATH scanning and tool discovery
  manifest/        → YAML schema for export/import manifests
  pkgmgr/          → Package manager integration (brew, winget, apt, choco, snap, npm)
  registry/        → Tool definitions, version comparison, marketplace YAML
  selfupdate/      → Self-update from GitHub Releases (download, extract, replace binary)
  tui/             → Bubbletea interactive UI (model, view, commands, styles)
```

See [AGENTS.md](AGENTS.md) for detailed architecture documentation.

## Version Injection

When building from source, version information is injected via ldflags:

```bash
go build -trimpath -ldflags "\
  -s -w \
  -X github.com/nassiharel/klim/internal/build.Version=1.0.0 \
  -X github.com/nassiharel/klim/internal/build.Commit=$(git rev-parse --short HEAD) \
  -X github.com/nassiharel/klim/internal/build.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o klim ./cmd/klim
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
