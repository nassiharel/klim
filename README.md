<p align="center">
  <em>One command. Every machine. Every package manager.<br>
  klim installs a whole toolchain on macOS, Linux, or Windows — picking the right native
  package manager for you, so the same command just works everywhere.
  </em>
</p>


## Install

```bash
# Homebrew cask is the supported install path to avoid the legacy formula collision.
brew install --cask nassiharel/tap/klim            # macOS / Linux
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
brew install --cask --version 0.1.7 nassiharel/tap/klim
# Or install via Go directly:
go install github.com/nassiharel/klim/cmd/klim@v0.1.7
```

see [Releases](https://github.com/nassiharel/klim/releases/latest).
</details>

## Quick start
