package version

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name      string
		installed string
		latest    string
		want      Status
		wantErr   bool
	}{
		{
			name:      "same version",
			installed: "2.40.1",
			latest:    "2.40.1",
			want:      StatusUpToDate,
		},
		{
			name:      "upgrade available",
			installed: "2.40.1",
			latest:    "2.41.0",
			want:      StatusUpgradable,
		},
		{
			name:      "installed newer (dev build)",
			installed: "2.41.0",
			latest:    "2.40.1",
			want:      StatusUpToDate,
		},
		{
			name:      "not installed",
			installed: "",
			latest:    "2.40.1",
			want:      StatusNotInstalled,
		},
		{
			name:      "no latest available",
			installed: "2.40.1",
			latest:    "",
			want:      StatusError,
			wantErr:   true,
		},
		{
			name:      "tolerant parse missing patch",
			installed: "1.23",
			latest:    "1.23.4",
			want:      StatusUpgradable,
		},
		{
			name:      "tolerant parse installed has patch",
			installed: "1.23.4",
			latest:    "1.23",
			want:      StatusUpToDate,
		},
		{
			name:      "major upgrade",
			installed: "1.0.0",
			latest:    "2.0.0",
			want:      StatusUpgradable,
		},
		{
			name:      "with v prefix",
			installed: "v1.28.3",
			latest:    "v1.35.0",
			want:      StatusUpgradable,
		},
		{
			name:      "mixed v prefix",
			installed: "1.28.3",
			latest:    "v1.35.0",
			want:      StatusUpgradable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareVersions(tt.installed, tt.latest)
			if tt.wantErr {
				if err == nil {
					t.Errorf("CompareVersions() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("CompareVersions() unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d (%s), want %d (%s)",
					tt.installed, tt.latest, got, StatusString(got), tt.want, StatusString(tt.want))
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusUpToDate, "✓ up to date"},
		{StatusUpgradable, "⬆ upgrade available"},
		{StatusNotInstalled, "✗ not found"},
		{StatusLoading, "⏳ loading"},
		{StatusError, "? error"},
	}

	for _, tt := range tests {
		if got := StatusString(tt.status); got != tt.want {
			t.Errorf("StatusString(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
