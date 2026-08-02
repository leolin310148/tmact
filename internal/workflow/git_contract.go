package workflow

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/leolin310148/tmact/internal/workspacelease"
)

type gitWorkItemState struct {
	Head   string
	Branch string
}

func (e *Engine) acquireWorkspaceLease() (func(), error) {
	policy := e.Loaded.Config.Workspace.Git
	if policy == nil || !policy.Lease {
		return func() {}, nil
	}
	lease, err := workspacelease.Acquire(e.Loaded.Config.Workspace.Root, e.Store.RunID)
	if err != nil {
		return nil, err
	}
	return lease.Release, nil
}

func ProbeWorkspaceLease(loaded Loaded) error {
	return workspacelease.CheckAvailable(loaded.Config.Workspace.Root, "")
}

func inspectWorkItemStart(cfg Config, item WorkItemConfig) (gitWorkItemState, error) {
	root := cfg.Workspace.Root
	status, err := gitBytes(root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return gitWorkItemState{}, err
	}
	if len(status) != 0 {
		return gitWorkItemState{}, fmt.Errorf("work item %s requires a clean worktree before dispatch", item.ID)
	}
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG", "rebase-merge", "rebase-apply"} {
		path, pathErr := gitText(root, "rev-parse", "--git-path", marker)
		if pathErr != nil {
			return gitWorkItemState{}, pathErr
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if _, statErr := os.Stat(path); statErr == nil {
			return gitWorkItemState{}, fmt.Errorf("work item %s cannot start while git operation %s is active", item.ID, marker)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return gitWorkItemState{}, statErr
		}
	}
	head, err := gitText(root, "rev-parse", "HEAD")
	if err != nil {
		return gitWorkItemState{}, err
	}
	branch, err := gitText(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return gitWorkItemState{}, fmt.Errorf("work item %s requires an attached branch: %w", item.ID, err)
	}
	checked, err := checkboxState(cfg, item, true)
	if err != nil {
		return gitWorkItemState{}, err
	}
	if !checked {
		return gitWorkItemState{}, fmt.Errorf("work item %s does not have exactly one unchecked checkbox in %s", item.ID, item.CheckboxPath)
	}
	return gitWorkItemState{Head: head, Branch: branch}, nil
}

func verifyWorkItemReport(cfg Config, item WorkItemConfig, dispatch Dispatch, outcome string) error {
	root := cfg.Workspace.Root
	status, err := gitBytes(root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return err
	}
	if len(status) != 0 {
		return fmt.Errorf("work item %s report rejected: worktree is not clean", item.ID)
	}
	branch, err := gitText(root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != dispatch.Branch {
		return fmt.Errorf("work item %s report rejected: branch changed from %s to %s", item.ID, dispatch.Branch, branch)
	}
	head, err := gitText(root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	complete := contains(item.CompleteOutcomes, outcome)
	if !complete {
		if head != dispatch.BaseHead {
			return fmt.Errorf("work item %s non-completion report rejected: HEAD changed", item.ID)
		}
		return nil
	}
	if head == dispatch.BaseHead {
		return fmt.Errorf("work item %s completion report rejected: HEAD did not advance", item.ID)
	}
	if _, err := gitBytes(root, "merge-base", "--is-ancestor", dispatch.BaseHead, head); err != nil {
		return fmt.Errorf("work item %s completion report rejected: new HEAD is not a descendant of %s", item.ID, dispatch.BaseHead)
	}
	checked, err := checkboxState(cfg, item, false)
	if err != nil {
		return err
	}
	if !checked {
		return fmt.Errorf("work item %s completion report rejected: checkbox is not checked", item.ID)
	}
	diff, err := gitBytes(root, "diff", "--unified=0", dispatch.BaseHead+".."+head, "--", item.CheckboxPath)
	if err != nil {
		return err
	}
	if !checkboxTransition(diff, item.ID) {
		return fmt.Errorf("work item %s completion report rejected: checkbox transition is not committed after %s", item.ID, dispatch.BaseHead)
	}
	if err := rejectOtherCheckboxChanges(diff, item.ID); err != nil {
		return err
	}
	return nil
}

func rejectOtherCheckboxChanges(diff []byte, id string) error {
	for _, line := range strings.Split(string(diff), "\n") {
		if (!strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "-")) || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		if (strings.Contains(line, "[ ]") || strings.Contains(line, "[x]")) && !containsWorkItemID(line, id) {
			return fmt.Errorf("work item %s completion report rejected: another checkbox changed", id)
		}
	}
	return nil
}

func checkboxState(cfg Config, item WorkItemConfig, unchecked bool) (bool, error) {
	path, err := safeWorkspacePath(cfg.Workspace.Root, item.CheckboxPath)
	if err != nil {
		return false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	needle := "[x]"
	if unchecked {
		needle = "[ ]"
	}
	count := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, needle) && containsWorkItemID(line, item.ID) {
			count++
		}
	}
	return count == 1, nil
}

func checkboxTransition(diff []byte, id string) bool {
	removed, added := false, false
	for _, line := range strings.Split(string(diff), "\n") {
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") && strings.Contains(line, "[ ]") && containsWorkItemID(line, id) {
			removed = true
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") && strings.Contains(line, "[x]") && containsWorkItemID(line, id) {
			added = true
		}
	}
	return removed && added
}

func containsWorkItemID(line, id string) bool {
	for offset := 0; ; {
		index := strings.Index(line[offset:], id)
		if index < 0 {
			return false
		}
		index += offset
		beforeOK := index == 0 || !isWorkItemIDByte(line[index-1])
		after := index + len(id)
		afterOK := after == len(line) || !isWorkItemIDByte(line[after])
		if beforeOK && afterOK {
			return true
		}
		offset = index + 1
	}
}

func isWorkItemIDByte(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '_' || value == '-' || value == '.'
}

func gitText(dir string, args ...string) (string, error) {
	out, err := gitBytes(dir, args...)
	return strings.TrimSpace(string(out)), err
}

func gitBytes(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
