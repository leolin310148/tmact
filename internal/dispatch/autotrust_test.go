package dispatch

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAutoTrustReason(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	empty := filepath.Join(root, "empty")
	repo := filepath.Join(root, "repo")
	worktree := filepath.Join(root, "wt")
	for _, dir := range []string{empty, repo} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git(repo, "init", "-q")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	git(repo, "add", ".")
	git(repo, "commit", "-qm", "init")
	git(repo, "worktree", "add", "-q", worktree)
	if err := os.Mkdir(filepath.Join(worktree, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "sub", "b.txt"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	forged := filepath.Join(root, "forged")
	if err := os.Mkdir(forged, 0o755); err != nil {
		t.Fatal(err)
	}
	// A .git file naming another checkout's worktree admin dir has no
	// back-link to itself and must not count.
	if err := os.WriteFile(filepath.Join(forged, ".git"), []byte("gitdir: "+filepath.Join(repo, ".git", "worktrees", "wt")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		dir  string
		want string
	}{
		{"empty dir", empty, AutoTrustEmptyDir},
		{"main checkout", repo, ""},
		{"linked worktree root", worktree, AutoTrustLinkedWorktree},
		{"linked worktree subdir", filepath.Join(worktree, "sub"), ""},
		{"forged pointer", forged, ""},
		{"missing dir", filepath.Join(root, "nope"), ""},
	}
	for _, tt := range tests {
		if got := autoTrustReason(tt.dir); got != tt.want {
			t.Errorf("%s: autoTrustReason(%s) = %q, want %q", tt.name, tt.dir, got, tt.want)
		}
	}
}
