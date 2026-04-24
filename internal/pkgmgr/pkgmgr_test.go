package pkgmgr

import (
	"encoding/json"
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

func TestParseBrewVersions(t *testing.T) {
	// brew list --versions output: "formula version [version ...]"
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"single version", "git 2.43.0", "2.43.0"},
		{"multiple versions", "python@3.12 3.12.1 3.11.7", "3.12.1"},
		{"empty output", "", ""},
		{"name only", "git", ""},
		{"with trailing whitespace", "git 2.43.0  \n", "2.43.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Fields(strings.TrimSpace(tt.output))
			got := ""
			if len(parts) >= 2 {
				got = parts[1]
			}
			if got != tt.want {
				t.Errorf("brew parse(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestParseBrewJSON(t *testing.T) {
	// brew info --json=v2 output
	tests := []struct {
		name string
		json string
		want string
	}{
		{
			"valid",
			`{"formulae":[{"versions":{"stable":"2.43.0"}}]}`,
			"2.43.0",
		},
		{
			"empty formulae",
			`{"formulae":[]}`,
			"",
		},
		{
			"invalid json",
			`not json`,
			"",
		},
		{
			"empty",
			"",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBrewInfoJSON(tt.json)
			if got != tt.want {
				t.Errorf("parseBrewInfoJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSnapList(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			"standard output",
			"Name    Version  Rev  Tracking  Publisher  Notes\nkubectl 1.28.4   3456 latest    canonical  -\n",
			"1.28.4",
		},
		{"header only", "Name Version Rev\n", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSnapListVersion(tt.output)
			if got != tt.want {
				t.Errorf("parseSnapListVersion(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestParseSnapInfo(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			"standard output",
			"name: kubectl\nlatest/stable: 1.29.0 2024-01-15\ninstalled: 1.28.4\n",
			"1.29.0",
		},
		{"no stable line", "name: kubectl\ninstalled: 1.28.4\n", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSnapInfoLatest(tt.output)
			if got != tt.want {
				t.Errorf("parseSnapInfoLatest(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestParseNpmGlobals(t *testing.T) {
	tests := []struct {
		name string
		json string
		want map[string]string
	}{
		{
			"standard output",
			`{"dependencies":{"prettier":{"version":"3.1.1"},"eslint":{"version":"8.56.0"}}}`,
			map[string]string{"prettier": "3.1.1", "eslint": "8.56.0"},
		},
		{
			"empty dependencies",
			`{"dependencies":{}}`,
			map[string]string{},
		},
		{"invalid json", "not json", nil},
		{"empty", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseNpmGlobalsJSON(tt.json)
			if tt.want == nil {
				if got != nil {
					t.Errorf("parseNpmGlobalsJSON() = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseNpmGlobalsJSON() has %d entries, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseNpmGlobalsJSON()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestParseAptCachePolicy(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			"standard output",
			"git:\n  Installed: 1:2.39.2-1ubuntu1.1\n  Candidate: 1:2.39.5-0ubuntu1\n",
			"2.39.5",
		},
		{
			"candidate none",
			"fake-pkg:\n  Installed: (none)\n  Candidate: (none)\n",
			"",
		},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAptCacheLatest(tt.output)
			if got != tt.want {
				t.Errorf("parseAptCacheLatest(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestParseScoopList(t *testing.T) {
	// Representative output from `scoop list <pkg>` — includes the header
	// row, separator line, and a data row. Scoop localises headers via the
	// console culture, but the default English form is what we test here.
	standard := "\nInstalled apps:\n\n" +
		"Name Version Source Updated             Info\n" +
		"---- ------- ------ -------             ----\n" +
		"bat  0.24.0  main   2024-01-15 10:11:12\n"

	// Case-insensitive match: scoop sometimes prints the name exactly as
	// stored in the manifest, which may differ in case from user input.
	mixedCase := "Name Version Source\n---- ------- ------\nBat  0.24.0  main\n"

	// Multiple results (e.g. when `scoop list` is called without a pattern).
	multi := "Name Version Source\n---- ------- ------\n" +
		"7zip 23.01   main\n" +
		"bat  0.24.0  main\n" +
		"gh   2.44.1  main\n"

	// Not installed — only the header survives.
	notFound := "Name Version Source\n---- ------- ------\n"

	tests := []struct {
		name   string
		output string
		pkg    string
		want   string
	}{
		{"standard", standard, "bat", "0.24.0"},
		{"case-insensitive", mixedCase, "bat", "0.24.0"},
		{"multi rows pick match", multi, "bat", "0.24.0"},
		{"multi rows other match", multi, "gh", "2.44.1"},
		{"not installed", notFound, "bat", ""},
		{"empty output", "", "bat", ""},
		// Header-looking row whose package name literally equals "Name"
		// should be skipped, not returned as version "Version".
		{"skip header row", "Name Version Source\n---- ------- ------\n", "Name", ""},
		// Separator row starting with dashes should be skipped.
		{"skip separator row", "---- ------- ------\n", "----", ""},
		// ANSI color codes in scoop output should be stripped.
		{"ansi color codes", "Installed apps matching 'jq':\n\n\x1b[32;1mName\x1b[0m \x1b[32;1mVersion\x1b[0m \x1b[32;1mSource\x1b[0m\n\x1b[32;1m----\x1b[0m \x1b[32;1m-------\x1b[0m \x1b[32;1m------\x1b[0m\njq   1.8.1   main\n", "jq", "1.8.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseScoopList(tt.output, tt.pkg)
			if got != tt.want {
				t.Errorf("parseScoopList(..., %q) = %q, want %q", tt.pkg, got, tt.want)
			}
		})
	}
}

// --- Test helper functions that mirror the parsing logic ---

func parseBrewInfoJSON(jsonStr string) string {
	if jsonStr == "" {
		return ""
	}
	var result struct {
		Formulae []struct {
			Versions struct {
				Stable string `json:"stable"`
			} `json:"versions"`
		} `json:"formulae"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err == nil && len(result.Formulae) > 0 {
		return result.Formulae[0].Versions.Stable
	}
	return ""
}

func parseSnapListVersion(output string) string {
	lines := strings.Split(output, "\n")
	if len(lines) >= 2 {
		fields := strings.Fields(lines[1])
		if len(fields) >= 2 {
			return fields[1]
		}
	}
	return ""
}

func parseSnapInfoLatest(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "latest/stable:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}
	return ""
}

func parseNpmGlobalsJSON(jsonStr string) map[string]string {
	if jsonStr == "" {
		return nil
	}
	var result struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil
	}
	m := make(map[string]string, len(result.Dependencies))
	for name, dep := range result.Dependencies {
		m[name] = dep.Version
	}
	return m
}

func parseAptCacheLatest(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Candidate:") {
			ver := strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
			if ver != "(none)" {
				return cleanDebianVersion(ver)
			}
		}
	}
	return ""
}
