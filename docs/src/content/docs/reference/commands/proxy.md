---
title: "clim proxy"
description: Manage auto-install shims for CLI tools
---

Create lightweight shims that auto-install tools on first use. When you run a shimmed tool that isn't installed, clim automatically installs it via the best available package manager, then runs it.

## Subcommands

| Command | Description |
|---------|-------------|
| `clim proxy setup` | Create the shims directory and show PATH instructions |
| `clim proxy add <tool> [tool...]` | Create shims for one or more tools |
| `clim proxy remove <tool> [tool...]` | Remove shims |
| `clim proxy list` | List active shims |

## Setup

```bash
# Create the shims directory
clim proxy setup

# Add shims directory to your PATH (shown by setup)
export PATH="$HOME/.config/clim/shims:$PATH"

# Create shims for tools
clim proxy add kubectl terraform helm
```

## How It Works

1. `clim proxy add kubectl` creates a lightweight shim script in `~/.config/clim/shims/`
2. When you run `kubectl`, the shim checks if the real `kubectl` is installed elsewhere in PATH
3. If found → runs it directly
4. If not found → installs via the best available package manager, then runs it

## Examples

```bash
# Set up and create shims
clim proxy setup
clim proxy add kubectl terraform helm jq

# List active shims
clim proxy list

# Remove a shim
clim proxy remove kubectl
```

## See Also

- [clim try](/reference/commands/try) — Try a tool temporarily
