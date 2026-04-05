package detector

import (
	"testing"
)

func TestFirstLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single line", "git version 2.43.0", "git version 2.43.0"},
		{"multiline", "Docker version 24.0.7, build afdd53b\nDocker Inc.", "Docker version 24.0.7, build afdd53b"},
		{"leading whitespace", "  v20.10.0\n", "v20.10.0"},
		{"empty", "", ""},
		{"only whitespace", "   \n  \n", ""},
		{"windows newlines", "Python 3.11.6\r\nMore info", "Python 3.11.6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstLine(tt.input)
			if got != tt.want {
				t.Errorf("firstLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
