---
title: "clim share"
description: Share your toolchain — generate and install from tokens
---

Share your installed tools as a compact token, or install tools from a token shared by a teammate.

## Usage

```bash
clim share                    # generate a share token
clim share open <token>       # install from a share token
clim share open <token> --yes # non-interactive install
```

## Generate a Token

```bash
clim share
```

Outputs a compact `clim:v1:...` token that encodes your installed tool names. Share it via Slack, Teams, email, or any chat.

## Install from a Token

```bash
clim share open "clim:v1:H4sIAAAA..."
```

Decodes the token, resolves tools from your local catalog, and installs via native package managers.

## How It Works

1. Scans for all installed tools
2. Encodes tool names into gzip-compressed, base64-encoded token
3. Recipients decode and install via their local catalog + package managers

## TUI Alternative

In the TUI, switch to the **★ Favorites** tab and press `s` to share just your favorited tools.

## See Also

- [`clim export`](/reference/commands/export) — Export to a YAML file (more detailed)
- [`clim diff`](/reference/commands/diff) — Compare against a token or manifest
