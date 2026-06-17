# Marketplace tools additions — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add 86 new tool YAMLs, 8 new tags, 9 pack updates, and 3 new packs to klim's marketplace, fully validated by `make marketplace-validate`.

**Architecture:** Each tool is a self-contained YAML file in `marketplace/tools/`. Packs reference tools by name. New tags MUST exist in `marketplace/tags.yaml` BEFORE any tool references them, otherwise the validator rejects the tool. This dictates ordering: tags first, then tools, then pack updates.

**Tech Stack:** YAML (gopkg.in/yaml.v3 strict mode), Go validator (`internal/marketplace/validate`), `make marketplace-validate`.

**Spec reference:** [`docs/superpowers/specs/2026-06-17-marketplace-tools-additions-design.md`](../specs/2026-06-17-marketplace-tools-additions-design.md)

---

## Schema reminders (read before starting)

The validator (`internal/marketplace/validate/main.go`) enforces:

1. **Allowed package manager keys** (and ONLY these): `winget`, `choco`, `scoop`, `brew`, `apt`, `snap`, `npm`.
   - **`pip`/`pipx`/`cargo`/`go` are NOT supported keys** — the spec mentions pipx/pip for some tools (checkov, gptme, etc.) but the YAML must only use the allowed keys. For those tools, use whatever standard PMs apply (brew/apt) and rely on the GitHub upstream as the install path of last resort.
2. **`name` regex:** `^[a-z0-9][a-z0-9-]*$` (lowercase + digits + hyphens).
3. **Filename must match `<name>.yaml`** exactly.
4. **Category must be in `categories.yaml`.** Allowed: VCS, Cloud, Containers, IaC, Language, PkgMgr, Editor, CLI, Media, Database, Network, Shell, AI, Security, Testing, Observability.
5. **Tags must be in `tags.yaml`.** Unknown tag → validation fails.
6. **Strict YAML decoding (`KnownFields(true)`):** unknown top-level fields cause errors. Only these fields are valid: `name`, `display_name`, `category`, `tags`, `binary_names`, `packages`, `github`.
7. **At least one package manager OR npm key must be set.**
8. **`github` is optional** but if set must match `owner/repo`.
9. **Pack references must point to existing tools** (validated cross-file).

YAML style (match existing files, e.g. `bat.yaml`):
- 4-space indent for list items under a key
- Single newline at EOF
- No trailing blank lines, no quotes unless required

---

## File Structure

**New files (86 tools + 3 packs):**
```
marketplace/tools/
  argo.yaml, atlas.yaml, bazel.yaml, boundary.yaml, buf.yaml, buildah.yaml,
  bw.yaml, caddy.yaml, checkov.yaml, cloudflared.yaml, codex.yaml,
  composer.yaml, crane.yaml, croc.yaml, cursor.yaml, cursor-agent.yaml,
  dbmate.yaml, doctl.yaml, dockle.yaml, earthly.yaml, elixir.yaml,
  emacs.yaml, flyctl.yaml, flyway.yaml, fnm.yaml, fx.yaml, gemini-cli.yaml,
  goose.yaml, goose-migrate.yaml, goreleaser.yaml, gptme.yaml, gron.yaml,
  helmfile.yaml, iperf3.yaml, kcat.yaml, kn.yaml, kompose.yaml, kotlin.yaml,
  krew.yaml, kubeseal.yaml, liquibase.yaml, lnav.yaml, lua.yaml, mage.yaml,
  make.yaml, marktext.yaml, micro.yaml, miller.yaml, nerdctl.yaml,
  netlify.yaml, nomad.yaml, nuclei.yaml, nvm.yaml, nx.yaml, op.yaml,
  osv-scanner.yaml, php.yaml, pipx.yaml, podman.yaml, poetry.yaml,
  pyenv.yaml, railway.yaml, render.yaml, socat.yaml, step.yaml, stripe.yaml,
  supabase.yaml, syft.yaml, tcpdump.yaml, tfsec.yaml, tilt.yaml, turbo.yaml,
  vercel.yaml, vim.yaml, wrangler.yaml, yt-dlp.yaml, zed.yaml

marketplace/packs/
  db-migrations.yaml, edge-platforms.yaml, supply-chain-security.yaml
```

**Modified files:**
```
marketplace/tags.yaml                 # +8 tags
marketplace/packs/ai-toolkit.yaml
marketplace/packs/containers.yaml
marketplace/packs/data-tools.yaml
marketplace/packs/dev-productivity.yaml
marketplace/packs/hashicorp-stack.yaml
marketplace/packs/k8s-advanced.yaml
marketplace/packs/node-developer.yaml
marketplace/packs/python-developer.yaml
marketplace/packs/security-toolkit.yaml
```

---

## Task ordering (CRITICAL)

Tags first → tools → pack updates → new packs → validate. If you add a tool that uses tag `edge` before adding `edge` to `tags.yaml`, validation fails. Commit at the end of each Task block (NOT after every micro-step within a task).

---

## Task 1: Add new tags to `tags.yaml`

**Files:**
- Modify: `marketplace/tags.yaml`

- [ ] **Step 1.1: Read current tags.yaml**

Run: `cat marketplace/tags.yaml`
Expected: Sections "Domain", "Language", "Function", "UX", "Vendor".

- [ ] **Step 1.2: Add 5 new Domain tags (alphabetical within section)**

Edit `marketplace/tags.yaml`. Under the `# --- Domain ---` block, insert these in alphabetical order:
- `edge` (after `docker`)
- `messaging` (after `kubernetes`)
- `paas` (after `observability`)
- `protobuf` (after `paas`)
- `serverless` (after `security`)

After edits, the Domain block should read:
```yaml
  # --- Domain ---
  - api
  - cloud
  - container
  - database
  - docker
  - edge
  - git
  - gitops
  - kubernetes
  - messaging
  - networking
  - observability
  - paas
  - protobuf
  - security
  - serverless
  - service-mesh
  - terraform
  - vcs
```

- [ ] **Step 1.3: Add 3 new Function tags (alphabetical within section)**

Under `# --- Function ---`, insert:
- `migrations` (between `media` and `package-manager`)
- `password-manager` (between `package-manager` and `process-monitoring`)
- `sbom` (between `remote-shell` and `scanner`)

After edits, ensure these new entries sit in correct alphabetical position relative to the existing entries. (Verify with `grep -n "  - " marketplace/tags.yaml` after the edit.)

- [ ] **Step 1.4: Validate**

Run: `make marketplace-validate`
Expected: `Marketplace validated: 162 tools, 24 packs, ... All checks passed.`

If validation fails on tag duplication, re-check the alphabetical inserts.

- [ ] **Step 1.5: Commit**

```bash
git add marketplace/tags.yaml
git commit -m "feat(marketplace): add 8 new tags for upcoming tools"
```

---

## Task 2: Add Section 1 tools — AI & Editors (11 files)

**Files:** all under `marketplace/tools/`

### Step 2.1 — Create `gemini-cli.yaml`

```yaml
name: gemini-cli
display_name: Gemini CLI
category: AI
tags:
    - ai
    - cli
    - google
binary_names:
    - gemini
packages:
    npm: "@google/gemini-cli"
    brew: gemini-cli
github: google-gemini/gemini-cli
```

**Note:** `google` tag exists in tags.yaml (Vendor). Verify with `grep '^  - google' marketplace/tags.yaml`.

### Step 2.2 — Create `goose.yaml`

```yaml
name: goose
display_name: goose (AI agent)
category: AI
tags:
    - ai
    - coding-assistant
    - cli
binary_names:
    - goose
packages:
    brew: block-goose-cli
github: block/goose
```

### Step 2.3 — Create `gptme.yaml`

```yaml
name: gptme
display_name: gptme
category: AI
tags:
    - ai
    - cli
binary_names:
    - gptme
packages:
    brew: gptme
github: gptme/gptme
```

### Step 2.4 — Create `codex.yaml`

```yaml
name: codex
display_name: OpenAI Codex CLI
category: AI
tags:
    - ai
    - coding-assistant
    - cli
binary_names:
    - codex
packages:
    npm: "@openai/codex"
    brew: codex
github: openai/codex
```

### Step 2.5 — Create `cursor-agent.yaml`

```yaml
name: cursor-agent
display_name: Cursor Agent
category: AI
tags:
    - ai
    - coding-assistant
    - cli
binary_names:
    - cursor-agent
packages:
    brew: cursor-agent
github: cursor/cursor
```

### Step 2.6 — Create `cursor.yaml`

Cursor IDE has no public GitHub repo. Omit the `github:` key entirely.

```yaml
name: cursor
display_name: Cursor
category: Editor
tags:
    - ide
    - gui
    - ai
binary_names:
    - cursor
packages:
    winget: Anysphere.Cursor
    brew: cursor
```

### Step 2.7 — Create `zed.yaml`

```yaml
name: zed
display_name: Zed
category: Editor
tags:
    - ide
    - gui
    - rust
binary_names:
    - zed
packages:
    brew: zed
    scoop: zed
github: zed-industries/zed
```

### Step 2.8 — Create `vim.yaml`

```yaml
name: vim
display_name: Vim
category: Editor
tags:
    - ide
    - tui
binary_names:
    - vim
packages:
    winget: vim.vim
    scoop: vim
    choco: vim
    brew: vim
    apt: vim
github: vim/vim
```

### Step 2.9 — Create `emacs.yaml`

```yaml
name: emacs
display_name: GNU Emacs
category: Editor
tags:
    - ide
    - tui
    - gui
binary_names:
    - emacs
packages:
    winget: GNU.Emacs
    scoop: emacs
    choco: emacs
    brew: emacs
    apt: emacs
    snap: emacs
github: emacs-mirror/emacs
```

### Step 2.10 — Create `micro.yaml`

```yaml
name: micro
display_name: micro
category: Editor
tags:
    - ide
    - tui
binary_names:
    - micro
packages:
    winget: zyedidia.micro
    scoop: micro
    choco: micro
    brew: micro
    snap: micro
github: zyedidia/micro
```

### Step 2.11 — Create `marktext.yaml`

```yaml
name: marktext
display_name: MarkText
category: Editor
tags:
    - gui
    - docs
    - cross-platform
binary_names:
    - marktext
packages:
    winget: marktext.marktext
    choco: marktext
    brew: marktext
github: marktext/marktext
```

- [ ] **Step 2.12: Validate**

Run: `make marketplace-validate`
Expected: `... 173 tools, 24 packs, ... All checks passed.`

Common failure modes:
- "unknown tag X" → not in tags.yaml; check spelling
- "invalid category X" → must be one of the 17 listed in categories.yaml
- "filename must match name field" → check `name:` matches filename

- [ ] **Step 2.13: Commit**

```bash
git add marketplace/tools/gemini-cli.yaml marketplace/tools/goose.yaml \
        marketplace/tools/gptme.yaml marketplace/tools/codex.yaml \
        marketplace/tools/cursor-agent.yaml marketplace/tools/cursor.yaml \
        marketplace/tools/zed.yaml marketplace/tools/vim.yaml \
        marketplace/tools/emacs.yaml marketplace/tools/micro.yaml \
        marketplace/tools/marktext.yaml
git commit -m "feat(marketplace): add 11 AI assistants and editors"
```

---

## Task 3: Add Section 2 tools — Cloud / Edge Platforms (10 files)

### Step 3.1 — Create `flyctl.yaml`

```yaml
name: flyctl
display_name: flyctl
category: Cloud
tags:
    - cloud
    - paas
    - edge
binary_names:
    - fly
    - flyctl
packages:
    brew: flyctl
    scoop: flyctl
github: superfly/flyctl
```

### Step 3.2 — Create `doctl.yaml`

```yaml
name: doctl
display_name: doctl
category: Cloud
tags:
    - cloud
    - cli
binary_names:
    - doctl
packages:
    winget: DigitalOcean.doctl
    scoop: doctl
    brew: doctl
    snap: doctl
github: digitalocean/doctl
```

### Step 3.3 — Create `vercel.yaml`

```yaml
name: vercel
display_name: Vercel CLI
category: Cloud
tags:
    - paas
    - edge
    - serverless
binary_names:
    - vercel
packages:
    npm: vercel
github: vercel/vercel
```

### Step 3.4 — Create `netlify.yaml`

```yaml
name: netlify
display_name: Netlify CLI
category: Cloud
tags:
    - paas
    - edge
    - serverless
binary_names:
    - netlify
packages:
    npm: netlify-cli
    brew: netlify-cli
github: netlify/cli
```

### Step 3.5 — Create `wrangler.yaml`

```yaml
name: wrangler
display_name: Cloudflare Wrangler
category: Cloud
tags:
    - edge
    - serverless
    - cloud
binary_names:
    - wrangler
packages:
    npm: wrangler
github: cloudflare/workers-sdk
```

### Step 3.6 — Create `supabase.yaml`

```yaml
name: supabase
display_name: Supabase CLI
category: Cloud
tags:
    - cloud
    - database
    - paas
binary_names:
    - supabase
packages:
    winget: Supabase.cli
    scoop: supabase
    brew: supabase
    npm: supabase
github: supabase/cli
```

### Step 3.7 — Create `cloudflared.yaml`

```yaml
name: cloudflared
display_name: cloudflared
category: Network
tags:
    - networking
    - tunneling
    - edge
binary_names:
    - cloudflared
packages:
    winget: Cloudflare.cloudflared
    brew: cloudflared
    apt: cloudflared
github: cloudflare/cloudflared
```

### Step 3.8 — Create `stripe.yaml`

```yaml
name: stripe
display_name: Stripe CLI
category: CLI
tags:
    - api
    - cli
binary_names:
    - stripe
packages:
    scoop: stripe
    brew: stripe
    apt: stripe
github: stripe/stripe-cli
```

### Step 3.9 — Create `railway.yaml`

```yaml
name: railway
display_name: Railway CLI
category: Cloud
tags:
    - paas
    - cloud
    - cli
binary_names:
    - railway
packages:
    scoop: railway
    brew: railway
    npm: "@railway/cli"
github: railwayapp/cli
```

### Step 3.10 — Create `render.yaml`

```yaml
name: render
display_name: Render CLI
category: Cloud
tags:
    - paas
    - cloud
binary_names:
    - render
packages:
    brew: render
github: render-oss/cli
```

- [ ] **Step 3.11: Validate**

Run: `make marketplace-validate`
Expected: `... 183 tools, 24 packs, ... All checks passed.`

- [ ] **Step 3.12: Commit**

```bash
git add marketplace/tools/flyctl.yaml marketplace/tools/doctl.yaml \
        marketplace/tools/vercel.yaml marketplace/tools/netlify.yaml \
        marketplace/tools/wrangler.yaml marketplace/tools/supabase.yaml \
        marketplace/tools/cloudflared.yaml marketplace/tools/stripe.yaml \
        marketplace/tools/railway.yaml marketplace/tools/render.yaml
git commit -m "feat(marketplace): add 10 cloud / edge platform CLIs"
```

---

## Task 4: Add Section 3 tools — Containers / Kubernetes (10 files)

### Step 4.1 — Create `podman.yaml`

```yaml
name: podman
display_name: Podman
category: Containers
tags:
    - container
    - docker
binary_names:
    - podman
packages:
    winget: RedHat.Podman
    scoop: podman
    choco: podman-cli
    brew: podman
    apt: podman
github: containers/podman
```

### Step 4.2 — Create `nerdctl.yaml`

```yaml
name: nerdctl
display_name: nerdctl
category: Containers
tags:
    - container
    - docker
binary_names:
    - nerdctl
packages:
    brew: nerdctl
    scoop: nerdctl
github: containerd/nerdctl
```

### Step 4.3 — Create `buildah.yaml`

```yaml
name: buildah
display_name: Buildah
category: Containers
tags:
    - container
    - image-builder
binary_names:
    - buildah
packages:
    brew: buildah
    apt: buildah
github: containers/buildah
```

### Step 4.4 — Create `helmfile.yaml`

```yaml
name: helmfile
display_name: Helmfile
category: Containers
tags:
    - kubernetes
    - configuration-management
binary_names:
    - helmfile
packages:
    brew: helmfile
    scoop: helmfile
github: helmfile/helmfile
```

### Step 4.5 — Create `kubeseal.yaml`

```yaml
name: kubeseal
display_name: Sealed Secrets (kubeseal)
category: Containers
tags:
    - kubernetes
    - secrets
    - encryption
binary_names:
    - kubeseal
packages:
    brew: kubeseal
github: bitnami-labs/sealed-secrets
```

### Step 4.6 — Create `krew.yaml`

```yaml
name: krew
display_name: krew
category: Containers
tags:
    - kubernetes
    - package-manager
binary_names:
    - kubectl-krew
packages:
    brew: krew
    scoop: krew
github: kubernetes-sigs/krew
```

### Step 4.7 — Create `kompose.yaml`

```yaml
name: kompose
display_name: Kompose
category: Containers
tags:
    - kubernetes
    - docker
    - configuration-management
binary_names:
    - kompose
packages:
    brew: kompose
    scoop: kompose
    choco: kubernetes-kompose
    apt: kompose
github: kubernetes/kompose
```

### Step 4.8 — Create `kn.yaml`

```yaml
name: kn
display_name: Knative Client
category: Containers
tags:
    - kubernetes
    - serverless
binary_names:
    - kn
packages:
    brew: knative-client
github: knative/client
```

### Step 4.9 — Create `argo.yaml`

```yaml
name: argo
display_name: Argo Workflows CLI
category: Containers
tags:
    - kubernetes
    - gitops
    - automation
binary_names:
    - argo
packages:
    brew: argo
github: argoproj/argo-workflows
```

### Step 4.10 — Create `tilt.yaml`

```yaml
name: tilt
display_name: Tilt
category: Containers
tags:
    - kubernetes
    - dev-workflow
    - local-cluster
binary_names:
    - tilt
packages:
    brew: tilt
    scoop: tilt
github: tilt-dev/tilt
```

- [ ] **Step 4.11: Validate**

Run: `make marketplace-validate`
Expected: `... 193 tools, 24 packs, ... All checks passed.`

- [ ] **Step 4.12: Commit**

```bash
git add marketplace/tools/podman.yaml marketplace/tools/nerdctl.yaml \
        marketplace/tools/buildah.yaml marketplace/tools/helmfile.yaml \
        marketplace/tools/kubeseal.yaml marketplace/tools/krew.yaml \
        marketplace/tools/kompose.yaml marketplace/tools/kn.yaml \
        marketplace/tools/argo.yaml marketplace/tools/tilt.yaml
git commit -m "feat(marketplace): add 10 container and k8s tools"
```

---

## Task 5: Add Section 4 tools — Security / Supply Chain (10 files)

### Step 5.1 — Create `syft.yaml`

```yaml
name: syft
display_name: Syft
category: Security
tags:
    - security
    - scanner
    - sbom
    - container
binary_names:
    - syft
packages:
    brew: syft
    scoop: syft
github: anchore/syft
```

### Step 5.2 — Create `osv-scanner.yaml`

```yaml
name: osv-scanner
display_name: OSV-Scanner
category: Security
tags:
    - security
    - scanner
    - google
binary_names:
    - osv-scanner
packages:
    brew: osv-scanner
    scoop: osv-scanner
github: google/osv-scanner
```

### Step 5.3 — Create `checkov.yaml`

```yaml
name: checkov
display_name: Checkov
category: Security
tags:
    - security
    - scanner
    - static-analysis
    - terraform
binary_names:
    - checkov
packages:
    brew: checkov
github: bridgecrewio/checkov
```

### Step 5.4 — Create `tfsec.yaml`

```yaml
name: tfsec
display_name: tfsec
category: Security
tags:
    - security
    - scanner
    - terraform
    - static-analysis
binary_names:
    - tfsec
packages:
    brew: tfsec
    scoop: tfsec
    choco: tfsec
github: aquasecurity/tfsec
```

### Step 5.5 — Create `nuclei.yaml`

```yaml
name: nuclei
display_name: Nuclei
category: Security
tags:
    - security
    - scanner
binary_names:
    - nuclei
packages:
    brew: nuclei
github: projectdiscovery/nuclei
```

### Step 5.6 — Create `step.yaml`

```yaml
name: step
display_name: Smallstep CLI
category: Security
tags:
    - security
    - auth
    - encryption
binary_names:
    - step
packages:
    winget: Smallstep.step
    scoop: step
    brew: step
github: smallstep/cli
```

### Step 5.7 — Create `bw.yaml`

```yaml
name: bw
display_name: Bitwarden CLI
category: Security
tags:
    - security
    - password-manager
    - secrets
binary_names:
    - bw
packages:
    scoop: bitwarden-cli
    choco: bitwarden-cli
    brew: bitwarden-cli
    npm: "@bitwarden/cli"
    snap: bw
github: bitwarden/clients
```

### Step 5.8 — Create `op.yaml`

1Password CLI has no public GitHub repo. Omit `github:`.

```yaml
name: op
display_name: 1Password CLI
category: Security
tags:
    - security
    - password-manager
    - secrets
binary_names:
    - op
packages:
    winget: AgileBits.1Password.CLI
    scoop: 1password-cli
    brew: 1password-cli
```

### Step 5.9 — Create `dockle.yaml`

```yaml
name: dockle
display_name: Dockle
category: Security
tags:
    - security
    - scanner
    - container
binary_names:
    - dockle
packages:
    brew: dockle
    scoop: dockle
github: goodwithtech/dockle
```

### Step 5.10 — Create `crane.yaml`

```yaml
name: crane
display_name: crane
category: Security
tags:
    - container
    - security
    - google
binary_names:
    - crane
packages:
    brew: crane
github: google/go-containerregistry
```

- [ ] **Step 5.11: Validate**

Run: `make marketplace-validate`
Expected: `... 203 tools, 24 packs, ... All checks passed.`

- [ ] **Step 5.12: Commit**

```bash
git add marketplace/tools/syft.yaml marketplace/tools/osv-scanner.yaml \
        marketplace/tools/checkov.yaml marketplace/tools/tfsec.yaml \
        marketplace/tools/nuclei.yaml marketplace/tools/step.yaml \
        marketplace/tools/bw.yaml marketplace/tools/op.yaml \
        marketplace/tools/dockle.yaml marketplace/tools/crane.yaml
git commit -m "feat(marketplace): add 10 security and supply-chain tools"
```

---

## Task 6: Add Section 5 tools — Database / Migrations / Data (10 files)

### Step 6.1 — Create `atlas.yaml`

```yaml
name: atlas
display_name: Atlas (Ariga)
category: Database
tags:
    - database
    - migrations
binary_names:
    - atlas
packages:
    brew: ariga/tap/atlas
github: ariga/atlas
```

### Step 6.2 — Create `goose-migrate.yaml`

```yaml
name: goose-migrate
display_name: goose (DB migrations)
category: Database
tags:
    - database
    - migrations
    - golang
binary_names:
    - goose
packages:
    brew: goose
    scoop: goose
github: pressly/goose
```

### Step 6.3 — Create `dbmate.yaml`

```yaml
name: dbmate
display_name: dbmate
category: Database
tags:
    - database
    - migrations
binary_names:
    - dbmate
packages:
    brew: dbmate
    scoop: dbmate
github: amacneil/dbmate
```

### Step 6.4 — Create `flyway.yaml`

```yaml
name: flyway
display_name: Flyway
category: Database
tags:
    - database
    - migrations
    - jvm
binary_names:
    - flyway
packages:
    brew: flyway
    scoop: flyway
github: flyway/flyway
```

### Step 6.5 — Create `liquibase.yaml`

```yaml
name: liquibase
display_name: Liquibase
category: Database
tags:
    - database
    - migrations
    - jvm
binary_names:
    - liquibase
packages:
    winget: Liquibase.Liquibase
    brew: liquibase
    scoop: liquibase
github: liquibase/liquibase
```

### Step 6.6 — Create `kcat.yaml`

```yaml
name: kcat
display_name: kcat (Kafka)
category: Database
tags:
    - messaging
    - cli
binary_names:
    - kcat
packages:
    brew: kcat
    apt: kafkacat
github: edenhill/kcat
```

### Step 6.7 — Create `miller.yaml`

```yaml
name: miller
display_name: Miller (mlr)
category: CLI
tags:
    - data-processing
    - cli
binary_names:
    - mlr
packages:
    winget: johnkerl.miller
    scoop: miller
    choco: miller
    brew: miller
    apt: miller
github: johnkerl/miller
```

### Step 6.8 — Create `fx.yaml`

```yaml
name: fx
display_name: fx
category: CLI
tags:
    - data-processing
    - tui
    - cli
binary_names:
    - fx
packages:
    scoop: fx
    brew: fx
    npm: fx
github: antonmedv/fx
```

### Step 6.9 — Create `gron.yaml`

```yaml
name: gron
display_name: gron
category: CLI
tags:
    - data-processing
    - cli
binary_names:
    - gron
packages:
    brew: gron
    scoop: gron
github: tomnomnom/gron
```

### Step 6.10 — Create `lnav.yaml`

```yaml
name: lnav
display_name: lnav
category: CLI
tags:
    - log-viewer
    - tui
    - cli
binary_names:
    - lnav
packages:
    brew: lnav
    snap: lnav
    apt: lnav
github: tstack/lnav
```

- [ ] **Step 6.11: Validate**

Run: `make marketplace-validate`
Expected: `... 213 tools, 24 packs, ... All checks passed.`

- [ ] **Step 6.12: Commit**

```bash
git add marketplace/tools/atlas.yaml marketplace/tools/goose-migrate.yaml \
        marketplace/tools/dbmate.yaml marketplace/tools/flyway.yaml \
        marketplace/tools/liquibase.yaml marketplace/tools/kcat.yaml \
        marketplace/tools/miller.yaml marketplace/tools/fx.yaml \
        marketplace/tools/gron.yaml marketplace/tools/lnav.yaml
git commit -m "feat(marketplace): add 10 database, migration, and data tools"
```

---

## Task 7: Add Section 6 tools — Languages & Version Managers (10 files)

### Step 7.1 — Create `php.yaml`

```yaml
name: php
display_name: PHP
category: Language
tags:
    - cli
binary_names:
    - php
packages:
    winget: PHP.PHP
    scoop: php
    choco: php
    brew: php
    apt: php
github: php/php-src
```

### Step 7.2 — Create `elixir.yaml`

```yaml
name: elixir
display_name: Elixir
category: Language
tags:
    - cli
binary_names:
    - elixir
packages:
    winget: Elixir.Elixir
    scoop: elixir
    choco: elixir
    brew: elixir
    apt: elixir
github: elixir-lang/elixir
```

### Step 7.3 — Create `kotlin.yaml`

```yaml
name: kotlin
display_name: Kotlin
category: Language
tags:
    - jvm
    - cli
binary_names:
    - kotlin
    - kotlinc
packages:
    scoop: kotlin
    choco: kotlin
    brew: kotlin
    snap: kotlin
github: JetBrains/kotlin
```

### Step 7.4 — Create `lua.yaml`

```yaml
name: lua
display_name: Lua
category: Language
tags:
    - lua
    - cli
binary_names:
    - lua
packages:
    winget: DEVCOM.Lua
    scoop: lua
    choco: lua
    brew: lua
    apt: lua5.4
github: lua/lua
```

### Step 7.5 — Create `pyenv.yaml`

```yaml
name: pyenv
display_name: pyenv
category: PkgMgr
tags:
    - python
    - version-manager
binary_names:
    - pyenv
packages:
    brew: pyenv
    apt: pyenv
github: pyenv/pyenv
```

### Step 7.6 — Create `nvm.yaml`

```yaml
name: nvm
display_name: nvm
category: PkgMgr
tags:
    - javascript
    - version-manager
binary_names:
    - nvm
packages:
    brew: nvm
github: nvm-sh/nvm
```

### Step 7.7 — Create `fnm.yaml`

```yaml
name: fnm
display_name: fnm
category: PkgMgr
tags:
    - javascript
    - version-manager
    - cross-platform
binary_names:
    - fnm
packages:
    winget: Schniz.fnm
    scoop: fnm
    choco: fnm
    brew: fnm
    apt: fnm
github: Schniz/fnm
```

### Step 7.8 — Create `poetry.yaml`

```yaml
name: poetry
display_name: Poetry
category: PkgMgr
tags:
    - python
    - package-manager
binary_names:
    - poetry
packages:
    brew: poetry
github: python-poetry/poetry
```

### Step 7.9 — Create `pipx.yaml`

```yaml
name: pipx
display_name: pipx
category: PkgMgr
tags:
    - python
    - package-manager
binary_names:
    - pipx
packages:
    scoop: pipx
    brew: pipx
    apt: pipx
github: pypa/pipx
```

### Step 7.10 — Create `composer.yaml`

```yaml
name: composer
display_name: Composer
category: PkgMgr
tags:
    - package-manager
binary_names:
    - composer
packages:
    winget: Composer.Composer
    scoop: composer
    choco: composer
    brew: composer
    apt: composer
github: composer/composer
```

- [ ] **Step 7.11: Validate**

Run: `make marketplace-validate`
Expected: `... 223 tools, 24 packs, ... All checks passed.`

- [ ] **Step 7.12: Commit**

```bash
git add marketplace/tools/php.yaml marketplace/tools/elixir.yaml \
        marketplace/tools/kotlin.yaml marketplace/tools/lua.yaml \
        marketplace/tools/pyenv.yaml marketplace/tools/nvm.yaml \
        marketplace/tools/fnm.yaml marketplace/tools/poetry.yaml \
        marketplace/tools/pipx.yaml marketplace/tools/composer.yaml
git commit -m "feat(marketplace): add 10 languages and version managers"
```

---

## Task 8: Add Section 7 tools — Build / Dev Workflow (8 files)

### Step 8.1 — Create `bazel.yaml`

```yaml
name: bazel
display_name: Bazel
category: CLI
tags:
    - build-system
    - google
binary_names:
    - bazel
packages:
    scoop: bazel
    choco: bazel
    brew: bazel
github: bazelbuild/bazel
```

### Step 8.2 — Create `buf.yaml`

```yaml
name: buf
display_name: Buf
category: CLI
tags:
    - protobuf
    - api
    - linter
binary_names:
    - buf
packages:
    brew: bufbuild/buf/buf
    scoop: buf
github: bufbuild/buf
```

### Step 8.3 — Create `make.yaml`

```yaml
name: make
display_name: GNU Make
category: CLI
tags:
    - build-system
    - automation
binary_names:
    - make
packages:
    winget: GnuWin32.Make
    choco: make
    brew: make
    apt: make
github: mirror/make
```

### Step 8.4 — Create `mage.yaml`

```yaml
name: mage
display_name: Mage
category: CLI
tags:
    - build-system
    - golang
binary_names:
    - mage
packages:
    brew: mage
    scoop: mage
github: magefile/mage
```

### Step 8.5 — Create `nx.yaml`

```yaml
name: nx
display_name: Nx
category: CLI
tags:
    - build-system
    - javascript
    - typescript
binary_names:
    - nx
packages:
    npm: nx
github: nrwl/nx
```

### Step 8.6 — Create `turbo.yaml`

```yaml
name: turbo
display_name: Turborepo
category: CLI
tags:
    - build-system
    - javascript
    - typescript
binary_names:
    - turbo
packages:
    npm: turbo
    brew: turbo
github: vercel/turborepo
```

### Step 8.7 — Create `earthly.yaml`

```yaml
name: earthly
display_name: Earthly
category: CLI
tags:
    - build-system
    - container
    - automation
binary_names:
    - earthly
packages:
    brew: earthly/earthly/earthly
    scoop: earthly
    apt: earthly
github: earthly/earthly
```

### Step 8.8 — Create `goreleaser.yaml`

```yaml
name: goreleaser
display_name: GoReleaser
category: CLI
tags:
    - build-system
    - golang
    - automation
    - continuous-delivery
binary_names:
    - goreleaser
packages:
    brew: goreleaser/tap/goreleaser
    scoop: goreleaser
    apt: goreleaser
github: goreleaser/goreleaser
```

- [ ] **Step 8.9: Validate**

Run: `make marketplace-validate`
Expected: `... 231 tools, 24 packs, ... All checks passed.`

- [ ] **Step 8.10: Commit**

```bash
git add marketplace/tools/bazel.yaml marketplace/tools/buf.yaml \
        marketplace/tools/make.yaml marketplace/tools/mage.yaml \
        marketplace/tools/nx.yaml marketplace/tools/turbo.yaml \
        marketplace/tools/earthly.yaml marketplace/tools/goreleaser.yaml
git commit -m "feat(marketplace): add 8 build and dev-workflow tools"
```

---

## Task 9: Add Section 8 tools — Networking / Server / Misc (8 files)

### Step 9.1 — Create `caddy.yaml`

```yaml
name: caddy
display_name: Caddy
category: Network
tags:
    - networking
    - http-client
binary_names:
    - caddy
packages:
    winget: CaddyServer.Caddy
    scoop: caddy
    choco: caddy
    brew: caddy
    apt: caddy
github: caddyserver/caddy
```

### Step 9.2 — Create `iperf3.yaml`

```yaml
name: iperf3
display_name: iperf3
category: Network
tags:
    - networking
    - benchmarking
binary_names:
    - iperf3
packages:
    scoop: iperf3
    choco: iperf3
    brew: iperf3
    apt: iperf3
github: esnet/iperf
```

### Step 9.3 — Create `socat.yaml`

```yaml
name: socat
display_name: socat
category: Network
tags:
    - networking
    - cli
binary_names:
    - socat
packages:
    brew: socat
    apt: socat
github: craSH/socat
```

### Step 9.4 — Create `tcpdump.yaml`

```yaml
name: tcpdump
display_name: tcpdump
category: Network
tags:
    - networking
    - cli
binary_names:
    - tcpdump
packages:
    brew: tcpdump
    apt: tcpdump
github: the-tcpdump-group/tcpdump
```

### Step 9.5 — Create `nomad.yaml`

```yaml
name: nomad
display_name: Nomad
category: IaC
tags:
    - hashicorp
    - container
    - automation
binary_names:
    - nomad
packages:
    winget: HashiCorp.Nomad
    brew: nomad
    apt: nomad
github: hashicorp/nomad
```

### Step 9.6 — Create `boundary.yaml`

```yaml
name: boundary
display_name: Boundary
category: Security
tags:
    - hashicorp
    - security
    - auth
binary_names:
    - boundary
packages:
    winget: HashiCorp.Boundary
    brew: boundary
    apt: boundary
github: hashicorp/boundary
```

### Step 9.7 — Create `yt-dlp.yaml`

```yaml
name: yt-dlp
display_name: yt-dlp
category: Media
tags:
    - media
    - cli
binary_names:
    - yt-dlp
packages:
    winget: yt-dlp.yt-dlp
    scoop: yt-dlp
    choco: yt-dlp
    brew: yt-dlp
github: yt-dlp/yt-dlp
```

### Step 9.8 — Create `croc.yaml`

```yaml
name: croc
display_name: croc
category: CLI
tags:
    - file-transfer
    - cli
binary_names:
    - croc
packages:
    winget: schollz.croc
    scoop: croc
    choco: croc
    brew: croc
github: schollz/croc
```

- [ ] **Step 9.9: Validate**

Run: `make marketplace-validate`
Expected: `... 239 tools, 24 packs, ... All checks passed.`

- [ ] **Step 9.10: Commit**

```bash
git add marketplace/tools/caddy.yaml marketplace/tools/iperf3.yaml \
        marketplace/tools/socat.yaml marketplace/tools/tcpdump.yaml \
        marketplace/tools/nomad.yaml marketplace/tools/boundary.yaml \
        marketplace/tools/yt-dlp.yaml marketplace/tools/croc.yaml
git commit -m "feat(marketplace): add 8 networking and misc utilities"
```

---

## Task 10: Update existing packs

**Goal:** add the new tools to the 9 packs that should reference them.

Each pack file has the shape:
```yaml
name: <pack-name>
display_name: <Display>
description: <text>
tools:
    - tool1
    - tool2
```

- [ ] **Step 10.1: Update `ai-toolkit.yaml`**

Add `gemini-cli`, `goose`, `cursor-agent`, `gptme` to the `tools:` list (alphabetical or grouped, your call — match the existing style of that file). Final file:

```yaml
name: ai-toolkit
display_name: AI Toolkit
description: AI coding assistants and LLM tools.
tools:
    - claude
    - copilot
    - aider
    - codex
    - cursor-agent
    - gemini-cli
    - goose
    - gptme
    - ollama
```

- [ ] **Step 10.2: Update `containers.yaml`**

Read it first:
```bash
cat marketplace/packs/containers.yaml
```

Then add `podman`, `nerdctl`, `buildah` to its tools list. Keep existing entries; append the new ones at the end of the list.

- [ ] **Step 10.3: Update `k8s-advanced.yaml`**

Read first, then add: `helmfile`, `kubeseal`, `krew`, `kompose`, `kn`.

- [ ] **Step 10.4: Update `security-toolkit.yaml`**

Read first, then add: `syft`, `osv-scanner`, `checkov`, `tfsec`, `nuclei`, `step`, `dockle`.

- [ ] **Step 10.5: Update `data-tools.yaml`**

Read first, then add: `miller`, `fx`, `gron`, `lnav`, `kcat`.

- [ ] **Step 10.6: Update `hashicorp-stack.yaml`**

Read first, then add: `nomad`, `boundary`.

- [ ] **Step 10.7: Update `python-developer.yaml`**

Read first, then add: `poetry`, `pipx`, `pyenv`.

- [ ] **Step 10.8: Update `node-developer.yaml`**

Read first, then add: `fnm`, `turbo`, `nx`. (Do NOT add `nvm`.)

- [ ] **Step 10.9: Update `dev-productivity.yaml`**

Read first, then add: `croc`, `yt-dlp`, `make`.

- [ ] **Step 10.10: Validate**

Run: `make marketplace-validate`
Expected: `... 239 tools, 24 packs, ... All checks passed.`

Common failure: "references unknown tool X" — means a tool wasn't added or has a name mismatch.

- [ ] **Step 10.11: Commit**

```bash
git add marketplace/packs/ai-toolkit.yaml marketplace/packs/containers.yaml \
        marketplace/packs/k8s-advanced.yaml marketplace/packs/security-toolkit.yaml \
        marketplace/packs/data-tools.yaml marketplace/packs/hashicorp-stack.yaml \
        marketplace/packs/python-developer.yaml marketplace/packs/node-developer.yaml \
        marketplace/packs/dev-productivity.yaml
git commit -m "feat(marketplace): expand 9 existing packs with new tools"
```

---

## Task 11: Add new packs

### Step 11.1 — Create `marketplace/packs/edge-platforms.yaml`

```yaml
name: edge-platforms
display_name: Edge Platforms
description: CLIs for modern edge / PaaS providers (Fly, Vercel, Netlify, Cloudflare, Supabase, DigitalOcean).
tools:
    - flyctl
    - vercel
    - netlify
    - wrangler
    - cloudflared
    - supabase
    - doctl
```

### Step 11.2 — Create `marketplace/packs/db-migrations.yaml`

```yaml
name: db-migrations
display_name: Database Migrations
description: Schema migration and versioning tools across SQL databases.
tools:
    - atlas
    - goose-migrate
    - dbmate
    - flyway
    - liquibase
```

### Step 11.3 — Create `marketplace/packs/supply-chain-security.yaml`

```yaml
name: supply-chain-security
display_name: Supply Chain Security
description: SBOM generation, vulnerability scanning, image signing, and container hardening.
tools:
    - syft
    - osv-scanner
    - cosign
    - trivy
    - grype
    - dockle
    - crane
```

- [ ] **Step 11.4: Validate**

Run: `make marketplace-validate`
Expected: `... 239 tools, 27 packs, ... All checks passed.`

If "references unknown tool cosign/trivy/grype" — those already exist (verify with `ls marketplace/tools/cosign.yaml marketplace/tools/trivy.yaml marketplace/tools/grype.yaml`). If any is missing, drop it from the pack — out of scope here.

- [ ] **Step 11.5: Commit**

```bash
git add marketplace/packs/edge-platforms.yaml marketplace/packs/db-migrations.yaml \
        marketplace/packs/supply-chain-security.yaml
git commit -m "feat(marketplace): add edge-platforms, db-migrations, supply-chain-security packs"
```

---

## Task 12: Final end-to-end verification

- [ ] **Step 12.1: Run full validation**

Run: `make marketplace-validate`
Expected: `Marketplace validated: 239 tools, 27 packs, ... All checks passed.`

- [ ] **Step 12.2: Run build**

Run: `make build`
Expected: builds successfully, produces `bin/klim` (or `bin/klim.exe` on Windows).

- [ ] **Step 12.3: Run unit tests**

Run: `make test`
Expected: all tests pass. (No new tests are needed for this change — it's data-only — but existing tests should still pass.)

- [ ] **Step 12.4: Run lint**

Run: `make lint`
Expected: zero issues. (Lint targets Go; YAML changes won't trip it but we run for safety.)

- [ ] **Step 12.5: Spot-check the assembled marketplace**

Run: `make marketplace-assemble`
Expected: writes/updates `marketplace.yaml`. Open it and confirm a couple of new tools appear (e.g. `grep '^- name: podman' marketplace.yaml`).

If `make marketplace-assemble` hits the GitHub API and you're rate-limited or offline, this step is best-effort — note it in the commit message but don't block on it.

- [ ] **Step 12.6: Verify total counts match the spec**

Run:
```bash
ls marketplace/tools | wc -l
ls marketplace/packs | wc -l
```
Expected: 239 tools (153 existing + 86 new), 27 packs (24 existing + 3 new).

If counts are off, list the new tools you committed and reconcile against the spec section tables.

- [ ] **Step 12.7: No-commit step.** This task is verification only — no new files are added.

---

## Done

After Task 12 passes, the spec is fully implemented. The branch is ready for code review (`/requesting-code-review` or PR).

---

## Notes for the implementer

- **Be ruthless about copy-pasting the YAML blocks verbatim.** Indentation matters (4-space lists). Don't "fix" whitespace.
- **If the validator complains about a tag**, re-check Task 1 was committed before Task 2 — tags MUST exist first.
- **If a package ID 404s in winget/scoop/brew at runtime**, that's a separate issue — the validator only checks the YAML schema, not whether the package actually installs. Reporting wrong package IDs is a follow-up bug, not a plan failure.
- **The expected counts in each task** (e.g. "183 tools") assume you start at 162 (the spec's stated baseline). If `ls marketplace/tools | wc -l` shows a different starting count, adjust the expected numbers accordingly — but the deltas (+11, +10, +10, etc.) must hold.
- **YAML quoting:** strings like `@scope/package` (npm scoped names) and `5.4` (lua apt id) need double-quotes to avoid being parsed as something else. The blocks above already quote them; preserve the quoting.
