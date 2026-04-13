package detector

import (
	"testing"
)

func TestVersionRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"fzf", "0.71.0 (62899fd7)", "0.71.0"},
		{"simple semver", "1.2.3", "1.2.3"},
		{"two segments", "8.1", "8.1"},
		{"four segments", "1.2.3.4", "1.2.3.4"},
		{"python", "Python 3.12.1", "3.12.1"},
		{"docker", "Docker version 27.5.1, build 9f9e405", "27.5.1"},
		{"git", "git version 2.53.0", "2.53.0"},
		{"v-prefix", "v22.12.0", "22.12.0"},
		{"v-prefix with label", "Terraform v1.10.5", "1.10.5"},
		{"kubectl", "Client Version: v1.33.3", "1.33.3"},
		{"go version", "go version go1.23.4 windows/amd64", "1.23.4"},
		{"no version", "some random text", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ""
			if m := versionRe.FindStringSubmatch(tt.input); len(m) >= 2 {
				got = m[1]
			}
			if got != tt.want {
				t.Errorf("versionRe on %q = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDetectGoBuildInfo_NonExistent(t *testing.T) {
	got := detectGoBuildInfo("/nonexistent/path/to/binary")
	if got != "" {
		t.Errorf("detectGoBuildInfo(nonexistent) = %q, want empty", got)
	}
}

func TestFallbackDetect_NonExistent(t *testing.T) {
	got := FallbackDetect("/nonexistent/path/to/binary")
	if got != "" {
		t.Errorf("FallbackDetect(nonexistent) = %q, want empty", got)
	}
}
