---
title: "clim completion"
description: Generate shell completion scripts
---

Generate native tab completion scripts for your shell.

## Usage

```bash
clim completion <bash|zsh|fish|powershell>
```

## Supported Shells

| Shell | Setup |
|-------|-------|
| bash | `source <(clim completion bash)` |
| zsh | `source <(clim completion zsh)` |
| fish | `clim completion fish \| source` |
| powershell | `clim completion powershell \| Out-String \| Invoke-Expression` |

## Persistent Setup

```bash
# bash — add to ~/.bashrc
echo 'source <(clim completion bash)' >> ~/.bashrc

# zsh — add to ~/.zshrc
echo 'source <(clim completion zsh)' >> ~/.zshrc

# fish — save to completions directory
clim completion fish > ~/.config/fish/completions/clim.fish

# powershell — add to $PROFILE
Add-Content $PROFILE 'clim completion powershell | Out-String | Invoke-Expression'
```

## See Also

- [clim hook](/reference/commands/hook) — Shell hooks for auto-checking .clim.yaml
- [Shell Integration guide](/guides/shell-integration)
