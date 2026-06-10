package enrich

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBranchAtCwd(t *testing.T) {
	t.Parallel()

	t.Run("empty cwd", func(t *testing.T) {
		t.Parallel()
		if got := BranchAtCwd(""); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("no git dir", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if got := BranchAtCwd(dir); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("normal branch", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		gitDir := filepath.Join(dir, ".git")
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatalf("mkdir .git: %v", err)
		}
		if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feat/dashboard\n"), 0o644); err != nil {
			t.Fatalf("write HEAD: %v", err)
		}
		if got, want := BranchAtCwd(dir), "feat/dashboard"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("detached HEAD returns short sha", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		gitDir := filepath.Join(dir, ".git")
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatalf("mkdir .git: %v", err)
		}
		if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("abcdef0123456789\n"), 0o644); err != nil {
			t.Fatalf("write HEAD: %v", err)
		}
		if got, want := BranchAtCwd(dir), "abcdef0"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("walks up to find .git", func(t *testing.T) {
		t.Parallel()
		repo := t.TempDir()
		gitDir := filepath.Join(repo, ".git")
		if err := os.MkdirAll(gitDir, 0o755); err != nil {
			t.Fatalf("mkdir .git: %v", err)
		}
		if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
			t.Fatalf("write HEAD: %v", err)
		}
		sub := filepath.Join(repo, "a", "b", "c")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("mkdir sub: %v", err)
		}
		if got, want := BranchAtCwd(sub), "main"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestRepoFromCwd(t *testing.T) {
	t.Parallel()
	if got := RepoFromCwd(""); got != "" {
		t.Errorf("empty cwd: got %q", got)
	}
	// Non-git dir → empty.
	dir := t.TempDir()
	if got := RepoFromCwd(dir); got != "" {
		t.Errorf("non-repo: got %q, want empty", got)
	}
	// Git dir → basename.
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := RepoFromCwd(dir); got == "" {
		t.Errorf("git repo: got empty, want non-empty")
	}
}