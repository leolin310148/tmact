package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunOpenSpecValidationMarksStaleWhenArtifactsChange(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	changeDir := filepath.Join(root, "openspec", "changes", "demo")
	writeFile(t, filepath.Join(changeDir, "proposal.md"), "# Demo\n")
	writeFile(t, filepath.Join(changeDir, "specs", "feature", "spec.md"), "## ADDED Requirements\n")

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(binDir, "openspec")
	script := "#!/bin/sh\nprintf '# Demo changed\\n' > openspec/changes/demo/proposal.md\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := RunOpenSpecValidation(context.Background(), "demo", changeDir)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stale {
		t.Fatalf("expected stale validation result: %#v", result)
	}
	if result.Passed {
		t.Fatalf("stale validation must not pass: %#v", result)
	}
	if result.ChangeHash == "" || result.HashAfterRun == "" || result.ChangeHash == result.HashAfterRun {
		t.Fatalf("hashes do not show artifact change: before=%q after=%q", result.ChangeHash, result.HashAfterRun)
	}
	if !strings.Contains(strings.Join(result.Args, " "), "demo") {
		t.Fatalf("validation args missing change name: %#v", result.Args)
	}
}
