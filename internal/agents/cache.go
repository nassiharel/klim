package agents

import (
	"os"
	"time"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// cacheSchemaVersion bumps whenever the on-disk schema for Cache
// (or any nested type marshalled into it) changes shape in a way
// that would lose data when read with the new code. The loader
// treats a mismatched version as "missing cache" so the caller
// rescans rather than silently dropping fields whose yaml keys
// were renamed.
//
// History:
//
//	1 — original Snapshot layout (yaml.v3 defaults: BinPath -> binpath,
//	     ProviderStatus -> providerstatus, etc.)
//	2 — explicit snake_case yaml tags on every Snapshot/sub-field
//	     for structured-output alignment. Bumping forces a rescan so
//	     BinPath / ProviderStatus don't read as empty from old files.
const cacheSchemaVersion = 2

// Cache is the on-disk Snapshot cache. Same pattern as scancache —
// atomic write through fileutil.
type Cache struct {
	// SchemaVersion is checked on load; older versions trigger a
	// rescan so renamed yaml keys don't silently drop data.
	SchemaVersion int       `yaml:"schema_version"`
	WrittenAt     time.Time `yaml:"written_at"`
	Snapshot      Snapshot  `yaml:"snapshot"`
}

// LoadCache reads the on-disk cache. Returns (cache, true, nil) on hit,
// (zero, false, nil) when the file doesn't exist or its schema is
// outdated, (zero, false, err) on parse error.
func LoadCache() (Cache, bool, error) {
	p, err := paths.AgentsCache()
	if err != nil {
		return Cache{}, false, err
	}
	var c Cache
	found, err := fileutil.ReadYAML(p, &c)
	if err != nil {
		return Cache{}, false, err
	}
	if !found {
		return Cache{}, false, nil
	}
	if c.SchemaVersion != cacheSchemaVersion {
		// Old layout — its yaml keys don't match the current
		// type tags, so trusting it would surface as missing
		// BinPath / ProviderStatus / etc. Treat as a cold cache.
		return Cache{}, false, nil
	}
	return c, true, nil
}

// SaveCache writes the Snapshot to disk atomically.
func SaveCache(snap Snapshot) error {
	p, err := paths.AgentsCache()
	if err != nil {
		return err
	}
	c := Cache{
		SchemaVersion: cacheSchemaVersion,
		WrittenAt:     time.Now().UTC(),
		Snapshot:      snap,
	}
	return fileutil.WriteYAML(p, &c, "# klim agents scan cache — auto-generated\n")
}

// DeleteCache removes the cache file. Best-effort; missing file is not an error.
func DeleteCache() error {
	p, err := paths.AgentsCache()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
