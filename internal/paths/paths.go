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
func BaseDir() (string, error) {
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
