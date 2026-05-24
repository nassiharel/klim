package badge

import (
	"strings"
	"testing"
)

func TestBadgeURL_EscapesAndDoubleHyphens(t *testing.T) {
	b := Badge{Label: "klim score", Value: "93/100 A", Color: "brightgreen"}
	u := b.URL()
	if !strings.HasPrefix(u, "https://img.shields.io/badge/") {
		t.Errorf("url prefix wrong: %s", u)
	}
	// Spaces should be %20-encoded.
	if !strings.Contains(u, "klim%20score") {
		t.Errorf("space not escaped: %s", u)
	}
	if !strings.Contains(u, "brightgreen") {
		t.Errorf("color missing: %s", u)
	}
}

func TestBadgeURL_DoubleHyphenEscape(t *testing.T) {
	// Shields.io needs literal hyphens escaped as `--` so they don't
	// split the path. The current Build never produces a hyphen
	// inside Label/Value because we use spaces, but the URL function
	// should defend against it anyway.
	b := Badge{Label: "a-b", Value: "1", Color: "red"}
	if u := b.URL(); !strings.Contains(u, "a--b") {
		t.Errorf("hyphen in label should be doubled: %s", u)
	}
}

func TestMarkdown_WrapsLinkWhenPresent(t *testing.T) {
	b := Badge{Label: "klim", Value: "ok", Color: "green", Link: "https://example.test"}
	md := b.Markdown()
	if !strings.HasPrefix(md, "[") {
		t.Errorf("expected link-wrapped markdown, got %s", md)
	}
	if !strings.Contains(md, "https://example.test") {
		t.Errorf("link target missing: %s", md)
	}
}

func TestBuild_FourBadgesWithStableIDs(t *testing.T) {
	in := Inputs{ScorePoints: 90, ScoreMax: 100, ScoreGrade: "A", ToolCount: 50, AuditIssues: 0, FreshPercent: 95}
	bs := Build(in)
	if len(bs) != 4 {
		t.Fatalf("expected 4 badges, got %d", len(bs))
	}
	want := []string{"score", "tools", "audit", "fresh"}
	for i, b := range bs {
		if b.ID != want[i] {
			t.Errorf("badge %d ID = %q, want %q", i, b.ID, want[i])
		}
	}
}

func TestByID_FiltersAndPreservesOrder(t *testing.T) {
	in := Inputs{ScorePoints: 50, ScoreMax: 100, ScoreGrade: "C", ToolCount: 10, AuditIssues: 5, FreshPercent: 80}
	got := ByID(in, "fresh", "audit")
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	// Build order is score/tools/audit/fresh; ByID preserves that.
	if got[0].ID != "audit" || got[1].ID != "fresh" {
		t.Errorf("order = %v, want [audit, fresh]", []string{got[0].ID, got[1].ID})
	}
}

func TestColorThresholds(t *testing.T) {
	cases := []struct {
		pct  int
		want string
	}{
		{100, "brightgreen"},
		{90, "brightgreen"},
		{80, "green"},
		{70, "yellow"},
		{45, "orange"},
		{0, "red"},
	}
	for _, c := range cases {
		if got := colorByPercent(c.pct); got != c.want {
			t.Errorf("colorByPercent(%d) = %q, want %q", c.pct, got, c.want)
		}
	}
}

func TestAuditBadge_Thresholds(t *testing.T) {
	clean := auditBadge(Inputs{AuditIssues: 0})
	if clean.Value != "clean" || clean.Color != "brightgreen" {
		t.Errorf("clean audit: %+v", clean)
	}
	mid := auditBadge(Inputs{AuditIssues: 3})
	if mid.Color != "yellow" {
		t.Errorf("3-issue audit color = %q, want yellow", mid.Color)
	}
	bad := auditBadge(Inputs{AuditIssues: 10})
	if bad.Color != "red" {
		t.Errorf("10-issue audit color = %q, want red", bad.Color)
	}
}

func TestPctOf_HandlesZeroDenom(t *testing.T) {
	if pctOf(0, 0) != 0 || pctOf(5, 0) != 0 {
		t.Error("zero denominator should yield 0")
	}
	if pctOf(50, 100) != 50 {
		t.Error("simple percent failed")
	}
}
