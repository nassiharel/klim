// Package paths provides a single source of truth for all klim
// configuration and data file paths. Every package that needs to
// locate a file under ~/.klim should call a function here instead
// of computing the path itself.
package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// BaseDir returns the klim root directory (~/.klim).
//
// klim uses a single dotfile directory under the user's home for
// every platform, matching the convention of git, npm, kubectl, aws,
// terraform, helm, etc. — tools developers tend to want in one
// predictable place. Cross-platform `os.UserHomeDir()` resolves to
// `$HOME` on Unix and `%USERPROFILE%` on Windows.
//
// KLIM_HOME overrides the default. This is primarily a test hook —
// it lets `t.Setenv("KLIM_HOME", t.TempDir())` actually isolate a
// test from the real user dotfile (a previous version of the
// self-update cache test ignored this and polluted real users'
// caches when `go test ./...` ran). Production users shouldn't
// normally need to set it; if they do, every klim path moves with
// it as a unit (no per-file overrides).
func BaseDir() (string, error) {
	if v := os.Getenv("KLIM_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".klim"), nil
}

// Join returns BaseDir()/segments... as a single path.
func Join(segments ...string) (string, error) {
	base, err := BaseDir()
	if err != nil {
		return "", err
	}
	parts := append([]string{base}, segments...)
	return filepath.Join(parts...), nil
}

// Config returns the path to config.yaml.
func Config() (string, error) {
	return Join("config", "config.yaml")
}

// Favorites returns the path to favorites.yaml.
func Favorites() (string, error) {
	return Join("favorites", "favorites.yaml")
}

// CustomPacks returns the path to custom-packs.yaml.
func CustomPacks() (string, error) {
	return Join("marketplace", "custom-packs.yaml")
}

// ScanCache returns the path to the scan cache file.
func ScanCache() (string, error) {
	return Join("cache", "scan-cache.yaml")
}

// CatalogCache returns the path to the marketplace cache file.
func CatalogCache() (string, error) {
	return Join("marketplace", "marketplace-cache.yaml")
}

// BackupsDir returns the path to the backups directory.
func BackupsDir() (string, error) {
	return Join("backups")
}

// PathBackupsDir returns the directory where PATH backups live before
// Health → Issues applies any PATH-modifying fix. Kept separate from
// the toolchain export backups so the user can wipe one without
// touching the other.
func PathBackupsDir() (string, error) {
	return Join("backups", "path")
}

// CheckpointsDir returns the directory where named "snapshot" files
// from `klim checkpoint` are stored, one per checkpoint.
func CheckpointsDir() (string, error) {
	return Join("checkpoints")
}

// LogFile returns the path to the log file.
func LogFile() (string, error) {
	return Join("logs", "klim.log")
}

// ShimsDir returns the path to the proxy shims directory.
func ShimsDir() (string, error) {
	return Join("shims")
}

// CompliancePolicy returns the path to the compliance policy file.
func CompliancePolicy() (string, error) {
	return Join("compliance", "policy.yaml")
}

// ComplianceCachePath returns the path to the cached remote policy
// (unkeyed default — used as a fallback when no source key is given).
func ComplianceCachePath() (string, error) {
	return Join("compliance", "policy-cache.yaml")
}

// ComplianceCachePathFor returns a per-source cache path. Keying the
// cache by source URL hash keeps different policy hosts from clobbering
// each other's cached payloads — switching compliance.url to a
// different endpoint no longer silently reuses the old URL's policy.
func ComplianceCachePathFor(key string) (string, error) {
	if key == "" {
		return ComplianceCachePath()
	}
	sum := sha256.Sum256([]byte(key))
	short := hex.EncodeToString(sum[:6]) // 12 hex chars — enough to avoid collisions
	return Join("compliance", "policy-cache-"+short+".yaml")
}

// VulnCachePath returns the unkeyed default path for the vulnerability
// scan cache. Per-source variants live alongside via VulnCachePathFor.
func VulnCachePath() (string, error) {
	return Join("vuln", "cache.yaml")
}

// VulnCachePathFor returns a per-source vulnerability cache path,
// keyed by sha256 of the source URL. Same machinery as
// ComplianceCachePathFor — switching vuln.url to a different mirror
// produces a distinct cache file.
func VulnCachePathFor(key string) (string, error) {
	if key == "" {
		return VulnCachePath()
	}
	sum := sha256.Sum256([]byte(key))
	short := hex.EncodeToString(sum[:6])
	return Join("vuln", "cache-"+short+".yaml")
}

// TrailDir returns the path to the trail (env-history) directory.
func TrailDir() (string, error) {
	return Join("trail")
}

// TrailHEAD returns the path to the trail HEAD file (single-line index pointer).
func TrailHEAD() (string, error) {
	return Join("trail", "HEAD")
}

// TrailLog returns the path to the trail log file (ordered list of entries).
func TrailLog() (string, error) {
	return Join("trail", "log.yaml")
}

// TrailLock returns the path to the trail advisory lock file.
// The lock guards read-modify-write on TrailLog and TrailHEAD across processes.
func TrailLock() (string, error) {
	return Join("trail", "log.lock")
}

// TrailObjects returns the path to the trail content-addressed objects directory.
// Snapshots live under TrailObjects()/<aa>/<bb...>.yaml where the leading two
// hex chars provide directory fanout to keep any single dir small.
func TrailObjects() (string, error) {
	return Join("trail", "objects")
}

// AgentsDir returns the root directory for all agents-related state
// under ~/.klim. Everything written by the Agents tab — scan cache,
// per-source marketplace catalog files, costs cache, search index,
// bookmarks — lives under this directory so the layout is
// self-contained and easy to inspect.
//
// Note: pre-0.1.4 versions of klim wrote ~/.klim/agent-bookmarks.yaml
// at the top level; bookmarks.Load() migrates that file once on first
// read (see AgentBookmarksLegacy below) and is the only legacy path
// still honored. New code should never touch the legacy location.
func AgentsDir() (string, error) {
	return Join("agents")
}

// AgentsCache returns the path to the agents tab scan cache file.
// Cached entries: detected providers, plugins, skills, MCPs, sessions, and
// marketplace status per host. Invalidated by `r` in the TUI or `--refresh`.
func AgentsCache() (string, error) {
	return Join("agents", "cache.yaml")
}

// AgentsCatalogDir returns the directory holding per-source agent
// marketplace catalog caches. Each remote source (Anthropic
// marketplace, GitHub copilot-plugins, MCP registry) gets its own
// file inside this directory so a stale fetch on one source doesn't
// invalidate the others.
func AgentsCatalogDir() (string, error) {
	return Join("agents", "catalog")
}

// AgentCostsCache returns the path to the per-session token-count cache
// used by the Agents → Costs sub-tab. Keyed by transcript mtime so we
// only reparse sessions that actually changed.
func AgentCostsCache() (string, error) {
	return Join("agents", "costs.yaml")
}

// AgentSearchIndex returns the path to the persisted full-text search
// index for agent session transcripts.
func AgentSearchIndex() (string, error) {
	return Join("agents", "search-index.yaml")
}

// AgentBookmarks returns the path to the session-bookmarks file
// (persistent across runs, written atomically on each toggle/note).
//
// Klim < 0.1.4 wrote this file directly at the root of ~/.klim/ —
// the only state file not nested under a subdirectory. From 0.1.4
// on, it lives at ~/.klim/agents/bookmarks.yaml alongside other
// agent-specific state. bookmarks.Load() migrates the legacy file
// transparently on first read.
func AgentBookmarks() (string, error) {
	return Join("agents", "bookmarks.yaml")
}

// AgentBookmarksLegacy returns the pre-0.1.4 location of the
// agent-bookmarks file (~/.klim/agent-bookmarks.yaml). Used by the
// bookmarks package's migration path; new callers should not use it.
func AgentBookmarksLegacy() (string, error) {
	return Join("agent-bookmarks.yaml")
}
