package plan

import (
	"fmt"
	"strings"

	"github.com/nassiharel/klim/internal/registry"
)

// computeConfidence returns a 0-100 score plus the breakdown of every
// signal that produced it. Only meaningful for upgrades; returns
// 100/no-factors for installs and removes (those are decisions, not
// transitions, and have no "before" state to evaluate).
//
// The scoring intentionally favours legibility over arithmetic
// precision — every factor's delta is shown to the user, so the
// score's exact value matters less than the per-factor reasoning.
//
// Score categories (computed in order):
//
//  1. Semantic version jump: patch / minor / major → small / medium / large penalty.
//  2. Known-fragile-tool penalty: kubectl, node, python, etc. carry
//     their own ecosystem-specific risk regardless of bump size.
//  3. Plugin ecosystem: when a tool has a plugin manager that lags
//     mainline releases (terraform providers, kubectl plugins) and
//     evidence of it is in the installed set, lower confidence.
//  4. Installed-ecosystem coupling: upgrading a foundational tool
//     (node, python, go, ruby) while the user has lots of installed
//     packages bound to it carries extra risk.
//  5. Community-signal placeholder: we can't query GitHub at plan
//     time, but we leave the hook explicit so a later pass can wire
//     up the GitHubInfo we already collect at marketplace assemble.
func computeConfidence(c Change, tools []registry.Tool) (int, []ConfidenceFactor) {
	if c.Kind != ChangeUpgrade {
		return 100, nil
	}

	score := 100
	var factors []ConfidenceFactor

	apply := func(name string, delta int, reason string) {
		score += delta
		factors = append(factors, ConfidenceFactor{Name: name, Delta: delta, Reason: reason})
	}

	// Detect downgrade up front so semver / fragility factors can
	// pick neutral or rollback-specific wording. Rollback plans
	// surface as ChangeUpgrade with ToVersion < FromVersion.
	isDowngrade := c.FromVersion != "" && c.ToVersion != "" &&
		registry.CompareVersions(c.ToVersion, c.FromVersion) < 0

	// 1. Semantic version delta.
	switch versionBumpKind(c.FromVersion, c.ToVersion) {
	case bumpMajor:
		reason := fmt.Sprintf("%s → %s crosses a major boundary — breaking changes are likely", c.FromVersion, c.ToVersion)
		if isDowngrade {
			reason = fmt.Sprintf("%s → %s crosses a major boundary — state / config written by the newer major may not load", c.FromVersion, c.ToVersion)
		}
		apply("semver: major change", -25, reason)
	case bumpMinor:
		reason := fmt.Sprintf("%s → %s adds new features that may shift defaults", c.FromVersion, c.ToVersion)
		if isDowngrade {
			reason = fmt.Sprintf("%s → %s drops features added in the newer minor — review usage", c.FromVersion, c.ToVersion)
		}
		apply("semver: minor change", -8, reason)
	case bumpPatch:
		reason := fmt.Sprintf("%s → %s is a patch release — typically safe", c.FromVersion, c.ToVersion)
		if isDowngrade {
			reason = fmt.Sprintf("%s → %s is a patch-level rollback — typically safe but may reintroduce a fixed bug", c.FromVersion, c.ToVersion)
		}
		apply("semver: patch change", -2, reason)
	}

	// 2. Tool-specific fragility.
	tool := strings.ToLower(c.Tool)
	switch tool {
	case "kubectl":
		reason := "kubectl ±1 minor version of the API server is supported; further skew breaks features"
		if isDowngrade {
			reason = "kubectl rollback can land outside the supported ±1 minor skew with the API server; verify before applying"
		}
		apply("kubectl: client-server skew", -20, reason)
	case "node", "nodejs":
		reason := "native modules built against the previous Node ABI need rebuild after the upgrade"
		if isDowngrade {
			reason = "native modules built against the newer Node ABI need rebuild after the rollback"
		}
		apply("node: native modules", -15, reason)
	case "python", "python3":
		reason := "existing venvs hard-link to the interpreter and become orphaned after a Python upgrade"
		if isDowngrade {
			reason = "existing venvs hard-link to the interpreter and become orphaned after a Python rollback"
		}
		apply("python: virtualenv coupling", -15, reason)
	case "docker", "docker-desktop", "podman":
		reason := "engine upgrade can require container restarts and re-pulling base images"
		if isDowngrade {
			reason = "engine rollback can require container restarts; images built against the newer engine may need rebuilding"
		}
		apply("docker: engine restart", -10, reason)
	case "terraform", "tofu":
		reason := "provider lockfile pinned to the old binary may require `terraform init -upgrade`"
		if isDowngrade {
			reason = "state files written by the newer version may not load; back up state before the rollback"
		}
		apply("terraform: provider lockfile", -8, reason)
	case "go":
		reason := "build cache becomes invalid; first build after upgrade is full-fresh"
		if isDowngrade {
			reason = "build cache becomes invalid; first build after rollback is full-fresh"
		}
		apply("go: build cache invalidation", -5, reason)
	case "rust", "rustc", "cargo":
		reason := "all target crates rebuild on the new toolchain"
		if isDowngrade {
			reason = "all target crates rebuild on the rolled-back toolchain"
		}
		apply("rust: target rebuild", -5, reason)
	}

	// 3. Plugin / ecosystem coupling — only deducts when actual
	// dependent tools are present locally.
	if dep, n := pluginCoupling(tool, tools); n > 0 {
		apply("plugin ecosystem detected", -dep,
			fmt.Sprintf("%d related %s plugins/tools installed locally — review compatibility", n, c.DisplayName))
	}

	// 4. Foundational-tool ecosystem size: upgrading a language
	// runtime when the user has 30+ installed tools.
	if isFoundationalRuntime(tool) && len(tools) >= 30 {
		apply("large installed ecosystem", -5,
			fmt.Sprintf("%d tools detected; foundational runtime upgrades ripple through many of them", len(tools)))
	}

	// 5. Community-signal placeholder. Left undeducted today so the
	// score doesn't drift when this lights up; the factor list shows
	// the slot is reserved.
	apply("community signal", 0,
		"community issue volume not consulted at plan time (offline)")

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score, factors
}

// bumpKind categorises the change between two semver-shaped versions.
type bumpKind int

const (
	bumpNone bumpKind = iota
	bumpPatch
	bumpMinor
	bumpMajor
)

func versionBumpKind(from, to string) bumpKind {
	if from == "" || to == "" {
		return bumpNone
	}
	f := splitVersionNumeric(from)
	t := splitVersionNumeric(to)
	// Pad to length 3 for safe comparison.
	for len(f) < 3 {
		f = append(f, 0)
	}
	for len(t) < 3 {
		t = append(t, 0)
	}
	switch {
	case f[0] != t[0]:
		return bumpMajor
	case f[1] != t[1]:
		return bumpMinor
	case f[2] != t[2]:
		return bumpPatch
	}
	return bumpNone
}

// pluginCoupling returns (deltaWeight, count) when the tool being
// upgraded has known plugin/related tools and any of them are
// installed locally.
func pluginCoupling(tool string, tools []registry.Tool) (int, int) {
	groups := map[string][]string{
		"kubectl":   {"helm", "kustomize", "k9s", "kubectx", "kubens", "kind", "minikube", "stern", "krew"},
		"terraform": {"tflint", "terragrunt", "tfsec", "checkov", "infracost", "atlantis"},
		"tofu":      {"tflint", "terragrunt", "tfsec", "checkov", "infracost"},
		"docker":    {"docker-compose", "buildx", "dive", "ctop", "lazydocker"},
		"node":      {"npm", "yarn", "pnpm", "bun"},
		"python":    {"pip", "poetry", "uv", "pyenv", "pipx", "pipenv"},
		"go":        {"golangci-lint", "goreleaser", "gopls", "delve", "air"},
	}
	related, ok := groups[tool]
	if !ok {
		return 0, 0
	}
	set := make(map[string]bool, len(related))
	for _, r := range related {
		set[r] = true
	}
	count := 0
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		if set[strings.ToLower(t.Name)] {
			count++
		}
	}
	if count == 0 {
		return 0, 0
	}
	// Cap penalty so a huge ecosystem doesn't push score below 0
	// from this factor alone (other factors still apply on top).
	delta := 3 + count
	if delta > 15 {
		delta = 15
	}
	return delta, count
}

func isFoundationalRuntime(tool string) bool {
	switch tool {
	case "node", "nodejs", "python", "python3", "go", "ruby", "java", "openjdk", "rust", "rustc":
		return true
	}
	return false
}
