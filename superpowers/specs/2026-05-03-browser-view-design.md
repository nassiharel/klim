# klim browser — local web UI for the toolchain

## Problem

The TUI is the most feature-rich way to use klim, but it has hard limits:

- **No mouse-friendly affordances** for users who prefer click navigation.
- **No persistent multi-pane layout** — the TUI is one view at a time.
- **Hard to share** — pointing a teammate at a screen requires SSH + a terminal that supports the TUI's wide character set.
- **Awkward for richer content** — long descriptions, GitHub READMEs, embedded charts, copy-paste of share tokens.

Goal: ship a `klim browser` subcommand that boots a local HTTP server, opens
the user's default browser, and presents the same information the TUI does
in a familiar mouse-driven layout. The browser view eventually reaches
parity with the TUI; this spec covers the foundation and Phase 1 scope.

## Approach

The browser view is **a thin frontend over the existing service layer**.
No new business logic moves into the web package; every page is rendered
from `service.ToolService` calls and pure rendering helpers. The web
package is a peer of `internal/cli` and `internal/tui`, all three of
which call into the same composition root.

```
+---------------------------+
|        klim browser       |  cobra subcommand
+-------------+-------------+
              |
              v
+---------------------------+
|    internal/web (NEW)     |  net/http server, html/template,
|  - server lifecycle       |  embed.FS for static assets
|  - HTML pages             |
|  - JSON API at /api/*     |
+-------------+-------------+
              |
              v
+---------------------------+
|  internal/service         |  unchanged; reused as-is
+---------------------------+
```

## Stack choices (and why)

| Decision | Choice | Reason |
|---|---|---|
| Server | Go stdlib `net/http` | No new heavy dep; uniform with the rest of the binary. |
| Routing | stdlib `http.ServeMux` (Go 1.22+ pattern matching) | Sufficient for this surface; avoids router churn. |
| Templates | `html/template` | Stdlib, escapes by default (XSS-safe), embeds fine. |
| Frontend JS | Minimal vanilla JS | No npm; binary stays single-file; pages work without JS. |
| Styles | One hand-rolled CSS file | No build step; ships embedded. |
| Asset embedding | `embed.FS` | Single binary; offline-capable. |
| Browser open | platform-specific `os/exec` (`xdg-open` / `open` / `rundll32 url.dll`) | No third-party dep; gracefully no-ops on failure. |

Server-side rendering is the right fit for an MVP: the data model is
already fully resolved before the page renders, the surface is mostly
read-only, and we keep the option of progressively enhancing with JS or
swapping to an SPA later.

## CLI surface

```
klim browser [flags]

Flags:
  --port int          listen port (0 = pick a free one) (default 0)
  --bind string       bind address (default "127.0.0.1")
  --no-open           do not open the browser automatically
  --insecure-bind     allow non-loopback bind addresses
```

Behavior:

- Bind defaults to `127.0.0.1`. Refuses any non-loopback address unless
  `--insecure-bind` is passed (so `klim browser --bind 0.0.0.0` is an
  explicit decision, not an accident).
- Default port is `0` (kernel picks a free one). Print the chosen URL
  to stderr; tools like `xdg-open` use the printed URL.
- `--no-open` skips the browser-open step (useful in CI / headless test).
- Ctrl-C triggers graceful shutdown via `http.Server.Shutdown`.

## HTTP surface (Phase 1 MVP)

| Path | Method | Renders |
|---|---|---|
| `/` | GET | Installed tools (the most common use case) |
| `/tools/{name}` | GET | Tool detail page (CLI-info equivalent) |
| `/dashboard` | GET | Aggregate stats |
| `/trail` | GET | Trail entry list |
| `/trail/{ref}` | GET | Snapshot at ref |
| `/healthz` | GET | `200 ok` for liveness probes |
| `/static/*` | GET | Embedded CSS / JS / favicon |

JSON API mirrors the same handlers under `/api`:

| Path | Returns |
|---|---|
| `/api/tools` | All resolved tools |
| `/api/tools/{name}` | One resolved tool + GitHub info |
| `/api/dashboard` | Stats payload |
| `/api/trail` | Entries |
| `/api/trail/{ref}` | Snapshot body |

JSON responses use the same shapes the existing CLI commands emit so
existing tooling (and tests) can read both indistinguishably.

> **Note (post-Phase 1):** what shipped in PR #48 went well past the
> Phase 1 scope described above. **Updates**, **Discover**,
> **Backup**, **Favorites**, **For You**, **Packs**, **Projects**,
> and **Config** are all live (Phase 2–5 of the same PR). The
> "Phase 2+" roadmap below tracks what came after.

## Testing strategy

- `httptest.NewServer` for handler tests; assert HTTP status + body
  contains key fields, not raw HTML byte-equality (templates evolve).
- JSON handlers are tested with the same fixtures used by `klim list
  --output json` and `klim info --output json` so contract drift fails
  one of the two.
- Browser-open is behind an interface so tests can substitute a no-op.
- No headless-browser tests in MVP — handler + JSON coverage is enough.

## Security

- 127.0.0.1 bind by default. Loopback-only is the threat model.
- `--insecure-bind` is required for any other interface; we print a
  warning that the server has no auth.
- No mutating endpoints in Phase 1, so the surface area for misuse is
  read-only catalog + scan data.
- All HTML is rendered through `html/template` for XSS safety.

## Phase 2+ (out of scope here, but supported by the architecture)

- Mutating actions (install / upgrade / remove) with confirmation
  dialogs, mirroring TUI keybinds.
- Favorites + share tokens.
- Config editor.
- Live updates via Server-Sent Events on `/api/events`.
- Auth token (auto-generated, written to stderr) for `--insecure-bind`.
- Marketplace browse & filter.
- Keyboard-driven navigation parity (j/k, /, ?).

## Risks

- **Browser auto-open misbehaves on niche platforms.** Mitigation:
  always print the URL; auto-open is best-effort.
- **Templates drift from JSON shape.** Mitigation: shared structs
  between page and JSON handlers per route.
- **Embedded asset size grows.** Mitigation: keep CSS/JS hand-rolled;
  bail out to a build step only when we genuinely need it.

## File layout

```
internal/web/
  server.go         server lifecycle, mux, graceful shutdown
  server_test.go
  pages.go          HTML page handlers
  pages_test.go
  api.go            JSON API handlers
  api_test.go
  open.go           cross-platform default-browser launcher
  embed.go          //go:embed for templates + static
  templates/
    layout.html
    installed.html
    tool.html
    dashboard.html
    trail.html
    snapshot.html
    stub.html
  static/
    styles.css
    app.js
    favicon.svg

internal/cli/
  browser.go        cobra subcommand wiring

docs/src/content/docs/reference/commands/browser.md
```

## Definition of done (this spec)

- `klim browser` boots a server on a free port, opens the user's
  browser, and serves the Installed page populated from a real scan.
- Tool detail, Dashboard, Trail list, and Snapshot pages render with
  real data from the existing service layer.
- JSON API counterpart for each page.
- Stub pages for the four remaining TUI tabs.
- Handler tests cover happy paths and 404 / 405 cases.
- `klim browser --help`, the docs reference page, and the docs sidebar
  all surface the new subcommand.
- Build is single-binary, offline-capable, no new heavy deps.
