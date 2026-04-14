package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Logging.Level != "debug" {
		t.Errorf("default logging level = %q, want debug", cfg.Logging.Level)
	}
	if cfg.Logging.File != true {
		t.Error("default logging file should be true")
	}
	if cfg.Logging.Verbose != false {
		t.Error("default logging verbose should be false")
	}
	if cfg.Marketplace.URL != DefaultMarketplaceURL {
		t.Errorf("default marketplace URL = %q, want DefaultMarketplaceURL", cfg.Marketplace.URL)
	}
	if cfg.Marketplace.AutoRefresh != false {
		t.Error("default auto_refresh should be false")
	}
	if cfg.Marketplace.RefreshInterval.Duration != 24*time.Hour {
		t.Errorf("default refresh_interval = %v, want 24h", cfg.Marketplace.RefreshInterval)
	}
	if cfg.Performance.Concurrency != 0 {
		t.Errorf("default concurrency = %d, want 0", cfg.Performance.Concurrency)
	}
	if cfg.Performance.CommandTimeout.Duration != 30*time.Second {
		t.Errorf("default command_timeout = %v, want 30s", cfg.Performance.CommandTimeout)
	}
	if cfg.UI.DefaultTab != "installed" {
		t.Errorf("default tab = %q, want installed", cfg.UI.DefaultTab)
	}
	if cfg.UI.ShowPath != true {
		t.Error("default show_path should be true")
	}
	if cfg.UI.SidebarRight != false {
		t.Error("default sidebar_right should be false")
	}
}

func TestDuration_MarshalYAML(t *testing.T) {
	d := Duration{10 * time.Second}
	v, err := d.MarshalYAML()
	if err != nil {
		t.Fatal(err)
	}
	if v != "10s" {
		t.Errorf("MarshalYAML() = %v, want 10s", v)
	}
}

func TestDuration_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    time.Duration
		wantErr bool
	}{
		{"seconds", "10s", 10 * time.Second, false},
		{"hours", "24h", 24 * time.Hour, false},
		{"minutes", "5m", 5 * time.Minute, false},
		{"complex", "1h30m", 90 * time.Minute, false},
		{"invalid", "notaduration", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Duration
			node := &yaml.Node{Kind: yaml.ScalarNode, Value: tt.yaml}
			err := d.UnmarshalYAML(node)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d.Duration != tt.want {
				t.Errorf("UnmarshalYAML(%q) = %v, want %v", tt.yaml, d.Duration, tt.want)
			}
		})
	}
}

func TestConfig_RoundTrip(t *testing.T) {
	// Test that a Config can be marshaled and unmarshaled without loss.
	original := Default()
	original.Marketplace.URL = "http://localhost:5000/marketplace.yaml"
	original.UI.SidebarRight = true

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}

	var restored Config
	if err := yaml.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}

	if restored.Marketplace.URL != original.Marketplace.URL {
		t.Errorf("URL = %q, want %q", restored.Marketplace.URL, original.Marketplace.URL)
	}
	if restored.UI.SidebarRight != original.UI.SidebarRight {
		t.Errorf("SidebarRight = %v, want %v", restored.UI.SidebarRight, original.UI.SidebarRight)
	}
	if restored.Performance.CommandTimeout.Duration != original.Performance.CommandTimeout.Duration {
		t.Errorf("CommandTimeout = %v, want %v",
			restored.Performance.CommandTimeout.Duration,
			original.Performance.CommandTimeout.Duration)
	}
}
