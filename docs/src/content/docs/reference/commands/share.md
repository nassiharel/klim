---
title: "klim share"
description: Share your toolchain — generate and install from tokens
---

Share your installed tools as a compact token, or install tools from a token shared by a teammate.

## Usage

```bash
klim share                    # generate a share token
klim share open <token>       # install from a share token
klim share open <token> --yes # non-interactive install
```

## Generate a Token

```bash
klim share
```

Outputs a compact `klim:v1:...` token that encodes your installed tool names. Share it via Slack, Teams, email, or any chat.

For scripting, request structured output:

```bash
klim share --output json
# {
#   "token": "klim:v1:...",
#   "tool_count": 24,
#   "tools": ["az", "gh", "git", ...]
# }
```

The `tools` array is sorted, so the same installed set always produces the same token. When no tools are installed, `token` is omitted from the JSON payload.

## Install from a Token

```bash
klim share open "klim:v1:H4sIAAAA..."
```

Decodes the token, resolves tools from your local catalog, and installs via native package managers.

## How It Works

1. Scans for all installed tools
2. Encodes tool names into gzip-compressed, base64-encoded token
3. Recipients decode and install via their local catalog + package managers

## TUI Alternative

In the TUI, switch to the **★ Favorites** tab and press `s` to share just your favorited tools.

## See Also

- [`klim export`](/reference/commands/export) — Export to a YAML file (more detailed)
- [`klim diff`](/reference/commands/diff) — Compare against a token or manifest
