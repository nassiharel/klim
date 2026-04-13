package pkgmgr

import (
	"strings"
	"testing"
)

func TestParseKeyValue(t *testing.T) {
	tests := []struct {
		name   string
		output string
		key    string
		want   string
	}{
		{
			"winget version",
			"Found Golang.Go [Golang.Go]\nVersion: 1.23.4\nPublisher: Go Authors\n",
			"Version",
			"1.23.4",
		},
		{
			"winget publisher",
			"Found Golang.Go [Golang.Go]\nVersion: 1.23.4\nPublisher: Go Authors\n",
			"Publisher",
			"Go Authors",
		},
		{"missing key", "Version: 1.0\nName: foo\n", "License", ""},
		{"empty output", "", "Version", ""},
		{"key with extra spaces", "  Version:  2.0  \n", "Version", "2.0"},
		{"colon in value", "URL: https://example.com\n", "URL", "https://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseKeyValue(tt.output, tt.key)
			if got != tt.want {
				t.Errorf("parseKeyValue(%q, %q) = %q, want %q",
					tt.output, tt.key, got, tt.want)
			}
		})
	}
}

func TestParsePipeSeparated(t *testing.T) {
	tests := []struct {
		name   string
		output string
		pkg    string
		want   string
	}{
		{
			"choco list format",
			"git|2.43.0\n",
			"git",
			"2.43.0",
		},
		{
			"case insensitive match",
			"Git|2.43.0\n",
			"git",
			"2.43.0",
		},
		{
			"multiple packages",
			"nodejs|20.10.0\ngit|2.43.0\npython|3.12.1\n",
			"git",
			"2.43.0",
		},
		{"package not found", "nodejs|20.10.0\n", "git", ""},
		{"empty output", "", "git", ""},
		{
			"trailing whitespace",
			"git|2.43.0  \n",
			"git",
			"2.43.0",
		},
		{
			"no pipe separator",
			"git 2.43.0\n",
			"git",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePipeSeparated(tt.output, tt.pkg)
			if got != tt.want {
				t.Errorf("parsePipeSeparated(%q, %q) = %q, want %q",
					tt.output, tt.pkg, got, tt.want)
			}
		})
	}
}

func TestCleanDebianVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple version", "1.2.3", "1.2.3"},
		{"epoch prefix", "1:2.53.0", "2.53.0"},
		{"distro suffix", "2.53.0-1ubuntu2", "2.53.0"},
		{"both epoch and suffix", "2:3.12.1-1+focal1", "3.12.1"},
		{"no transformations needed", "7.6.0", "7.6.0"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanDebianVersion(tt.input)
			if got != tt.want {
				t.Errorf("cleanDebianVersion(%q) = %q, want %q",
					tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDpkgStatus(t *testing.T) {
	// parseDpkgStatus reads from /var/lib/dpkg/status which is Linux-only.
	// We test the line parsing logic indirectly through dpkgInstalledVersion
	// and cleanDebianVersion. A full integration test would require a mock file.
	// Here we at least verify cleanDebianVersion handles the common patterns.
	t.Run("epoch and suffix stripping", func(t *testing.T) {
		cases := []struct {
			raw, want string
		}{
			{"2:8.2.3995-1ubuntu3.2", "8.2.3995"},
			{"1:3.4.1-4build2", "3.4.1"},
			{"2.39.2-1ubuntu1.1", "2.39.2"},
		}
		for _, c := range cases {
			got := cleanDebianVersion(c.raw)
			if got != c.want {
				t.Errorf("cleanDebianVersion(%q) = %q, want %q", c.raw, got, c.want)
			}
		}
	})
}

func TestCleanWingetOutput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string // expected after cleaning — \r normalised to \n
	}{
		{
			"clean output",
			"Name Id Version Source\nfzf junegunn.fzf 0.71.0 winget\n",
			"Name Id Version Source\nfzf junegunn.fzf 0.71.0 winget\n",
		},
		{
			"CRLF line endings",
			"Name Id Version Source\r\nfzf junegunn.fzf 0.71.0 winget\r\n",
			"Name Id Version Source\nfzf junegunn.fzf 0.71.0 winget\n",
		},
		{
			"spinner with CR overwrites",
			"-\r\\\r|\r/\r-\rName Id Version Source\r\n---\r\nfzf junegunn.fzf 0.71.0 winget\r\n",
			"-\n\\\n|\n/\n-\nName Id Version Source\n---\nfzf junegunn.fzf 0.71.0 winget\n",
		},
		{
			"empty output",
			"",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanWingetOutput(tt.raw)
			if got != tt.want {
				t.Errorf("cleanWingetOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWingetInstalledVersionParsing(t *testing.T) {
	// We can't call wingetInstalledVersion directly (it runs a subprocess),
	// but we can test the parsing logic by simulating what cleanWingetOutput
	// produces and running the same field-matching loop.

	tests := []struct {
		name   string
		output string // cleaned winget list output
		id     string
		want   string
	}{
		{
			"standard output",
			"Name Id Version Source\n---------------------------------\nfzf  junegunn.fzf 0.71.0  winget\n",
			"junegunn.fzf",
			"0.71.0",
		},
		{
			"FFmpeg with short version",
			"Name   Id           Version Source\n---------------------------------\nFFmpeg Gyan.FFmpeg 8.1     winget\n",
			"Gyan.FFmpeg",
			"8.1",
		},
		{
			"Git with long name",
			"Name Id      Version  Source\n-----------------------------\nGit  Git.Git 2.47.1.2 winget\n",
			"Git.Git",
			"2.47.1.2",
		},
		{
			"not found",
			"Name Id Version Source\n---------------------------------\n",
			"junegunn.fzf",
			"",
		},
		{
			"empty output",
			"",
			"junegunn.fzf",
			"",
		},
		{
			"spinner noise before data",
			"-\n\\\n|\n/\n-\nName Id Version Source\n---------------------------------\nfzf  junegunn.fzf 0.71.0  winget\n",
			"junegunn.fzf",
			"0.71.0",
		},
		{
			"separator line skipped",
			"Name Id Version Source\n---------------------------------\n",
			"---------------------------------",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWingetListVersion(tt.output, tt.id)
			if got != tt.want {
				t.Errorf("parseWingetListVersion(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// parseWingetListVersion extracts the version from cleaned winget list output.
// This mirrors the parsing logic in wingetInstalledVersion for testability.
func parseWingetListVersion(output, id string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		for i, f := range fields {
			if strings.EqualFold(f, id) && i+1 < len(fields) {
				ver := fields[i+1]
				if strings.HasPrefix(ver, "-") {
					continue
				}
				return ver
			}
		}
	}
	return ""
}
