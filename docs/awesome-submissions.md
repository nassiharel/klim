# Awesome-list submissions

Ready-to-submit entries for getting klim listed on curated "awesome" lists. These are
**evergreen backlinks** — high-ROI, durable traffic — but every list has a maturity gate, so
they can't all be submitted on day one. Submit each when its gate clears.

> ⚠️ **Eligibility timing** — klim's first commit was **2026-05-06**.
> - **Awesome Go** requires ≥5 months of history → eligible **~2026-10-06**.
> - **Awesome CLI Apps** requires age >90 days **and** >20 GitHub stars → eligible **~2026-08-04**
>   (subject to the star count).
>
> Submitting before the gate triggers an automatic rejection. Track the dates and submit then.

---

## 1. Awesome Go — `avelino/awesome-go`

**Entry line** (exact format `- [name](url) - Description.`, placed **alphabetically** within
the category):

```markdown
- [klim](https://github.com/nassiharel/klim) - Cross-platform dev-tools manager: install whole toolchains over native package managers (brew, winget, scoop, apt) on macOS, Linux, and Windows.
```

**Category:** `DevOps Tools` (or `Utilities` if a reviewer prefers). Place alphabetically —
between the entries that bracket "klim".

**Pre-flight checklist** (all are blocking PR checks):
- [ ] ≥5 months since first commit (eligible ~2026-10-06).
- [x] Open-source license (MIT).
- [x] `go.mod` at repo root (`github.com/nassiharel/klim`).
- [x] At least one `vX.Y.Z` SemVer tag (`v0.1.5`).
- [ ] Go Report Card grade A-/A/A+ — verify at <https://goreportcard.com/report/github.com/nassiharel/klim> (README already badges "A+").
- [ ] Test coverage ≥80% for non-data packages — confirm current number before submitting.
- [x] English README + pkg.go.dev docs.
- [ ] One item per PR.

**Links required in the PR body:**
- Forge: <https://github.com/nassiharel/klim>
- pkg.go.dev: <https://pkg.go.dev/github.com/nassiharel/klim>
- Go Report Card: <https://goreportcard.com/report/github.com/nassiharel/klim>
- Coverage report: _add a Codecov/Coveralls link before submitting_ (not currently wired — see note below).

> **Action before submitting:** wire a coverage report (Codecov or Coveralls) into CI, since
> the PR body must link one. `make cover` produces a local HTML report today, but the list
> wants a hosted, reachable coverage link.

---

## 2. Awesome CLI Apps — `agarrharr/awesome-cli-apps`

**Entry line** (format `[APP_NAME](LINK) - DESCRIPTION.`, added at the **bottom** of the
category — no alphabetical ordering; description starts capitalized, ends with a period, avoid
the words "CLI"/"terminal"):

```markdown
- [klim](https://github.com/nassiharel/klim) - Install and standardize whole developer toolchains over the native package managers you already trust, on macOS, Linux, and Windows.
```

**Category:** `Productivity` (or `Package Managers` if present).

**Pre-flight checklist:**
- [ ] Repo older than 90 days (eligible ~2026-08-04).
- [ ] >20 GitHub stars.
- [x] Free/open-source license (MIT).
- [x] Simple to install, well documented.
- [ ] One PR per app, titled `Add klim`, using the repo's PR template.

---

## 3. Opportunistic (after the two above land)

- `toolleeo/cli-apps` — another CLI list; similar rules.
- `awesome-devops` lists — klim fits "environment/setup" sections.
- Cross-post into `avelino/awesome-go` **Utilities** only if DevOps Tools reviewer suggests it
  (one item per PR — don't double-submit).

## How to submit (when a gate clears)

```bash
# Fork the target list, then on your fork:
gh repo fork avelino/awesome-go --clone
cd awesome-go
git checkout -b add-klim
# edit README.md — add the entry line in the right category/position
git commit -am "Add klim"
gh pr create --title "Add klim" --body "<links + checklist from above>"
```
