package cli

import (
	"testing"
)

// TestBadgeCmd_NoArgs verifies the badge command rejects positional
// args (CLI-CONVENTIONS.md:48).
func TestBadgeCmd_NoArgs(t *testing.T) {
	if err := badgeCmd.Args(badgeCmd, []string{"oops"}); err == nil {
		t.Error("badgeCmd.Args(['oops']) returned nil; want error")
	}
}

// TestBadgeCmd_HasExpectedFlags catches accidental flag removal.
func TestBadgeCmd_HasExpectedFlags(t *testing.T) {
	for _, name := range []string{"score", "tools", "audit", "fresh", "all", "refresh", "output"} {
		if badgeCmd.Flags().Lookup(name) == nil {
			t.Errorf("badgeCmd missing flag --%s", name)
		}
	}
}

// TestSelectedBadgeIDs_Defaults: with no flag set, the command
// should render every badge (this is the help text's promise).
func TestSelectedBadgeIDs_Defaults(t *testing.T) {
	saveAll, saveScore, saveTools, saveAudit, saveFresh := badgeAll, badgeScore, badgeTools, badgeAudit, badgeFresh
	t.Cleanup(func() {
		badgeAll = saveAll
		badgeScore = saveScore
		badgeTools = saveTools
		badgeAudit = saveAudit
		badgeFresh = saveFresh
	})
	badgeAll, badgeScore, badgeTools, badgeAudit, badgeFresh = false, false, false, false, false
	got := selectedBadgeIDs()
	// nil means "no filter — render every badge in default order".
	if got != nil {
		t.Errorf("selectedBadgeIDs with no flags = %v; want nil (means all-badges)", got)
	}
}

// TestSelectedBadgeIDs_PerFlag: setting just --score returns only
// the score badge.
func TestSelectedBadgeIDs_PerFlag(t *testing.T) {
	saveAll, saveScore, saveTools, saveAudit, saveFresh := badgeAll, badgeScore, badgeTools, badgeAudit, badgeFresh
	t.Cleanup(func() {
		badgeAll = saveAll
		badgeScore = saveScore
		badgeTools = saveTools
		badgeAudit = saveAudit
		badgeFresh = saveFresh
	})
	badgeAll, badgeScore, badgeTools, badgeAudit, badgeFresh = false, true, false, false, false
	got := selectedBadgeIDs()
	if len(got) != 1 || got[0] != "score" {
		t.Errorf("selectedBadgeIDs(--score)=%v; want [score]", got)
	}
}
