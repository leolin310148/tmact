package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashChangeDirIncludesSpecDeltasAndNormalizesCRLF(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "proposal.md"), "hello\r\n")
	writeFile(t, filepath.Join(dir, "design.md"), "design\n")
	writeFile(t, filepath.Join(dir, "tasks.md"), "tasks\n")
	writeFile(t, filepath.Join(dir, "specs", "feature", "spec.md"), "scenario\n")

	first, paths, err := HashChangeDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 4 {
		t.Fatalf("paths = %#v", paths)
	}

	writeFile(t, filepath.Join(dir, "proposal.md"), "hello\n")
	second, _, err := HashChangeDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("CRLF-normalized hash changed: %s != %s", first, second)
	}

	writeFile(t, filepath.Join(dir, "specs", "feature", "spec.md"), "changed\n")
	third, _, err := HashChangeDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if third == second {
		t.Fatalf("spec delta change did not affect hash")
	}
}

func writeFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
