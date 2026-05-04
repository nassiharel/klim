package cli

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
)

// Action discriminates the three sibling action commands.
type Action string

// Action values match the subcommand verb so they can be used directly in
// log lines and JSON output.
const (
	ActionInstall Action = "install"
	ActionUpgrade Action = "upgrade"
	ActionRemove  Action = "remove"
)

// String returns the verb for human-readable headers.
func (a Action) String() string { return string(a) }

// actionPlan holds a single tool-level action with its resolved command.
type actionPlan struct {
	name    string
	display string
	cmdArgs []string
	source  string
}

// bucketEntry pairs a stable tool name with its user-facing display
// label. Buckets that surface both in JSON (machine-readable, by name)
// and stderr text (human-readable, by display) need both.
type bucketEntry struct {
	Name    string
	Display string
}

// actionSummary classifies the requested targets into outcome buckets.
// Each bucket maps to a different stderr / JSON section so the caller can
// render and execute them independently.
type actionSummary struct {
	toExec []actionPlan

	// Targets that didn't make it into toExec, grouped by reason.
	// bucketEntry slices keep stable Name (used as JSON skipped[].name)
	// separate from Display (used in stderr text).
	alreadyInstalled []bucketEntry // install only
	notInstalled     []bucketEntry // upgrade / remove only
	upToDate         []bucketEntry // upgrade only
	selfProtected    []string      // remove only — refused for safety; name only
	unknown          []string      // name not in registry — no display known
	noPackage        []string      // no package ID for the current OS
	noPkgMgr         []string      // package ID exists but its PM isn't on PATH

	// Action records the verb used to build this summary so render code
	// doesn't need it threaded separately.
	Action Action
}

// resolveSource picks the source hint for an action invocation.
//
// Precedence (highest to lowest):
//  1. --source <pm> flag (per invocation)
//  2. cfg.Defaults.PreferredSource (global config)
//  3. "" — let resolveInstallPlan / resolveUpgradePlan / resolveRemovePlan
//     fall back to PackageIDs.BestInstallSource.
func resolveSource(flagHint string, cfg *config.Config) string {
	if v := strings.TrimSpace(flagHint); v != "" {
		return v
	}
	if cfg != nil {
		return strings.TrimSpace(cfg.Defaults.PreferredSource)
	}
	return ""
}

// validateSource ensures a non-empty --source value is one of the known
// package managers. Empty is always accepted (means "use precedence").
func validateSource(flagValue string) error {
	v := strings.TrimSpace(flagValue)
	if v == "" {
		return nil
	}
	switch registry.InstallSource(v) {
	case registry.SourceWinget, registry.SourceChoco, registry.SourceScoop,
		registry.SourceBrew, registry.SourceApt, registry.SourceSnap,
		registry.SourceNPM:
		return nil
	}
	return usageErrorf("unknown --source %q (expected one of: winget/choco/scoop/brew/apt/snap/npm)", v)
}

// resolveUpgradePlan picks the upgrade command for an installed tool.
//
// Source precedence:
//  1. sourceHint (--source flag or config default), if it maps to a
//     package id.
//  2. installedSource — the package manager the tool was actually
//     installed from (rt.PrimaryInstance().Source). This matches what
//     the TUI / web UI do and avoids running 'winget upgrade jq' on a
//     jq that was installed via scoop.
//  3. PackageIDs.BestInstallSource() — last-ditch OS-priority fallback
//     for the rare case where the installed source can't be detected.
//
// A nil plan with hasAny=true means "package id exists but no PM
// available"; with hasAny=false means "no package id for this OS".
func resolveUpgradePlan(name, display string, pkgs registry.PackageIDs, sourceHint string, installedSource registry.InstallSource) (*actionPlan, bool) {
	src := chooseActionSource(pkgs, sourceHint, installedSource, pkgs.UpgradeArgs)
	args := pkgs.UpgradeArgs(src)
	if args == nil {
		return nil, pkgs.HasAnyPackageForOS()
	}
	return &actionPlan{
		name:    name,
		display: cmp.Or(display, name),
		cmdArgs: args,
		source:  string(src),
	}, true
}

// resolveRemovePlan picks the remove command for an installed tool.
// Source precedence is the same as resolveUpgradePlan — using the
// installed source by default avoids the worst case where a different
// package manager owns the same name and we'd uninstall the wrong
// thing.
func resolveRemovePlan(name, display string, pkgs registry.PackageIDs, sourceHint string, installedSource registry.InstallSource) (*actionPlan, bool) {
	src := chooseActionSource(pkgs, sourceHint, installedSource, pkgs.RemoveArgs)
	args := pkgs.RemoveArgs(src)
	if args == nil {
		return nil, pkgs.HasAnyPackageForOS()
	}
	return &actionPlan{
		name:    name,
		display: cmp.Or(display, name),
		cmdArgs: args,
		source:  string(src),
	}, true
}

// chooseActionSource implements the upgrade/remove source precedence:
// flag/config hint first, then the installed source, then the
// OS-priority best source. argsFor is the verb-specific args function
// (UpgradeArgs / RemoveArgs) used to confirm a candidate source maps
// to a real command for this tool.
func chooseActionSource(pkgs registry.PackageIDs, sourceHint string, installedSource registry.InstallSource, argsFor func(registry.InstallSource) []string) registry.InstallSource {
	if sourceHint != "" {
		preferred := registry.InstallSource(sourceHint)
		if argsFor(preferred) != nil {
			return preferred
		}
	}
	if installedSource != "" {
		if argsFor(installedSource) != nil {
			return installedSource
		}
	}
	return pkgs.BestInstallSource()
}

// expandTargets merges positional tool names with --pack expansions and
// deduplicates by name. Unknown pack names are returned in the second
// slice so the caller can report and exit with a usage error.
func expandTargets(toolNames []string, packNames []string, packs []registry.Pack) (targets []string, unknownPacks []string) {
	seen := make(map[string]bool, len(toolNames))
	addName := func(n string) {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			return
		}
		seen[n] = true
		targets = append(targets, n)
	}
	for _, n := range toolNames {
		addName(n)
	}

	if len(packNames) > 0 {
		packMap := make(map[string]*registry.Pack, len(packs))
		for i := range packs {
			packMap[packs[i].Name] = &packs[i]
		}
		for _, pn := range packNames {
			pn = strings.TrimSpace(pn)
			if pn == "" {
				continue
			}
			pk, ok := packMap[pn]
			if !ok {
				unknownPacks = append(unknownPacks, pn)
				continue
			}
			for _, tn := range pk.ToolNames {
				addName(tn)
			}
		}
	}
	return targets, unknownPacks
}

// buildActionPlan classifies each target according to the action's
// semantics and produces an actionSummary ready for confirmation /
// execution. regMap is the catalog-resolved map of registry tools (with
// installed instances populated where applicable).
func buildActionPlan(action Action, targets []string, regMap map[string]*registry.Tool, sourceHint string) actionSummary {
	ps := actionSummary{Action: action}

	for _, name := range targets {
		// Self-protection (Remove only): refuse to uninstall clim
		// itself before the catalog lookup. Triggers regardless of
		// whether clim is in the marketplace, so the guard remains in
		// place if the catalog gains a clim entry later.
		if action == ActionRemove && name == "clim" {
			ps.selfProtected = append(ps.selfProtected, name)
			continue
		}

		rt, exists := regMap[name]
		if !exists {
			ps.unknown = append(ps.unknown, name)
			continue
		}
		display := cmp.Or(rt.DisplayName, rt.Name)

		switch action {
		case ActionInstall:
			if rt.IsInstalled() {
				ps.alreadyInstalled = append(ps.alreadyInstalled, bucketEntry{Name: rt.Name, Display: display})
				continue
			}
			plan, hasAny := resolveInstallPlanFor(rt, sourceHint)
			if plan == nil {
				ps.appendNoPM(name, hasAny)
				continue
			}
			ps.toExec = append(ps.toExec, *plan)

		case ActionUpgrade:
			if !rt.IsInstalled() {
				ps.notInstalled = append(ps.notInstalled, bucketEntry{Name: rt.Name, Display: display})
				continue
			}
			if !rt.HasUpdate() {
				ps.upToDate = append(ps.upToDate, bucketEntry{Name: rt.Name, Display: display})
				continue
			}
			plan, hasAny := resolveUpgradePlan(name, display, rt.Packages, sourceHint, installedSourceOf(rt))
			if plan == nil {
				ps.appendNoPM(name, hasAny)
				continue
			}
			ps.toExec = append(ps.toExec, *plan)

		case ActionRemove:
			if !rt.IsInstalled() {
				ps.notInstalled = append(ps.notInstalled, bucketEntry{Name: rt.Name, Display: display})
				continue
			}
			plan, hasAny := resolveRemovePlan(name, display, rt.Packages, sourceHint, installedSourceOf(rt))
			if plan == nil {
				ps.appendNoPM(name, hasAny)
				continue
			}
			ps.toExec = append(ps.toExec, *plan)
		}
	}
	return ps
}

// installedSourceOf returns the package manager the tool was actually
// installed from, or "" if no instance is recorded. Used as the
// default upgrade/remove source.
func installedSourceOf(rt *registry.Tool) registry.InstallSource {
	if inst := rt.PrimaryInstance(); inst != nil {
		return inst.Source
	}
	return ""
}

// resolveInstallPlanFor wraps resolveInstallPlan for registry tools so
// the install/upgrade/remove paths can share one shape.
func resolveInstallPlanFor(rt *registry.Tool, sourceHint string) (*actionPlan, bool) {
	plan, hasAny := resolveInstallPlan(rt.Name, rt.DisplayName, rt.Packages, sourceHint)
	if plan == nil {
		return nil, hasAny
	}
	return &actionPlan{
		name:    plan.name,
		display: plan.display,
		cmdArgs: plan.cmdArgs,
		source:  plan.source,
	}, true
}

func (ps *actionSummary) appendNoPM(name string, hasAny bool) {
	if hasAny {
		ps.noPkgMgr = append(ps.noPkgMgr, name)
	} else {
		ps.noPackage = append(ps.noPackage, name)
	}
}

// printActionSummary writes a human-readable summary to stderr.
func printActionSummary(ps actionSummary) {
	fmt.Fprintf(os.Stderr, "\n──── %s Plan ────\n\n", titleCase(string(ps.Action)))

	emitStrings := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		sorted := append([]string(nil), items...)
		sort.Strings(sorted)
		fmt.Fprintf(os.Stderr, "  %s (%d):\n", label, len(sorted))
		for _, s := range sorted {
			fmt.Fprintf(os.Stderr, "    · %s\n", s)
		}
		fmt.Fprintln(os.Stderr)
	}
	emitEntries := func(label string, items []bucketEntry) {
		if len(items) == 0 {
			return
		}
		// Render via Display (human-readable) so titles like "Visual
		// Studio Code" stay readable; Name (canonical) is emitted only
		// in the JSON output path.
		labels := make([]string, len(items))
		for i, e := range items {
			labels[i] = e.Display
		}
		emitStrings(label, labels)
	}

	switch ps.Action {
	case ActionInstall:
		emitEntries("✓ Already installed", ps.alreadyInstalled)
	case ActionUpgrade:
		emitEntries("✓ Up to date", ps.upToDate)
		emitEntries("· Not installed (skipped)", ps.notInstalled)
	case ActionRemove:
		emitEntries("· Not installed (skipped)", ps.notInstalled)
		emitStrings("⚠ Refused (clim self-removal blocked)", ps.selfProtected)
	}
	emitStrings("⚠ Not in catalog", ps.unknown)
	emitStrings(fmt.Sprintf("⚠ No package for %s", runtime.GOOS), ps.noPackage)
	emitStrings("⚠ No supported package manager", ps.noPkgMgr)

	if len(ps.toExec) > 0 {
		fmt.Fprintf(os.Stderr, "  To %s (%d):\n", ps.Action, len(ps.toExec))
		for _, p := range ps.toExec {
			fmt.Fprintf(os.Stderr, "    · %-20s  via %s\n", p.display, p.source)
		}
		fmt.Fprintln(os.Stderr)
	}
}

// executeActionPlans runs each plan sequentially. Returns per-tool
// results so callers can build structured output (JSON) and counts.
//
// streamStdout controls whether the subprocess's stdout is forwarded to
// our stdout. In text mode this is `true` so the user sees package-
// manager progress live; in JSON mode it MUST be `false` so the final
// JSON object on stdout stays parseable — instead the subprocess
// stdout is merged into stderr alongside its stderr.
func executeActionPlans(ctx context.Context, ps actionSummary, streamStdout bool) []actionExecResult {
	verb := presentParticiple(ps.Action)
	results := make([]actionExecResult, 0, len(ps.toExec))
	for _, p := range ps.toExec {
		fmt.Fprintf(os.Stderr, "\n──── %s %s via %s ────\n", verb, p.display, p.source)

		c := exec.CommandContext(ctx, p.cmdArgs[0], p.cmdArgs[1:]...) //nolint:gosec // G204: PM binary + package id, no user shell
		c.Stdin = os.Stdin
		if streamStdout {
			c.Stdout = os.Stdout
		} else {
			// JSON mode: keep our stdout reserved for the final JSON
			// payload. Forward the subprocess's stdout to our stderr
			// so the user still sees PM progress.
			c.Stdout = os.Stderr
		}
		c.Stderr = os.Stderr

		runErr := c.Run()
		res := actionExecResult{Plan: p}
		if runErr != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s failed: %s\n", p.display, runErr)
			res.Err = runErr
		} else {
			fmt.Fprintf(os.Stderr, "  ✓ %s %s\n", p.display, pastTense(ps.Action))
		}
		results = append(results, res)
	}
	return results
}

// presentParticiple returns the "-ing" form of an action verb. Hand-
// rolled because naive concatenation produces "installing" but also
// "upgradeing" / "removeing".
func presentParticiple(a Action) string {
	switch a {
	case ActionInstall:
		return "Installing"
	case ActionUpgrade:
		return "Upgrading"
	case ActionRemove:
		return "Removing"
	}
	// Fallback: best-effort title-cased verb without the trailing -e.
	v := strings.TrimSuffix(string(a), "e")
	return titleCase(v + "ing")
}

// actionExecResult captures the outcome of a single plan execution.
type actionExecResult struct {
	Plan actionPlan
	Err  error // nil on success
}

// countResults splits per-tool results into success/failure counts.
func countResults(rs []actionExecResult) (succeeded, failed int) {
	for _, r := range rs {
		if r.Err == nil {
			succeeded++
		} else {
			failed++
		}
	}
	return
}

func pastTense(a Action) string {
	switch a {
	case ActionInstall:
		return "installed"
	case ActionUpgrade:
		return "upgraded"
	case ActionRemove:
		return "removed"
	}
	return string(a) + "d"
}

// titleCase upper-cases the first ASCII letter of s. Sufficient for our
// fixed verb set ("install", "upgrade", "remove", "installing", etc.) —
// avoids strings.Title (deprecated) and the cases.Title import.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	b := []byte(s)
	if b[0] >= 'a' && b[0] <= 'z' {
		b[0] -= 'a' - 'A'
	}
	return string(b)
}
