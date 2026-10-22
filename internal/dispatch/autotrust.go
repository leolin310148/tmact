package dispatch

import (
	"os"
	"path/filepath"
	"strings"
)

// Auto-trust reasons reported when a dispatch opts into folder trust on its
// own. Both describe a directory with no foreign code to vet: an empty one,
// or a linked worktree whose content comes from a repository already on this
// machine. A freshly cloned repository is neither and still needs an explicit
// --trust-folder.
const (
	AutoTrustEmptyDir       = "empty_dir"
	AutoTrustLinkedWorktree = "linked_worktree"
)

// autoTrustReason reports why dir may skip the explicit --trust-folder opt-in,
// or "" when it may not. Any read error fails closed.
func autoTrustReason(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	if len(entries) == 0 {
		return AutoTrustEmptyDir
	}
	if isLinkedWorktreeRoot(dir) {
		return AutoTrustLinkedWorktree
	}
	return ""
}

// isLinkedWorktreeRoot reports whether dir is the root of a `git worktree add`
// checkout: dir/.git is a file pointing at <common>/worktrees/<name>, that
// admin dir has a commondir file, and its gitdir file points back at dir/.git.
// A main checkout (.git directory) or a submodule (no back-link) does not match.
func isLinkedWorktreeRoot(dir string) bool {
	dotGit := filepath.Join(dir, ".git")
	info, err := os.Lstat(dotGit)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	gitDir, ok := readGitPointer(dotGit, "gitdir:")
	if !ok {
		return false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	if filepath.Base(filepath.Dir(gitDir)) != "worktrees" {
		return false
	}
	if info, err := os.Stat(filepath.Join(gitDir, "commondir")); err != nil || !info.Mode().IsRegular() {
		return false
	}
	backLink, ok := readGitPointer(filepath.Join(gitDir, "gitdir"), "")
	if !ok {
		return false
	}
	return sameFile(backLink, dotGit)
}

func readGitPointer(path, prefix string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	if prefix != "" {
		if !strings.HasPrefix(line, prefix) {
			return "", false
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, prefix))
	}
	return line, line != ""
}

func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}
