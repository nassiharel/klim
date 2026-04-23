// Package paths provides a single source of truth for all clim
// configuration and data file paths. Every package that needs to
// locate a file under ~/.config/clim should call a function here
// instead of computing the path itself.
package paths

import (
	"os"
	"path/filepath"
)

// BaseDir returns the clim root directory (~/.config/clim or OS equivalent).
func BaseDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clim"), nil
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

// LogFile returns the path to the log file.
func LogFile() (string, error) {
	return Join("logs", "clim.log")
}
