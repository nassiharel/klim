package share

import (
	"errors"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		tools []string
	}{
		{"single tool", []string{"git"}},
		{"three tools", []string{"git", "gh", "az"}},
		{"21 tools", []string{
			"git", "gh", "az", "azd", "kubectl", "docker", "terraform",
			"helm", "go", "node", "python", "jq", "fzf", "ripgrep",
			"bat", "fd", "eza", "zoxide", "delta", "starship", "nvim",
		}},
		{"all 70 tools", []string{
			"git", "gh", "lazygit", "az", "azd", "aws", "gcloud",
			"docker", "docker-compose", "lazydocker", "kubectl", "helm",
			"k9s", "kubelogin", "kind", "minikube", "skaffold", "istioctl",
			"argocd", "flux", "terraform", "pulumi", "terragrunt", "kustomize",
			"ansible", "vagrant", "packer", "consul", "vault", "go", "node",
			"deno", "bun", "python", "dotnet", "rustc", "cargo", "java",
			"ruby", "zig", "npm", "uv", "code", "nvim", "jq", "yq", "fzf",
			"ripgrep", "bat", "fd", "eza", "zoxide", "delta", "dust", "procs",
			"tldr", "httpie", "wget", "tree", "cmake", "pandoc", "ffmpeg",
			"magick", "sqlite3", "mongosh", "nmap", "pwsh", "starship",
			"claude", "copilot",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := Encode(tt.tools)
			if err != nil {
				t.Fatalf("Encode() error: %v", err)
			}
			if !strings.HasPrefix(token, tokenPrefix) {
				t.Errorf("token should start with %q, got %q", tokenPrefix, token[:20])
			}
			t.Logf("token length for %d tools: %d chars", len(tt.tools), len(token))

			decoded, err := Decode(token)
			if err != nil {
				t.Fatalf("Decode() error: %v", err)
			}
			if len(decoded) != len(tt.tools) {
				t.Fatalf("decoded %d tools, want %d", len(decoded), len(tt.tools))
			}
			for i, name := range decoded {
				if name != tt.tools[i] {
					t.Errorf("tool[%d] = %q, want %q", i, name, tt.tools[i])
				}
			}
		})
	}
}

func TestEncode_Empty(t *testing.T) {
	_, err := Encode(nil)
	if !errors.Is(err, ErrEmptyToolList) {
		t.Fatalf("expected ErrEmptyToolList, got %v", err)
	}
	_, err = Encode([]string{})
	if !errors.Is(err, ErrEmptyToolList) {
		t.Fatalf("expected ErrEmptyToolList, got %v", err)
	}
}

func TestDecode_InvalidPrefix(t *testing.T) {
	_, err := Decode("garbage")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestDecode_UnknownVersion(t *testing.T) {
	_, err := Decode("clim:v99:somedata")
	if err == nil {
		t.Fatal("expected error for unknown version")
	}
	if !strings.Contains(err.Error(), "unsupported token version") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecode_InvalidBase64(t *testing.T) {
	_, err := Decode("clim:v1:!!!not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
	if !strings.Contains(err.Error(), "invalid token encoding") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecode_CorruptGzip(t *testing.T) {
	_, err := Decode("clim:v1:aGVsbG8") // valid base64 but not gzip
	if err == nil {
		t.Fatal("expected error for non-gzip data")
	}
	if !strings.Contains(err.Error(), "invalid token data") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecode_EmptyPayload(t *testing.T) {
	_, err := Decode("clim:v1:")
	if !errors.Is(err, ErrEmptyToken) {
		t.Fatalf("expected ErrEmptyToken, got %v", err)
	}
}

func TestDecode_WhitespaceHandling(t *testing.T) {
	tools := []string{"git", "gh", "az"}
	token, _ := Encode(tools)

	// Token with leading/trailing whitespace should still decode.
	decoded, err := Decode("  " + token + "  \n")
	if err != nil {
		t.Fatalf("Decode() error with whitespace: %v", err)
	}
	if len(decoded) != len(tools) {
		t.Errorf("decoded %d tools, want %d", len(decoded), len(tools))
	}
}

func TestTokenCompactness(t *testing.T) {
	// 21 tools — should produce a reasonably short token.
	tools := []string{
		"git", "gh", "az", "azd", "kubectl", "docker", "terraform",
		"helm", "go", "node", "python", "jq", "fzf", "ripgrep",
		"bat", "fd", "eza", "zoxide", "delta", "starship", "nvim",
	}
	token, err := Encode(tools)
	if err != nil {
		t.Fatal(err)
	}
	// A token for 21 tools should be under 200 characters.
	if len(token) > 200 {
		t.Errorf("token too long for 21 tools: %d chars (want < 200)", len(token))
	}
	t.Logf("21-tool token: %d chars", len(token))
	t.Logf("token: %s", token)
}
