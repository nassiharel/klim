// Package postcheck verifies that a klim apply left the developer
// machine in a working state. It is invoked by `klim plan apply` right
// after the PM commands return; failure surfaces a concrete rollback
// affordance to the user.
//
// Design goals (in priority order):
//
//  1. TRUST — every reported failure must be a regression caused by
//     the just-applied changes, not a pre-existing condition the
//     user has been living with. We accomplish this by always
//     comparing a Before snapshot to an After snapshot and only
//     flagging deltas. Pre-existing problems are still surfaced,
//     but as warnings, not failures.
//
//  2. ROBUST — every probe is timeout-bounded, every subprocess is
//     spawned via exec.CommandContext, every error path returns a
//     diagnostic instead of panicking. Missing PM binaries are
//     reported as Skip, not Fail.
//
//  3. EFFICIENT — binary probes run through a worker pool with a
//     configurable concurrency cap (default = runtime.NumCPU) and a
//     hard wall-clock ceiling on the entire run. Skipped probes are
//     reported as Skip so users know where the budget went.
//
//  4. CROSS-PLATFORM — Windows path case-insensitivity, .exe binary
//     extensions, and PowerShell-friendly PM probes are handled
//     explicitly. The package depends only on os/exec + filepath, no
//     OS-specific build tags.
//
//  5. WELL-DESIGNED — the public surface is a single Run function
//     returning a Result with a stable JSON schema. New checks plug
//     in by appending to runChecks.
package postcheck

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nassiharel/klim/internal/registry"
)

// Status enumerates per-check outcomes. Stable JSON identifiers.
type Status string

// Severity levels — pass/warn/fail/skip. Mirrors the doctor package
// conventions so UIs can reuse the same colour palette.
const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn" // surfaced to the user but does not trip auto-rollback
	StatusFail Status = "fail" // trips auto-rollback when invoked from `klim plan apply`
	StatusSkip Status = "skip" // not run (binary missing, no tools to probe, etc.)
)

// Check is one validation outcome.
type Check struct {
	Name   string   `json:"name"`
	Status Status   `json:"status"`
	Detail string   `json:"detail,omitempty"`
	Items  []string `json:"items,omitempty"`
	// Took is per-check elapsed time. Useful for users debugging
	// "why did postcheck take 12 seconds?" — the slow check is
	// obvious in the rendered table.
	Took time.Duration `json:"took"`
}

// Result is the aggregated outcome of one Run.
type Result struct {
	Checks  []Check       `json:"checks"`
	Failed  bool          `json:"failed"`
	Started time.Time     `json:"started"`
	Took    time.Duration `json:"took"`
	// Regressions enumerates the tool names that worked in Before
	// but fail post-apply. The CLI rendering highlights these
	// separately because they're the most actionable subset.
	Regressions []string `json:"regressions,omitempty"`
}

// Options tunes the run. Zero value runs every check with safe
// defaults.
type Options struct {
	// Concurrency caps parallel binary probes. 0 = runtime.NumCPU.
	Concurrency int
	// PerProbeTimeout is the timeout for a single subprocess
	// invocation. Default 4s.
	PerProbeTimeout time.Duration
	// WallClockBudget caps total Run time. Default 60s. Probes
	// still in flight when the budget elapses are recorded as
	// Skip with a "timed out" detail so users can tell which
	// checks didn't complete.
	WallClockBudget time.Duration
	// MaxBinaryProbes limits how many tools we probe; useful in
	// environments with hundreds of tools. 0 = no cap.
	MaxBinaryProbes int
	// SkipManagerIntegrity short-circuits the PM probe — e.g. in
	// CI containers where invoking brew is impossible.
	SkipManagerIntegrity bool
}

// Run executes every check and returns the aggregated Result.
// `before` is the snapshot captured immediately before the apply
// (typically the same tool slice fed to plan.Build); `after` is the
// freshly-rescanned post-apply state. Passing nil for Before disables
// regression detection — every failure is reported uncategorised.
func Run(before, after []registry.Tool, opts Options) Result {
	opts = opts.withDefaults()

	rootCtx, cancel := context.WithTimeout(context.Background(), opts.WallClockBudget)
	defer cancel()

	r := Result{Started: time.Now()}
	beforeIndex := indexInstalled(before)

	runChecks := []func(context.Context, []registry.Tool, map[string]registry.Tool, Options) Check{
		checkShellResolution,
		checkBinaryValidation,
		checkPATHConsistency,
		checkManagerIntegrity,
	}
	for _, fn := range runChecks {
		c := fn(rootCtx, after, beforeIndex, opts)
		r.Checks = append(r.Checks, c)
		if c.Status == StatusFail {
			r.Failed = true
		}
	}

	r.Regressions = computeRegressions(beforeIndex, after, r.Checks)
	r.Took = time.Since(r.Started)
	return r
}

func (o Options) withDefaults() Options {
	if o.Concurrency <= 0 {
		o.Concurrency = runtime.NumCPU()
	}
	if o.PerProbeTimeout <= 0 {
		o.PerProbeTimeout = 4 * time.Second
	}
	if o.WallClockBudget <= 0 {
		o.WallClockBudget = 60 * time.Second
	}
	return o
}

// indexInstalled produces a name → Tool lookup of every Tool with
// at least one Instance. Used to compute regressions later. Returns
// nil (not an empty map) when input is nil so downstream nil-checks
// distinguish "no pre-apply state available" from "pre-apply state
// captured zero installed tools."
func indexInstalled(tools []registry.Tool) map[string]registry.Tool {
	if tools == nil {
		return nil
	}
	idx := make(map[string]registry.Tool, len(tools))
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		idx[t.Name] = t
	}
	return idx
}

// computeRegressions walks every failure item and keeps the tool
// names that appeared in `before` but failed any check. The set is
// deduplicated and sorted for stable output.
func computeRegressions(before map[string]registry.Tool, _ []registry.Tool, checks []Check) []string {
	if before == nil {
		return nil
	}
	seen := make(map[string]bool)
	for _, c := range checks {
		if c.Status != StatusFail {
			continue
		}
		for _, item := range c.Items {
			name := strings.SplitN(item, " ", 2)[0]
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := before[name]; ok {
				seen[name] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// --- Individual checks ------------------------------------------------

// checkShellResolution: every previously-installed tool should still
// resolve via exec.LookPath on the current PATH. Membership in the
// `before` index already implies the tool resolved at pre-apply
// scan time (finder.FindAll only records an Instance when a real
// binary was found), so we don't re-probe — that would re-query
// the current PATH which already reflects the post-apply state.
//
// Decision matrix per tool that fails to resolve post-apply:
//
//	in before  →  regression (Fail)
//	not in before, before known →  pre-existing miss (Warn)
//	before unknown (nil) →  unclassified, surface as regression
func checkShellResolution(_ context.Context, after []registry.Tool, before map[string]registry.Tool, _ Options) Check {
	start := time.Now()
	check := Check{Name: "shell resolution"}

	if len(after) == 0 {
		check.Status = StatusSkip
		check.Detail = "no tools in scope"
		check.Took = time.Since(start)
		return check
	}

	var regressed, preexisting []string
	probed := 0
	for _, t := range after {
		if !t.IsInstalled() {
			continue
		}
		probed++
		if resolves(t) {
			continue
		}
		switch {
		case before == nil:
			regressed = append(regressed, t.Name)
		case mapContains(before, t.Name):
			regressed = append(regressed, t.Name)
		default:
			preexisting = append(preexisting, t.Name)
		}
	}

	switch {
	case probed == 0:
		check.Status = StatusSkip
		check.Detail = "no installed tools to probe"
	case len(regressed) > 0:
		check.Status = StatusFail
		check.Detail = fmt.Sprintf("%d tool(s) no longer on PATH (regression vs pre-apply)", len(regressed))
		check.Items = regressed
	case len(preexisting) > 0:
		check.Status = StatusWarn
		check.Detail = fmt.Sprintf("%d tool(s) not on PATH (also missing before apply)", len(preexisting))
		check.Items = preexisting
	default:
		check.Status = StatusPass
		check.Detail = fmt.Sprintf("%d installed tool(s) resolve via PATH", probed)
	}
	check.Took = time.Since(start)
	return check
}

func mapContains(m map[string]registry.Tool, key string) bool {
	_, ok := m[key]
	return ok
}

func resolves(t registry.Tool) bool {
	for _, bin := range candidateBinaries(t) {
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// candidateBinaries returns the set of names we'll try to resolve.
// We always try every BinaryNames entry, plus the bare tool name,
// plus the .exe-suffixed variant on Windows so finder-style
// matching stays consistent here.
func candidateBinaries(t registry.Tool) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(name string) {
		if name == "" || seen[strings.ToLower(name)] {
			return
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	for _, b := range t.BinaryNames {
		add(b)
	}
	add(t.Name)
	if runtime.GOOS == "windows" {
		// Mirror the variants finder.binaryCandidateNames produces
		// so resolution semantics match a real PATH walk.
		for _, b := range append([]string{}, out...) {
			if !strings.EqualFold(filepath.Ext(b), ".exe") {
				add(b + ".exe")
			}
			if !strings.EqualFold(filepath.Ext(b), ".cmd") {
				add(b + ".cmd")
			}
		}
	}
	return out
}

// checkBinaryValidation: each installed tool's primary binary
// exists, is non-empty, and (best-effort) runs. The probe tolerates
// tools that exit non-zero on --version by treating ANY output as a
// success signal — only a complete refusal-to-execute counts as
// broken.
//
// To distinguish post-apply regressions from pre-existing breakage
// we probe BOTH the after-binary and (when available) the
// before-binary. A tool is only a regression when the before-probe
// passed and the after-probe failed — anything else is reported as
// a pre-existing warning.
//
// Probes run through a worker pool sized by Options.Concurrency
// (default runtime.NumCPU) so we don't sequentially wait through
// 100 tools at 4s each. The parent context's wall-clock budget
// further caps the total time; probes still in-flight when it
// elapses get reported as Skip.
func checkBinaryValidation(ctx context.Context, after []registry.Tool, before map[string]registry.Tool, opts Options) Check {
	start := time.Now()
	check := Check{Name: "binary validation"}

	type job struct {
		name    string
		afterP  string
		beforeP string // empty when no pre-apply state captured for this tool
	}
	var jobs []job
	for _, t := range after {
		if !t.IsInstalled() {
			continue
		}
		if opts.MaxBinaryProbes > 0 && len(jobs) >= opts.MaxBinaryProbes {
			break
		}
		inst := t.PrimaryInstance()
		if inst == nil || inst.Path == "" {
			continue
		}
		j := job{name: t.Name, afterP: inst.Path}
		if before != nil {
			if pre, ok := before[t.Name]; ok {
				if preInst := pre.PrimaryInstance(); preInst != nil {
					j.beforeP = preInst.Path
				}
			}
		}
		jobs = append(jobs, j)
	}
	if len(jobs) == 0 {
		check.Status = StatusSkip
		check.Detail = "no installed tools to probe"
		check.Took = time.Since(start)
		return check
	}

	type result struct {
		name           string
		afterFailure   string // empty = post-apply probe passed
		beforeFailure  string // empty = pre-apply probe passed (or no pre-apply state)
		hadBeforeProbe bool
		skipped        bool
	}
	results := make([]result, len(jobs))

	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, j job) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				results[i] = result{name: j.name, skipped: true}
				return
			}
			r := result{name: j.name}
			r.afterFailure = probeBinary(ctx, j.afterP, opts.PerProbeTimeout)
			if j.beforeP != "" {
				r.hadBeforeProbe = true
				// Same path before and after? Skip the second
				// probe — the result is identical and we save
				// the budget. (Common after a "no-op" apply
				// where the same binary stays in place.)
				if j.beforeP == j.afterP {
					r.beforeFailure = r.afterFailure
				} else {
					r.beforeFailure = probeBinary(ctx, j.beforeP, opts.PerProbeTimeout)
				}
			}
			results[i] = r
		}(i, j)
	}
	wg.Wait()

	var regressed, preexisting []string
	var skipped int
	passed := 0
	for _, r := range results {
		switch {
		case r.skipped:
			skipped++
		case r.afterFailure == "":
			passed++
		case r.hadBeforeProbe && r.beforeFailure == "":
			// Worked before, fails now → real regression.
			regressed = append(regressed, r.name+": "+r.afterFailure)
		default:
			// Either no pre-apply probe (no before-state) or
			// the pre-apply binary was also broken.
			preexisting = append(preexisting, r.name+": "+r.afterFailure)
		}
	}

	switch {
	case len(regressed) > 0:
		check.Status = StatusFail
		check.Detail = fmt.Sprintf("%d binary(ies) broken after apply (regression vs pre-apply)", len(regressed))
		check.Items = regressed
	case before == nil && len(preexisting) > 0:
		// No pre-apply state available — surface every failure
		// as Fail. Without a baseline we can't claim "pre-existing."
		check.Status = StatusFail
		check.Detail = fmt.Sprintf("%d binary(ies) failed validation (no pre-apply baseline)", len(preexisting))
		check.Items = preexisting
	case len(preexisting) > 0:
		check.Status = StatusWarn
		check.Detail = fmt.Sprintf("%d binary(ies) failed but were already broken before apply", len(preexisting))
		check.Items = preexisting
	default:
		detail := fmt.Sprintf("%d binary(ies) verified", passed)
		if skipped > 0 {
			detail += fmt.Sprintf(" (%d skipped — wall-clock budget)", skipped)
		}
		if skipped == len(results) {
			check.Status = StatusSkip
			check.Detail = "no binaries probed (wall-clock budget exhausted)"
		} else {
			check.Status = StatusPass
			check.Detail = detail
		}
	}
	check.Took = time.Since(start)
	return check
}

// probeStat is retained as a fast-path "does this file plausibly
// exist as an executable" check. Kept available for future checks
// even though binary validation now re-probes — having a cheap
// stat-only check on the package surface saves callers from
// reinventing it.
//
//nolint:unused // public-shaped helper kept for future check plug-ins
func probeStat(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

// probeBinary returns "" when the binary is callable, or a short
// human-readable reason string when it is broken. The probe is
// deliberately generous: any combination of (exit 0) or (any output)
// counts as success. Only "could not start the process" or "no
// output and non-zero exit" is treated as failure.
func probeBinary(parent context.Context, path string, timeout time.Duration) string {
	info, err := os.Stat(path)
	if err != nil {
		return "stat failed: " + err.Error()
	}
	if info.IsDir() {
		return "not a regular file"
	}
	if info.Size() == 0 {
		return "binary is empty (zero bytes)"
	}
	if runtime.GOOS != "windows" {
		// Owner-executable bit. POSIX-specific; on Windows the
		// .exe extension is the marker (already filtered by
		// PATHEXT machinery).
		if info.Mode()&0o111 == 0 {
			return "binary is not executable"
		}
	}

	// Multi-flag probe. Stop on first success.
	for _, flag := range []string{"--version", "-V", "version", "-v", "--help"} {
		ctx, cancel := context.WithTimeout(parent, timeout)
		c := exec.CommandContext(ctx, path, flag)
		out, err := c.CombinedOutput()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			// Long-running invocation (e.g. `tool` with no flag
			// starts an interactive REPL). The fact that it ran
			// long enough to time out implies the binary is fine.
			return ""
		}
		if err == nil || len(out) > 0 {
			return ""
		}
		// "exec format error" or "permission denied" surface
		// through err — those are real breakage signals.
		var execErr *exec.Error
		if errors.As(err, &execErr) && execErr.Err != nil {
			if errors.Is(execErr.Err, exec.ErrNotFound) {
				return "binary not found"
			}
		}
	}
	return "binary did not respond to --version/-V/version/-v/--help"
}

// checkPATHConsistency: $PATH entries all exist as directories, no
// duplicates. This is a snapshot of the post-apply PATH state — we
// do NOT diff against a pre-apply baseline. Most users have one or
// two stale PATH entries permanently, and tripping auto-rollback on
// those would punish the user for the state of their machine rather
// than the apply itself. The check therefore reports issues as
// StatusWarn (visible, but doesn't flip Result.Failed).
//
// A future iteration could accept pre/post PATH snapshots and
// promote NEW issues to StatusFail, but doing so would require
// threading the env state through Run; today's contract is "warn-
// only". The signature still takes `before` for API symmetry with
// the other check functions but the parameter is intentionally
// unused.
func checkPATHConsistency(_ context.Context, _ []registry.Tool, _ map[string]registry.Tool, _ Options) Check {
	start := time.Now()
	check := Check{Name: "PATH consistency"}

	raw := os.Getenv("PATH")
	if raw == "" {
		check.Status = StatusFail
		check.Detail = "$PATH is empty"
		check.Took = time.Since(start)
		return check
	}
	parts := filepath.SplitList(raw)
	seen := make(map[string]int, len(parts))
	var issues []string
	for i, entry := range parts {
		clean := strings.TrimSpace(entry)
		if clean == "" {
			continue
		}
		norm := normalisePath(clean)
		if prev, ok := seen[norm]; ok {
			issues = append(issues, fmt.Sprintf("duplicate at position %d (first seen at %d): %s", i+1, prev+1, clean))
			continue
		}
		seen[norm] = i
		info, err := os.Stat(clean) //nolint:gosec // G304/G703: clean originates from $PATH; auditing PATH is the point of this check.
		switch {
		case os.IsNotExist(err):
			issues = append(issues, "missing directory: "+clean)
		case err != nil && !os.IsPermission(err):
			issues = append(issues, "stat error: "+clean+": "+err.Error())
		case err == nil && !info.IsDir():
			issues = append(issues, "not a directory: "+clean)
		}
	}
	switch {
	case len(issues) == 0:
		check.Status = StatusPass
		check.Detail = fmt.Sprintf("%d PATH entr%s verified", len(parts), pluralIE(len(parts)))
	default:
		// PATH issues rarely correlate with the apply being
		// broken; surface as Warn so they don't trip auto-rollback.
		check.Status = StatusWarn
		check.Detail = fmt.Sprintf("%d PATH issue(s)", len(issues))
		check.Items = issues
	}
	check.Took = time.Since(start)
	return check
}

func normalisePath(p string) string {
	p = filepath.Clean(strings.TrimSpace(p))
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// checkManagerIntegrity: every package manager that owns at least
// one installed tool responds healthily to a fast probe. PMs that
// aren't on PATH at all (e.g. brew on Windows) are skipped, not
// failed — the user explicitly opted in by having tools tracked
// under that source.
func checkManagerIntegrity(ctx context.Context, after []registry.Tool, _ map[string]registry.Tool, opts Options) Check {
	start := time.Now()
	check := Check{Name: "manager integrity"}

	if opts.SkipManagerIntegrity {
		check.Status = StatusSkip
		check.Detail = "skipped by caller"
		check.Took = time.Since(start)
		return check
	}
	managers := managerSet(after)
	if len(managers) == 0 {
		check.Status = StatusSkip
		check.Detail = "no PM-tracked tools installed"
		check.Took = time.Since(start)
		return check
	}

	type result struct {
		source registry.InstallSource
		err    error
		skip   bool
	}
	results := make([]result, len(managers))
	probes := managerProbes()

	var wg sync.WaitGroup
	for i, m := range managers {
		args, hasProbe := probes[m]
		if !hasProbe {
			results[i] = result{source: m, skip: true}
			continue
		}
		wg.Add(1)
		go func(i int, m registry.InstallSource, args []string) {
			defer wg.Done()
			// Fast existence check first — saves a process spawn
			// when the PM isn't installed at all.
			if _, err := exec.LookPath(args[0]); err != nil {
				results[i] = result{source: m, skip: true}
				return
			}
			cctx, cancel := context.WithTimeout(ctx, opts.PerProbeTimeout)
			defer cancel()
			c := exec.CommandContext(cctx, args[0], args[1:]...)
			out, err := c.CombinedOutput()
			if err == nil || len(out) > 0 {
				results[i] = result{source: m}
				return
			}
			results[i] = result{source: m, err: err}
		}(i, m, args)
	}
	wg.Wait()

	var broken, skipped []string
	healthy := 0
	for _, r := range results {
		switch {
		case r.skip:
			skipped = append(skipped, string(r.source))
		case r.err != nil:
			broken = append(broken, fmt.Sprintf("%s (%s)", r.source, r.err.Error()))
		default:
			healthy++
		}
	}
	switch {
	case len(broken) > 0:
		check.Status = StatusFail
		check.Detail = fmt.Sprintf("%d package manager(s) failed health probe", len(broken))
		check.Items = broken
	case healthy == 0:
		check.Status = StatusSkip
		check.Detail = fmt.Sprintf("no probable PMs on PATH (%d skipped)", len(skipped))
	default:
		detail := fmt.Sprintf("%d package manager(s) healthy", healthy)
		if len(skipped) > 0 {
			detail += fmt.Sprintf(" (%d not on PATH — skipped)", len(skipped))
		}
		check.Status = StatusPass
		check.Detail = detail
	}
	check.Took = time.Since(start)
	return check
}

func managerSet(tools []registry.Tool) []registry.InstallSource {
	seen := make(map[registry.InstallSource]bool)
	var out []registry.InstallSource
	for _, t := range tools {
		for _, inst := range t.Instances {
			src := inst.Source
			if src == "" || src == registry.SourceManual {
				continue
			}
			if !seen[src] {
				seen[src] = true
				out = append(out, src)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func managerProbes() map[registry.InstallSource][]string {
	return map[registry.InstallSource][]string{
		registry.SourceWinget: {"winget", "--version"},
		registry.SourceChoco:  {"choco", "--version"},
		registry.SourceScoop:  {"scoop", "--version"},
		registry.SourceBrew:   {"brew", "--version"},
		registry.SourceApt:    {"apt", "--version"},
		registry.SourceSnap:   {"snap", "version"},
		registry.SourceNPM:    {"npm", "--version"},
	}
}

func pluralIE(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
