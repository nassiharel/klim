---
title: "clim shell completion"
description: Generate shell completion scripts
---

Generate native tab completion scripts for your shell.

## Usage

```bash
clim shell completion <bash|zsh|fish|powershell>
```

## Supported Shells

| Shell | Setup |
|-------|-------|
| bash | `source <(clim shell completion bash)` |
| zsh | `source <(clim shell completion zsh)` |
| fish | `clim shell completion fish \| source` |
| powershell | `clim shell completion powershell \| Out-String \| Invoke-Expression` |

## Persistent Setup

```bash
# bash — add to ~/.bashrc
echo 'source <(clim shell completion bash)' >> ~/.bashrc

# zsh — add to ~/.zshrc
echo 'source <(clim shell completion zsh)' >> ~/.zshrc

# fish — save to completions directory
clim shell completion fish > ~/.config/fish/completions/clim.fish

# powershell — add to $PROFILE
Add-Content $PROFILE 'clim shell completion powershell | Out-String | Invoke-Expression'
```

## See Also

- [clim hook](/reference/commands/hook) — Shell hooks for auto-checking .clim.yaml
- [Shell Integration guide](/guides/shell-integration)
