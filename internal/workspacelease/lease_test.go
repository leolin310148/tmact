package workspacelease

import (
	"os/exec"
	"strings"
	"testing"
)

func initLeaseRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return dir
}

func TestLeaseBlocksOtherOwnerAndAllowsCurrentOwner(t *testing.T) {
	dir := initLeaseRepo(t)
	lease, err := Acquire(dir, "wf-owner")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if err := CheckAvailable(dir, ""); err == nil || !strings.Contains(err.Error(), "wf-owner") {
		t.Fatalf("other owner error=%v", err)
	}
	if err := CheckAvailable(dir, "wf-owner"); err != nil {
		t.Fatalf("current owner rejected: %v", err)
	}
	lease.Release()
	if err := CheckAvailable(dir, ""); err != nil {
		t.Fatalf("released lease unavailable: %v", err)
	}
}

func TestLeaseCheckIgnoresNonGitDirectories(t *testing.T) {
	if err := CheckAvailable(t.TempDir(), ""); err != nil {
		t.Fatal(err)
	}
}
