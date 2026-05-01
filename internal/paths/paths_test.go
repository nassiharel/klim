package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBaseDir(t *testing.T) {
	dir, err := BaseDir()
	if err != nil {
		t.Fatalf("BaseDir: %v", err)
	}
	if !strings.HasSuffix(dir, "clim") {
		t.Fatalf("unexpected base dir: %s", dir)
	}
}

func TestJoin(t *testing.T) {
	p, err := Join("a", "b.yaml")
	if err != nil {
		t.Fatalf("Join: %v", err)
	}
	if filepath.Base(p) != "b.yaml" {
		t.Fatalf("unexpected: %s", p)
	}
}

func TestAllPaths(t *testing.T) {
	fns := []struct {
		name string
		fn   func() (string, error)
		base string
	}{
		{"Config", Config, "config.yaml"},
		{"Favorites", Favorites, "favorites.yaml"},
		{"CustomPacks", CustomPacks, "custom-packs.yaml"},
		{"ScanCache", ScanCache, "scan-cache.yaml"},
		{"CatalogCache", CatalogCache, "marketplace-cache.yaml"},
		{"BackupsDir", BackupsDir, "backups"},
		{"LogFile", LogFile, "clim.log"},
		{"CompliancePolicy", CompliancePolicy, "policy.yaml"},
	}
	for _, tc := range fns {
		t.Run(tc.name, func(t *testing.T) {
			p, err := tc.fn()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if filepath.Base(p) != tc.base {
				t.Fatalf("%s: got %q, want base %q", tc.name, p, tc.base)
			}
		})
	}
}
