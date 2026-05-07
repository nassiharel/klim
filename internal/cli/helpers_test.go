package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/audit"
	"github.com/nassiharel/klim/internal/doctor"
	"github.com/nassiharel/klim/internal/manifest"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/teamfile"
)

// captureStdout swaps os.Stdout for a pipe, runs fn, restores Stdout,
// and returns whatever fn wrote.
func captureStdout(t *testing.T, fn func()) []byte {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan []byte, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.Bytes()
	}()

	fn()
	_ = w.Close()
	return <-done
}

// --- action.go helpers (not already covered by action_test.go) ---

func TestAction_String(t *testing.T) {
	cases := map[Action]string{
		ActionInstall: "install",
		ActionUpgrade: "upgrade",
		ActionRemove:  "remove",
	}
	for a, want := range cases {
		if got := a.String(); got != want {
			t.Errorf("%v.String(): want %q, got %q", a, want, got)
		}
	}
}

func TestCountResults(t *testing.T) {
	rs := []actionExecResult{
		{Err: nil}, {Err: errors.New("boom")}, {Err: nil}, {Err: errors.New("more")},
	}
	got, failed := countResults(rs)
	if got != 2 || failed != 2 {
		t.Errorf("want 2 / 2, got %d / %d", got, failed)
	}
	g, f := countResults(nil)
	if g != 0 || f != 0 {
		t.Errorf("nil: want 0 / 0, got %d / %d", g, f)
	}
}

// --- browser.go helper ---

func TestIsLoopbackAddr(t *testing.T) {
	cases := map[string]bool{
		"":             true,
		"localhost":    true,
		"  localhost ": true,
		"127.0.0.1":    true,
		"::1":          true,
		"127.1.2.3":    true,
		"8.8.8.8":      false,
		"192.168.1.1":  false,
		"not-an-ip":    false,
	}
	for in, want := range cases {
		if got := isLoopbackAddr(in); got != want {
			t.Errorf("isLoopbackAddr(%q): want %v, got %v", in, want, got)
		}
	}
	// Sanity: net.ParseIP agrees with our loopback verdict for known cases.
	if !net.ParseIP("127.0.0.1").IsLoopback() {
		t.Errorf("net.ParseIP sanity check failed")
	}
}

// --- compliance.go helper ---

func TestJoin(t *testing.T) {
	if got := join(nil); got != "" {
		t.Errorf("nil: want empty, got %q", got)
	}
	if got := join([]string{"a"}); got != "a" {
		t.Errorf("single: got %q", got)
	}
	if got := join([]string{"a", "b", "c"}); got != "a, b, c" {
		t.Errorf("triple: got %q", got)
	}
}

// --- audit.go helpers ---

func TestSortMapByCount(t *testing.T) {
	m := map[string]int{"MIT": 3, "Apache-2.0": 1, "BSD": 3, "GPL": 2}
	got := sortMapByCount(m)
	if len(got) != 4 {
		t.Fatalf("want 4 entries, got %d", len(got))
	}
	// Descending count, then name ascending for ties.
	wantOrder := []string{"BSD", "MIT", "GPL", "Apache-2.0"}
	for i, w := range wantOrder {
		if got[i].name != w {
			t.Errorf("position %d: want %s, got %s (full=%v)", i, w, got[i].name, got)
		}
	}
}

func TestSortMapByCount_Empty(t *testing.T) {
	if got := sortMapByCount(nil); len(got) != 0 {
		t.Errorf("nil: want empty, got %v", got)
	}
}

func TestPrintAuditJSON(t *testing.T) {
	// printAuditJSON returns a non-nil error when warnings > 0 so the
	// CLI exits with a non-zero status. Use a clean run here so we
	// can assert success.
	out := captureStdout(t, func() {
		if err := printAuditJSON(nil, 5, 0, 0, map[string]int{"MIT": 5}); err != nil {
			t.Errorf("clean printAuditJSON: %v", err)
		}
	})
	var got auditReport
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput=%s", err, string(out))
	}
	if got.Summary.TotalInstalled != 5 {
		t.Errorf("Summary.TotalInstalled: want 5, got %d", got.Summary.TotalInstalled)
	}

	// And separately: with warnings > 0 the helper signals via error.
	err := printAuditJSON(
		[]audit.Finding{{Severity: "warning", Tool: "git", Category: "Unmanaged"}},
		1, 1, 0, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "warning") {
		t.Errorf("warnings>0: want warning error, got %v", err)
	}
}

// --- check.go helpers ---

func TestPrintCheckJSON(t *testing.T) {
	tf := &teamfile.TeamFile{Name: "demo"}
	// All-satisfied case (no error).
	results := []teamfile.CheckResult{
		{Tool: teamfile.RequiredTool{Name: "git"}, Status: teamfile.StatusOK, Version: "2.50"},
	}
	out := captureStdout(t, func() {
		if err := printCheckJSON(tf, "/path/to/.klim.yaml", results, 1, 0, 0, 0); err != nil {
			t.Errorf("printCheckJSON ok: %v", err)
		}
	})
	var got jsonCheckOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v\noutput=%s", err, string(out))
	}
	if got.Project != "demo" {
		t.Errorf("project: want demo, got %s", got.Project)
	}
	if got.Summary.OK != 1 {
		t.Errorf("summary: want 1 ok, got %+v", got.Summary)
	}

	// With unsatisfied results, the helper returns an error so the
	// CLI exits non-zero. Capture and discard its stdout to avoid
	// noise; assert the error shape.
	captureStdout(t, func() {
		err := printCheckJSON(tf, "/p", []teamfile.CheckResult{
			{Tool: teamfile.RequiredTool{Name: "kubectl"}, Status: teamfile.StatusMissing},
		}, 0, 1, 0, 0)
		if err == nil {
			t.Errorf("missing tool: want error, got nil")
		}
	})
}

// printCheckLine writes formatted text; we don't pin the exact string,
// just verify it doesn't panic on each status type.
func TestPrintCheckLine_AllStatuses(t *testing.T) {
	for _, st := range []teamfile.CheckStatus{
		teamfile.StatusOK, teamfile.StatusMissing,
		teamfile.StatusOutdated, teamfile.StatusUnknown,
	} {
		// printCheckLine writes to stderr; capturing isn't critical
		// here — coverage is. Wrap in a recover guard so a misformat
		// doesn't kill the suite.
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("printCheckLine(status=%v) panicked: %v", st, r)
				}
			}()
			printCheckLine(teamfile.CheckResult{Tool: teamfile.RequiredTool{Name: "x"}, Version: "1", Status: st})
		}()
	}
}

// --- diff.go helper ---

func TestVersionsEqual(t *testing.T) {
	cases := []struct {
		local, remote string
		want          bool
	}{
		{"", "", true},                   // no remote constraint → match
		{"2.50", "", true},               // empty remote → match
		{"", "2.50", false},              // missing local → mismatch
		{"2.50", "2.50", true},           // exact
		{"v2.50", "2.50", true},          // local v-prefix tolerated
		{"2.50", "v2.50", true},          // remote v-prefix tolerated
		{"v2.50", "v2.50", true},         // both v-prefixed
		{"2.50", "2.51", false},          // mismatch
		{"2.50.0", "2.50", false},        // pin-level mismatch
	}
	for _, c := range cases {
		if got := versionsEqual(c.local, c.remote); got != c.want {
			t.Errorf("versionsEqual(%q,%q): want %v, got %v", c.local, c.remote, c.want, got)
		}
	}
}

// --- doctor.go helpers ---

func TestSeverityIcon(t *testing.T) {
	cases := map[doctor.Severity]string{
		doctor.SeverityError:   "✗",
		doctor.SeverityWarning: "⚠",
		doctor.SeverityInfo:    "ℹ",
		doctor.Severity("?"):   "?", // unknown returns literal "?" sentinel
	}
	for s, want := range cases {
		if got := severityIcon(s); got != want {
			t.Errorf("severityIcon(%v): want %q, got %q", s, want, got)
		}
	}
}

func TestSplitLines(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a\nb", []string{"a", "b"}},
		{"a\n\nb\n", []string{"a", "b"}}, // empty lines dropped
		{"\n", nil},
	}
	for _, c := range cases {
		got := splitLines(c.in)
		if !equalStrSlices(got, c.want) {
			t.Errorf("splitLines(%q): want %v, got %v", c.in, c.want, got)
		}
	}
}

func TestPrintDoctorJSON(t *testing.T) {
	// Healthy case: no issues, no error.
	out := captureStdout(t, func() {
		if err := printDoctorJSON(nil, 0, 0, 0); err != nil {
			t.Errorf("clean printDoctorJSON: %v", err)
		}
	})
	if !bytes.Contains(out, []byte("healthy")) {
		t.Errorf("expected healthy field in output, got %s", out)
	}

	// With errors: returns an error so CLI exits non-zero.
	captureStdout(t, func() {
		err := printDoctorJSON([]doctor.Issue{
			{Severity: doctor.SeverityError, Title: "PATH issue"},
		}, 1, 0, 0)
		if err == nil {
			t.Errorf("issues: want error, got nil")
		}
	})
}

// --- env.go helpers ---

func TestSortedMapKeys(t *testing.T) {
	m := map[string]bool{"banana": true, "apple": false, "cherry": true}
	got := sortedMapKeys(m)
	want := []string{"apple", "banana", "cherry"}
	if !equalStrSlices(got, want) {
		t.Errorf("want %v, got %v", want, got)
	}
	if got := sortedMapKeys(nil); len(got) != 0 {
		t.Errorf("nil: want empty, got %v", got)
	}
}

func TestJoinOrDash(t *testing.T) {
	if got := joinOrDash(nil); got != "—" {
		t.Errorf("nil: want dash, got %q", got)
	}
	if got := joinOrDash([]string{"a"}); got != "a" {
		t.Errorf("single: got %q", got)
	}
	if got := joinOrDash([]string{"a", "b"}); got != "a,b" {
		t.Errorf("multi: got %q (note: comma-separated, no space)", got)
	}
}

// --- errors.go ---

func TestUsageError(t *testing.T) {
	wrapped := errors.New("bad flag")
	ue := &UsageError{Err: wrapped}
	if ue.Error() != "bad flag" {
		t.Errorf("Error: want 'bad flag', got %q", ue.Error())
	}
	if !errors.Is(ue, wrapped) {
		t.Errorf("Is wrapped: want true")
	}
	// Unwrap via the explicit method.
	if got := ue.Unwrap(); got != wrapped {
		t.Errorf("Unwrap: want wrapped, got %v", got)
	}
}

func TestUsageErrorf(t *testing.T) {
	err := usageErrorf("missing %s", "name")
	if err.Error() != "missing name" {
		t.Errorf("Error: want 'missing name', got %q", err.Error())
	}
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Errorf("usageErrorf result should be a *UsageError")
	}
}

func TestPartialFailureError(t *testing.T) {
	err := &PartialFailureError{Op: "install", Succeeded: 3, Failed: 2}
	want := "install: 3 succeeded, 2 failed"
	if err.Error() != want {
		t.Errorf("want %q, got %q", want, err.Error())
	}
}

// --- import.go helper ---

func TestValidateManifest(t *testing.T) {
	m := &manifest.Manifest{
		Tools: []manifest.Tool{
			{Name: "git"},
			{Name: "kubectl"},
		},
	}
	if err := validateManifest(m); err != nil {
		t.Errorf("good manifest: want nil, got %v", err)
	}
	bad := &manifest.Manifest{
		Tools: []manifest.Tool{
			{Name: "git"},
			{Name: "  "},
		},
	}
	err := validateManifest(bad)
	if err == nil || !strings.Contains(err.Error(), "index 1") {
		t.Errorf("bad manifest: want error mentioning index 1, got %v", err)
	}
}

// --- info.go helpers ---

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"git", "git", 0},
		{"git", "got", 1},   // single substitution
		{"kitten", "sitting", 3},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q): want %d, got %d", c.a, c.b, c.want, got)
		}
	}
}

func TestMinInt(t *testing.T) {
	if got := minInt(2, 5); got != 2 {
		t.Errorf("want 2, got %d", got)
	}
	if got := minInt(7, 1); got != 1 {
		t.Errorf("want 1, got %d", got)
	}
	if got := minInt(3, 3); got != 3 {
		t.Errorf("equal: want 3, got %d", got)
	}
}

func TestClosestToolName(t *testing.T) {
	tools := []registry.Tool{
		{Name: "kubectl"}, {Name: "kubectx"}, {Name: "git"},
	}
	// Typo close to kubectl.
	if got := closestToolName(tools, "kubectn"); got != "kubectl" {
		t.Errorf("kubectn: want kubectl, got %s", got)
	}
	// Far enough to not match (distance > 3).
	if got := closestToolName(tools, "completely-different"); got != "" {
		t.Errorf("far miss: want empty, got %s", got)
	}
}

func TestDashIfEmpty(t *testing.T) {
	if got := dashIfEmpty(""); got != "—" {
		t.Errorf("empty: want dash, got %q", got)
	}
	if got := dashIfEmpty("foo"); got != "foo" {
		t.Errorf("non-empty: passthrough, got %q", got)
	}
}

func TestWordWrapStr(t *testing.T) {
	got := wordWrapStr("hello world", 5)
	if len(got) < 1 {
		t.Errorf("expected at least one wrapped line")
	}
}

// --- shared utility for slice comparison ---

func equalStrSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- spinner helper introduced earlier in this branch family ---

func TestSpinnerFor_TextVsStructured(t *testing.T) {
	textSp := spinnerFor(OutputText, "scanning")
	if textSp == nil {
		t.Errorf("text spinner should not be nil")
	}
	textSp.Stop()

	jsonSp := spinnerFor(OutputJSON, "scanning")
	if jsonSp == nil {
		t.Errorf("json spinner should not be nil")
	}
	// Calling all the methods on the silent spinner must be safe.
	jsonSp.Update("x")
	jsonSp.Done("ok")
	jsonSp.Fail("nope")
	jsonSp.Stop()
}
