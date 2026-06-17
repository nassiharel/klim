# Marketplace tools additions — design

**Date:** 2026-06-17
**Status:** Draft — pending review
**Scope:** Add ~75 missing tools to `marketplace/tools/`, update existing packs, add 3 new packs, register new tags/categories.

---

## Goal

klim's marketplace currently ships 162 tools across 17 categories. After surveying the existing
catalogue against modern developer workflows, several high-value gaps are visible: cloud/edge
PaaS CLIs, the post-Docker container ecosystem (podman/nerdctl/buildah), supply-chain security
tooling (syft/osv-scanner/checkov/tfsec), database migrations (atlas/dbmate/goose), and the next
wave of AI assistants (gemini, goose, cursor).

This spec defines a focused expansion (~75 new tool YAMLs) plus three new packs and a small set
of new tags. It does **not** change any existing tool definitions, the schema, or klim's Go code.

---

## Non-goals

- No changes to the YAML schema, validator, or assembler.
- No changes to klim's Go source.
- No re-categorising of existing tools.
- No deletion of existing tools or packs.
- No new CLI commands or UI changes.

---

## Constraints

- **Windows availability:** *lenient* — include the tool even if no winget/scoop/choco package
  exists, so long as at least one platform package source exists (brew/apt/snap or github). The
  YAML omits keys that don't apply.
- **Schema:** match the existing tool YAML format exactly:
  ```yaml
  name: <id>
  display_name: <Display>
  category: <one of categories.yaml>
  tags:
      - tag1
      - tag2
  binary_names:
      - bin
  packages:
      winget: <id>
      scoop: <id>
      choco: <id>
      brew: <id>
      apt: <id>
      snap: <id>
  github: owner/repo
  ```
- **Validation:** every new tool must pass `make marketplace-validate`.
- **Conflicts:** `goose` is registered as the Block AI CLI; `goose-migrate` is the pressly DB
  migration tool. Documented below.

---

## New tools (by section)

### Section 1 — AI & Editors (10)

| File | category | display_name | notes |
|---|---|---|---|
| `gemini-cli.yaml` | AI | Gemini CLI | Google's official CLI agent. github: `google-gemini/gemini-cli`. |
| `goose.yaml` | AI | goose | Block's agent. github: `block/goose`. Renamed from generic `goose` to avoid the DB tool. |
| `gptme.yaml` | AI | gptme | github: `gptme/gptme`. brew/pipx only. |
| `codex.yaml` | AI | OpenAI Codex CLI | github: `openai/codex`. npm + brew. |
| `cursor-agent.yaml` | AI | Cursor Agent | github: `getcursor/cursor` (CLI subset). |
| `cursor.yaml` | Editor | Cursor | The IDE binary. winget: `Anysphere.Cursor`. No GitHub. |
| `zed.yaml` | Editor | Zed | github: `zed-industries/zed`. brew + scoop + apt. |
| `vim.yaml` | Editor | Vim | github: `vim/vim`. universal. |
| `emacs.yaml` | Editor | GNU Emacs | github: `emacs-mirror/emacs`. universal. |
| `micro.yaml` | Editor | micro | github: `zyedidia/micro`. universal. |
| `marktext.yaml` | Editor | MarkText | Markdown editor (GUI). github: `marktext/marktext`. winget: `marktext.marktext`, choco: `marktext`, brew (cask): `marktext`. Tags: `gui`, `docs`, `cross-platform`. |

### Section 2 — Cloud / Edge platforms (10)

| File | category | display_name | notes |
|---|---|---|---|
| `flyctl.yaml` | Cloud | flyctl | github: `superfly/flyctl`. brew + scoop. |
| `doctl.yaml` | Cloud | doctl | github: `digitalocean/doctl`. all platforms. |
| `vercel.yaml` | Cloud | Vercel CLI | npm only. github: `vercel/vercel`. |
| `netlify.yaml` | Cloud | Netlify CLI | npm + brew. github: `netlify/cli`. binary: `netlify`. |
| `wrangler.yaml` | Cloud | Cloudflare Wrangler | npm only. github: `cloudflare/workers-sdk`. |
| `supabase.yaml` | Cloud | Supabase CLI | brew/scoop/winget/npm. github: `supabase/cli`. |
| `cloudflared.yaml` | Network | cloudflared | brew/winget/apt. github: `cloudflare/cloudflared`. |
| `stripe.yaml` | CLI | Stripe CLI | brew/scoop/apt. github: `stripe/stripe-cli`. |
| `railway.yaml` | Cloud | Railway CLI | brew/scoop/npm. github: `railwayapp/cli`. |
| `render.yaml` | Cloud | Render CLI | brew. github: `render-oss/cli`. |

### Section 3 — Containers / Kubernetes (10)

| File | category | display_name | notes |
|---|---|---|---|
| `podman.yaml` | Containers | Podman | github: `containers/podman`. all platforms. |
| `nerdctl.yaml` | Containers | nerdctl | github: `containerd/nerdctl`. brew/scoop. |
| `buildah.yaml` | Containers | Buildah | github: `containers/buildah`. apt + brew. |
| `helmfile.yaml` | Containers | Helmfile | github: `helmfile/helmfile`. brew/scoop. |
| `kubeseal.yaml` | Containers | Sealed Secrets (kubeseal) | github: `bitnami-labs/sealed-secrets`. brew. |
| `krew.yaml` | Containers | krew | kubectl plugin manager. github: `kubernetes-sigs/krew`. brew/scoop. |
| `kompose.yaml` | Containers | Kompose | github: `kubernetes/kompose`. brew/scoop/apt. |
| `kn.yaml` | Containers | Knative Client | github: `knative/client`. brew. |
| `argo.yaml` | Containers | Argo Workflows CLI | github: `argoproj/argo-workflows`. brew. |
| `tilt.yaml` | Containers | Tilt | github: `tilt-dev/tilt`. brew/scoop. |

### Section 4 — Security / Supply chain (10)

| File | category | display_name | notes |
|---|---|---|---|
| `syft.yaml` | Security | Syft | SBOM generator. github: `anchore/syft`. brew/scoop. |
| `osv-scanner.yaml` | Security | OSV-Scanner | github: `google/osv-scanner`. brew/scoop. |
| `checkov.yaml` | Security | Checkov | github: `bridgecrewio/checkov`. brew + pipx. |
| `tfsec.yaml` | Security | tfsec | github: `aquasecurity/tfsec`. brew/scoop/choco. |
| `nuclei.yaml` | Security | Nuclei | github: `projectdiscovery/nuclei`. brew. |
| `step.yaml` | Security | Smallstep CLI | github: `smallstep/cli`. brew/scoop/winget. |
| `bw.yaml` | Security | Bitwarden CLI | github: `bitwarden/clients`. brew/scoop/npm. |
| `op.yaml` | Security | 1Password CLI | brew/scoop/winget. No github (proprietary). |
| `dockle.yaml` | Security | Dockle | github: `goodwithtech/dockle`. brew/scoop. |
| `crane.yaml` | Security | crane | github: `google/go-containerregistry`. brew. |

### Section 5 — Database / Migrations / Data (10)

| File | category | display_name | notes |
|---|---|---|---|
| `atlas.yaml` | Database | Atlas (Ariga) | github: `ariga/atlas`. brew. |
| `goose-migrate.yaml` | Database | goose (DB migrations) | github: `pressly/goose`. brew/scoop. Binary: `goose`. |
| `dbmate.yaml` | Database | dbmate | github: `amacneil/dbmate`. brew/scoop. |
| `flyway.yaml` | Database | Flyway | github: `flyway/flyway`. brew/scoop. |
| `liquibase.yaml` | Database | Liquibase | github: `liquibase/liquibase`. brew/scoop. |
| `kcat.yaml` | Database | kcat (Kafka) | github: `edenhill/kcat`. brew/apt. |
| `miller.yaml` | CLI | Miller (mlr) | CSV/TSV/JSON swiss army. github: `johnkerl/miller`. brew/scoop/apt. |
| `fx.yaml` | CLI | fx | JSON viewer. github: `antonmedv/fx`. brew/scoop/npm. |
| `gron.yaml` | CLI | gron | grep-able JSON. github: `tomnomnom/gron`. brew/scoop. |
| `lnav.yaml` | CLI | lnav | log navigator. github: `tstack/lnav`. brew/snap. |

### Section 6 — Languages + Version managers (10)

| File | category | display_name | notes |
|---|---|---|---|
| `php.yaml` | Language | PHP | brew/winget/apt. github: `php/php-src`. |
| `elixir.yaml` | Language | Elixir | brew/scoop/apt. github: `elixir-lang/elixir`. |
| `kotlin.yaml` | Language | Kotlin | brew/scoop/snap. github: `JetBrains/kotlin`. |
| `lua.yaml` | Language | Lua | brew/scoop/apt. github: `lua/lua`. |
| `pyenv.yaml` | PkgMgr | pyenv | github: `pyenv/pyenv`. brew + apt. Linux/macOS only (no Windows; pyenv-win is separate). |
| `nvm.yaml` | PkgMgr | nvm | github: `nvm-sh/nvm`. brew + script. Linux/macOS only. |
| `fnm.yaml` | PkgMgr | fnm | github: `Schniz/fnm`. brew/scoop/winget. Cross-platform alt to nvm. |
| `poetry.yaml` | PkgMgr | Poetry | github: `python-poetry/poetry`. brew + pipx. |
| `pipx.yaml` | PkgMgr | pipx | github: `pypa/pipx`. brew/scoop/apt. |
| `composer.yaml` | PkgMgr | Composer | github: `composer/composer`. brew/scoop/apt. PHP package manager. |

### Section 7 — Build / Dev workflow (8)

| File | category | display_name | notes |
|---|---|---|---|
| `bazel.yaml` | CLI | Bazel | github: `bazelbuild/bazel`. brew/scoop/choco. |
| `buf.yaml` | CLI | Buf | protobuf. github: `bufbuild/buf`. brew/scoop. |
| `make.yaml` | CLI | GNU Make | brew/winget/choco/apt. github: `mirror/make`. |
| `mage.yaml` | CLI | Mage | Go make. github: `magefile/mage`. brew/scoop. |
| `nx.yaml` | CLI | Nx | npm. github: `nrwl/nx`. |
| `turbo.yaml` | CLI | Turborepo | npm/brew. github: `vercel/turborepo`. |
| `earthly.yaml` | CLI | Earthly | github: `earthly/earthly`. brew/scoop/apt. |
| `goreleaser.yaml` | CLI | GoReleaser | github: `goreleaser/goreleaser`. brew/scoop/apt. |

### Section 8 — Networking / Server / Misc (8)

| File | category | display_name | notes |
|---|---|---|---|
| `caddy.yaml` | Network | Caddy | github: `caddyserver/caddy`. brew/scoop/apt. |
| `iperf3.yaml` | Network | iperf3 | github: `esnet/iperf`. brew/scoop/apt. |
| `socat.yaml` | Network | socat | brew/apt. No Windows. |
| `tcpdump.yaml` | Network | tcpdump | brew/apt. No Windows native. |
| `nomad.yaml` | IaC | Nomad | github: `hashicorp/nomad`. brew/winget. |
| `boundary.yaml` | Security | Boundary | github: `hashicorp/boundary`. brew/winget. |
| `yt-dlp.yaml` | Media | yt-dlp | github: `yt-dlp/yt-dlp`. brew/scoop/winget. |
| `croc.yaml` | CLI | croc | file transfer. github: `schollz/croc`. brew/scoop. |

**Section 8 total:** 8. With sections 1–7 (10+10+10+10+10+10+8) = **86 tools**. Slightly over budget;
acceptable.

---

## New tags (added to `marketplace/tags.yaml`)

Add to the appropriate facet group:

**Domain:**
- `edge` — Cloudflare/Fly/Vercel-style edge platforms
- `paas` — managed platforms
- `serverless`
- `messaging` — Kafka, NATS, RabbitMQ tools
- `protobuf`

**Function:**
- `migrations` — DB schema migration tools
- `password-manager`
- `sbom` — software bill of materials

(Reuse existing tags wherever possible; e.g. `auth` for IAM-ish tools, `scanner` for vulnerability
scanners.)

---

## New categories

None. All proposed tools fit existing categories.

---

## Pack updates

**Existing packs (additions only — no removals):**

| pack | additions |
|---|---|
| `ai-toolkit` | `gemini-cli`, `goose`, `cursor-agent`, `gptme` |
| `containers` | `podman`, `nerdctl`, `buildah` |
| `k8s-advanced` | `helmfile`, `kubeseal`, `krew`, `kompose`, `kn` |
| `security-toolkit` | `syft`, `osv-scanner`, `checkov`, `tfsec`, `nuclei`, `step`, `dockle` |
| `data-tools` | `miller`, `fx`, `gron`, `lnav`, `kcat` |
| `hashicorp-stack` | `nomad`, `boundary` |
| `python-developer` | `poetry`, `pipx`, `pyenv` |
| `node-developer` | `fnm`, `turbo`, `nx` (omit `nvm` since `fnm` is cross-platform) |
| `dev-productivity` | `croc`, `yt-dlp`, `make` |

**New packs:**

1. `edge-platforms.yaml` — `flyctl`, `vercel`, `netlify`, `wrangler`, `cloudflared`, `supabase`, `doctl`
2. `db-migrations.yaml` — `atlas`, `goose-migrate`, `dbmate`, `flyway`, `liquibase`
3. `supply-chain-security.yaml` — `syft`, `osv-scanner`, `cosign`, `trivy`, `grype`, `dockle`, `crane`

---

## Naming convention for conflicts

- `goose` → Block's AI agent (`block/goose`), category AI.
- `goose-migrate` → pressly's DB migration tool (`pressly/goose`), category Database. Binary
  remains `goose` (no rename of the binary; klim resolves by tool `name`, not binary).

Documented in tool YAML `display_name` so users see "goose (DB migrations)" in the TUI.

---

## File layout

```
marketplace/
├── tags.yaml                  # +8 new tags
├── tools/
│   ├── argo.yaml              # NEW
│   ├── atlas.yaml             # NEW
│   ├── bazel.yaml             # NEW
│   ├── boundary.yaml          # NEW
│   ├── buf.yaml               # NEW
│   ├── buildah.yaml           # NEW
│   ├── bw.yaml                # NEW
│   ├── caddy.yaml             # NEW
│   ├── checkov.yaml           # NEW
│   ├── cloudflared.yaml       # NEW
│   ├── codex.yaml             # NEW
│   ├── composer.yaml          # NEW
│   ├── crane.yaml             # NEW
│   ├── croc.yaml              # NEW
│   ├── cursor.yaml            # NEW
│   ├── cursor-agent.yaml      # NEW
│   ├── dbmate.yaml            # NEW
│   ├── doctl.yaml             # NEW
│   ├── dockle.yaml            # NEW
│   ├── earthly.yaml           # NEW
│   ├── elixir.yaml            # NEW
│   ├── emacs.yaml             # NEW
│   ├── flyctl.yaml            # NEW
│   ├── flyway.yaml            # NEW
│   ├── fnm.yaml               # NEW
│   ├── fx.yaml                # NEW
│   ├── gemini-cli.yaml        # NEW
│   ├── goose.yaml             # NEW (AI)
│   ├── goose-migrate.yaml     # NEW (DB)
│   ├── goreleaser.yaml        # NEW
│   ├── gptme.yaml             # NEW
│   ├── gron.yaml              # NEW
│   ├── helmfile.yaml          # NEW
│   ├── iperf3.yaml            # NEW
│   ├── kcat.yaml              # NEW
│   ├── kn.yaml                # NEW
│   ├── kompose.yaml           # NEW
│   ├── kotlin.yaml            # NEW
│   ├── krew.yaml              # NEW
│   ├── kubeseal.yaml          # NEW
│   ├── liquibase.yaml         # NEW
│   ├── lnav.yaml              # NEW
│   ├── lua.yaml               # NEW
│   ├── mage.yaml              # NEW
│   ├── make.yaml              # NEW
│   ├── marktext.yaml          # NEW
│   ├── micro.yaml             # NEW
│   ├── miller.yaml            # NEW
│   ├── nerdctl.yaml           # NEW
│   ├── netlify.yaml           # NEW
│   ├── nomad.yaml             # NEW
│   ├── nuclei.yaml            # NEW
│   ├── nvm.yaml               # NEW
│   ├── nx.yaml                # NEW
│   ├── op.yaml                # NEW
│   ├── osv-scanner.yaml       # NEW
│   ├── php.yaml               # NEW
│   ├── pipx.yaml              # NEW
│   ├── podman.yaml            # NEW
│   ├── poetry.yaml            # NEW
│   ├── pyenv.yaml             # NEW
│   ├── railway.yaml           # NEW
│   ├── render.yaml            # NEW
│   ├── socat.yaml             # NEW
│   ├── step.yaml              # NEW
│   ├── stripe.yaml            # NEW
│   ├── supabase.yaml          # NEW
│   ├── syft.yaml              # NEW
│   ├── tcpdump.yaml           # NEW
│   ├── tfsec.yaml             # NEW
│   ├── tilt.yaml              # NEW
│   ├── turbo.yaml             # NEW
│   ├── vercel.yaml            # NEW
│   ├── vim.yaml               # NEW
│   ├── wrangler.yaml          # NEW
│   ├── yt-dlp.yaml            # NEW
│   └── zed.yaml               # NEW
└── packs/
    ├── ai-toolkit.yaml             # MODIFIED
    ├── containers.yaml             # MODIFIED
    ├── data-tools.yaml             # MODIFIED
    ├── dev-productivity.yaml       # MODIFIED
    ├── hashicorp-stack.yaml        # MODIFIED
    ├── k8s-advanced.yaml           # MODIFIED
    ├── node-developer.yaml         # MODIFIED
    ├── python-developer.yaml       # MODIFIED
    ├── security-toolkit.yaml       # MODIFIED
    ├── db-migrations.yaml          # NEW
    ├── edge-platforms.yaml         # NEW
    └── supply-chain-security.yaml  # NEW
```

---

## Testing / validation

1. `make marketplace-validate` — must pass with zero schema errors.
2. `make build && bin/klim list` — sanity check that new tools appear.
3. `bin/klim install --pack edge-platforms --dry-run` (or equivalent) — confirm new packs resolve.
4. On Windows, macOS, Linux: spot-check 3–5 random new tools install via `klim install`.

---

## Risks / open questions

- **`goose` binary collision** — both tools install a binary called `goose` on PATH. klim
  resolves by tool name, but a user with both packs installed will have one `goose` shadow the
  other. Mitigation: note this in the display names; klim could later add a "conflicts with"
  field — out of scope here.
- **Some tools have no winget/scoop/choco package** (`socat`, `tcpdump`, `pyenv`, `nvm`,
  `bw`/`op`/`gptme` in places). Per the *lenient* policy, we still include them; the YAML
  omits Windows package keys. Users on Windows will see a "not available for this platform"
  message.
- **`cursor` (IDE)** has no GitHub repo (it's proprietary). The YAML will omit `github:` —
  verify the validator allows this; otherwise use a placeholder or skip.
- **`make` on Windows** — winget has `GnuWin32.Make`, but the canonical experience is via
  msys2/chocolatey. Acceptable trade-off.
- **`composer`** depends on `php` being present — listing PHP first in the spec is
  intentional but klim does not (yet) model tool dependencies.

---

## Out of scope (for follow-up specs)

- Tool dependency graphs (composer → php, krew → kubectl).
- Per-tool post-install hooks (e.g. `krew` needs PATH update).
- Schema extension for "conflicts with" / "alternative to".
- Marketplace browsing UI improvements to surface the new packs.
