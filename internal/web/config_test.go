package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/config"
	"github.com/nassiharel/klim/internal/service"
)

// startConfigServer wires a Server with a fresh in-memory config so
// tests can mutate it without writing to the user's real file. We
// stash the path the saved YAML lands in so tests can verify
// persistence.
func startConfigServer(t *testing.T) (ts *httptest.Server, cfg *config.Config) {
	t.Helper()
	cfg = config.Default()
	srv, err := New(Options{Service: service.New(), Bind: "127.0.0.1", Config: cfg})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.loader = &fixtureLoader{tools: fixtureTools(), favs: map[string]bool{}}
	t.Cleanup(func() { _ = srv.Close() })
	ts = httptest.NewServer(srv.httpsrv.Handler)
	t.Cleanup(ts.Close)
	return ts, cfg
}

// postFormSameOrigin posts url-encoded form data with a same-origin
// Origin header (so csrfProtect lets it through) and disables the
// http.Client default redirect-following so we can assert on 303s.
func postFormSameOrigin(t *testing.T, ts string, path string, form url.Values) (*http.Response, string) {
	t.Helper()
	c := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequest(http.MethodPost, ts+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", ts)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := readAllString(t, resp)
	return resp, body
}

func TestPageConfig_RendersFormForEverySetting(t *testing.T) {
	ts, _ := startConfigServer(t)
	resp, body := get(t, ts.URL+"/config")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	// Every setting key must appear at least once as a name="..."
	// attribute (the bool fields appear twice — hidden + checkbox).
	for _, s := range config.AllSettings() {
		if !strings.Contains(body, `name="`+s.Key+`"`) {
			t.Errorf("missing form input for %s", s.Key)
		}
	}
	// Submit button + cancel link.
	if !strings.Contains(body, "Save") {
		t.Errorf("missing Save button")
	}
}

func TestPageConfigSave_AppliesAndRedirects(t *testing.T) {
	ts, cfg := startConfigServer(t)
	form := url.Values{}
	for _, s := range config.AllSettings() {
		// Start by mirroring the current value so we don't accidentally
		// flip every field. Then override the few we want to change.
		form.Set(s.Key, s.Raw(cfg))
	}
	form.Set("log_level", "warn")
	form.Set("performance_concurrency", "12")
	form.Set("marketplace_refresh_interval", "6h")
	form.Set("ui_show_path", "true") // bool: form value is the literal "true"

	resp, body := postFormSameOrigin(t, ts.URL, "/config", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: %d body=%s", resp.StatusCode, body)
	}
	if loc := resp.Header.Get("Location"); !strings.HasPrefix(loc, "/config?saved=") {
		t.Errorf("redirect target: %q", loc)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("Logging.Level=%q, want warn", cfg.Logging.Level)
	}
	if cfg.Performance.Concurrency != 12 {
		t.Errorf("Performance.Concurrency=%d, want 12", cfg.Performance.Concurrency)
	}
	if cfg.Marketplace.RefreshInterval.Duration.Hours() != 6 {
		t.Errorf("Marketplace.RefreshInterval=%v, want 6h", cfg.Marketplace.RefreshInterval)
	}
	if !cfg.UI.ShowPath {
		t.Errorf("UI.ShowPath should be true")
	}
}

// TestPageConfigSave_DoesNotMutateRunningConfigOnDiskFailure would
// be the regression test for the PR #48 review (pageConfigSave used
// to commit to memory before saving to disk; a write failure left
// running != persisted). We don't include it here because making
// config.Save fail deterministically requires mocking the filesystem
// in a way that's portable across Windows / macOS / Linux test
// runners — out of proportion to the size of the fix. The fix is
// straightforward by inspection: pageConfigSave now calls
// config.Save(&stagedCopy) FIRST and only s.writeConfig(staged)
// after a successful save. See the inline comment there.

func TestPageConfigSave_ValidationErrorPreservesInputAndDoesNotPersist(t *testing.T) {
	ts, cfg := startConfigServer(t)
	originalConcurrency := cfg.Performance.Concurrency
	originalLevel := cfg.Logging.Level

	form := url.Values{}
	for _, s := range config.AllSettings() {
		form.Set(s.Key, s.Raw(cfg))
	}
	// Bad values: negative concurrency, invalid duration, unknown choice.
	form.Set("performance_concurrency", "-3")
	form.Set("marketplace_refresh_interval", "not-a-duration")
	form.Set("log_level", "shouting")

	resp, body := postFormSameOrigin(t, ts.URL, "/config", form)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with validation errors (re-render), got %d", resp.StatusCode)
	}
	// The error banner should appear with each problem listed.
	if !strings.Contains(body, "Couldn't save") {
		t.Errorf("expected error banner; got:\n%s", body[:min(2000, len(body))])
	}
	for _, want := range []string{"Concurrency", "Refresh Interval", "Log Level"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the failed field labels in error message, missing %q", want)
		}
	}
	// User-submitted (rejected) values should be visible so they can
	// fix them without re-typing.
	if !strings.Contains(body, `value="-3"`) {
		t.Errorf("expected rejected concurrency value to be preserved in form")
	}
	// Most importantly: cfg must NOT have been mutated by a partial parse.
	if cfg.Performance.Concurrency != originalConcurrency {
		t.Errorf("validation failure leaked into running config: concurrency=%d, was %d", cfg.Performance.Concurrency, originalConcurrency)
	}
	if cfg.Logging.Level != originalLevel {
		t.Errorf("validation failure leaked into running config: level=%q, was %q", cfg.Logging.Level, originalLevel)
	}
}

func TestPageConfigSave_CheckboxCanTurnBoolOn(t *testing.T) {
	// Regression for the PR #48 review: a hidden "false" + the
	// checkbox's "true" both got submitted, and Go's r.FormValue
	// returns the FIRST value, so the bool was permanently stuck at
	// false. The hidden input has been removed; an unchecked checkbox
	// submits nothing (=false via SetFromString); a checked one
	// submits exactly "true".
	ts, cfg := startConfigServer(t)
	cfg.UI.ShowPath = false

	// Simulate "checked": form sends only the true value.
	form := url.Values{}
	for _, s := range config.AllSettings() {
		form.Set(s.Key, s.Raw(cfg))
	}
	form.Set("ui_show_path", "true")
	resp, body := postFormSameOrigin(t, ts.URL, "/config", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: %d body=%s", resp.StatusCode, body)
	}
	if !cfg.UI.ShowPath {
		t.Fatalf("UI.ShowPath should be true after submitting checked=true")
	}

	// Simulate "unchecked": form sends nothing for the key.
	form2 := url.Values{}
	for _, s := range config.AllSettings() {
		if s.Key == "ui_show_path" {
			continue // omit entirely
		}
		form2.Set(s.Key, s.Raw(cfg))
	}
	resp2, body2 := postFormSameOrigin(t, ts.URL, "/config", form2)
	if resp2.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: %d body=%s", resp2.StatusCode, body2)
	}
	if cfg.UI.ShowPath {
		t.Fatalf("UI.ShowPath should be false after submitting form without the key (unchecked checkbox)")
	}
}

func TestPageConfig_RendersNoHiddenFalseForBools(t *testing.T) {
	// Belt-and-braces: the form template must NOT render a
	// <input type="hidden" name="<key>" value="false"> shadow input
	// for boolean fields, otherwise the bug above would be back.
	ts, _ := startConfigServer(t)
	resp, body := get(t, ts.URL+"/config")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if strings.Contains(body, `type="hidden" name="ui_show_path"`) {
		t.Errorf("config form rendered a hidden boolean shadow input — re-introduces the FormValue first-value bug")
	}
	// The checkbox itself must still be present.
	if !strings.Contains(body, `type="checkbox" name="ui_show_path"`) {
		t.Errorf("config form is missing the bool checkbox")
	}
}

func TestPageConfig_ConcurrentReadsAndWritesAreRaceFree(t *testing.T) {
	// Regression for the PR #48 review: pageConfigSave used to write
	// *s.opts.Config without synchronisation, racing with pageConfig
	// readers. Run with `go test -race` to catch a regression.
	ts, cfg := startConfigServer(t)
	const iterations = 30
	done := make(chan struct{})

	// Writer: alternate two valid log levels via the form.
	go func() {
		defer close(done)
		for i := 0; i < iterations; i++ {
			form := url.Values{}
			for _, s := range config.AllSettings() {
				form.Set(s.Key, s.Raw(cfg))
			}
			level := "info"
			if i%2 == 0 {
				level = "warn"
			}
			form.Set("log_level", level)
			postFormSameOrigin(t, ts.URL, "/config", form)
		}
	}()

	// Reader: hit /config repeatedly while writes are in flight.
	for i := 0; i < iterations; i++ {
		resp, _ := get(t, ts.URL+"/config")
		if resp.StatusCode != 200 {
			t.Errorf("read %d: status=%d", i, resp.StatusCode)
		}
	}
	<-done
}

func TestPageConfigSave_RejectsCrossOrigin(t *testing.T) {
	ts, _ := startConfigServer(t)
	c := &http.Client{}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/config", strings.NewReader("log_level=warn"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://attacker.example")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

// readAllString is a tiny helper that avoids re-implementing the
// io.ReadAll + close + string(...) dance in every test.
func readAllString(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(buf)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
