package tui

import (
	"testing"
	"time"
)

func TestSelfUpdateCache_RoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KLIM_HOME", tmp)

	in := &selfUpdateCache{
		CheckedAt:      time.Now().Truncate(time.Second),
		CurrentVersion: "v2.1.92",
		LatestVersion:  "v2.2.0",
		Available:      true,
	}
	if err := saveSelfUpdateCache(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok := loadSelfUpdateCache()
	if !ok {
		t.Fatal("expected cache to load")
	}
	if got.CurrentVersion != "v2.1.92" || got.LatestVersion != "v2.2.0" || !got.Available {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestSelfUpdateCheckTTL(t *testing.T) {
	if selfUpdateCheckTTL != 24*time.Hour {
		t.Errorf("TTL changed from 24h to %v — confirm intentional", selfUpdateCheckTTL)
	}
}
