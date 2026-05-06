---
title: "klim shell completion"
description: Generate shell completion scripts
---

Generate native tab completion scripts for your shell.

## Usage

```bash
klim shell completion <bash|zsh|fish|powershell>
```

## Supported Shells

| Shell | Setup |
|-------|-------|
| bash | `source <(klim shell completion bash)` |
| zsh | `source <(klim shell completion zsh)` |
| fish | `klim shell completion fish \| source` |
| powershell | `klim shell completion powershell \| Out-String \| Invoke-Expression` |

## Persistent Setup

```bash
# bash — add to ~/.bashrc
echo 'source <(klim shell completion bash)' >> ~/.bashrc

# zsh — add to ~/.zshrc
echo 'source <(klim shell completion zsh)' >> ~/.zshrc

# fish — save to completions directory
klim shell completion fish > ~/.config/fish/completions/klim.fish

# powershell — add to $PROFILE
Add-Content $PROFILE 'klim shell completion powershell | Out-String | Invoke-Expression'
```

## See Also

- [klim hook](./hook.md) — Shell hooks for auto-checking .klim.yaml
- [Shell Integration guide](../../guides/shell-integration.mdx)
