package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initGitWorkItemRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "WORK_ITEMS.md"), []byte("- [ ] P1-W1 implement guard\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "WORK_ITEMS.md")
	run("commit", "-qm", "initialize queue")
	return dir
}

func commitGitWorkItem(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "WORK_ITEMS.md"), []byte("- [x] P1-W1 implement guard\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "result.txt"), []byte("done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", "WORK_ITEMS.md", "result.txt")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}
	cmd = exec.Command("git", "commit", "-qm", "complete P1-W1")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
}

func TestGitWorkItemCompletionContract(t *testing.T) {
	dir := initGitWorkItemRepo(t)
	cfg := Config{Workspace: WorkspaceConfig{Root: dir}}
	item := WorkItemConfig{ID: "P1-W1", CheckboxPath: "WORK_ITEMS.md", CompleteOutcomes: []string{"complete"}}
	start, err := inspectWorkItemStart(cfg, item)
	if err != nil {
		t.Fatal(err)
	}
	commitGitWorkItem(t, dir)
	dispatch := Dispatch{WorkItem: item.ID, BaseHead: start.Head, Branch: start.Branch}
	if err := verifyWorkItemReport(cfg, item, dispatch, "complete"); err != nil {
		t.Fatal(err)
	}
}

func TestGitWorkItemRejectsDirtyStartAndUncommittedCompletion(t *testing.T) {
	dir := initGitWorkItemRepo(t)
	cfg := Config{Workspace: WorkspaceConfig{Root: dir}}
	item := WorkItemConfig{ID: "P1-W1", CheckboxPath: "WORK_ITEMS.md", CompleteOutcomes: []string{"complete"}}
	start, err := inspectWorkItemStart(cfg, item)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "WORK_ITEMS.md"), []byte("- [x] P1-W1 implement guard\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectWorkItemStart(cfg, item); err == nil || !strings.Contains(err.Error(), "clean worktree") {
		t.Fatalf("dirty preflight error=%v", err)
	}
	dispatch := Dispatch{WorkItem: item.ID, BaseHead: start.Head, Branch: start.Branch}
	if err := verifyWorkItemReport(cfg, item, dispatch, "complete"); err == nil || !strings.Contains(err.Error(), "not clean") {
		t.Fatalf("completion error=%v", err)
	}
}

func TestWorkspaceLeaseIsExclusive(t *testing.T) {
	dir := initGitWorkItemRepo(t)
	loaded := Loaded{Config: Config{Workspace: WorkspaceConfig{Root: dir, Git: &WorkspaceGitConfig{Lease: true}}}}
	first := &Engine{Loaded: loaded}
	release, err := first.acquireWorkspaceLease()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	second := &Engine{Loaded: loaded}
	if _, err := second.acquireWorkspaceLease(); err == nil || !strings.Contains(err.Error(), "leased") {
		t.Fatalf("second lease error=%v", err)
	}
}
