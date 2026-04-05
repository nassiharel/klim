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
  <a href="LICENSE"><img src="https://img.shields.io/github/license/nassiharel/clim?style=flat-square" alt="License"></a>
</p>

```
     ___  _     ___  __  __
    / __|| |   |_ _||  \/  |
   | (__ | |__  | | | |\/| |
    \___||____||___||_|  |_|

    detect . check . upgrade
```

```
$ clim
TOOL                 VERSION   LATEST    PATH                                       STATUS
Azure CLI            2.83.0    2.84.0    /usr/local/bin/az                          upgrade available
Azure Dev CLI        1.23.13   1.23.13   /usr/local/bin/azd                         up to date
GitHub CLI           2.88.1    2.89.0    /usr/bin/gh                                upgrade available
kubectl              1.33.3    1.35.3    /usr/local/bin/kubectl                     upgrade available
Docker               29.3.1    29.3.1    /usr/bin/docker                            up to date
Go                   1.23.4    1.26.1    /usr/local/go/bin/go                       upgrade available
Node.js              24.13.1   25.9.0    /usr/local/bin/node                        upgrade available
Python               3.13.12   3.14.3    /usr/bin/python3                           upgrade available
Git                  2.53.0    2.53.0    /usr/bin/git                               up to date
```

---

## The Problem

Modern developer environments rely on dozens of CLI tools -- `az`, `kubectl`, `docker`, `node`, `terraform`, and many more. Keeping track of them is harder than it should be:

- **No visibility** -- There is no standard way to list all installed CLI tools across a system. They come from brew, apt, npm, pip, manual installs, and more.
- **Version fragmentation** -- Every tool exposes its version differently (`--version`, `version`, `-v`, JSON output). There is no consistent way to aggregate this.
- **Unknown install locations** -- Understanding whether you're running `/usr/bin/python3` or `/usr/local/bin/python3` requires manual inspection.
- **No centralized upgrade awareness** -- `brew outdated`, `apt list --upgradable`, and `npm outdated` are all siloed. None give you a single cross-tool view.
- **Security & maintenance risk** -- Outdated CLI tools can contain vulnerabilities, break API compatibility, and cause inconsistent behavior across environments.

---

## The Solution

**clim** gives you a unified interface to manage your CLI tools:

| Capability | What clim does |
|---|---|
| **Discover** | Scans `$PATH` to detect 12 popular developer CLI tools |
| **Inspect** | Shows installed version, binary location, and install source |
| **Compare** | Checks latest available versions from GitHub, PyPI, npm, and more |
| **Upgrade** | Runs the right native package manager command (`brew`, `winget`, `apt`) |

All in a single command, with an interactive TUI or scriptable output.

---

## Features

- Detects 12 popular developer CLI tools automatically
- Shows installed version, latest available version, and install path side by side
- Interactive TUI with keyboard navigation, filtering, and live refresh
- Non-interactive mode for scripting and CI pipelines
- Cross-platform: Windows, macOS, Linux
- Checks latest versions from GitHub Releases, PyPI, npm, go.dev, nodejs.org, and endoflife.date
- Caches version lookups for 1 hour -- fast repeated runs with no API spam
- Upgrade tools via their native package managers (brew, winget, apt, choco)
- Concurrent detection and version checking -- all tools are scanned in parallel

---

## How It Compares

| Capability | Package Managers | Version Managers (asdf, mise) | Ad-hoc Scripts | **clim** |
|---|---|---|---|---|
| List all installed CLIs | Only their own packages | Only managed tools | Fragile | **All detected tools** |
| Show versions | Per-manager | Per-manager | Partial | **Unified view** |
| Show install paths | No | No | Manual | **Automatic** |
| Cross-manager support | No | Limited plugins | No | **Yes** |
| Upgrade suggestions | Per-manager | Per-manager | No | **Single view** |
| Interactive UI | No | No | No | **Full TUI** |

---

## Install

### Go install

```bash
go install github.com/nassiharel/clim@latest
```

### Download binary

Download from [GitHub Releases](https://github.com/nassiharel/clim/releases/latest).

Binaries are available for **macOS**, **Linux**, and **Windows** on both `amd64` and `arm64`.

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

Launches a full-screen interactive interface. Tools are detected and version-checked concurrently -- results stream in as they arrive.

| Key | Action |
|-----|--------|
| `j/k` or arrows | Navigate up/down |
| `/` | Filter tools by name |
| `r` | Refresh all tools |
| `q` or `Ctrl+C` | Quit |

### Non-interactive commands

```bash
# List all tools in a table
clim list

# Check a specific tool
clim check go
clim check az

# Upgrade a tool via its native package manager
clim upgrade gh
clim upgrade az

# Show clim version
clim version
```

### JSON output (for scripting)

```bash
clim list --json | jq '.[] | select(.status == "upgradable")'
```

---

## Supported Tools

| Tool | Detection Command | Latest Version Source |
|------|-------------------|---------------------|
| Azure CLI (`az`) | `az version` | PyPI |
| Azure Dev CLI (`azd`) | `azd version` | GitHub Releases |
| GitHub CLI (`gh`) | `gh --version` | GitHub Releases |
| GitHub Copilot CLI | `github-copilot-cli --version` | npm Registry |
| kubectl | `kubectl version --client` | GitHub Releases |
| Docker | `docker --version` | GitHub Releases |
| Terraform | `terraform version` | GitHub Releases |
| Helm | `helm version --short` | GitHub Releases |
| Go | `go version` | go.dev |
| Node.js | `node --version` | nodejs.org |
| Python | `python3 --version` | endoflife.date |
| Git | `git --version` | GitHub Releases |

---

## Architecture

clim is built for speed. Every tool is detected and version-checked **concurrently** using goroutines:

```
                    ┌──[parallel]──> Detect all tools (exec.LookPath + run binary)
DefaultTools() ────┤
                    └──[parallel]──> Check latest versions (HTTP APIs, cache-first)
```

**Version sources** are queried through a pluggable `Checker` interface:

| Source | Used by | API |
|--------|---------|-----|
| GitHub Releases | gh, kubectl, docker, terraform, helm, git, azd | `api.github.com/repos/:owner/:repo/releases/latest` |
| PyPI | az | `pypi.org/pypi/:package/json` |
| npm | copilot | `registry.npmjs.org/:package` |
| Custom | go, node, python | go.dev, nodejs.org, endoflife.date |

**Cache**: Latest version results are cached to `cache.json` with a 1-hour TTL. The cache is thread-safe and loaded at startup. This means repeated runs within an hour make zero network requests.

---

## Use Cases

### Developers
- Get a quick snapshot of your local environment
- Know which tools need updating before starting a project

### DevOps / Platform Teams
- Standardize developer environments across a team
- Audit toolchain versions in CI

### Security Teams
- Detect outdated CLI tools that may contain vulnerabilities
- Verify tools are running from expected paths

### CI/CD Pipelines
- Validate tool versions before execution
- Fail fast if a required tool is missing or outdated

---

## Configuration

### GitHub Token

Set `GITHUB_TOKEN` or `GH_TOKEN` to raise the GitHub API rate limit from 60 to 5,000 requests/hour:

```bash
export GITHUB_TOKEN=ghp_your_token_here
```

### Cache

Version lookups are cached for 1 hour at:

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/clim/cache.json` |
| Linux | `~/.config/clim/cache.json` |
| Windows | `%AppData%\clim\cache.json` |

---

## Future Enhancements

- SBOM export for installed CLI tools
- CVE / vulnerability scanning integration
- Auto-upgrade workflows
- Team policy enforcement (require minimum versions)
- Custom tool definitions via config file

---

## Contributing

Contributions are welcome! Please open issues or submit pull requests on [GitHub](https://github.com/nassiharel/clim).

See [AGENTS.md](./AGENTS.md) for detailed architecture, coding conventions, and contribution guidelines.

## License

MIT
