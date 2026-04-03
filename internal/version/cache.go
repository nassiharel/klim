package version

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const cacheTTL = 1 * time.Hour

// Cache stores latest version lookups to avoid redundant API calls.
// Thread-safe. Persists to disk as JSON.
type Cache struct {
	path    string
	mu      sync.RWMutex
	entries map[string]CacheEntry
}

// CacheEntry is a single cached version lookup.
type CacheEntry struct {
	Version   string    `json:"version"`
	FetchedAt time.Time `json:"fetched_at"`
}

// LoadCache reads the cache from disk. Returns an empty cache on any error.
func LoadCache() *Cache {
	c := &Cache{
		entries: make(map[string]CacheEntry),
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return c
	}

	c.path = filepath.Join(dir, "clim", "cache.json")

	data, err := os.ReadFile(c.path)
	if err != nil {
		return c
	}

	_ = json.Unmarshal(data, &c.entries)
	return c
}

// Get returns a cached version if it exists and is not expired.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Since(entry.FetchedAt) > cacheTTL {
		return "", false
	}
	return entry.Version, true
}

// Set stores a version in the cache.
func (c *Cache) Set(key, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = CacheEntry{
		Version:   version,
		FetchedAt: time.Now(),
	}
}

// Save writes the cache to disk. Errors are silently ignored.
func (c *Cache) Save() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.path == "" {
		return
	}

	// Ensure directory exists.
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(c.path, data, 0o644)
}
