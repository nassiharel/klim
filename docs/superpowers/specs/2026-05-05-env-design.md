# env — design

> Status: implementation-ready. Authored autonomously per the "Let's implement" framing; user review will happen at PR time.

## Problem

`clim share` produces a compact chat token, but only carries tool **names**. `clim export` produces a YAML file with tools + OS + arch, but is awkward for chat ("paste this 200-line YAML"). Neither captures the wider environment shape — favorites, custom packs, available package managers, the user's clim version, observational health/audit signals.

Result: when one engineer wants to reproduce another's environment ("set me up like your laptop"), they manually combine `clim share`, `clim export`, hand-written notes about favorites, etc.

## Goal

A single artifact ("**env**") that captures enough of an environment to share, diff, and reproduce — with two encodings of the same data:

- **Token form**: paste-friendly base64 string (`clim:env:v1:<gz+b64>`) for chat / quick share.
- **File form**: human-readable YAML for git, code review, and `clim` itself.

## Non-goals

- Reproducing per-user state (login sessions, env vars, dotfiles).
- Hosting the artifact (no server; everything is offline / paste-friendly).
- Cross-OS reproducibility magic — apt-only tools won't materialize on Windows. Apply just attempts what's possible and reports what was skipped.

## Schema (YAML, source of truth)

```yaml
schema_version: 1
clim:
  version: v1.2.3
  commit: 97b8857
generated_at: "2026-05-05T17:30:00Z"
hash: f4a7b9c1de32   # first 12 hex of sha256 over the canonical payload
                    # (excluding hash + generated_at fields), so two
                    # envs that differ only in capture time still
                    # share a hash

os:
  goos: linux
  arch: amd64
  distro: "Ubuntu 22.04"   # best-effort; empty when not detectable

package_managers:           # which PM binaries exist on $PATH today
  brew:   true
  apt:    true
  npm:    true
  snap:   false
  winget: false
  scoop:  false
  choco:  false

tools:                      # installed tools — compact subset of internal/manifest.Tool
  - name: jq
    version: "1.7"
    source: brew
    category: CLI
  - ...

favorites: [fzf, jq, ripgrep]

packs:                      # user's custom packs (NOT marketplace packs)
  - name: my-cli
    tools: [fzf, jq, ripgrep]

security:                   # observational; not enforced on apply
  audit_warnings: 3
  audit_infos:    2
  verdicts:
    clean:   12
    watch:    4
    risk:     1
    unknown:  0
```

### Privacy / redaction

- No hostname, username, or absolute paths.
- No environment variables.
- No detected scripts / dotfile contents.
- The token is deterministic from the env state alone (modulo `generated_at`), so passing it to a coworker leaks **only what `clim list` already shows**.

## Token encoding

```
clim:env:v1:<base64url(gzip(yaml(profile)))>
```

- Versioned prefix lets future schemas decode older tokens via fallback.
- Gzip keeps the token small (typical 30-tool env: ~600 chars).
- Base64-url so it pastes safely in URLs, JSON, Markdown without escaping.
- Length cap: 256 KB encoded / 64 KB decompressed (mirrors `internal/share`).

## Commands

```
clim env                              # print token to stdout
clim env --output {text,yaml,json}    # explicit form (default text = token)
clim env show <token-or-file>         # decode + pretty-print
clim env diff <token-or-file>         # diff vs current env
clim env apply <token-or-file>        # install missing tools + set favorites + add packs (--yes to skip prompts)
```

All commands follow CLI conventions: human text on stderr, machine output (`--output json`) on stdout.

`apply` is a thin wrapper that:
1. Resolves the manifest from the token.
2. Calls into the existing install-plan machinery (`internal/cli/installplan.go`) for tool install — same code path as `clim import` so we don't duplicate logic.
3. Adds favorites via the existing `favorites.Add`/`Set`.
4. Registers custom packs via `custompacks.Add` (existing packs with the same name are preserved; users delete from `~/.config/clim/marketplace/custom-packs.yaml` or via the TUI to replace).

Cross-OS / cross-PM gaps surface in the report as "skipped: no winget package on linux" etc., never as errors.

## Package: `internal/envid`

| File | Responsibility |
|------|---------------|
| `types.go` | `Profile`, `Tool`, `Pack`, `Security` types with yaml/json tags |
| `build.go` | `Build(ctx, svc, cfg) (*Profile, error)` — assembles a profile from the live system; canonicalizes favorites and pack tool lists at collection time |
| `token.go` | `Encode(p) (string, error)` / `Decode(s) (*Profile, error)` — gzip+b64 token I/O with versioned `clim:env:v1:` prefix |
| `file.go` | `ReadFile` / `WriteFile` for the YAML form |
| `hash.go` | `ComputeHash` + `canonicalize` helpers — sorts/dedups slices and zeros `GeneratedAt`/`Hash` so two captures of the same env share an identifier |
| `distro.go` | best-effort Linux distro hint via `/etc/os-release`; macOS / Windows return canonical names |
| `*_test.go` | round-trip, schema-version mismatch, decode-tampered, hash stability across time, hash sensitivity to content |

Privacy is achieved by never collecting hostname/username/paths/env-vars in the first place — the spec previously mentioned a `redact.go` layer, but with the actual `Build` keeping its scope tight there's nothing to filter out at marshal time. If a future schema field needs scrubbing, that's where it would land.

Public API surface:
```go
package envid

func Build(ctx context.Context, svc *service.ToolService, cfg *config.Config) (*Profile, error)
func Encode(p *Profile) (string, error)
func Decode(token string) (*Profile, error)
func ReadFile(path string) (*Profile, error)
func WriteFile(path string, p *Profile) error
```

## Surfaces

- **CLI**: `clim env` umbrella + 3 subcommands (above).
- **TUI**: Backup tab gets a row "env" with [Generate] and [Apply] keys; existing favorites/packs sections gain a "Share via env" hint.
- **Web view**: out of scope for v1 — document as CLI/TUI feature; web can render the token via the existing /backup page in a follow-up.
- **Docs**: `docs/src/content/docs/reference/commands/env.md` (full reference).

## Tests

- `envid_test.go`: round-trip preserves all fields; tampered token → typed error; old schema_version → typed error; hash unchanged when only `generated_at` differs.
- `cli env` tests: encode & show stub round-trip; apply uses a fake install plan so we don't shell out to PMs.

## Out of scope (deferred)

- Encryption / signing (the data is non-sensitive by design; signing can land later if a verified-share use case appears).
- Web view "Generate env" button.
- Diff visualization beyond a flat per-tool list.
- Cross-machine sync server.
