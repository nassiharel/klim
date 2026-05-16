package agents

import (
	"os"
	"time"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// Cache is the on-disk Snapshot cache. Same pattern as scancache —
// atomic write through fileutil.
type Cache struct {
	// PR #77 review: HostID was previously declared here as a
	// scancache parity placeholder but never populated or read.
	// Dropped to avoid stale-schema drift; if multi-host
	// disambiguation becomes necessary later we can wire it up
	// alongside the read/write paths.
	WrittenAt time.Time `yaml:"written_at"`
	Snapshot  Snapshot  `yaml:"snapshot"`
}

// LoadCache reads the on-disk cache. Returns (cache, true, nil) on hit,
// (zero, false, nil) when the file doesn't exist, (zero, false, err)
// on parse error.
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
	return c, true, nil
}

// SaveCache writes the Snapshot to disk atomically.
func SaveCache(snap Snapshot) error {
	p, err := paths.AgentsCache()
	if err != nil {
		return err
	}
	c := Cache{
		WrittenAt: time.Now().UTC(),
		Snapshot:  snap,
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
