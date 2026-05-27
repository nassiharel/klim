<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/klim-wordmark-inter-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="assets/klim-wordmark-inter-light.svg">
    <img src="assets/klim-wordmark-inter-light.svg" alt="klim" width="340">
  </picture>
</p>

<p align="center">
  <strong>Reignite Dev Experience.</strong>
</p>

<p align="center">
  <a href="https://github.com/nassiharel/klim/releases/latest"><img src="https://img.shields.io/github/v/release/nassiharel/klim?style=flat-square&color=brightgreen&label=release" alt="Release"></a>
  <a href="https://github.com/nassiharel/klim/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/nassiharel/klim/ci.yml?style=flat-square&label=build" alt="CI"></a>
  <a href="https://github.com/nassiharel/klim/actions/workflows/codeql.yml"><img src="https://img.shields.io/github/actions/workflow/status/nassiharel/klim/codeql.yml?style=flat-square&label=CodeQL" alt="CodeQL"></a>
  <a href="https://goreportcard.com/report/github.com/nassiharel/klim"><img src="https://img.shields.io/badge/go%20report-A+-brightgreen?style=flat-square" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/nassiharel/klim"><img src="https://img.shields.io/badge/godoc-reference-blue?style=flat-square" alt="Go Reference"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/nassiharel/klim?style=flat-square&label=go&logo=go&logoColor=white" alt="Go version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/nassiharel/klim?style=flat-square" alt="License"></a>
</p>

<p align="center">
  <a href="https://github.com/nassiharel/klim/commits/main"><img src="https://img.shields.io/github/last-commit/nassiharel/klim?style=flat-square&label=last%20commit" alt="Last commit"></a>
  <a href="https://github.com/nassiharel/klim/graphs/contributors"><img src="https://img.shields.io/github/contributors/nassiharel/klim?style=flat-square&color=blue" alt="Contributors"></a>
  <a href="https://github.com/nassiharel/klim/issues"><img src="https://img.shields.io/github/issues/nassiharel/klim?style=flat-square" alt="Open issues"></a>
  <a href="https://github.com/nassiharel/klim/blob/main/CONTRIBUTING.md"><img src="https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square" alt="PRs welcome"></a>
  <img src="https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey?style=flat-square" alt="Platforms">
</p>

---

Klim is a deterministic, cross-platform layer over native package managers for discovering, standardizing, securing, and automating the dev tools every project depends on — the same portable contracts and predictable operations for humans, teams, CI, and AI agents.

https://github.com/user-attachments/assets/54969cc1-47b7-47b7-af35-06d0649da466

## Install

Install with your package manager (recommended) or the bootstrap script. Verify with `klim version`.

**macOS / Linux** — Homebrew

```bash
brew install nassiharel/tap/klim
```

**Windows** — winget or Scoop

```powershell
winget install nassiharel.klim
```

```powershell
scoop bucket add nassiharel https://github.com/nassiharel/scoop-bucket
scoop install klim
```

**Go 1.25+**

```bash
go install github.com/nassiharel/klim/cmd/klim@latest
```

**Bootstrap script** (no package manager required)

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/nassiharel/klim/main/install.sh | bash
```

```powershell
# Windows
irm https://raw.githubusercontent.com/nassiharel/klim/main/install.ps1 | iex
```

<details>
<summary>Other install options (deb / rpm / direct binary / pinned versions)</summary>

#### Debian / Ubuntu

```bash
curl -LO https://github.com/nassiharel/klim/releases/latest/download/klim_<version>_linux_<arch>.deb
sudo dpkg -i klim_<version>_linux_<arch>.deb
```

#### Fedora / CentOS / RHEL

```bash
curl -LO https://github.com/nassiharel/klim/releases/latest/download/klim_<version>_linux_<arch>.rpm
sudo rpm -i klim_<version>_linux_<arch>.rpm
```

#### Direct binary

Pre-built archives for every platform are attached to each [GitHub Release](https://github.com/nassiharel/klim/releases/latest), each with a CycloneDX SBOM and an entry in `checksums.txt`.

```bash
sha256sum klim_<version>_<platform>.tar.gz
# compare against checksums.txt
```

#### Pin a specific version

```bash
curl -fsSL https://raw.githubusercontent.com/nassiharel/klim/main/install.sh | bash -s -- --version v0.1.2
go install github.com/nassiharel/klim/cmd/klim@v0.1.2
brew install nassiharel/tap/klim@0.1.2
winget install nassiharel.klim --version 0.1.2
scoop install klim@0.1.2
```

</details>

## Quick start

```bash
klim                                  # interactive TUI
klim check --output json              # validate this project's .klim.yaml
klim install --pack go-developer      # install a curated bundle
klim diff teammate.yaml               # compare environments
klim security audit --sbom            # audit + emit CycloneDX SBOM
```

---

## Features

### Discover
Scan `PATH` and native package managers for installed tools, versions, sources, binary paths, GitHub metadata, and update status. Commands: `klim list`, `klim info <tool>`, `klim why`, `klim try`, plus role-based recommendations and related-tool suggestions.

### Standardize
Versioned `.klim.yaml` contracts declare required/optional tools and version constraints. `klim init` auto-generates them from `package.json`, `go.mod`, Dockerfiles, CI workflows, Helm, Terraform, Bicep, and more. `klim check` validates locally or in CI; `klim generate github-action` emits a workflow; shell hooks run checks on `cd`.

### Reproduce
Export, import, share, and diff environments across machines and OSes. `klim env` captures a privacy-safe token; `klim trail` records content-addressed snapshots that can be labeled, diffed, and pruned. OS-aware mapping picks the best package manager on each target.

### Automate
Delegates installs and upgrades to managers you already trust — winget, Homebrew, apt, Chocolatey, Scoop, snap, npm — with selection, JSON output, exit codes, dry runs, and packs (110+ curated tools). Commands: `klim install`, `klim upgrade`, `klim remove`, `klim watch`. `klim proxy` creates auto-install shims; custom marketplace URLs let you merge internal catalogs.

### Plan & rollback
`klim plan` produces a Terraform-style preview with a confidence score; `klim apply` auto-checkpoints and runs shell-resolution, binary-validation, PATH-consistency, and manager-integrity postchecks. `klim checkpoint <name>` / `klim rollback <name>` manage named snapshots. PATH backups are captured before any Health-tab fix.

### Audit & security
`klim health` flags PATH conflicts, unmanaged installs, archived upstreams, and stale repos, with an interactive PATH-fix wizard. `klim security audit` runs vulnerability lookup via OSV.dev, license inventory, policy enforcement, and CycloneDX SBOM output. `klim score` summarizes overall toolchain health.

### Interfaces
Interactive TUI with nine tabs — My Tools, Marketplace, Project, Dashboard, My Profile (with My Score breakdown), Health, Security, Backup, Config — plus an optional local web view. Every action also runs from a deterministic CLI with `--output json`, stable exit codes, and shell completions.

---

## For AI agents

Agents handle judgment; Klim handles operations that must be the same every time. Call `klim check`, `klim install`, `klim diff`, or `klim security audit` with `--output json` to get stable, auditable results — no prompt drift, no improvised `curl | bash`.

---

## Architecture

Go, Bubble Tea TUI, Cobra CLI. Runtime flow:

```text
ToolService
  -> ToolCatalog     fetch/cache marketplace.yaml from GitHub
  -> ToolFinder      scan PATH and detect install sources
  -> VersionResolver query native package managers for installed/latest versions
```

Version data comes from native package managers (not a private registry):

| Manager | Platforms |
| --- | --- |
| winget, Chocolatey | Windows |
| Homebrew | macOS, Linux |
| apt / dpkg, snap | Debian/Ubuntu, Linux |
| npm | All |

The marketplace is fetched from `https://raw.githubusercontent.com/nassiharel/klim/marketplace/marketplace.yaml` and cached locally for offline use.

## Configuration

User data lives under `~/.klim/` on every OS; the marketplace cache is at `~/.klim/marketplace/marketplace-cache.yaml`.

```bash
klim config path
klim config edit
```

## Troubleshooting

| Problem | Solution |
| --- | --- |
| `klim: command not found` | Ensure the install directory is on `PATH` (`which klim` / `where klim`). |
| Tool not detected | Verify the binary is on `PATH`, then press `r` in the TUI or pass `--refresh`. |
| Permission denied on upgrade | The native package manager needs elevation — use `sudo` or an Administrator shell. |
| Stale version info | Run `klim security health`, use `--refresh`, or clear the scan cache. |
| Self-update fails | Download the latest archive from [Releases](https://github.com/nassiharel/klim/releases/latest) and replace the binary. |

## Contributing

See [AGENTS.md](./AGENTS.md) for architecture, conventions, and development commands.

## License

MIT
