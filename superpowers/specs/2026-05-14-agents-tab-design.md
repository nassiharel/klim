# Agents Tab — Design Spec

**Date:** 2026-05-14
**Status:** Approved (single-PR delivery)
**Owner:** klim core
**Related:** `superpowers/specs/2026-05-03-browser-view-design.md`, `superpowers/specs/2026-05-05-env-design.md`

---

## 1. Problem & Goals

Developers increasingly run **multiple agent CLIs side-by-side** (Claude Code, GitHub Copilot CLI, …) and each ships its own ecosystem of plugins, skills, MCP servers, and persisted sessions. Today there is no single place to:

- **Discover** what plugins/skills/MCPs are installed across all agents, where they came from, and whether they're enabled.
- **Search** for an installable plugin or skill without remembering which marketplace it lives in.
- **Manage** (install / uninstall / enable / disable / refresh) entries uniformly.
- **Launch** a session against a known skill, plugin, or saved session without typing the exact CLI invocation.

klim already centralizes developer **tooling** (binaries, package managers). The Agents tab extends that model to **agent ecosystem entities**.

### Goals

1. Single TUI tab listing Marketplaces, Plugins, Skills, MCPs, and Sessions across all detected agent CLIs.
2. Fuzzy search — scoped per sub-view and global across all entities.
3. Read curated remote catalogs (Anthropic's official marketplace, GitHub's `copilot-plugins` / `awesome-copilot`, official MCP registry) and merge with locally detected state.
4. Install / uninstall / enable / disable / refresh / launch / delete via the underlying CLIs (klim never re-implements an agent's install logic).
5. Mirror functionality on the command line under `klim agents …`.
6. v1 supports **Claude Code + GitHub Copilot CLI**; extensible to Cursor / Windsurf / Continue / Aider (MCP-only, later).

### Non-goals (v1)

- No embedded chat UI — launching a session **suspends klim** and hands the terminal to the agent CLI (`tea.ExecProcess`).
- No replacement for `claude agents` (multi-session view) or `/resume` pickers — klim **augments**, it doesn't duplicate.
- No write-through editing of arbitrary plugin/skill files — klim drives the CLIs, the CLIs own their state.
- No sync / sharing between machines for plugins or skills (separate future feature).

---

## 2. Background — Research Summary

Detailed research is captured in the chat history of this design session; only the design-relevant takeaways are summarized here.

**Shared standards across Claude Code and Copilot CLI:**

| Concern | Standard | Location |
|---|---|---|
| Plugin manifest | `plugin.json` (kebab-case `name`, optional `version`, `description`, `author`, …) | Claude: `.claude-plugin/plugin.json`. Copilot: `.plugin/plugin.json` → `plugin.json` → `.github/plugin/plugin.json` → `.claude-plugin/plugin.json` (first found) |
| Marketplace manifest | `marketplace.json` with `plugins[]` | `.claude-plugin/marketplace.json` (read by both tools) |
| Skill | `SKILL.md` directory with YAML frontmatter (`name`, `description`, `when_to_use`, `allowed-tools`, …) | `agentskills.io` standard |
| MCP server config | `{"mcpServers": { <name>: { command/url, args, env } }}` | Claude: `~/.claude.json` (user+local), `.mcp.json` (project). Copilot: `~/.copilot/mcp-config.json`, `.github/mcp.json`, `.mcp.json` |

**Filesystem roots klim must scan:**

```
~/.claude.json                              # Claude user-level config + MCP
~/.claude/skills/, ~/.claude/agents/         # personal skills/agents
~/.claude/projects/<repo>/                   # auto-memory + session transcripts (path TBD)
<repo>/.claude/{skills,agents,settings.json} # project scope
<repo>/.mcp.json                             # project MCP

~/.copilot/mcp-config.json
~/.copilot/installed-plugins/<marketplace>/<plugin>/
~/.copilot/skills/, ~/.copilot/agents/
~/.cache/copilot/marketplaces/  (or platform equiv.)
<repo>/.github/{skills,agents,plugin,mcp.json}
```

**Open verification points** (flagged inline below as `[TBV]`):

- Claude Code plugin install cache path — likely `~/.claude/plugins/` or `~/.claude/marketplaces/<m>/<p>/`. Confirm with `claude plugin install` → `find ~/.claude -name plugin.json`.
- Claude Code session transcript filename/path inside `~/.claude/projects/`.
- Copilot CLI session transcript path (`~/.copilot/...`).
- Output format of `claude mcp list`, `claude plugin list`, `copilot plugin list` (JSON vs. text). All providers must be parsed; prefer `--output json` if available, fall back to text + regex.

---

## 3. Architecture

### 3.1 Package layout

```
internal/agents/
  types.go              # Plugin, Skill, MCP, Session, Marketplace, Scope, Status
  provider.go           # Provider interface + Registry
  service.go            # AgentService: composition root, scan, search, mutate, launch
  fuzzy.go              # Score-based subsequence matcher (no third-party dep)
  cache.go              # ~/.klim/cache/agents-cache.yaml (per-host)
  catalog.go            # Curated remote catalog fetch + cache
  exec.go               # Build CLI exec specs (claude/copilot) for tea.ExecProcess

  providers/
    claudecode/         # Claude Code provider impl
    copilotcli/         # Copilot CLI provider impl
    mcpregistry/        # registry.modelcontextprotocol.io read-only provider (MCPs only)

internal/cli/
  agents.go             # `klim agents` Cobra umbrella
  agents_plugins.go
  agents_skills.go
  agents_mcps.go
  agents_sessions.go
  agents_marketplaces.go
  agents_launch.go

internal/tui/
  agents_tab.go         # Agents tab model + sub-views
  agents_search.go      # Scoped + global fuzzy search overlay
  agents_detail.go      # Right-pane detail view (one per entity type)
  agents_launch_modal.go# Pre-launch confirmation modal
```

### 3.2 Provider interface

```go
package agents

type Provider interface {
    ID() string                  // "claude-code" | "copilot-cli" | "mcp-registry"
    DisplayName() string
    Detect(ctx context.Context) Status

    // Read-only enumeration. Each may return ErrNotSupported.
    Marketplaces(ctx context.Context) ([]Marketplace, error)
    Plugins(ctx context.Context) ([]Plugin, error)
    Skills(ctx context.Context) ([]Skill, error)
    MCPs(ctx context.Context) ([]MCP, error)
    Sessions(ctx context.Context) ([]Session, error)

    // Mutations. Each may return ErrNotSupported.
    AddMarketplace(ctx context.Context, spec string) error
    RemoveMarketplace(ctx context.Context, name string) error
    InstallPlugin(ctx context.Context, ref PluginRef) error
    UninstallPlugin(ctx context.Context, id string) error
    EnablePlugin(ctx context.Context, id string, enabled bool) error
    AddMCP(ctx context.Context, spec MCPSpec) error
    RemoveMCP(ctx context.Context, name string) error
    EnableMCP(ctx context.Context, name string, enabled bool) error
    DeleteSession(ctx context.Context, id string) error

    // Launch: build the command to exec. The TUI/CLI runs it.
    BuildLaunch(LaunchSpec) (ExecPlan, error)
}

type Status struct {
    Installed bool
    Version   string
    BinPath   string
    Error     error
}

type ExecPlan struct {
    Bin   string
    Args  []string
    Env   []string
    Cwd   string
    Note  string // shown in confirmation modal
}
```

`ErrNotSupported` is a sentinel; a provider that doesn't support sessions returns it from `Sessions()` and the service quietly omits it for that provider — no banner spam.

### 3.3 AgentService

```go
type Service struct {
    providers []Provider
    cache     *Cache
    catalog   *RemoteCatalog
    sem       chan struct{} // resolveSem-style worker pool
}

func (s *Service) LoadAll(ctx context.Context, opts LoadOpts) (*Snapshot, error)
func (s *Service) Search(query string, scope EntityType) []SearchResult
func (s *Service) Resolve(typ EntityType, id string) (any, error)
func (s *Service) Mutate(action Action) error
func (s *Service) Launch(spec LaunchSpec) (ExecPlan, error)
func (s *Service) Refresh(ctx context.Context) error
func (s *Service) Invalidate()
```

A `Snapshot` is an in-memory tree of every entity with a `Source` enum (`local-claude`, `local-copilot`, `catalog-anthropic`, `catalog-github-copilot`, `catalog-mcp-registry`, etc.) so the UI can show provenance and the search index can de-duplicate (e.g. an installed plugin also present in its marketplace).

### 3.4 Caching

- `~/.klim/cache/agents-cache.yaml` — per-host scan cache; same `scancache`-style writer (atomic, `fileutil.AtomicWrite`).
- `~/.klim/marketplace/agents-catalog-cache.yaml` — fetched remote catalog (Anthropic marketplace JSON, GitHub copilot-plugins, MCP registry). Conditional GET via `If-None-Match`/`ETag` where the source supports it.
- Cache invalidated by: `r` key in TUI, `--refresh` CLI flag, any successful mutation.

### 3.5 Fuzzy search

Lightweight subsequence matcher in `internal/agents/fuzzy.go` (no new dependency):

- Score = base subsequence match + bonuses for: prefix match, word-boundary match, consecutive-char run, case-insensitive equality on whole words, type-prefix match (`skill:foo`).
- Returns `[]SearchResult{ Score, EntityType, ID, Provider, MatchedRanges }` so the UI can highlight matched chars.

Global search syntax: `<type>:<query>` (e.g. `mcp:postgres`, `skill:react`) filters results to that entity type without leaving the global view.

---

## 4. TUI

### 4.1 Tab strip

`Agents` slots into **position 4**, between Dashboard and My Profile. New order:

| # | Tab |
|---|---|
| 1 | My Tools |
| 2 | Marketplace |
| 3 | Project |
| 4 | Dashboard |
| **5** | **Agents** ← new |
| 6 | My Profile |
| 7 | Health |
| 8 | Security |
| 9 | Backup |
| 0 | Config |

All tab-shortcut docs (AGENTS.md table, help screen) update accordingly.

> **Decision recorded earlier in this session** said position 4; placing it at 5 keeps Dashboard's umbrella role first and lets number keys continue mapping 1:1 to visible tab order. The earlier recommendation is updated here based on tab-strip re-review.

### 4.2 Layout

Three-column layout (validated by k9s / lazygit prior art):

```
┌────────────────────────────────────────────────────────────────────────┐
│ Marketplaces · Plugins · Skills · MCPs · Sessions   [subtabs · 1..5]   │
├────────────────────────────────────────────────────────────────────────┤
│ Search:  ▸ react_                          [/ scope · g/ global · ?]   │
├────────────┬────────────────────────────────────┬──────────────────────┤
│ ▾ Source   │ ★ react-helper                     │ Plugin               │
│  □ Claude  │   plugin · superpowers-marketplace │ ─────────────────    │
│  ☑ Copilot │   Installed · enabled · 12 skills  │ Source: Claude Code  │
│  ☑ Catalog │   by anthropic · v0.4.2            │ Status: enabled      │
│            │ ────────────────────────────────── │ Skills:              │
│ ▾ Status   │ ▸ react-component-builder          │  • component-builder │
│  ☑ Installd│   skill · react-helper             │  • use-effect-audit  │
│  ☐ Avail.  │   Claude Code · enabled            │ MCPs:                │
│            │ ▸ react-server-mcp                 │  • react-docs        │
│ ▾ Type     │   mcp · registry.modelcontext...   │ Actions:             │
│  ☑ Plugin  │   Available                        │  [i] install         │
│  ☑ Skill   │                                    │  [x] uninstall       │
│  ☑ MCP     │                                    │  [e] toggle enable   │
└────────────┴────────────────────────────────────┴──────────────────────┘
 1-5 subtab   / search   g/ global   enter detail   l launch   r refresh
```

Detail pane renders contextually per entity type:

- **Marketplace** — owner, plugin count, last sync, source URL, plugin list.
- **Plugin** — author, version, description, source marketplace, contained skills/MCPs, scope, install state.
- **Skill** — full `SKILL.md` frontmatter, source plugin/scope, `when_to_use`, `allowed-tools`, file path.
- **MCP** — transport type, command/URL, env keys (values redacted), enabled-state, scope, tools list (if known).
- **Session** — title, project path, last modified, turn count, provider, "Recent files" if available.

### 4.3 Keybindings

Within Agents tab (extend existing global keys, not override):

| Key | Action |
|---|---|
| `1`..`5` | Jump to sub-view (Marketplaces / Plugins / Skills / MCPs / Sessions) |
| `Tab` / `Shift-Tab` | Cycle sub-views |
| `j`/`k` or `↑`/`↓` | Item navigation |
| `Enter` | Open detail pane |
| `Esc` | Close detail pane / clear search |
| `/` | Scoped fuzzy search (current sub-view) |
| `g` then `/` | Global fuzzy search across all entity types |
| `l` | Launch session (skill/plugin/session/MCP context) — opens confirmation modal |
| `i` | Install (plugin/MCP) |
| `x` | Uninstall / delete (with confirmation) |
| `e` | Toggle enable/disable |
| `a` | Add (marketplace URL, MCP form) |
| `r` | Refresh — rescan local + reload remote catalog |
| `?` | Tab-specific help overlay |

Search input uses the existing klim filter input component for consistency.

### 4.4 Launch flow

1. User presses `l` on a skill / plugin / session, or `Enter` on a session row → Resume.
2. **Confirmation modal** renders the exact `ExecPlan` (bin, args, env vars added, cwd) — gives the user agency before klim hands over the terminal.
3. On confirm: TUI sends a `tea.ExecMsg` carrying the `ExecPlan`. Bubbletea suspends, exec's the command with inherited stdin/stdout/stderr, and resumes on exit.
4. On the agent CLI's exit, klim re-enters the Agents tab and shows the exit code in the status bar (success = silent; non-zero = red banner with last 200 chars of stderr if we captured any).

Sessions specifically:

- Resume: `claude -r <id>` or `copilot --resume=<id>`.
- Skill: `claude` (new session with skill bias via `--skill` if supported; otherwise launch new and instruct user to invoke `/<skill-name>`). Decision: launch new + show post-launch hint banner.
- Plugin: not directly launchable — `l` on a plugin opens "launch a session that has this plugin enabled", which becomes a normal `claude` / `copilot` invocation in the current project, with a banner reminding the user the plugin is active.

### 4.5 Empty / error states

- Provider not installed → sub-view shows a single info row: `Claude Code not detected · klim install claude` (linking to the existing tool install flow, since `claude` lives in klim's marketplace catalog).
- Remote catalog fetch failed → fall back to cache + render banner in status bar with timestamp of last successful fetch.
- Mutating action fails → modal with stderr + exit code; entity row stays in its previous state.

---

## 5. CLI

`klim agents` umbrella, following the existing CLI conventions (canonical `--output={text,json,yaml}`, exit code 2 for usage errors, `PartialFailureError` for multi-target ops, human prose to stderr).

```
klim agents                              # alias for `klim agents list`
klim agents list [--type plugin|skill|mcp|session|marketplace]
                 [--provider claude-code|copilot-cli|all]
                 [--installed|--available] [--enabled|--disabled]
                 [--search <query>] [--refresh]
klim agents search <query>               # global fuzzy across all entities

klim agents marketplaces [list|refresh|add <url>|remove <name>]
klim agents plugins      [list|info <id>|install <id>|uninstall <id>|enable <id>|disable <id>]
klim agents skills       [list|info <id>]
klim agents mcps         [list|info <name>|install <spec>|remove <name>|enable|disable]
klim agents sessions     [list|show <id>|resume <id>|delete <id>]

klim agents launch       [--provider <id>] [--skill <name>] [--session <id>]
                         [--plugin <name>] [--cwd <path>] [--print-only]
klim agents doctor                       # provider detection, cache freshness, MCP reachability
klim agents refresh                      # rescan local + reload remote catalogs
```

`launch --print-only` is a recommended escape hatch — emits the exact command without exec'ing, useful for scripting and for users who prefer to inspect first (matches the `klim plan` philosophy).

All `list`/`info`/`search` commands honor `--output={text,json,yaml}` via the shared `addOutputFlag` / `addPersistentOutputFlag` helpers.

---

## 6. Config

`config.yaml` gains an `agents:` section:

```yaml
agents:
  enabled_providers: [claude-code, copilot-cli]  # disable to hide a sub-view
  paths:
    claude_home: ""                              # override ~/.claude
    copilot_home: ""                             # override ~/.copilot (COPILOT_HOME also respected)
  remote_catalogs:
    refresh_interval: 24h
    sources:
      - id: anthropic-official
        url: https://anthropic.com/claude-code/marketplace.schema.json
        enabled: true
      - id: github-copilot-plugins
        url: https://raw.githubusercontent.com/github/copilot-plugins/main/.github/plugin/marketplace.json
        enabled: true
      - id: mcp-registry
        url: https://registry.modelcontextprotocol.io/v0/servers
        enabled: true
  launch:
    confirm: true                                # show pre-launch modal
    autopilot: false                             # whether to default-add --autopilot for copilot
```

All fields have sensible defaults; users only edit to override.

---

## 7. Testing

**Unit tests:**

- `fuzzy.go`: rank stability, case-insensitive matching, type-prefix syntax, empty/whitespace input.
- Each provider against a fixture filesystem tree (`t.TempDir()` + a fake `claude` / `copilot` binary on PATH via `os.Setenv("PATH", ...)`). Golden YAML for `Snapshot`.
- `BuildLaunch`: tests build exec plans for every supported launch combination — no actual exec.

**TUI tests:**

- Existing pattern: model-level tests for sub-view switching, search filter, detail open/close, action dispatch (`atomic.Pointer` test hook for the service).
- Render snapshots for empty / loading / error states.

**CLI tests:**

- Cobra command test: each subcommand parses, validates flags, dispatches to the service with the right args, emits the correct format. Fake service via interface injection.

**Integration tests** (`-tags=integration`):

- Behind a build tag, run real `claude --version` / `copilot --version` if found on PATH; skip otherwise. Smoke-test `list` outputs.

**Excluded for v1:** end-to-end launch tests (we don't want to spawn agent CLIs in CI).

---

## 8. Delivery — Single PR

Per the user's directive, this ships as **one PR** in the following commit sequence (rebased/squashed to taste before merge):

1. `internal/agents`: types, Provider interface, fuzzy matcher, cache, service skeleton — with unit tests.
2. `internal/agents/providers/claudecode` + `.../copilotcli` + `.../mcpregistry` — provider impls with unit tests against fixture trees.
3. `internal/cli/agents*.go` — Cobra commands wired to the service.
4. `internal/tui/agents_*.go` — Agents tab + sub-views + search + detail + launch modal; shift existing tabs.
5. Docs: AGENTS.md tab table update, help screen entries, CLI reference under existing docs/ (now in klim-web — open follow-up PR there).

CI runs the existing matrix (`go test -race -count=1 -timeout=5m ./...`); no new tooling required.

---

## 9. Risks & Open Questions

| Risk | Mitigation |
|---|---|
| `claude plugin list` / `copilot plugin list` output format unstable | Prefer `--output json` (or equivalent); fall back to text parsing behind a feature flag; integration tests gated by a build tag |
| Plugin install cache paths undocumented | Hands-on verification step at start of impl; provider impls accept a path override in config so user can patch around drift |
| Remote catalog rate limits (Anthropic marketplace, MCP registry) | ETag + cached negative responses; nightly refresh + manual `r`; never block UI on remote |
| `tea.ExecProcess` quirks on Windows (alternate screen restore) | Bubbletea v2 handles it; we already use it in install/upgrade flows |
| User confusion: which provider owns a skill/plugin? | Every row tags the provider; sidebar Source filter; detail pane lists exact file path |
| Tab renumbering breaks muscle memory | Document in CHANGELOG + a one-shot toast on first run after upgrade |

**[TBV]** items (verify hands-on during impl):

- Claude plugin cache filesystem layout.
- Claude `~/.claude/projects/` path encoding scheme (`%2F` vs `-`).
- Copilot session transcript file location.
- JSON output availability for `claude mcp list`, `claude plugin list`, `copilot plugin list`.

---

## 10. Out of scope (future work)

- Cursor / Windsurf / Continue MCP-only providers (Provider interface already accommodates them).
- Smithery integration as a third MCP marketplace.
- Sync / share agent configs between machines via klim export/import.
- Embedded chat UI for headless `claude -p` / `copilot -p` flows.
- `klim agents bg` for managing background sessions (overlaps with `claude agents` view; defer).
