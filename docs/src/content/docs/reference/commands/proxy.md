---
title: "klim proxy"
description: Manage auto-install shims for CLI tools
---

Create lightweight shims that auto-install tools on first use. When you run a shimmed tool that isn't installed, klim automatically installs it via the best available package manager, then runs it.

## Subcommands

| Command | Description |
|---------|-------------|
| `klim proxy setup` | Create the shims directory and show PATH instructions |
| `klim proxy add <tool> [tool...]` | Create shims for one or more tools |
| `klim proxy remove <tool> [tool...]` | Remove shims |
| `klim proxy list` | List active shims |

## Setup

```bash
# Create the shims directory
klim proxy setup

# Add shims directory to your PATH (shown by setup)
export PATH="$HOME/.config/klim/shims:$PATH"

# Create shims for tools
klim proxy add kubectl terraform helm
```

## How It Works

1. `klim proxy add kubectl` creates a lightweight shim script in `~/.config/klim/shims/`
2. When you run `kubectl`, the shim checks if the real `kubectl` is installed elsewhere in PATH
3. If found → runs it directly
4. If not found → installs via the best available package manager, then runs it

## Examples

```bash
# Set up and create shims
klim proxy setup
klim proxy add kubectl terraform helm jq

# List active shims
klim proxy list

# Remove a shim
klim proxy remove kubectl
```

## See Also

- [klim try](./try.md) — Try a tool temporarily
