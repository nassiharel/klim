---
title: "klim update"
description: Update klim to the latest version
---

Check GitHub Releases for a newer version of klim and download/install it in-place.

## Usage

```bash
klim update [flags]
```

## Flags

| Flag | Description |
|------|-------------|
| `--check` | Check for updates without installing |

## Examples

```bash
# Download and install the latest version
klim update

# Check only — don't install
klim update --check
```

## How It Works

1. Queries the GitHub Releases API for the latest version
2. Compares against the currently running version
3. If newer, downloads the appropriate binary for your OS/architecture
4. Replaces the current binary in-place

### Windows Note

On Windows, the running executable cannot be deleted. klim renames the current binary to `.old` and places the new binary at the original path. The `.old` file is cleaned up on the next launch.

## Alternative

If you installed klim via Homebrew:

```bash
brew upgrade klim
```
