package web

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSweepDirRemovesOldFilesOnly(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.png")
	freshPath := filepath.Join(dir, "fresh.png")
	for _, p := range []string{oldPath, freshPath} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-uploadsMaxAge - time.Hour)
	if err := os.Chtimes(oldPath, old, old); err != nil {
		t.Fatal(err)
	}

	s := &Server{}
	s.sweepDir(dir)

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old file should be removed, stat err = %v", err)
	}
	if _, err := os.Stat(freshPath); err != nil {
		t.Errorf("fresh file should remain: %v", err)
	}
}

func TestSweepDirMissingDirNoop(t *testing.T) {
	s := &Server{}
	// Must not panic or error visibly when the directory does not exist.
	s.sweepDir(filepath.Join(t.TempDir(), "missing"))
}
