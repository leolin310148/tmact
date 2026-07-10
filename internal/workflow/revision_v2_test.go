package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFilesDigestIsSortedAndNormalizesCRLF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := RevisionConfig{Files: &FilesRevisionConfig{Paths: []string{"b.txt", "a.txt"}}}
	first, err := ComputeRevision(dir, cfg, TemplateData{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Files.Paths = []string{"a.txt", "b.txt"}
	second, err := ComputeRevision(dir, cfg, TemplateData{})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("order changed digest: %s != %s", first, second)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := ComputeRevision(dir, cfg, TemplateData{})
	if err != nil {
		t.Fatal(err)
	}
	if second != third {
		t.Fatalf("CRLF changed digest: %s != %s", second, third)
	}
}

func TestGitDigestCoversTrackedAndUntracked(t *testing.T) {
	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	runGit("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "tracked"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "tracked")
	runGit("commit", "-qm", "init")
	cfg := RevisionConfig{Git: &GitRevisionConfig{}}
	data := TemplateData{}
	base, err := ComputeRevision(dir, cfg, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	withUntracked, err := ComputeRevision(dir, cfg, data)
	if err != nil {
		t.Fatal(err)
	}
	if base == withUntracked {
		t.Fatal("untracked file did not change git digest")
	}
	runGit("add", "untracked")
	withIndex, err := ComputeRevision(dir, cfg, data)
	if err != nil {
		t.Fatal(err)
	}
	if withIndex == withUntracked {
		t.Fatal("index did not change git digest")
	}
	if err := os.WriteFile(filepath.Join(dir, "tracked"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	withWorktree, err := ComputeRevision(dir, cfg, data)
	if err != nil {
		t.Fatal(err)
	}
	if withWorktree == withIndex {
		t.Fatal("worktree did not change git digest")
	}
}
