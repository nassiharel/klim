---
title: "clim hook"
description: Generate shell hook for automatic .clim.yaml checking
---

Generate a shell hook that automatically runs `clim check` when you `cd` into a directory with a `.clim.yaml` file — like nvm or direnv for your toolchain.

## Usage

```bash
clim hook <bash|zsh|fish|powershell>
```

## How It Works

1. You add the hook to your shell startup file
2. Every time you `cd` into a directory, the hook walks up the directory tree
3. If it finds a `.clim.yaml`, it runs `clim check` silently
4. Only prints output when tools are missing or outdated

## Setup

```bash
# bash — add to ~/.bashrc
eval "$(clim hook bash)"

# zsh — add to ~/.zshrc
eval "$(clim hook zsh)"

# fish — save to conf.d
clim hook fish > ~/.config/fish/conf.d/clim-hook.fish

# powershell — add to $PROFILE
clim hook powershell | Out-String | Invoke-Expression
```

## Example Output

When you `cd` into a project with missing tools:

```
    ✗ kubectl              —            (>=1.28)
    ⚠ terraform            1.5.0        (>=1.7)
  Run 'clim check' for details or 'clim import' to install missing tools.
```

## See Also

- [clim completion](/reference/commands/completion) — Shell tab completions
- [clim check](/reference/commands/check) — Manual project validation
- [Shell Integration guide](/guides/shell-integration)
