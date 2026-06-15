<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/klim-wordmark-inter-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="assets/klim-wordmark-inter-light.svg">
    <img src="assets/klim-wordmark-inter-light.svg" alt="klim" width="340">
  </picture>
</p>

<h3 align="center">Ultimate dev tools manager</h3>

<p align="center">
  <a href="https://github.com/nassiharel/klim/releases/latest"><img src="https://img.shields.io/github/v/release/nassiharel/klim?style=flat-square&color=brightgreen&label=release" alt="Release"></a>
  <a href="https://github.com/nassiharel/klim/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/nassiharel/klim/ci.yml?style=flat-square&label=build" alt="CI"></a>
  <a href="https://github.com/nassiharel/klim/actions/workflows/codeql.yml"><img src="https://img.shields.io/github/actions/workflow/status/nassiharel/klim/codeql.yml?style=flat-square&label=CodeQL" alt="CodeQL"></a>
<a href="LICENSE"><img src="https://img.shields.io/github/license/nassiharel/klim?style=flat-square" alt="License"></a>
</p>

<p align="center">
  <a href="https://goreportcard.com/report/github.com/nassiharel/klim"><img src="https://img.shields.io/badge/go%20report-A+-brightgreen?style=flat-square" alt="Go Report Card"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/nassiharel/klim?style=flat-square&label=go&logo=go&logoColor=white" alt="Go version"></a>
  <a href="https://pkg.go.dev/github.com/nassiharel/klim"><img src="https://img.shields.io/badge/godoc-reference-blue?style=flat-square" alt="Go Reference"></a>
  <img src="https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey?style=flat-square" alt="Platforms">
</p>

https://github.com/user-attachments/assets/54969cc1-47b7-47b7-af35-06d0649da466

<p align="center">
  <em>Single command. Every machine. Every package manager.<br>
  klim is a cross-platform layer over the native package managers you already trust
  </em>
</p>


## Install

```bash
brew install nassiharel/tap/klim                       # macOS / Linux
winget install nassiharel.klim                         # Windows
scoop install klim                                     # Windows (Scoop)
go install github.com/nassiharel/klim/cmd/klim@latest  # any Go 1.25+
```

<details>
<summary>More install options</summary>

```bash
# Script
curl -fsSL https://raw.githubusercontent.com/nassiharel/klim/main/install.sh | bash   # macOS / Linux
irm https://raw.githubusercontent.com/nassiharel/klim/main/install.ps1 | iex          # Windows

# Linux packages
sudo dpkg -i klim_<version>_linux_<arch>.deb    # Debian / Ubuntu
sudo rpm -i  klim_<version>_linux_<arch>.rpm    # Fedora / CentOS / RHEL

# Pin a version
brew install nassiharel/tap/klim@0.1.2
go install github.com/nassiharel/klim/cmd/klim@v0.1.2
```

see [Releases](https://github.com/nassiharel/klim/releases/latest).
</details>

## Quick start

Five commands from zero to a reproducible environment:

```bash
klim                                  # interactive TUI
klim check --output json              # validate this project's .klim.yaml
klim install --pack go-developer      # install a curated bundle
klim diff teammate.yaml               # compare environments
```

## What it does

### 📦 One command. Every OS.
`klim install --pack go-developer` and your whole toolchain — **162 tools, 24 curated packs** — lands on macOS, Linux, and Windows. klim picks the right native manager per platform, so the same pack ID just works everywhere.

### 🧬 Standardize with one YAML
`klim init` reads your `package.json`, `go.mod`, `Dockerfile`, CI workflows, Helm, Terraform, then writes a `.klim.yaml` that pins the toolchain. `klim check` validates it locally and in CI.

### 🤖 Manage your agents
`klim agents` unifies **Claude Code** and **GitHub Copilot CLI** into one searchable inventory of plugins, skills, MCP servers, marketplaces, and live sessions.

## Learn more

[Docs](https://nassiharel.github.io/klim-web/docs) · [Website](https://nassiharel.github.io/klim-web) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Releases](https://github.com/nassiharel/klim/releases)

---

MIT © Nassi Harel — <em>made by devs tired of typing <code>command not found</code>.</em>
