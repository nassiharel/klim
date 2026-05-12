package plan

import (
	"fmt"
	"strings"

	"github.com/nassiharel/klim/internal/registry"
)

// AnalyseRisks runs every registered risk heuristic over the plan
// and returns the resulting warnings. Exported so callers building a
// plan from outside this package (e.g. tests, plug-in TUI views) can
// re-run the analysis on a synthetic Change slice.
func AnalyseRisks(changes []Change, tools []registry.Tool) []Risk {
	var risks []Risk
	for _, c := range changes {
		risks = append(risks, risksFor(c, tools)...)
	}
	// Stable ordering: severity desc, then by tool, then by message
	// so repeated runs produce the same output.
	sortRisks(risks)
	return risks
}

func risksFor(c Change, tools []registry.Tool) []Risk {
	var out []Risk
	tool := strings.ToLower(c.Tool)

	// Detect downgrade once so we can tailor copy across both the
	// major-bump warning and the per-tool advisories. Rollback
	// plans surface as ChangeUpgrade with ToVersion < FromVersion.
	isDowngrade := c.Kind == ChangeUpgrade &&
		c.FromVersion != "" && c.ToVersion != "" &&
		registry.CompareVersions(c.ToVersion, c.FromVersion) < 0

	if c.Kind == ChangeUpgrade && isMajorBump(c.FromVersion, c.ToVersion) {
		verb := "upgrading"
		direction := "bump"
		if isDowngrade {
			verb = "downgrading"
			direction = "rollback"
		}
		out = append(out, Risk{
			Severity: SeverityWarning,
			Tool:     c.Tool,
			Message:  fmt.Sprintf("major-version %s %s → %s — review breaking changes before %s", direction, c.FromVersion, c.ToVersion, verb),
		})
	}

	// Per-tool advisories are tuned for upgrades; rephrase the
	// most direction-sensitive ones when we're rolling back so the
	// printed risk stays accurate.
	switch tool {
	case "kubectl":
		if c.Kind == ChangeUpgrade {
			out = append(out, Risk{
				Severity: SeverityWarning,
				Tool:     c.Tool,
				Message:  "kubectl is sensitive to client-server skew — confirm your cluster's API server version supports " + c.ToVersion,
			})
		}
	case "terraform", "tofu":
		if c.Kind == ChangeUpgrade {
			msg := "Terraform/OpenTofu provider lockfile may need refresh after upgrade — run `terraform init -upgrade` in your modules"
			if isDowngrade {
				msg = "Terraform/OpenTofu rollback can fail to read state written by the newer version — back up state and re-run `terraform init` in your modules"
			}
			out = append(out, Risk{
				Severity: SeverityInfo,
				Tool:     c.Tool,
				Message:  msg,
			})
		}
	case "node", "nodejs":
		if c.Kind == ChangeUpgrade {
			msg := "Node upgrade can invalidate native modules — rebuild with `npm rebuild` in projects that ship .node binaries"
			if isDowngrade {
				msg = "Node rollback can invalidate native modules built against the newer ABI — rebuild with `npm rebuild` in projects that ship .node binaries"
			}
			out = append(out, Risk{
				Severity: SeverityWarning,
				Tool:     c.Tool,
				Message:  msg,
			})
		}
	case "go":
		if c.Kind == ChangeUpgrade {
			msg := "Go compiler upgrade invalidates the on-disk build cache — first build after upgrade will be slower"
			if isDowngrade {
				msg = "Go compiler rollback invalidates the on-disk build cache — first build after rollback will be slower"
			}
			out = append(out, Risk{
				Severity: SeverityInfo,
				Tool:     c.Tool,
				Message:  msg,
			})
		}
	case "rust", "rustc", "cargo":
		if c.Kind == ChangeUpgrade {
			msg := "Rust toolchain upgrade rebuilds every target crate — incremental builds will run fresh once"
			if isDowngrade {
				msg = "Rust toolchain rollback rebuilds every target crate — incremental builds will run fresh once"
			}
			out = append(out, Risk{
				Severity: SeverityInfo,
				Tool:     c.Tool,
				Message:  msg,
			})
		}
	case "python", "python3":
		if c.Kind == ChangeUpgrade {
			msg := "Python upgrade orphans existing virtualenvs — recreate venvs that point at the upgraded interpreter"
			if isDowngrade {
				msg = "Python rollback orphans existing virtualenvs built against the newer interpreter — recreate venvs that point at the rolled-back interpreter"
			}
			out = append(out, Risk{
				Severity: SeverityWarning,
				Tool:     c.Tool,
				Message:  msg,
			})
		}
	case "docker", "docker-desktop":
		if c.Kind == ChangeUpgrade {
			msg := "Docker engine upgrade may require restarting containers and rebuilding images with outdated base layers"
			if isDowngrade {
				msg = "Docker engine rollback may require restarting containers; images built against the newer engine may need rebuilding"
			}
			out = append(out, Risk{
				Severity: SeverityWarning,
				Tool:     c.Tool,
				Message:  msg,
			})
		}
	}

	// Removal of a tool that's referenced by an installed project's
	// .klim.yaml or a pack is worth flagging here too — but plan
	// doesn't yet know about projects. Leaving the hook open for a
	// follow-up.
	if c.Kind == ChangeRemove {
		out = append(out, Risk{
			Severity: SeverityWarning,
			Tool:     c.Tool,
			Message:  fmt.Sprintf("removing %s — verify no project depends on it before applying", c.DisplayName),
		})
	}
	return out
}

// isMajorBump returns true when the two versions differ in their
// leading numeric component. Treats malformed inputs as "not a major
// bump" — safer than over-warning.
func isMajorBump(from, to string) bool {
	if from == "" || to == "" {
		return false
	}
	fParts := splitVersionNumeric(from)
	tParts := splitVersionNumeric(to)
	if len(fParts) == 0 || len(tParts) == 0 {
		return false
	}
	return fParts[0] != tParts[0]
}

func splitVersionNumeric(s string) []int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	var out []int
	cur := 0
	seenDigit := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			cur = cur*10 + int(r-'0')
			seenDigit = true
		case seenDigit:
			out = append(out, cur)
			cur = 0
			seenDigit = false
			if r != '.' {
				// Stop at the first non-numeric, non-dot
				// boundary so "1.8.0-beta" splits to [1, 8, 0].
				return out
			}
		}
	}
	if seenDigit {
		out = append(out, cur)
	}
	return out
}

func sortRisks(risks []Risk) {
	rank := func(s Severity) int {
		switch s {
		case SeverityError:
			return 0
		case SeverityWarning:
			return 1
		case SeverityInfo:
			return 2
		}
		return 3
	}
	// Insertion sort — risk lists are short.
	for i := 1; i < len(risks); i++ {
		for j := i; j > 0; j-- {
			if less := risksLess(risks[j], risks[j-1], rank); !less {
				break
			}
			risks[j], risks[j-1] = risks[j-1], risks[j]
		}
	}
}

func risksLess(a, b Risk, rank func(Severity) int) bool {
	if rank(a.Severity) != rank(b.Severity) {
		return rank(a.Severity) < rank(b.Severity)
	}
	if a.Tool != b.Tool {
		return strings.ToLower(a.Tool) < strings.ToLower(b.Tool)
	}
	return a.Message < b.Message
}
