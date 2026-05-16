// Package health runs a set of read-only diagnostic checks against
// the live agent ecosystem (Snapshot + provider detection results)
// and reports findings as Issue records.
package health

import (
	"context"
	"sort"
	"time"
)

// Severity orders an Issue by urgency. Errors are user-blocking
// (broken JSON, unreachable HTTP MCPs); warnings are likely-broken
// configs (shadowed skills, duplicate MCP names); info is a heads-up
// (provider not installed, catalog hasn't synced recently).
type Severity int

// Severity values.
const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
)

// String returns a short label for a Severity.
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "ERROR"
	case SeverityWarn:
		return "WARN"
	}
	return "INFO"
}

// EntityKind describes which kind of object an Issue is about.
type EntityKind string

// EntityKind values.
const (
	KindMarketplace EntityKind = "marketplace"
	KindPlugin      EntityKind = "plugin"
	KindSkill       EntityKind = "skill"
	KindMCP         EntityKind = "mcp"
	KindSession     EntityKind = "session"
	KindProvider    EntityKind = "provider"
	KindConfigFile  EntityKind = "config_file"
)

// Issue is one diagnostic finding.
type Issue struct {
	CheckID  string
	Severity Severity
	Kind     EntityKind
	Subject  string
	Title    string
	Detail   string
	Hint     string
	Provider string
}

// Snapshot is the minimal view of the agent ecosystem each check
// needs. The agents package adapts its rich Snapshot type into this
// shape so the health package has no upstream cycle.
type Snapshot struct {
	Marketplaces []MarketplaceRef
	Plugins      []PluginRef
	Skills       []SkillRef
	MCPs         []MCPRef
	Providers    []ProviderRef
	ConfigFiles  []ConfigFileRef
}

// MarketplaceRef is the bare minimum we need to evaluate a marketplace.
type MarketplaceRef struct {
	Name       string
	Provider   string
	LastSynced time.Time
	URL        string
	Source     string
}

// PluginRef is the bare minimum we need to evaluate a plugin.
type PluginRef struct {
	Name        string
	Provider    string
	Marketplace string
	Installed   bool
	Enabled     bool
	InstallPath string
	Version     string
}

// SkillRef is the bare minimum we need to evaluate a skill.
type SkillRef struct {
	Name         string
	Provider     string
	Scope        string
	SourcePlugin string
	Path         string
}

// MCPRef is the bare minimum we need to evaluate an MCP.
type MCPRef struct {
	Name      string
	Provider  string
	Scope     string
	Transport string
	Command   string
	Args      []string
	URL       string
}

// ProviderRef carries a provider's detect result.
type ProviderRef struct {
	ID        string
	Installed bool
	Version   string
}

// ConfigFileRef describes a file we should attempt to parse as JSON.
type ConfigFileRef struct {
	Path     string
	Provider string
}

// Check is one named diagnostic.
type Check interface {
	ID() string
	Run(ctx context.Context, snap Snapshot) []Issue
}

// Run executes every check against the snapshot and returns the
// merged issue list sorted by severity then check id then subject.
func Run(ctx context.Context, snap Snapshot, checks []Check) []Issue {
	var issues []Issue
	for _, c := range checks {
		issues = append(issues, c.Run(ctx, snap)...)
	}
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Severity != issues[j].Severity {
			return issues[i].Severity > issues[j].Severity
		}
		if issues[i].CheckID != issues[j].CheckID {
			return issues[i].CheckID < issues[j].CheckID
		}
		return issues[i].Subject < issues[j].Subject
	})
	return issues
}

// DefaultChecks returns the registered set of built-in checks.
func DefaultChecks(httpProbe func(ctx context.Context, url string) error) []Check {
	if httpProbe == nil {
		httpProbe = DefaultHTTPProbe
	}
	return []Check{
		&checkMCPReach{HTTPProbe: httpProbe},
		&checkDuplicateMCP{},
		&checkShadowedSkill{},
		&checkPluginManifest{},
		&checkBrokenJSON{},
		&checkStaleCatalog{ThresholdDays: 14},
		&checkProviderInstalled{},
	}
}

// IssueCounts is a quick severity histogram used by the TUI header.
type IssueCounts struct {
	Error int
	Warn  int
	Info  int
}

// CountIssues returns a histogram across the issues list.
func CountIssues(issues []Issue) IssueCounts {
	var c IssueCounts
	for _, i := range issues {
		switch i.Severity {
		case SeverityError:
			c.Error++
		case SeverityWarn:
			c.Warn++
		case SeverityInfo:
			c.Info++
		}
	}
	return c
}
