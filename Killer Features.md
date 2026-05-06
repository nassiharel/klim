Killer Features — Ranked by Impact

## 🏆 Tier 1 — Game-Changers

📋 **Team Manifests (.klim.yaml)** ✅ SHIPPED — project-aware tool detection, `klim check`/`klim init`, multi-project management, required + optional tools, CI integration with `--json` output, 70+ file type detection.

🩺 **`klim doctor` — Universal Environment Health Check** ✅ SHIPPED — Detects conflicting tool versions (multiple installations with different versions), broken/missing/inaccessible PATH directories, duplicate PATH entries, missing package managers, stale scan cache, unresolved versions, and outdated tools summary. CLI: `klim doctor` with `--json` and `--refresh` flags. TUI: dedicated Doctor tab (key 9) with scrollable color-coded output. Exit code 0 = no errors, 1 = errors found.

🐚 **Shell Integration — Hook + Completions** ✅ SHIPPED — (1) **Hook:** `eval "$(klim hook bash)"` (zsh/fish/powershell). Auto-runs `klim check` when you `cd` into a project with `.klim.yaml` — only shows output when tools are missing or outdated. The nvm/direnv model. (2) **Completions:** `klim completion bash|zsh|fish|powershell` — native tab completion for all commands, flags, and tool names via Cobra.

🔀 **`klim diff` — Environment Comparison** ✅ SHIPPED — Compare your installed tools against a manifest file (`klim diff my-tools.yaml`) or share token (`klim diff klim:v1:abc...`). Side-by-side table showing tool name, local version/source, remote version/source, and status (✓ match / ≠ differs / ← local only / → remote only). Exit code 0 = match, 1 = differences found. Supports `--refresh` for fresh scan.

⚡ **`klim proxy` — Auto-Install Shims** ✅ SHIPPED — `klim proxy setup` creates a managed shims directory. `klim proxy add kubectl terraform` creates lightweight shims that auto-install tools on first use via the best available package manager. Shims delegate to `klim proxy run` which finds the real binary (skipping the shims dir), or installs if missing, then executes transparently. Supports `setup`, `add`, `remove`, `list` subcommands. Cross-platform (`.cmd` on Windows, shell scripts on Unix).

🔐 **`klim audit` — Security, Compliance & SBOM** ✅ SHIPPED — Audits installed tools for: unmanaged installs (manual/unknown source), archived upstream repos, stale projects (no activity 12+ months), missing version info, outdated tools. Reports license inventory across your toolchain. `klim audit --sbom` generates CycloneDX 1.5 SBOM with tool metadata, licenses, source paths, and VCS references. `--json` for CI pipelines. Exit code 0 = clean, 1 = warnings found.

📸 **Environment Snapshots & Profiles** ✅ SHIPPED — `klim snapshot save/list/show/delete` for timestamped snapshots of installed tools. `klim snapshot profile save/list/show/delete` for named profiles ("work", "personal"). Snapshots stored under `~/.config/klim/snapshots/`, profiles under `~/.config/klim/profiles/`. Fuzzy name matching for show/delete. Built on the existing manifest format.

## 🥈 Tier 2 — Strong Differentiators

🎓 **`klim onboard` — Interactive Setup Wizard** ✅ SHIPPED — 6 dev roles (web, devops, data, mobile, systems, security). Scores marketplace tools by category/tag overlap + GitHub stars. Shows top 15 recommendations with descriptions. `klim onboard devops --list` for preview, or interactive mode with install prompt. Bulk installs via best available PM.

🔍 **`klim why <tool>` — Reverse Dependency Map** ✅ SHIPPED — Shows install status, version info, all references across .klim.yaml projects and packs, available package managers, and related installed tools by tag/category overlap.

🔔 **`klim watch` — Update Monitor** ✅ SHIPPED — `klim watch` does a fresh scan and reports all available updates. `--json` for machine-readable output. Designed for cron/Task Scheduler integration. Always forces a fresh scan for authoritative results.

🏎️ **`klim benchmark` — PM Speed Comparison** — `klim benchmark terraform` → "scoop: install 4.2s, query 0.8s ★ fastest / winget: install 12.1s, query 2.3s". Recommendation: "Switch terraform to scoop for 2.9x faster installs."

🧪 **`klim try` — Tool Playground** ✅ SHIPPED — `klim try bat -- README.md` installs a tool, runs it with args, then asks "Keep or remove?". `--keep` flag to skip the prompt. If already installed, just runs it. Cleanup uses the correct PM remove command.

## 🧠 Tier 3 — Visionary / Long-term

🏅 **`klim score` — Environment Health Score** ✅ SHIPPED — Single 0-100 metric combining tool freshness (30pts), doctor health (25pts), audit findings (20pts), compliance (15pts), and managed sources (10pts). Grade scale A+ to F. CLI with `--json` for CI and `--badge` for shields.io URL. TUI Dashboard shows score gauge. Gamifies environment management.

📡 **Plugin System for Custom PMs** — Allow enterprises to add internal package managers (Artifactory, internal registries). Simple interface: `InstalledVersion()`, `LatestVersion()`, `InstallCmd()`. Custom marketplace URLs are ✅ SHIPPED via `klim marketplace add <url>` — multiple marketplace YAML sources are merged at load time. Full PM plugin interface is future work.

📊 **Smart History Analysis** — Opt-in: analyze shell history. "You ran `jq` 47 times last month but it's not in your favorites." Suggest tools based on actual usage, not just what's installed. "You haven't used terraform in 45 days. Remove?"

🤖 **AI Tool Discovery** — "I need to process JSON" → suggests jq, gron, fx, jless. Natural language search over the marketplace. Could use embeddings on tool descriptions + tags.

🏗️ **`klim generate`** ✅ SHIPPED — Auto-generate CI/container configs from `.klim.yaml`: `klim generate github-action` (workflow with install + verify steps), `klim generate dockerfile` (apt/brew/npm installs), `klim generate devcontainer` (VS Code / GitHub Codespaces with Dev Container Features mapping). Resolves tool names to package IDs from the marketplace. Supports `--output` for file writing and `--base` for custom Docker images.

---

## 🚀 Tier 1+ — AI-Era Moats (Proposed 2026-05)

Three ranked candidates for the next strategic moat. Each is a distinct
bet on where developer tooling is headed — pick one as the headline,
the other two are valid follow-ups.

🤖 **`klim mcp` — Model Context Protocol server** — Expose every klim
primitive (install, check, diff, audit, score, generate) over MCP so
AI coding agents (Claude Code, Cursor, GitHub Copilot CLI, Codeium,
Continue, etc.) can call them natively. `klim mcp serve --stdio` for
desktop agents, `klim mcp serve --http :7423` for multi-tenant.
Resources: `tools://installed`, `tools://catalog`, `manifest://current`,
`audit://current`, `score://current`. Tools: `klim.install`,
`klim.upgrade`, `klim.check`, `klim.diff`, `klim.search`, `klim.audit`.
Prompts: `setup_project_environment`, `review_audit_findings`,
`generate_devcontainer`. **Demo:** in Claude Code — "I just cloned a
Rust project that needs SQLite, install whatever I'm missing" →
agent calls `klim.check` → `klim.install sqlite3 cargo-watch` → done,
zero klim CLI knowledge required. **Why it's a moat:** strategic
timing (MCP becoming the standard in 2025), no competitor (asdf,
mise, brew, scoop, choco) has anything like it on roadmap, every
existing klim feature gets 10× more valuable for free. **Effort:**
~1500 LOC, 1–2 weeks; mostly a wire-protocol shim over existing
service / catalog / finder code.

🌍 **`klim sync` — End-to-end encrypted multi-machine sync** — "I just
got a new laptop" → 5 minutes → full toolchain restored.
`klim sync init` generates an X25519 keypair, prints a sync URL.
`klim sync push` encrypts the manifest + snapshot and uploads to a
relay. `klim sync pull` downloads, decrypts, and runs the install
plan. `klim sync watch` daemonizes auto-push on changes.
`klim sync team --org acme-corp` for shared org environments.
Three transport choices: built-in self-hosted relay (small Go server
on a VPS / S3 / R2), GitHub-backed (private gist or repo, no infra),
or local LAN (mDNS discovery, direct push between trusted machines).
**Demo:** two-laptop side-by-side, fresh macOS install, `curl ... |
sh` to bootstrap klim, `klim sync pull <url>`, watch 47 tools install
in parallel in ~3 min. **Why it's a moat:** solves a daily,
hair-on-fire pain (every dev sets up a new machine 1–3×/year and
dreads it); E2E + self-hostable differentiates from cloud-only
Devbox/GitPod-presets; team sync drives org-wide adoption.
**Effort:** ~3–4 weeks — real crypto, conflict resolution, transport
security, and self-hosted relay = ops burden.

🧠 **`klim analyze` + `doctor --fix` — Zero-config + auto-remediate**
— No more writing `.klim.yaml` by hand; no more "doctor said X is
broken, now what". `klim analyze .` reads `package.json` engines +
scripts, `Cargo.toml` + `rust-toolchain`, `go.mod` + `tools.go`,
`pyproject.toml` / `requirements.txt`, `Dockerfile` `RUN apt install
…`, `.github/workflows/setup-*` actions, `Makefile`, `bin/*` shebangs,
and README "Prerequisites" sections to infer the project's toolchain.
`klim analyze . --write` updates `.klim.yaml`. `klim analyze . --apply`
also runs the install plan. `klim doctor --fix` auto-remediates the
issues doctor already detects: appends missing PATH entries, removes
duplicate tool installs (keeping the newest), clears stale caches,
re-resolves versions. `--fix --dry-run` for preview. **Demo:**
`git clone some-project && cd $_ && klim analyze . --apply` — repo
goes from blank slate to full working environment in 90s. New
contributor `klim onboard` Just Works™. **Why it's a moat:** removes
the biggest friction in klim today (writing `.klim.yaml`); compounds
with shipped features into one fluent loop (analyze → check →
install → score). **Effort:** ~2–3 weeks for v1 (heuristic-only
analyzer, deterministic doctor fixers); v2 adds an optional LLM
inference layer for prose / edge cases.

### Recommendation

Ship **`klim mcp`** first. Best timing-vs-effort ratio, least new
infrastructure, hardest for competitors to copy without a year of
runway. The viral demo writes itself, and it makes every existing
klim feature reachable from every AI agent on day one.

---

## 🌟 Tier 0 — Category-Creating Ideas (Proposed 2026-05)

Beyond the AI-era moats above, four candidates that don't *extend*
klim — they make it a **new category of tool**. Each has a 30-second
demo that sells itself.

🐚 **`klim shell` — Project-pinned reproducible shell** — `nix-shell`
for the rest of us. Cross-platform, PM-agnostic, every tool — not
just languages. `klim shell` (or auto-drop on `cd` via the existing
hook) constructs a per-project shim directory under
`~/.local/share/klim/shells/<project-hash>/bin/` containing
version-pinned shims for every tool in `.klim.yaml`, layers it onto
PATH, applies any project env vars, and spawns `$SHELL` with a
`[klim:project]` prompt prefix. Inside the shell `kubectl` is
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

🧬 **`klim trail` — Git for your dev environment** ✅ SHIPPED (Phase 1) —
Content-addressed snapshots of your toolchain with git-style verbs.
`klim trail capture [--label X]` records the current env;
`klim trail log` lists entries newest-first; `klim trail show <ref>`
displays the toolchain at a point; `klim trail diff <ref> [<ref>]`
shows added/removed/version-changed/source-changed; `klim trail prune
--keep N` trims and GCs orphan objects. Refs accept `HEAD`, `HEAD~N`,
`@<index>`, content hashes (full or 7+ char prefix), and labels.
Two captures of an identical env share storage automatically (canonical
hashing). All read verbs accept `--output json`. Cross-process file
locking guards `log.yaml`/`HEAD` updates; strict YAML decoding rejects
unknown fields and unknown schema versions. **Phase 2 (auto-capture on
install/upgrade/remove) and Phase 3 (`revert`, `bisect`) are
follow-ups.**

🪐 **`klim portal` — Install literally anything** — Marketplace gating
dies today. `klim portal install` accepts a GitHub release URL (asset
auto-picked for OS+arch), a tarball URL, a `pip` / `cargo` / `npm`
package, a one-line installer script (with `--audit-script` and
sandbox), or any HTTPS binary URL. After install the tool is
first-class — `klim list` shows it, `klim watch` tracks updates from
its source, `klim audit` includes it, `klim share`/`export`
round-trips it. **Demo:** `klim portal install
https://github.com/sharkdp/bat/releases/download/v0.24.0/...zip` →
`bat` is now in your PATH and klim knows about it. **Why it's
category-creating:** every "this tool isn't in your catalog"
complaint disappears; your marketplace becomes infinite. With MCP,
AI agents can install any binary from any source — safely,
auditably, reversibly. **Effort:** ~3–4 weeks; multi-source resolver,
asset-matching heuristics, sandbox for install scripts, version
detection from arbitrary binaries.

🌀 **`klim warp` — Share your env as a link** — Mid-pair-debug over
Zoom: "I can't reproduce it" → you `klim warp` → terminal prints
`klim:warp:abc123…` → paste in chat → they `klim warp open
abc123…` → 2 min later they have your *exact* tool versions, env
vars, PATH order, optionally shell history. End-to-end encrypted,
expires after 24h. Built on `klim share` + `klim sync`. **Why it's
category-creating:** turns environment-state into a URL.
Pair-debug-as-a-service. Support teams can ask customers to
`klim warp` their broken env. **Effort:** ~2 weeks on top of
`klim sync`.

### Recommendation (Tier 0)

Ship **`klim shell`** + **`klim trail`** as a coordinated narrative:
**"git for your dev environment"**. Both are achievable in ~4 weeks
combined, and together they form a category-defining story that no
competitor can match in a single release cycle.

`klim mcp` (Tier 1+) and `klim shell` + `klim trail` (Tier 0) are
**not** alternatives — they layer cleanly. MCP gives AI agents a
voice; shell+trail give the human a reproducible, time-travel-able
substrate for that voice to act on.