package config

import (
	"testing"
	"time"
)

func TestAllSettings_CoversAllConfigFields(t *testing.T) {
	settings := AllSettings()
	want := []string{
		"log_level", "log_file", "log_verbose",
		"marketplace_url", "marketplace_auto_refresh", "marketplace_refresh_interval",
		"performance_concurrency", "performance_command_timeout",
		"ui_default_tab", "ui_show_path", "ui_sidebar_right",
		"defaults_preferred_source",
		"compliance_policy", "compliance_url", "compliance_auto_refresh",
		"compliance_refresh_interval", "compliance_block_installs",
	}
	got := map[string]bool{}
	for _, s := range settings {
		got[s.Key] = true
	}
	for _, k := range want {
		if !got[k] {
			t.Errorf("missing setting %q in AllSettings()", k)
		}
	}
	if len(settings) != len(want) {
		t.Errorf("AllSettings()=%d entries, want %d", len(settings), len(want))
	}
}

func TestSettingByKey(t *testing.T) {
	if _, ok := SettingByKey("log_level"); !ok {
		t.Error("expected to find log_level")
	}
	if _, ok := SettingByKey("not_a_real_key"); ok {
		t.Error("unexpected hit on bogus key")
	}
}

func TestSetting_SetFromString_BoolAcceptsCheckboxIdioms(t *testing.T) {
	cfg := Default()
	s, _ := SettingByKey("log_file")
	for _, raw := range []string{"true", "on", "1", "yes"} {
		cfg.Logging.File = false
		if err := s.SetFromString(cfg, raw); err != nil {
			t.Errorf("%q: %v", raw, err)
		}
		if !cfg.Logging.File {
			t.Errorf("%q should set true", raw)
		}
	}
	for _, raw := range []string{"false", "off", "0", "no", ""} {
		cfg.Logging.File = true
		if err := s.SetFromString(cfg, raw); err != nil {
			t.Errorf("%q: %v", raw, err)
		}
		if cfg.Logging.File {
			t.Errorf("%q should set false", raw)
		}
	}
	if err := s.SetFromString(cfg, "maybe"); err == nil {
		t.Error("expected error for non-boolean input")
	}
}

func TestSetting_SetFromString_IntRejectsNegative(t *testing.T) {
	cfg := Default()
	s, _ := SettingByKey("performance_concurrency")
	if err := s.SetFromString(cfg, "-5"); err == nil {
		t.Error("expected negative int to be rejected")
	}
	if err := s.SetFromString(cfg, "abc"); err == nil {
		t.Error("expected non-numeric int to be rejected")
	}
	if err := s.SetFromString(cfg, ""); err != nil {
		t.Errorf("empty should set zero (auto), got %v", err)
	}
	if cfg.Performance.Concurrency != 0 {
		t.Errorf("empty should reset to 0 (auto), got %d", cfg.Performance.Concurrency)
	}
	if err := s.SetFromString(cfg, "8"); err != nil {
		t.Fatal(err)
	}
	if cfg.Performance.Concurrency != 8 {
		t.Errorf("got %d, want 8", cfg.Performance.Concurrency)
	}
}

func TestSetting_SetFromString_DurationParses(t *testing.T) {
	cfg := Default()
	s, _ := SettingByKey("marketplace_refresh_interval")
	if err := s.SetFromString(cfg, "12h"); err != nil {
		t.Fatal(err)
	}
	if cfg.Marketplace.RefreshInterval.Duration != 12*time.Hour {
		t.Errorf("got %v, want 12h", cfg.Marketplace.RefreshInterval)
	}
	if err := s.SetFromString(cfg, "not-a-duration"); err == nil {
		t.Error("expected error for malformed duration")
	}
}

func TestSetting_SetFromString_ChoiceRejectsUnknown(t *testing.T) {
	cfg := Default()
	s, _ := SettingByKey("log_level")
	if err := s.SetFromString(cfg, "warn"); err != nil {
		t.Fatal(err)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("got %q, want warn", cfg.Logging.Level)
	}
	if err := s.SetFromString(cfg, "shouting"); err == nil {
		t.Error("expected unknown choice to be rejected")
	}
}

func TestSetting_DisplayReplacesEmptyAndZero(t *testing.T) {
	cfg := Default()
	cfg.Marketplace.URL = ""
	cfg.Performance.Concurrency = 0
	cfg.Defaults.PreferredSource = ""
	urlSetting, _ := SettingByKey("marketplace_url")
	if got := urlSetting.Display(cfg); got != "(default)" {
		t.Errorf("empty string display = %q, want (default)", got)
	}
	concSetting, _ := SettingByKey("performance_concurrency")
	if got := concSetting.Display(cfg); got != "auto" {
		t.Errorf("zero int display = %q, want auto", got)
	}
	// Empty-choice settings should render as "(default)" too — otherwise
	// the TUI / web editor shows a blank cell that's indistinguishable
	// from a missing value.
	srcSetting, _ := SettingByKey("defaults_preferred_source")
	if got := srcSetting.Display(cfg); got != "(default)" {
		t.Errorf("empty choice display = %q, want (default)", got)
	}
	cfg.Defaults.PreferredSource = "brew"
	if got := srcSetting.Display(cfg); got != "brew" {
		t.Errorf("non-empty choice display = %q, want brew", got)
	}
}
