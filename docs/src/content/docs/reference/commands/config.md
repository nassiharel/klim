---
title: "clim config"
description: Manage clim configuration
---

View and edit the clim configuration file.

## Usage

```bash
clim config <subcommand>
```

## Subcommands

### config path

Print the path to `config.yaml`:

```bash
clim config path
# Output: /home/user/.config/clim/config/config.yaml
```

### config edit

Open `config.yaml` in your default editor (`$EDITOR` / `%EDITOR%`):

```bash
clim config edit
```

## TUI Alternative

The **Config** tab (press `8`) provides an in-TUI editor for all settings. Navigate with `↑`/`↓`, press `Enter` to edit a value, and `S` to save.

## See Also

- [Configuration Reference](/reference/configuration) — All config.yaml options
