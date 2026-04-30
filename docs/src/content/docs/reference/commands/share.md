---
title: "clim share"
description: Generate a share token of your installed tools
---

Generate a compact token that encodes your installed tools. Recipients can install the same tools with `clim open`.

## Usage

```bash
clim share
```

## Output

```
clim:v1:H4sIAAAA...
```

The token is a base64-encoded, gzip-compressed representation of your tool list. It's designed to be compact enough to share via Slack, Teams, email, or any messaging platform.

## How It Works

1. Scans for all installed tools (or uses cached results)
2. Encodes the tool list into a compact binary format
3. Compresses with gzip
4. Outputs as a `clim:v1:` prefixed base64 string

## TUI Alternative

In the TUI, switch to the **★ Favorites** tab and press `s` to share just your favorited tools.

## See Also

- [`clim open`](/reference/commands/open) — Install tools from a share token
- [`clim export`](/reference/commands/export) — Export to a YAML file (more detailed)
