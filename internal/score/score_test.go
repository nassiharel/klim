package score

import (
	"net/url"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/compliance"
	"github.com/nassiharel/klim/internal/doctor"
	"github.com/nassiharel/klim/internal/registry"
)

func installedTool(name, version, latest string, source registry.InstallSource) registry.Tool {
	return registry.Tool{
		Name:   name,
		Latest: latest,
		Instances: []registry.Instance{
			{Path: "/bin/" + name, Version: version, Source: source},
		},
	}
}

func TestGrade_Boundaries(t *testing.T) {
	cases := []struct {
		points, max int
		want        string
	}{
		{0, 0, "?"}, // divide-by-zero guard
		{0, 100, "F"},
		{59, 100, "F"},
		{60, 100, "D"},
		{69, 100, "D"},
		{70, 100, "C"},
		{79, 100, "C"},
		{80, 100, "B"},
		{89, 100, "B"},
		{90, 100, "A"},
		{94, 100, "A"},
		{95, 100, "A+"},
		{100, 100, "A+"},
	}
	for _, c := range cases {
		if got := grade(c.points, c.max); got != c.want {
			t.Errorf("grade(%d/%d): want %s, got %s", c.points, c.max, c.want, got)
		}
	}
}

func TestBadgeURL_ColorByPercent(t *testing.T) {
	cases := []struct {
		points, max int
		wantColor   string
	}{
		{0, 100, "red"},
		{49, 100, "red"},
		{50, 100, "orange"},
		{69, 100, "orange"},
		{70, 100, "yellow"},
		{84, 100, "yellow"},
		{85, 100, "yellowgreen"},
		{94, 100, "yellowgreen"},
		{95, 100, "brightgreen"},
		{100, 100, "brightgreen"},
		{0, 0, "red"}, // pct=0 hits the <50 branch even for the divide-by-zero case
	}
	for _, c := range cases {
		r := Result{Total: c.points, MaxTotal: c.max, Grade: grade(c.points, c.max)}
		got := BadgeURL(r)
		if !strings.HasSuffix(got, "-"+c.wantColor) {
			t.Errorf("BadgeURL(%d/%d): want trailing color %q, got %s", c.points, c.max, c.wantColor, got)
		}
		// The badge value should embed the score and grade.
		if !strings.Contains(got, url.PathEscape("klim score")) {
			t.Errorf("BadgeURL: missing klim score label: %s", got)
		}
	}
}

func TestScoreToolFreshness(t *testing.T) {
	// All up-to-date.
	tools := []registry.Tool{
		installedTool("git", "2.50", "2.50", registry.SourceBrew),
		installedTool("docker", "27.0", "27.0", registry.SourceBrew),
	}
	c := scoreToolFreshness(tools)
	if c.Points != 30 || c.MaxPts != 30 || c.Status != "ok" {
		t.Errorf("all-current: want 30/30 ok, got %+v", c)
	}

	// 1 of 2 outdated → 50% → 15 points, status warning.
	tools = []registry.Tool{
		installedTool("git", "2.50", "2.50", registry.SourceBrew),
		installedTool("docker", "26.0", "27.0", registry.SourceBrew),
	}
	c = scoreToolFreshness(tools)
	if c.Points != 15 || c.Status != "warning" {
		t.Errorf("half-outdated: want 15 points warning, got %+v", c)
	}

	// No tools installed → full marks.
	c = scoreToolFreshness(nil)
	if c.Points != 30 || c.Status != "ok" {
		t.Errorf("no-tools: want 30 ok, got %+v", c)
	}
}

func TestScoreDoctorHealth(t *testing.T) {
	// 1 error costs 5; 2 warnings cost 4; max=25 → 16 points.
	issues := []doctor.Issue{
		{Severity: "error"},
		{Severity: "warning"}, {Severity: "warning"},
	}
	c := scoreDoctorHealth(issues)
	if c.Points != 16 || c.Status != "error" {
		t.Errorf("err+2warn: want 16 error, got %+v", c)
	}

	// Warnings only → status warning.
	c = scoreDoctorHealth([]doctor.Issue{{Severity: "warning"}})
	if c.Status != "warning" || c.Points != 23 {
		t.Errorf("1warn: want 23 warning, got %+v", c)
	}

	// Penalty larger than maxPts is clamped at 0.
	heavy := make([]doctor.Issue, 20)
	for i := range heavy {
		heavy[i] = doctor.Issue{Severity: "error"}
	}
	c = scoreDoctorHealth(heavy)
	if c.Points != 0 {
		t.Errorf("clamped: want 0, got %d", c.Points)
	}
}

func TestScoreAuditClean(t *testing.T) {
	c := scoreAuditClean(0, 0)
	if c.Points != 20 || c.Status != "ok" {
		t.Errorf("clean: want 20 ok, got %+v", c)
	}
	c = scoreAuditClean(2, 3) // 2*3 + 3*1 = 9 → 11 points
	if c.Points != 11 || c.Status != "warning" {
		t.Errorf("2warn3info: want 11 warning, got %+v", c)
	}
	c = scoreAuditClean(0, 5) // status stays ok (no warnings) but details mention infos
	if c.Status != "ok" || c.Points != 15 {
		t.Errorf("infos-only: want 15 ok, got %+v", c)
	}
	if scoreAuditClean(50, 50).Points != 0 {
		t.Errorf("clamp: heavy penalty should clamp at 0")
	}
}

func TestScoreCompliance(t *testing.T) {
	// nil result, no error: full marks (no policy configured).
	c := scoreCompliance(nil, "")
	if c.Points != 15 || c.Status != "ok" {
		t.Errorf("no policy: want 15 ok, got %+v", c)
	}

	// Policy load failed.
	c = scoreCompliance(nil, "boom")
	if c.Points != 0 || c.Status != "error" {
		t.Errorf("load failed: want 0 error, got %+v", c)
	}

	// Compliant.
	c = scoreCompliance(&compliance.Result{Compliant: true}, "")
	if c.Points != 15 || c.Status != "ok" {
		t.Errorf("compliant: want 15 ok, got %+v", c)
	}

	// Errors penalised more than warnings.
	c = scoreCompliance(&compliance.Result{
		Compliant: false,
		Violations: []compliance.Violation{
			{Severity: "error"},
			{Severity: "warning"}, {Severity: "warning"},
		},
	}, "")
	if c.Status != "error" || c.Points != 6 { // 15 - 5 - 4
		t.Errorf("violations: want 6 error, got %+v", c)
	}
}

func TestScoreManagedSources(t *testing.T) {
	// All managed.
	tools := []registry.Tool{
		installedTool("a", "1", "1", registry.SourceBrew),
		installedTool("b", "1", "1", registry.SourceWinget),
	}
	c := scoreManagedSources(tools)
	if c.Points != 10 || c.Status != "ok" {
		t.Errorf("all-managed: want 10 ok, got %+v", c)
	}

	// 2 manual → -6 → 4 points warning.
	tools = []registry.Tool{
		installedTool("a", "1", "1", registry.SourceManual),
		installedTool("b", "1", "1", registry.SourceManual),
		installedTool("c", "1", "1", registry.SourceBrew),
	}
	c = scoreManagedSources(tools)
	if c.Points != 4 || c.Status != "warning" {
		t.Errorf("2-manual: want 4 warning, got %+v", c)
	}
}

func TestCompute_Integration(t *testing.T) {
	in := Input{
		Tools: []registry.Tool{
			installedTool("git", "2.50", "2.50", registry.SourceBrew),
		},
		// Doctor 0/0, audit 0/0, no compliance, all managed → perfect.
	}
	r := Compute(in)
	if r.Total != r.MaxTotal {
		t.Errorf("perfect inputs: want full marks, got %d/%d", r.Total, r.MaxTotal)
	}
	if r.Grade != "A+" {
		t.Errorf("perfect: want A+, got %s", r.Grade)
	}
	if got := len(r.Categories); got != 5 {
		t.Errorf("want 5 categories, got %d", got)
	}
	// MaxTotal must equal sum of category MaxPts (30+25+20+15+10 = 100).
	if r.MaxTotal != 100 {
		t.Errorf("max total: want 100, got %d", r.MaxTotal)
	}
}
