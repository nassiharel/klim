# clim

Interactive CLI manager — detect, check, and upgrade your developer tools.

```
$ clim list
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

## Features

- Detects 12 popular developer CLI tools
- Shows installed version, latest available version, and install path
- Interactive TUI with keyboard navigation and filtering
- Cross-platform: Windows, macOS, Linux
- Checks latest versions from GitHub Releases, PyPI, npm, and more
- Caches version lookups for fast repeated runs
- Upgrade tools via native package managers (brew, winget, apt)

## Install

### Go install

```bash
go install github.com/nassiharel/clim@latest
```

### Download binary

Download from [GitHub Releases](https://github.com/nassiharel/clim/releases/latest).

### Build from source

```bash
git clone https://github.com/nassiharel/clim.git
cd clim
make build
./bin/clim
```

## Usage

### Interactive TUI

```bash
clim
```

Launches the interactive interface with keyboard navigation:

| Key | Action |
|-----|--------|
| `j/k` or arrows | Navigate up/down |
| `/` | Filter tools |
| `r` | Refresh all |
| `q` | Quit |

### Non-interactive commands

```bash
# List all tools in a table
clim list

# Check a specific tool
clim check go
clim check az

# Upgrade a tool
clim upgrade gh
clim upgrade az
```

### JSON output (for scripting)

```bash
clim list --json | jq '.[] | select(.status == "upgradable")'
```

## Supported Tools

| Tool | Detection | Latest Version Source |
|------|-----------|---------------------|
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

## Configuration

### GitHub Token

Set `GITHUB_TOKEN` or `GH_TOKEN` to raise the GitHub API rate limit from 60 to 5000 requests/hour:

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

## License

MIT
