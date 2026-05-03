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
| `/tools/<name>` | Per-tool detail with **Install / Upgrade / Remove** buttons. |
| `/updates` | Outdated tools with current → latest version comparison. |
| `/discover` | Full marketplace catalog with category, tag, and free-text filters. |
| `/favorites` | Your favorited tools, with toggle action on each row. |
| `/dashboard` | Aggregate stats: counts, top categories, sample of pending updates. |
| `/trail` | Trail entry list. |
| `/trail/<ref>` | Snapshot at the given trail ref. |
| `/backup` | Export YAML download, share token, and manifest preview. |
| `/backup/export.yaml` | Direct YAML download (used by the Export button). |
| `/config` | Read-only YAML dump of the running configuration. |
| `/jobs/<id>` | Live progress for an Install / Upgrade / Remove job (SSE-streamed). |
| `/healthz` | Liveness probe (`200 ok`). Always unauthenticated. |

## Actions and live progress

The Tool detail page exposes the same Install / Upgrade / Remove
actions as the TUI. Submitting a button creates a job and redirects
to `/jobs/<id>`, which streams the package manager's combined
stdout / stderr line-by-line via Server-Sent Events. The page also
falls back to the snapshot endpoint if the browser doesn't support
SSE.

The action chooses a package manager automatically:
- **Install** uses the best available source for your OS (the same
  rule `clim list` uses for its install commands).
- **Upgrade** and **Remove** prefer the source the tool is already
  installed from.

## JSON API

A JSON counterpart to every page is exposed under `/api/*`. The shapes
mirror the existing CLI `--output json` payloads so existing scripts
read both indistinguishably.

| Path | Method | Returns |
|------|--------|---------|
| `/api/tools` | GET | All resolved tools + catalog summary. |
| `/api/tools/<name>` | GET | One resolved tool, including GitHub metadata. |
| `/api/dashboard` | GET | Stats payload used by `/dashboard`. |
| `/api/trail` | GET | Trail entries (newest first). |
| `/api/trail/<ref>` | GET | `{ "entry": ..., "snapshot": ... }`. |
| `/api/favorites` | GET | Your current favorite tool names (sorted). |
| `/api/favorites/<name>/toggle` | POST | Flip the favorite state; returns `{ "name", "favorite" }`. |
| `/api/jobs` | POST | Body: `{ "action": "install"\|"upgrade"\|"remove", "tool": "<name>" }`. Returns `202 Accepted` + the job snapshot. |
| `/api/jobs/<id>` | GET | JSON snapshot of the job. |
| `/api/jobs/<id>/stream` | GET | Server-Sent Events stream of the job's output (replays history on connect). |

State-changing endpoints (POST) require an `Origin` or `Referer` header
that matches the host serving the request. This blocks CSRF and DNS
rebinding even on loopback. Browsers send the right header
automatically; scripts must set `-H "Origin: http://127.0.0.1:<port>"`
explicitly.

## Examples

```bash
# Run on a fixed port without opening the browser (CI / headless).
clim browser --port 7777 --no-open

# Probe the API while the server is running.
curl -s http://127.0.0.1:7777/api/dashboard | jq .updates_available
```

## Security

- Loopback-only by default. `--insecure-bind` is required for any
  other interface.
- **`--insecure-bind` automatically enables bearer-token
  authentication.** clim generates a 32-byte token at startup and
  prints a `?token=<token>` URL to stderr. Visiting that URL once sets
  a session cookie; the token is also accepted via
  `Authorization: Bearer <token>` for scripts. `/healthz` stays open
  for liveness probes.
- Only one Install / Upgrade / Remove job runs per tool at a time.
  Submitting a second action for a tool that already has a running
  job redirects to the existing job's progress page (HTTP 303 for the
  HTML form path, HTTP 409 with `redirect_to` for the JSON API).
- All HTML is rendered through Go's `html/template`, which escapes
  values by default.
- State-changing endpoints (favorite toggle, install / upgrade /
  remove jobs) require an `Origin` or `Referer` header matching the
  request's `Host`. This blocks CSRF and DNS-rebinding attacks even
  when the server is reachable over loopback. Browsers send the
  header automatically for in-page navigation.
- Action jobs (install / upgrade / remove) shell out to the user's
  package managers using the same templates `clim list` uses. clim
  itself does not run anything as root; sudo prompts behave the same
  way they do from the terminal.

## See Also

- [`clim list`](/reference/commands/list) — Same data on the terminal.
- [`clim info`](/reference/commands/info) — Single-tool detail in the terminal.
- [`clim trail`](/reference/commands/trail) — Toolchain history.
