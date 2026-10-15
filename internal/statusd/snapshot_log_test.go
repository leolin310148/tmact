package statusd

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendLogRotatesAtSizeCap(t *testing.T) {
	saved := snapshotLogMaxBytes
	snapshotLogMaxBytes = 256
	defer func() { snapshotLogMaxBytes = saved }()

	path := filepath.Join(t.TempDir(), "statusd.jsonl")
	for i := 0; i < 50; i++ {
		if err := appendLog(path, Snapshot{Version: i}); err != nil {
			t.Fatalf("appendLog #%d: %v", i, err)
		}
	}

	for _, p := range []string{path, path + ".1"} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if info.Size() >= 2*snapshotLogMaxBytes {
			t.Fatalf("%s is %d bytes, want under %d", p, info.Size(), 2*snapshotLogMaxBytes)
		}
	}
	if _, err := os.Stat(path + ".2"); !os.IsNotExist(err) {
		t.Fatalf("expected a single backup, found %s.2 (err=%v)", path, err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	lines := 0
	for scanner := bufio.NewScanner(file); scanner.Scan(); lines++ {
	}
	if lines == 0 {
		t.Fatal("current log is empty after rotation")
	}
}
