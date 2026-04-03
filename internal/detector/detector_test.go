package detector

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		regex   string
		want    string
		wantErr bool
	}{
		{
			name:   "azure cli json",
			output: `{"azure-cli": "2.67.0", "azure-cli-core": "2.67.0"}`,
			regex:  `"azure-cli":\s*"(\d+\.\d+\.\d+)"`,
			want:   "2.67.0",
		},
		{
			name:   "azd version",
			output: "azd version 1.11.0 (commit abc1234)",
			regex:  `(\d+\.\d+\.\d+)`,
			want:   "1.11.0",
		},
		{
			name:   "github cli",
			output: "gh version 2.40.1 (2023-10-03)\nhttps://github.com/cli/cli/releases/tag/v2.40.1",
			regex:  `gh version (\d+\.\d+\.\d+)`,
			want:   "2.40.1",
		},
		{
			name:   "kubectl json",
			output: `{"clientVersion":{"major":"1","minor":"28","gitVersion":"v1.28.3","platform":"linux/amd64"}}`,
			regex:  `"gitVersion":\s*"v(\d+\.\d+\.\d+)"`,
			want:   "1.28.3",
		},
		{
			name:   "docker version",
			output: "Docker version 24.0.7, build afdd53b",
			regex:  `Docker version (\d+\.\d+\.\d+)`,
			want:   "24.0.7",
		},
		{
			name:   "terraform json",
			output: `{"terraform_version":"1.6.3","platform":"windows_amd64","terraform_outdated":false}`,
			regex:  `"terraform_version":\s*"(\d+\.\d+\.\d+)"`,
			want:   "1.6.3",
		},
		{
			name:   "helm short",
			output: "v3.13.2+g2a2fb3b",
			regex:  `v?(\d+\.\d+\.\d+)`,
			want:   "3.13.2",
		},
		{
			name:   "go version",
			output: "go version go1.23.4 windows/amd64",
			regex:  `go(\d+\.\d+\.?\d*)`,
			want:   "1.23.4",
		},
		{
			name:   "go version without patch",
			output: "go version go1.23 linux/amd64",
			regex:  `go(\d+\.\d+\.?\d*)`,
			want:   "1.23",
		},
		{
			name:   "node version",
			output: "v20.10.0",
			regex:  `v?(\d+\.\d+\.\d+)`,
			want:   "20.10.0",
		},
		{
			name:   "python version",
			output: "Python 3.11.6",
			regex:  `Python (\d+\.\d+\.\d+)`,
			want:   "3.11.6",
		},
		{
			name:   "git version linux",
			output: "git version 2.43.0",
			regex:  `git version (\d+\.\d+\.\d+)`,
			want:   "2.43.0",
		},
		{
			name:   "git version windows",
			output: "git version 2.43.0.windows.1",
			regex:  `git version (\d+\.\d+\.\d+)`,
			want:   "2.43.0",
		},
		{
			name:   "no match",
			output: "some garbage output",
			regex:  `v(\d+\.\d+\.\d+)`,
			want:   "",
		},
		{
			name:   "empty output",
			output: "",
			regex:  `(\d+\.\d+\.\d+)`,
			want:   "",
		},
		{
			name:    "invalid regex",
			output:  "v1.0.0",
			regex:   `(invalid[`,
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersion(tt.output, tt.regex)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseVersion() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ParseVersion() unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ParseVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
