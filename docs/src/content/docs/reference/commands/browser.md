---
title: "clim browser"
description: Local web UI for clim — Installed, Tool detail, Dashboard, and Trail in your browser
---

`clim browser` launches a small local HTTP server and opens the clim UI
in your default browser. The web view is a thin frontend over the same
service layer the TUI and other CLI commands use; every page renders
from a real PATH scan and version resolution, no separate data store.

## Usage

```bash
clim browser [flags]
```

By default the server picks a free port, binds to `127.0.0.1`, and
opens the resulting URL in your default browser. The URL is also
printed to stderr so you can copy-paste it manually if auto-open fails.

```
$ clim browser
clim browser listening on http://127.0.0.1:54321
  press Ctrl-C to stop
```

## Flags

| Flag | Description |
|------|-------------|
| `--port` | Listen port (`0` lets the kernel pick a free one). Default `0`. |
| `--bind` | Bind address. Default `127.0.0.1`. |
| `--no-open` | Do not auto-open the browser. |
| `--insecure-bind` | Allow non-loopback bind addresses. The server has **no authentication** — use with caution. |

`--bind` defaults to `127.0.0.1` and refuses any non-loopback address
unless `--insecure-bind` is also passed, so you can't accidentally
expose an unauthenticated server on a LAN.

## Pages

| Path | Renders |
|------|---------|
| `/` | Installed tools, with category and source filters. |
| `/tools/<name>` | Per-tool detail (CLI-info equivalent). |
| `/dashboard` | Aggregate stats: counts, top categories, sample of pending updates. |
| `/trail` | Trail entry list. |
| `/trail/<ref>` | Snapshot at the given trail ref. |
| `/healthz` | Liveness probe (`200 ok`). |

The Updates, Discover, Backup, Favorites, and Config tabs render
"Coming soon" placeholders in this release; the same data is available
in the TUI today.

## JSON API

A JSON counterpart to every page is exposed under `/api/*`. The shapes
mirror the existing CLI `--output json` payloads so existing scripts
read both indistinguishably.

| Path | Returns |
|------|---------|
| `/api/tools` | All resolved tools + catalog summary. |
| `/api/tools/<name>` | One resolved tool, including GitHub metadata. |
| `/api/dashboard` | Stats payload used by `/dashboard`. |
| `/api/trail` | Trail entries (newest first). |
| `/api/trail/<ref>` | `{ "entry": ..., "snapshot": ... }`. |

## Examples

```bash
# Run on a fixed port without opening the browser (CI / headless).
clim browser --port 7777 --no-open

# Probe the API while the server is running.
curl -s http://127.0.0.1:7777/api/dashboard | jq .updates_available
```

## Security

- Loopback-only by default. `--insecure-bind` is required for any
  other interface and prints a warning at startup.
- All HTML is rendered through Go's `html/template`, which escapes
  values by default.
- Phase 1 endpoints are read-only — there are no mutating actions
  (install / upgrade / remove). Mutating endpoints are tracked for a
  future release and will require explicit confirmation.

## See Also

- [`clim list`](/reference/commands/list) — Same data on the terminal.
- [`clim info`](/reference/commands/info) — Single-tool detail in the terminal.
- [`clim trail`](/reference/commands/trail) — Toolchain history.
