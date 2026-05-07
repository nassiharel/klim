---
title: "klim config"
description: Manage klim configuration
---

View and edit the klim configuration file.

## Usage

```bash
klim config <subcommand>
```

## Subcommands

### config path

Print the path to `config.yaml`:

```bash
klim config path
# Output: /home/user/.klim/config/config.yaml
```

### config edit

Open `config.yaml` in your default editor (`$EDITOR` / `%EDITOR%`):

```bash
klim config edit
```

## TUI Alternative

The **Config** tab (press `8`) provides an in-TUI editor for all settings. Navigate with `↑`/`↓`, press `Enter` to edit a value, and `S` to save.

## See Also

- [Configuration Reference](../configuration.md) — All config.yaml options
