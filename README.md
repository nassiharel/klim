<p align="center">
  <img src="assets/logo.svg" alt="clim logo" width="480">
</p>

<h1 align="center">clim</h1>

<p align="center">
  <strong>One command to discover, inspect, and upgrade every CLI on your system.</strong>
</p>

<p align="center">
  <a href="https://github.com/nassiharel/clim/releases/latest"><img src="https://img.shields.io/github/v/release/nassiharel/clim?style=flat-square" alt="Release"></a>
  <a href="https://github.com/nassiharel/clim/actions"><img src="https://img.shields.io/github/actions/workflow/status/nassiharel/clim/ci.yml?style=flat-square" alt="CI"></a>
  <a href="https://github.com/nassiharel/clim/actions/workflows/codeql.yml"><img src="https://img.shields.io/github/actions/workflow/status/nassiharel/clim/codeql.yml?style=flat-square&label=CodeQL" alt="CodeQL"></a>
  <a href="https://goreportcard.com/report/github.com/nassiharel/clim"><img src="https://img.shields.io/badge/go%20report-A+-brightgreen?style=flat-square" alt="Go Report Card"></a>
  <a href="https://pkg.go.dev/github.com/nassiharel/clim"><img src="https://img.shields.io/badge/godoc-reference-blue?style=flat-square" alt="Go Reference"></a>
  <a href="https://github.com/nassiharel/clim/releases"><img src="https://img.shields.io/github/downloads/nassiharel/clim/total?style=flat-square" alt="Downloads"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/nassiharel/clim?style=flat-square" alt="License"></a>
</p>

---

## Table of Contents

- [The Problem](#the-problem)
- [The Solution](#the-solution)
- [Screenshots](#screenshots)
- [Install](#install)
- [Usage](#usage)
- [Tool Catalog](#tool-catalog)
- [Architecture](#architecture)
- [Configuration](#configuration)
- [Contributing](#contributing)

---

## The Problem

Modern developer environments rely on dozens of CLI tools -- `az`, `kubectl`, `docker`, `node`, `terraform`, and many more. Keeping track of them is harder than it should be:

- **No visibility** -- There is no standard way to list all installed CLI tools across a system. They come from brew, apt, npm, winget, manual installs, and more.
- **Version fragmentation** -- Every tool exposes its version differently (`--version`, `version`, `-v`, JSON output). There is no consistent way to aggregate this.
- **Unknown install locations** -- Understanding whether you're running `/usr/bin/python3` or `/usr/local/bin/python3` requires manual inspection.
- **No centralized upgrade awareness** -- `brew outdated`, `apt list --upgradable`, and `npm outdated` are all siloed. None give you a single cross-tool view.
- **Security & maintenance risk** -- Outdated CLI tools can contain vulnerabilities, break API compatibility, and cause inconsistent behavior across environments.

---

## The Solution

**clim** gives you a unified interface to manage your CLI tools:

| Capability | What clim does |
|---|---|
| **Discover** | Scans `$PATH` to detect 80+ popular developer CLI tools |
| **Inspect** | Shows installed version, binary location, and install source |
| **Compare** | Checks latest available versions via native package managers |
| **Upgrade** | Runs the right native package manager command (`brew`, `winget`, `apt`, `choco`, `snap`, `npm`) |
| **Export / Import** | Saves your toolchain to a YAML manifest and reinstalls it on a new machine |
| **Self-update** | Updates clim itself to the latest version from GitHub Releases |

All in a single command, with an interactive TUI or scriptable output.

---

## Screenshots

<p align="center">
  <img src="assets/tui-installed.png" alt="Installed tab" width="720">
</p>

> The TUI shows all detected tools with version status, install source, and upgrade availability. Navigate between tabs to discover new tools, manage updates, and export your toolchain.

---

## Features

### 🔍 Discover & Install Tools
Browse 110+ curated developer tools from one place. Filter by category, tag, or platform. Install anything with one keypress — clim picks the right package manager for your OS.

### 📦 Packs — Curated Tool Bundles
Install entire toolchains in one shot. Cloud Essentials, K8s Starter, Python Developer — pick a pack and go. See which packs you've already completed with visual progress gauges.

### 🎒 Create Your Own Pack
Hand-pick tools from the marketplace, give your pack a name, and save it. Share it with teammates or use it to set up your next machine.

### 📋 Team Manifests (`.clim.yaml`)
Drop a `.clim.yaml` in your repo to define required and optional tools with version constraints. `clim check` validates every developer's environment — in the terminal or CI. `clim init` scans your project files (Dockerfile, go.mod, package.json, CI workflows, Helm charts, Terraform, Bicep, and 30+ more) and generates one automatically. Manage multiple projects from the TUI's Project tab — add tools, install missing dependencies, and keep everyone in sync.

### 🔄 Keep Everything Up to Date
See which tools have updates at a glance. Batch-upgrade everything with one keystroke, or pick and choose. clim queries native package managers — no custom registries.

### 💾 Backup & Restore Your Toolchain
Export your installed tools to a portable manifest. Import it on a new machine — same tools, same setup, zero guesswork. Backups are saved automatically so you never lose your setup.

### 🔗 Share Your Toolchain
Generate a compact share token and paste it in Slack, Teams, or email. Recipients run `clim open <token>` to get your exact toolchain. No files to send.

### 🖥️ Move Between OSes
Installed everything on macOS? Export and import on your new Linux box. clim maps each tool to the best available package manager on the target OS — winget, brew, apt, choco, scoop, snap, or npm.

### 📊 Dashboard
See your entire dev environment at a glance: installed vs available, update status, package manager breakdown, category distribution, pack completion, and recent backups — all with visual gauges and charts.

### ⚡ Smart Recommendations
clim analyzes your installed tools and suggests related ones you might like, ranked by tag overlap. Discover tools you didn't know existed.

### ⚙️ Built-in Config Editor
Tune clim from inside the TUI — log level, refresh interval, concurrency, default tab, sidebar position. Toggle, cycle, type, save. No need to find and edit a YAML file.

### 🔒 Native Package Managers Only
clim never installs anything itself. It delegates to the package managers you already have — winget, brew, apt, choco, scoop, snap, npm. What you see is what your system runs.

---

## How It Compares

| Capability | Package Managers | Version Managers (asdf, mise) | Ad-hoc Scripts | **clim** |
|---|---|---|---|---|
| List all installed CLIs | Only their own packages | Only managed tools | Fragile | **All detected tools** |
| Show versions | Per-manager | Per-manager | Partial | **Unified view** |
| Show install paths | No | No | Manual | **Automatic** |
| Cross-manager support | No | Limited plugins | No | **Yes** |
| Upgrade suggestions | Per-manager | Per-manager | No | **Single view** |
| Export/Import toolchain | No | Partial | No | **YAML manifest** |
| Interactive UI | No | No | No | **Full TUI** |

---

## Install

### Quick Install

> **Note:** Always review scripts before piping to your shell. You can inspect
> [install.sh](https://github.com/nassiharel/clim/blob/main/install.sh) and
> [install.ps1](https://github.com/nassiharel/clim/blob/main/install.ps1) first.

**macOS / Linux:**

```bash
curl -fsSL https://raw.githubusercontent.com/nassiharel/clim/main/install.sh | bash
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/nassiharel/clim/main/install.ps1 | iex
```

### Homebrew (macOS/Linux)

```bash
brew install nassiharel/tap/clim
```

### Go install

```bash
go install github.com/nassiharel/clim/cmd/clim@latest
```

### Download binary

Download from [GitHub Releases](https://github.com/nassiharel/clim/releases/latest).

Binaries are available for **macOS**, **Linux**, and **Windows** on both `amd64` and `arm64`.

### Linux packages

```bash
# Debian/Ubuntu
sudo dpkg -i clim_<version>_linux_amd64.deb

# RPM-based
sudo rpm -i clim_<version>_linux_amd64.rpm
```

### Build from source

```bash
git clone https://github.com/nassiharel/clim.git
cd clim
make build
./bin/clim
```

---

## Usage

### Interactive TUI

```bash
clim
```

Launches a full-screen interactive interface with 6 tabs. Tools are detected and version-checked concurrently -- results stream in as they arrive.

### Non-interactive commands

```bash
# Show help
clim --help

# List all tools in a table
clim list

# Export installed tools to a YAML manifest
clim export > my-tools.yaml

# Import and install tools from a manifest
clim import my-tools.yaml
clim import my-tools.yaml --yes    # non-interactive
```

```bash
# Update clim itself to the latest version
clim update
clim update --check                # check only, don't install

# Manage the tool catalog
clim tools path                    # show local catalog cache location

# Manage configuration
clim config path                   # show config.yaml location
clim config edit                   # open config in $EDITOR

# Show clim version
clim version
```

---

## Tool Catalog

clim ships with a curated catalog of **80+ developer tools** defined in `marketplace/tools/`. The catalog is assembled by CI and published to the `marketplace` branch. The CLI fetches it on first run and caches locally. New tools from upstream appear automatically after a refresh.

---

## Architecture

**Version sources** -- versions are queried from native package managers, not HTTP APIs:

| Package Manager | Platforms | Used for |
|-----------------|-----------|----------|
| winget | Windows | Installed + latest versions |
| Chocolatey | Windows | Installed + latest versions |
| Homebrew | macOS, Linux | Installed + latest versions |
| apt / dpkg | Debian/Ubuntu | Installed + latest versions |
| snap | Linux | Installed + latest versions |
| npm | All | Installed + latest versions |

**Self-update**: `clim update` queries the GitHub Releases API for the latest `nassiharel/clim` release, downloads the correct platform archive, and replaces the binary in-place using a rename-swap strategy.

---

## Use Cases

### Developers
- Get a quick snapshot of your local environment
- Know which tools need updating before starting a project

### DevOps / Platform Teams
- Standardize developer environments across a team
- Export a toolchain manifest and import it on new machines

### Security Teams
- Detect outdated CLI tools that may contain vulnerabilities
- Verify tools are running from expected paths

### CI/CD Pipelines
- Validate tool versions before execution
- Fail fast if a required tool is missing or outdated

---

## Configuration

### Tool Catalog

The user's tool catalog (customizations, enabled/disabled state, custom tools) is stored at:

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/clim/marketplace-cache.yaml` |
| Linux | `~/.config/clim/marketplace-cache.yaml` |
| Windows | `%AppData%\clim\marketplace-cache.yaml` |

The catalog is fetched from GitHub and cached locally for offline use.

Edit the user catalog with `clim tools edit` or directly. User customizations (enabled/disabled state, custom tools) are preserved across updates.

### Config File

clim's configuration file (`config.yaml`) lives in the same directory as the tool catalog. It controls GitHub source, marketplace refresh behavior, concurrency, timeouts, and UI preferences. All values are optional with sensible defaults.

```bash
clim config path   # show config.yaml location
clim config edit   # open config.yaml in $EDITOR
```

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `clim: command not found` | Ensure install directory is in `$PATH`. Run `which clim` (macOS/Linux) or `where clim` (Windows) to check. |
| Tool not detected | Verify binary is in `$PATH` with `which <tool>` / `where <tool>`. Run `clim` then press `r` to refresh. |
| Permission denied on upgrade | Package manager may need elevated privileges. Use `sudo` (Linux/macOS) or run as Administrator (Windows). |
| Stale version info | Delete local cache (`clim tools path` shows location) and relaunch to re-fetch from GitHub. |
| Self-update fails | Download manually from [Releases](https://github.com/nassiharel/clim/releases/latest) and replace binary. |

---

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](./CONTRIBUTING.md) before submitting a pull request.

See [AGENTS.md](./AGENTS.md) for detailed architecture documentation.

## Roadmap

- SBOM export for installed CLI tools
- CVE / vulnerability scanning integration
- Background update-available notifications
- Team policy enforcement (require minimum versions)
- Add more package managers (pip, gem, cargo, asdf, etc)

## License

MIT
