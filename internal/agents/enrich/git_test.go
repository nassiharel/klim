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

	t.Run("worktree .git file with relative gitdir resolves against the .git file's dir", func(t *testing.T) {
		t.Parallel()
		// Layout mirrors `git worktree add ../wt`:
		//   <root>/main/.git/             (real repo dir)
		//   <root>/main/.git/worktrees/wt (the wt's gitdir)
		//   <root>/wt/.git                 (a FILE containing "gitdir: ../main/.git/worktrees/wt")
		root := t.TempDir()
		mainRepo := filepath.Join(root, "main")
		wtDir := filepath.Join(root, "wt")
		gitdirReal := filepath.Join(mainRepo, ".git", "worktrees", "wt")
		if err := os.MkdirAll(gitdirReal, 0o755); err != nil {
			t.Fatalf("mkdir gitdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(gitdirReal, "HEAD"), []byte("ref: refs/heads/feat/wt\n"), 0o644); err != nil {
			t.Fatalf("write HEAD: %v", err)
		}
		if err := os.MkdirAll(wtDir, 0o755); err != nil {
			t.Fatalf("mkdir wt: %v", err)
		}
		// gitdir points at the real gitdir RELATIVELY from the wt's dir.
		rel := filepath.Join("..", "main", ".git", "worktrees", "wt")
		if err := os.WriteFile(filepath.Join(wtDir, ".git"), []byte("gitdir: "+rel+"\n"), 0o644); err != nil {
			t.Fatalf("write .git file: %v", err)
		}
		if got, want := BranchAtCwd(wtDir), "feat/wt"; got != want {
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
