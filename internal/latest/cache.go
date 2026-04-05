package latest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cache provides a simple disk-backed key-value cache with TTL.
// Thread-safe for concurrent reads and writes.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	path    string
	ttl     time.Duration
}

type cacheEntry struct {
	Value     string    `json:"value"`
	ExpiresAt time.Time `json:"expires_at"`
}

type cacheFile struct {
	Entries map[string]cacheEntry `json:"entries"`
}

// NewCache creates a cache backed by the given file path with the specified TTL.
func NewCache(path string, ttl time.Duration) *Cache {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		path:    path,
		ttl:     ttl,
	}
	c.load()
	return c
}

// DefaultCache returns a cache at the standard config location with 1-hour TTL.
func DefaultCache() *Cache {
	dir, err := os.UserConfigDir()
	if err != nil {
		return NewCache("", time.Hour)
	}
	return NewCache(filepath.Join(dir, "clim", "cache.json"), time.Hour)
}

// Get retrieves a cached value if it exists and hasn't expired.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.ExpiresAt) {
		return "", false
	}
	return entry.Value, true
}

// Set stores a value in the cache.
func (c *Cache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = cacheEntry{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Save persists the cache to disk.
func (c *Cache) Save() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.path == "" {
		return
	}

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	// Prune expired entries before saving.
	now := time.Now()
	live := make(map[string]cacheEntry)
	for k, v := range c.entries {
		if now.Before(v.ExpiresAt) {
			live[k] = v
		}
	}

	data, err := json.MarshalIndent(cacheFile{Entries: live}, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(c.path, append(data, '\n'), 0o644)
}

// load reads the cache from disk, discarding expired entries.
func (c *Cache) load() {
	if c.path == "" {
		return
	}

	data, err := os.ReadFile(c.path)
	if err != nil {
		return
	}

	var f cacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		return
	}

	now := time.Now()
	for k, v := range f.Entries {
		if now.Before(v.ExpiresAt) {
			c.entries[k] = v
		}
	}
}
