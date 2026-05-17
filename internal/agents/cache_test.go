package agents

import (
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

func TestLoadCache_RejectsOldSchemaVersion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KLIM_HOME", tmp)

	// Write an old-schema cache file by hand: it has no
	// schema_version key (defaults to 0).
	p, err := paths.AgentsCache()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if err := fileutil.WriteYAML(p, &struct {
		WrittenAt time.Time `yaml:"written_at"`
		Snapshot  Snapshot  `yaml:"snapshot"`
	}{WrittenAt: time.Now().UTC()}, "# legacy schema\n"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, ok, err := LoadCache()
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if ok {
		t.Error("LoadCache returned ok=true for cache with old schema version; want a forced rescan")
	}
}
