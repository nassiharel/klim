package enrich

import (
	"os"
	"path/filepath"
	"strings"
)

// BranchAtCwd reads the active git branch at `cwd` directly from
// .git/HEAD. We avoid shelling out to `git`:
//
//   - faster (no process spawn per session on every scan),
//   - safer (no PATH dependency), and
//   - keeps unit tests offline.
//
// The function walks up from cwd looking for a `.git` entry (matching
// `git rev-parse --git-dir` semantics for nested worktrees). When
// the file's first line is "ref: refs/heads/<name>" we return the
// branch name; when HEAD is detached we return the short commit
// SHA prefix; on any error we return the empty string.
func BranchAtCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	gitDir := findGitDir(cwd)
	if gitDir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(data))
	const refPrefix = "ref: refs/heads/"
	if strings.HasPrefix(head, refPrefix) {
		return head[len(refPrefix):]
	}
	// Detached HEAD: return short SHA.
	if len(head) >= 7 {
		return head[:7]
	}
	return head
}

// findGitDir walks up from `start` until it finds a `.git` entry.
// Returns the resolved git-dir path (handling both bare `.git`
// directories and `.git` files pointing to a worktree's gitdir), or
// the empty string when no repo is found.
//
// Stops walking at the filesystem root.
func findGitDir(start string) string {
	cur := filepath.Clean(start)
	for {
		candidate := filepath.Join(cur, ".git")
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return candidate
			}
			// .git file (worktree) — first line is "gitdir: <path>".
			data, err := os.ReadFile(candidate)
			if err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "gitdir:") {
						return strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
					}
				}
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

// RepoFromCwd derives a best-effort repository name from the cwd by
// returning its base directory name. The git config remote URL would
// give a more accurate name but requires another file read; for the
// dashboard's grouping purposes the directory name is good enough.
//
// Returns the empty string when cwd is empty or doesn't look like a
// directory (no separator and no trailing name).
func RepoFromCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	if findGitDir(cwd) == "" {
		return ""
	}
	return filepath.Base(filepath.Clean(cwd))
}
