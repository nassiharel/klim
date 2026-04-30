---
title: "clim update"
description: Update clim to the latest version
---

Check GitHub Releases for a newer version of clim and download/install it in-place.

## Usage

```bash
clim update [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--check` | Check for updates without installing |

## Examples

```bash
# Download and install the latest version
clim update

# Check only — don't install
clim update --check
```

## How It Works

1. Queries the GitHub Releases API for the latest version
2. Compares against the currently running version
3. If newer, downloads the appropriate binary for your OS/architecture
4. Replaces the current binary in-place

### Windows Note

On Windows, the running executable cannot be deleted. clim renames the current binary to `.old` and places the new binary at the original path. The `.old` file is cleaned up on the next launch.

## Alternative

If you installed clim via Homebrew:

```bash
brew upgrade clim
```
