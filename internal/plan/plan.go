// Package plan computes a "Terraform plan" for developer machines:
// given the current state of installed tools and a desired target
// (latest versions, a manifest, or an explicit tool list), it returns
// a structured Plan describing every change that would be made,
// along with risk warnings, disk-impact estimates, and a rough
// wall-clock time estimate.
//
// The package is purely declarative — it never installs, upgrades,
// or removes anything. CLI consumers render the Plan; the TUI can
// later offer to apply it via the existing install/upgrade/remove
// commands.
package plan

import (
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/registry"
)

// ChangeKind enumerates the kinds of state transitions a plan can
// propose.
type ChangeKind string

// Change kinds. Stable JSON identifiers.
const (
	ChangeInstall ChangeKind = "install"
	ChangeUpgrade ChangeKind = "upgrade"
	ChangeRemove  ChangeKind = "remove"
)

// Severity classifies a Risk.
type Severity string

// Severity levels. Mirrors the doctor package conventions so UIs can
// reuse the same colour palette.
const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

// Change is one proposed transition for a single tool.
type Change struct {
	Tool        string                 `json:"tool"`
	DisplayName string                 `json:"display_name,omitempty"`
	Source      registry.InstallSource `json:"source"`
	Kind        ChangeKind             `json:"kind"`
	FromVersion string                 `json:"from_version,omitempty"`
	ToVersion   string                 `json:"to_version,omitempty"`
	// Command is the human-readable command that would actually be
	// executed (e.g. `brew upgrade terraform`). It's the same string
	// `klim install` / `klim upgrade` / `klim remove` would invoke,
	// so a user comparing the plan to the running output sees the
	// same words.
	Command string `json:"command,omitempty"`
	// EstimatedTime is the per-tool wall-clock estimate baked into
	// the plan. Plan.Totals aggregates these.
	EstimatedTime time.Duration `json:"estimated_time_ns,omitempty"`
	// EstimatedDiskMB is the per-tool disk delta in megabytes.
	// Positive = added, negative = freed.
	EstimatedDiskMB int `json:"estimated_disk_mb,omitempty"`
	// Confidence is a 0-100 score reflecting how likely the change
	// is to apply cleanly without follow-up work. Computed by
	// computeConfidence from semantic-version delta, tool-specific
	// fragility, and installed-ecosystem signals. Only populated
	// for upgrades — installs and removes don't have a meaningful
	// "before" state to compare against.
	Confidence int `json:"confidence,omitempty"`
	// ConfidenceFactors enumerates every signal that contributed to
	// Confidence, in the order they were applied. Useful for the
	// "why is this only 48%?" follow-up the user inevitably has.
	ConfidenceFactors []ConfidenceFactor `json:"confidence_factors,omitempty"`
}

// ConfidenceFactor is one signal that nudged the confidence score
// up or down. Delta is the additive amount applied to the running
// score (negative = lowered confidence).
type ConfidenceFactor struct {
	Name   string `json:"name"`
	Delta  int    `json:"delta"`
	Reason string `json:"reason"`
}

// Risk is a heuristic warning attached to a planned change.
type Risk struct {
	Severity Severity `json:"severity"`
	Tool     string   `json:"tool,omitempty"`
	Message  string   `json:"message"`
}

// Totals carry the aggregated values rendered at the bottom of the
// plan output.
type Totals struct {
	ChangeCount       int           `json:"change_count"`
	EstimatedTime     time.Duration `json:"estimated_time_ns"`
	DiskAddedMB       int           `json:"disk_added_mb"`
	DiskReclaimableMB int           `json:"disk_reclaimable_mb"`
}

// Plan is the full "what would happen" model returned by Build.
type Plan struct {
	Changes []Change `json:"changes"`
	Risks   []Risk   `json:"risks,omitempty"`
	Totals  Totals   `json:"totals"`
}

// Options controls Plan construction.
type Options struct {
	// IncludeUpgrades = true plans upgrades for tools whose Latest
	// is newer than Installed. Default true.
	IncludeUpgrades bool
	// IncludeInstalls = true plans installs for desired tools that
	// are missing locally. Requires Desired to be non-nil.
	IncludeInstalls bool
	// IncludeRemoves = true plans removes for installed tools that
	// are explicitly listed for removal in Desired.
	IncludeRemoves bool
	// Desired is the optional target state: a map keyed by tool
	// name. When nil, Build treats "Latest version of every
	// installed tool" as the target — i.e. plan upgrades only.
	Desired map[string]DesiredState
	// OnlyTools restricts the plan to this set of tool names. nil
	// means "all".
	OnlyTools map[string]bool
}

// DesiredState declares the target version for a tool plus whether
// the tool is required or should be removed. Empty Version means
// "latest available".
type DesiredState struct {
	Version string
	Remove  bool
}

// Build computes a Plan from the current tool list and the supplied
// Options. The function is pure: no IO, no exec, no PATH lookups.
// Callers feed it a tool slice produced by service.LoadAndResolve
// (which is what `klim list` / the TUI use).
func Build(tools []registry.Tool, opts Options) Plan {
	if !opts.IncludeUpgrades && !opts.IncludeInstalls && !opts.IncludeRemoves {
		// Default behaviour when nothing is requested: upgrades
		// for every installed tool that has a newer Latest. That's
		// the most useful "tell me what's pending" call.
		opts.IncludeUpgrades = true
	}
	var changes []Change
	for _, t := range tools {
		if opts.OnlyTools != nil && !opts.OnlyTools[t.Name] {
			continue
		}
		change, ok := changeFor(t, opts)
		if !ok {
			continue
		}
		// Confidence is meaningful only for upgrades — there's no
		// "from" version to score against for installs or removes.
		// Leaving the fields zero for non-upgrades plays correctly
		// with the JSON `omitempty` tag so consumers don't see a
		// misleading "confidence: 100" on every install.
		if change.Kind == ChangeUpgrade {
			change.Confidence, change.ConfidenceFactors = computeConfidence(change, tools)
		}
		changes = append(changes, change)
	}

	// Sort by source then tool name so users always see PMs grouped
	// the same way in repeated runs.
	sort.SliceStable(changes, func(i, j int) bool {
		if changes[i].Source != changes[j].Source {
			return changes[i].Source < changes[j].Source
		}
		return strings.ToLower(changes[i].Tool) < strings.ToLower(changes[j].Tool)
	})

	risks := AnalyseRisks(changes, tools)
	totals := computeTotals(changes, tools)
	return Plan{Changes: changes, Risks: risks, Totals: totals}
}

// changeFor decides what Change (if any) applies to a single tool.
func changeFor(t registry.Tool, opts Options) (Change, bool) {
	source := t.Packages.BestInstallSource()
	if inst := t.PrimaryInstance(); inst != nil && inst.Source != "" {
		source = inst.Source
	}

	desired, hasDesired := opts.Desired[t.Name]

	// Remove path: explicit Desired.Remove.
	if opts.IncludeRemoves && hasDesired && desired.Remove && t.IsInstalled() {
		return Change{
			Tool:            t.Name,
			DisplayName:     fallbackName(t),
			Source:          source,
			Kind:            ChangeRemove,
			FromVersion:     t.InstalledVersion(),
			Command:         t.Packages.RemoveCmd(source),
			EstimatedTime:   timeFor(ChangeRemove, source),
			EstimatedDiskMB: -diskFor(ChangeRemove, source),
		}, true
	}

	// Install path: desired-but-not-installed.
	if opts.IncludeInstalls && hasDesired && !desired.Remove && !t.IsInstalled() {
		target := desired.Version
		if target == "" {
			target = t.Latest
		}
		return Change{
			Tool:            t.Name,
			DisplayName:     fallbackName(t),
			Source:          source,
			Kind:            ChangeInstall,
			ToVersion:       target,
			Command:         t.Packages.InstallCmd(source),
			EstimatedTime:   timeFor(ChangeInstall, source),
			EstimatedDiskMB: diskFor(ChangeInstall, source),
		}, true
	}

	// Upgrade path: installed with a newer Latest, or installed but
	// pinned to a specific Desired.Version that differs.
	if opts.IncludeUpgrades && t.IsInstalled() {
		current := t.InstalledVersion()
		target := t.Latest
		if hasDesired && desired.Version != "" && !desired.Remove {
			target = desired.Version
		}
		if target == "" || current == "" {
			return Change{}, false
		}
		if registry.VersionsMatch(current, target) {
			return Change{}, false
		}
		// Don't propose downgrades unless explicitly desired.
		if !hasDesired && registry.CompareVersions(target, current) <= 0 {
			return Change{}, false
		}
		return Change{
			Tool:            t.Name,
			DisplayName:     fallbackName(t),
			Source:          source,
			Kind:            ChangeUpgrade,
			FromVersion:     current,
			ToVersion:       target,
			Command:         t.Packages.UpgradeCmd(source),
			EstimatedTime:   timeFor(ChangeUpgrade, source),
			EstimatedDiskMB: diskFor(ChangeUpgrade, source),
		}, true
	}
	return Change{}, false
}

func computeTotals(changes []Change, tools []registry.Tool) Totals {
	var t Totals
	t.ChangeCount = len(changes)
	for _, c := range changes {
		t.EstimatedTime += c.EstimatedTime
		if c.EstimatedDiskMB > 0 {
			t.DiskAddedMB += c.EstimatedDiskMB
		} else {
			t.DiskReclaimableMB += -c.EstimatedDiskMB
		}
	}
	t.DiskReclaimableMB += reclaimableMBFromOldRuntimes(tools)
	return t
}

// reclaimableMBFromOldRuntimes scans Instances for tools with more
// than one installed copy and assumes the non-primary ones can be
// removed. Uses a rough per-source size heuristic to estimate the
// freed space.
func reclaimableMBFromOldRuntimes(tools []registry.Tool) int {
	total := 0
	for _, t := range tools {
		if len(t.Instances) < 2 {
			continue
		}
		for _, inst := range t.Instances[1:] {
			total += diskFor(ChangeRemove, inst.Source)
		}
	}
	return total
}

func fallbackName(t registry.Tool) string {
	if t.DisplayName != "" {
		return t.DisplayName
	}
	return t.Name
}
