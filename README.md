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

- Detects **80+ developer CLI tools** from a curated, extensible catalog (`marketplace/`)
- Shows installed version, latest available version, install source, and binary path
- Interactive full-screen TUI with 6 tabs: Installed, Updates, Discover, Disabled, Backup, Config
- Detail view with rich metadata (description, publisher, license, homepage)
- Keyboard-driven upgrade, install, and remove via native package managers
- Non-interactive mode for scripting (`clim list`)
- **Export/Import** -- save your toolchain to YAML and replicate it on another machine
- **Self-update** -- `clim update` downloads and installs the latest release
- Cross-platform: Windows, macOS, Linux
- Version detection via local package managers (winget, brew, apt, choco, snap, npm)
- Fallback binary detection via Go build info and PE version resources
- Concurrent scanning -- all tools are detected and resolved in parallel

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

| Key | Action |
|-----|--------|
| `j/k` or `↑/↓` | Navigate up/down |
| `←/→` or `Tab` | Switch tabs |
| `1`-`6` | Jump to tab directly |
| `/` | Filter tools by name |
| `Enter` | Open detail view / confirm action |
| `r` | Refresh all tools |
| `e/d` | Enable / disable a tool |
| `q` or `Ctrl+C` | Quit |

**TUI Tabs:**

| Tab | Content |
|-----|---------|
| Installed | All installed tools with versions and status |
| Updates | Tools with available updates |
| Discover | Tools not yet installed that could be added |
| Disabled | Tools you've hidden from the main view |
| Backup | Export your toolchain or import from a manifest |
| Config | Version info, paths, and package manager status |

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

<details>
<summary>Example export output</summary>

```yaml
generated_by: clim v0.x.x
os: windows
arch: amd64
tools:
  - name: git
    display_name: Git
    version: "2.44.0"
    source: winget
    category: version-control
    packages:
      winget: Git.Git
      brew: git
      apt: git
  - name: kubectl
    display_name: Kubectl
    version: "1.29.3"
    source: brew
    category: cloud
    packages:
      brew: kubernetes-cli
      winget: Kubernetes.kubectl
```

</details>

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

Tools include: `az`, `azd`, `gh`, `copilot`, `kubectl`, `docker`, `terraform`, `helm`, `go`, `node`, `python`, `git`, `jq`, `yq`, `ripgrep`, `fzf`, `bat`, `exa`, `fd`, `delta`, `zoxide`, `starship`, `tmux`, `neovim`, `curl`, `wget`, `make`, `cmake`, `rust/cargo`, `ruby`, `java`, `dotnet`, `aws`, `gcloud`, `pulumi`, `vault`, `consul`, `packer`, and many more.

To add custom tools, edit the user catalog:

```bash
clim tools edit
```

---

## Architecture

clim is built for speed. The tool catalog is fetched from GitHub and cached locally, PATH is scanned concurrently, and version queries go through native package managers in parallel.

```
marketplace/ (GitHub) ──► catalog.LoadOrFetch() ──► cache locally
                                      │
                         registry.ToolsFromBytes()
                                      │
                                      ├──[parallel]──► finder.FindAll()           (exec.LookPath across PATH)
                                      │
                                      ├──[parallel]──► pkgmgr.ResolveVersions()  (winget/brew/apt/choco/snap/npm)
                                      │
                                      └──[fallback]──► detector.EnrichFallback() (Go buildinfo / PE version)
```

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
