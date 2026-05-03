Killer Features — Ranked by Impact

## 🏆 Tier 1 — Game-Changers

📋 **Team Manifests (.clim.yaml)** ✅ SHIPPED — project-aware tool detection, `clim check`/`clim init`, multi-project management, required + optional tools, CI integration with `--json` output, 70+ file type detection.

🩺 **`clim doctor` — Universal Environment Health Check** ✅ SHIPPED — Detects conflicting tool versions (multiple installations with different versions), broken/missing/inaccessible PATH directories, duplicate PATH entries, missing package managers, stale scan cache, unresolved versions, and outdated tools summary. CLI: `clim doctor` with `--json` and `--refresh` flags. TUI: dedicated Doctor tab (key 9) with scrollable color-coded output. Exit code 0 = no errors, 1 = errors found.

🐚 **Shell Integration — Hook + Completions** ✅ SHIPPED — (1) **Hook:** `eval "$(clim hook bash)"` (zsh/fish/powershell). Auto-runs `clim check` when you `cd` into a project with `.clim.yaml` — only shows output when tools are missing or outdated. The nvm/direnv model. (2) **Completions:** `clim completion bash|zsh|fish|powershell` — native tab completion for all commands, flags, and tool names via Cobra.

🔀 **`clim diff` — Environment Comparison** ✅ SHIPPED — Compare your installed tools against a manifest file (`clim diff my-tools.yaml`) or share token (`clim diff clim:v1:abc...`). Side-by-side table showing tool name, local version/source, remote version/source, and status (✓ match / ≠ differs / ← local only / → remote only). Exit code 0 = match, 1 = differences found. Supports `--refresh` for fresh scan.

⚡ **`clim proxy` — Auto-Install Shims** ✅ SHIPPED — `clim proxy setup` creates a managed shims directory. `clim proxy add kubectl terraform` creates lightweight shims that auto-install tools on first use via the best available package manager. Shims delegate to `clim proxy run` which finds the real binary (skipping the shims dir), or installs if missing, then executes transparently. Supports `setup`, `add`, `remove`, `list` subcommands. Cross-platform (`.cmd` on Windows, shell scripts on Unix).

🔐 **`clim audit` — Security, Compliance & SBOM** ✅ SHIPPED — Audits installed tools for: unmanaged installs (manual/unknown source), archived upstream repos, stale projects (no activity 12+ months), missing version info, outdated tools. Reports license inventory across your toolchain. `clim audit --sbom` generates CycloneDX 1.5 SBOM with tool metadata, licenses, source paths, and VCS references. `--json` for CI pipelines. Exit code 0 = clean, 1 = warnings found.

📸 **Environment Snapshots & Profiles** ✅ SHIPPED — `clim snapshot save/list/show/delete` for timestamped snapshots of installed tools. `clim snapshot profile save/list/show/delete` for named profiles ("work", "personal"). Snapshots stored under `~/.config/clim/snapshots/`, profiles under `~/.config/clim/profiles/`. Fuzzy name matching for show/delete. Built on the existing manifest format.

## 🥈 Tier 2 — Strong Differentiators

🎓 **`clim onboard` — Interactive Setup Wizard** ✅ SHIPPED — 6 dev roles (web, devops, data, mobile, systems, security). Scores marketplace tools by category/tag overlap + GitHub stars. Shows top 15 recommendations with descriptions. `clim onboard devops --list` for preview, or interactive mode with install prompt. Bulk installs via best available PM.

🔍 **`clim why <tool>` — Reverse Dependency Map** ✅ SHIPPED — Shows install status, version info, all references across .clim.yaml projects and packs, available package managers, and related installed tools by tag/category overlap.

🔔 **`clim watch` — Update Monitor** ✅ SHIPPED — `clim watch` does a fresh scan and reports all available updates. `--json` for machine-readable output. Designed for cron/Task Scheduler integration. Always forces a fresh scan for authoritative results.

🧪 **`clim try` — Tool Playground** ✅ SHIPPED — `clim try bat -- README.md` installs a tool, runs it with args, then asks "Keep or remove?". `--keep` flag to skip the prompt. If already installed, just runs it. Cleanup uses the correct PM remove command.

## 🧠 Tier 3 — Visionary / Long-term

🏅 **`clim score` — Environment Health Score** ✅ SHIPPED — Single 0-100 metric combining tool freshness (30pts), doctor health (25pts), audit findings (20pts), compliance (15pts), and managed sources (10pts). Grade scale A+ to F. CLI with `--json` for CI and `--badge` for shields.io URL. TUI Dashboard shows score gauge. Gamifies environment management.

📡 **Plugin System for Custom PMs** — Allow enterprises to add internal package managers (Artifactory, internal registries). Simple interface: `InstalledVersion()`, `LatestVersion()`, `InstallCmd()`. Custom marketplace URLs are ✅ SHIPPED via `clim marketplace add <url>` — multiple marketplace YAML sources are merged at load time. Full PM plugin interface is future work.

📊 **Smart History Analysis** — Opt-in: analyze shell history. "You ran `jq` 47 times last month but it's not in your favorites." Suggest tools based on actual usage, not just what's installed. "You haven't used terraform in 45 days. Remove?"

🤖 **AI Tool Discovery** — "I need to process JSON" → suggests jq, gron, fx, jless. Natural language search over the marketplace. Could use embeddings on tool descriptions + tags.

🏗️ **`clim generate`** ✅ SHIPPED — Auto-generate CI/container configs from `.clim.yaml`: `clim generate github-action` (workflow with install + verify steps), `clim generate dockerfile` (apt/brew/npm installs), `clim generate devcontainer` (VS Code / GitHub Codespaces with Dev Container Features mapping). Resolves tool names to package IDs from the marketplace. Supports `--output` for file writing and `--base` for custom Docker images.

---

## 🚀 Tier 1+ — AI-Era Moats (Proposed 2026-05)

Three ranked candidates for the next strategic moat. Each is a distinct
bet on where developer tooling is headed — pick one as the headline,
the other two are valid follow-ups.

🤖 **`clim mcp` — Model Context Protocol server** — Expose every clim
primitive (install, check, diff, audit, score, generate) over MCP so
AI coding agents (Claude Code, Cursor, GitHub Copilot CLI, Codeium,
Continue, etc.) can call them natively. `clim mcp serve --stdio` for
desktop agents, `clim mcp serve --http :7423` for multi-tenant.
Resources: `tools://installed`, `tools://catalog`, `manifest://current`,
`audit://current`, `score://current`. Tools: `clim.install`,
`clim.upgrade`, `clim.check`, `clim.diff`, `clim.search`, `clim.audit`.
Prompts: `setup_project_environment`, `review_audit_findings`,
`generate_devcontainer`. **Demo:** in Claude Code — "I just cloned a
Rust project that needs SQLite, install whatever I'm missing" →
agent calls `clim.check` → `clim.install sqlite3 cargo-watch` → done,
zero clim CLI knowledge required. **Why it's a moat:** strategic
timing (MCP becoming the standard in 2025), no competitor (asdf,
mise, brew, scoop, choco) has anything like it on roadmap, every
existing clim feature gets 10× more valuable for free. **Effort:**
~1500 LOC, 1–2 weeks; mostly a wire-protocol shim over existing
service / catalog / finder code.

🌍 **`clim sync` — End-to-end encrypted multi-machine sync** — "I just
got a new laptop" → 5 minutes → full toolchain restored.
`clim sync init` generates an X25519 keypair, prints a sync URL.
`clim sync push` encrypts the manifest + snapshot and uploads to a
relay. `clim sync pull` downloads, decrypts, and runs the install
plan. `clim sync watch` daemonizes auto-push on changes.
`clim sync team --org acme-corp` for shared org environments.
Three transport choices: built-in self-hosted relay (small Go server
on a VPS / S3 / R2), GitHub-backed (private gist or repo, no infra),
or local LAN (mDNS discovery, direct push between trusted machines).
**Demo:** two-laptop side-by-side, fresh macOS install, `curl ... |
sh` to bootstrap clim, `clim sync pull <url>`, watch 47 tools install
in parallel in ~3 min. **Why it's a moat:** solves a daily,
hair-on-fire pain (every dev sets up a new machine 1–3×/year and
dreads it); E2E + self-hostable differentiates from cloud-only
Devbox/GitPod-presets; team sync drives org-wide adoption.
**Effort:** ~3–4 weeks — real crypto, conflict resolution, transport
security, and self-hosted relay = ops burden.

🧠 **`clim analyze` + `doctor --fix` — Zero-config + auto-remediate**
— No more writing `.clim.yaml` by hand; no more "doctor said X is
broken, now what". `clim analyze .` reads `package.json` engines +
scripts, `Cargo.toml` + `rust-toolchain`, `go.mod` + `tools.go`,
`pyproject.toml` / `requirements.txt`, `Dockerfile` `RUN apt install
…`, `.github/workflows/setup-*` actions, `Makefile`, `bin/*` shebangs,
and README "Prerequisites" sections to infer the project's toolchain.
`clim analyze . --write` updates `.clim.yaml`. `clim analyze . --apply`
also runs the install plan. `clim doctor --fix` auto-remediates the
issues doctor already detects: appends missing PATH entries, removes
duplicate tool installs (keeping the newest), clears stale caches,
re-resolves versions. `--fix --dry-run` for preview. **Demo:**
`git clone some-project && cd $_ && clim analyze . --apply` — repo
goes from blank slate to full working environment in 90s. New
contributor `clim onboard` Just Works™. **Why it's a moat:** removes
the biggest friction in clim today (writing `.clim.yaml`); compounds
with shipped features into one fluent loop (analyze → check →
install → score). **Effort:** ~2–3 weeks for v1 (heuristic-only
analyzer, deterministic doctor fixers); v2 adds an optional LLM
inference layer for prose / edge cases.

### Recommendation

Ship **`clim mcp`** first. Best timing-vs-effort ratio, least new
infrastructure, hardest for competitors to copy without a year of
runway. The viral demo writes itself, and it makes every existing
clim feature reachable from every AI agent on day one.

---

## 🌟 Tier 0 — Category-Creating Ideas (Proposed 2026-05)

Beyond the AI-era moats above, four candidates that don't *extend*
clim — they make it a **new category of tool**. Each has a 30-second
demo that sells itself.

🐚 **`clim shell` — Project-pinned reproducible shell** — `nix-shell`
for the rest of us. Cross-platform, PM-agnostic, every tool — not
just languages. `clim shell` (or auto-drop on `cd` via the existing
hook) constructs a per-project shim directory under
`~/.local/share/clim/shells/<project-hash>/bin/` containing
version-pinned shims for every tool in `.clim.yaml`, layers it onto
PATH, applies any project env vars, and spawns `$SHELL` with a
`[clim:project]` prompt prefix. Inside the shell `kubectl` is
exactly the version pinned by the manifest, regardless of what the
system has installed. Exit and you're back to system tools. **Demo:**
two terminals side-by-side, different `kubectl --version` output in
each, just from `cd`-ing. **Why it's category-creating:** mise / asdf
only handle language runtimes, nix-shell / devbox require Nix,
direnv only handles env vars — nothing today gives you a
PM-agnostic, project-pinned shell that works for *any* tool
(`kubectl`, `terraform`, `jq`, custom binaries, `awscli`). Compounds
with everything shipped: hook drops you in on `cd`, manifests become
enforceable boundaries, audit/score become per-shell-scoped,
snapshots become shell captures. Once a team uses it, "works on my
machine" is dead. **Effort:** ~2–3 weeks; reuses proxy shims +
manifest + version resolution; new work is shell launching, prompt
injection, and PATH stacking.

🧬 **`clim trail` — Git for your dev environment** ✅ SHIPPED (Phase 1) —
Content-addressed snapshots of your toolchain with git-style verbs.
`clim trail capture [--label X]` records the current env;
`clim trail log` lists entries newest-first; `clim trail show <ref>`
displays the toolchain at a point; `clim trail diff <ref> [<ref>]`
shows added/removed/version-changed/source-changed; `clim trail prune
--keep N` trims and GCs orphan objects. Refs accept `HEAD`, `HEAD~N`,
`@<index>`, content hashes (full or 7+ char prefix), and labels.
Two captures of an identical env share storage automatically (canonical
hashing). All read verbs accept `--output json`. Cross-process file
locking guards `log.yaml`/`HEAD` updates; strict YAML decoding rejects
unknown fields and unknown schema versions. **Phase 2 (auto-capture on
install/upgrade/remove) and Phase 3 (`revert`, `bisect`) are
follow-ups.**

🪐 **`clim portal` — Install literally anything** — Marketplace gating
dies today. `clim portal install` accepts a GitHub release URL (asset
auto-picked for OS+arch), a tarball URL, a `pip` / `cargo` / `npm`
package, a one-line installer script (with `--audit-script` and
sandbox), or any HTTPS binary URL. After install the tool is
first-class — `clim list` shows it, `clim watch` tracks updates from
its source, `clim audit` includes it, `clim share`/`export`
round-trips it. **Demo:** `clim portal install
https://github.com/sharkdp/bat/releases/download/v0.24.0/...zip` →
`bat` is now in your PATH and clim knows about it. **Why it's
category-creating:** every "this tool isn't in your catalog"
complaint disappears; your marketplace becomes infinite. With MCP,
AI agents can install any binary from any source — safely,
auditably, reversibly. **Effort:** ~3–4 weeks; multi-source resolver,
asset-matching heuristics, sandbox for install scripts, version
detection from arbitrary binaries.

🌀 **`clim warp` — Share your env as a link** — Mid-pair-debug over
Zoom: "I can't reproduce it" → you `clim warp` → terminal prints
`clim:warp:abc123…` → paste in chat → they `clim warp open
abc123…` → 2 min later they have your *exact* tool versions, env
vars, PATH order, optionally shell history. End-to-end encrypted,
expires after 24h. Built on `clim share` + `clim sync`. **Why it's
category-creating:** turns environment-state into a URL.
Pair-debug-as-a-service. Support teams can ask customers to
`clim warp` their broken env. **Effort:** ~2 weeks on top of
`clim sync`.

### Recommendation (Tier 0)

Ship **`clim shell`** + **`clim trail`** as a coordinated narrative:
**"git for your dev environment"**. Both are achievable in ~4 weeks
combined, and together they form a category-defining story that no
competitor can match in a single release cycle.

`clim mcp` (Tier 1+) and `clim shell` + `clim trail` (Tier 0) are
**not** alternatives — they layer cleanly. MCP gives AI agents a
voice; shell+trail give the human a reproducible, time-travel-able
substrate for that voice to act on.