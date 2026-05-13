package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tmact/internal/runmeta"
	"tmact/internal/tmux"
	"tmact/internal/workflow"
)

func TestListPrintsAndCachesNumberedTargets(t *testing.T) {
	t.Chdir(t.TempDir())
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	listAllTmuxPanes = func() ([]tmux.Pane, error) {
		return []tmux.Pane{{
			Session:        "IDLL",
			WindowIndex:    1,
			WindowName:     "roadmap-codex",
			PaneIndex:      0,
			PaneID:         "%42",
			CurrentCommand: "codex",
			CurrentPath:    "/repo",
			Active:         true,
		}}, nil
	}
	tmactNow = func() time.Time { return time.Date(2026, 5, 11, 9, 30, 0, 0, time.UTC) }

	out, err := captureRun(t, "ls")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"#", "target", "0", "%42", "IDLL", "1:roadmap-codex", "codex", "/repo"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ls output missing %q: %s", want, out)
		}
	}

	data, err := os.ReadFile(filepath.Join(".cache", "tmact-targets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"target": "%42"`) {
		t.Fatalf("cache = %s", data)
	}
}

func TestSendDryRunResolvesNumberedTarget(t *testing.T) {
	t.Chdir(t.TempDir())
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	tmactNow = func() time.Time { return time.Date(2026, 5, 11, 9, 30, 0, 0, time.UTC) }
	if err := writeTargetCache(targetCache{
		GeneratedAt: tmactNow(),
		Panes: []listPaneRow{{
			Index:  0,
			Target: "%42",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	listTargetTmuxPanes = func(target string) ([]tmux.Pane, error) {
		if target != "%42" {
			t.Fatalf("target = %q", target)
		}
		return []tmux.Pane{{PaneID: "%42"}}, nil
	}
	pasteTmuxText = func(string, string, bool) error {
		t.Fatal("dry-run should not paste")
		return nil
	}

	out, err := captureRun(t, "-t", "0", "send", "--command", "go test ./...")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dry-run: would send command to %42: go test ./...") {
		t.Fatalf("output = %q", out)
	}
}

func TestSendExecuteCommandCanClearLine(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	var sentKeys [][]string
	var pastedTarget, pastedText string
	var pastedEnter bool
	sendTmuxKeys = func(_ string, keys []string) error {
		sentKeys = append(sentKeys, append([]string(nil), keys...))
		return nil
	}
	pasteTmuxText = func(target string, text string, enter bool) error {
		pastedTarget = target
		pastedText = text
		pastedEnter = enter
		return nil
	}

	out, err := captureRun(t, "-t", "%42", "send", "--clear-line", "--command", "go test ./...", "--execute")
	if err != nil {
		t.Fatal(err)
	}
	if len(sentKeys) != 1 || strings.Join(sentKeys[0], ",") != "C-u" {
		t.Fatalf("sent keys = %#v", sentKeys)
	}
	if pastedTarget != "%42" || pastedText != "go test ./..." || !pastedEnter {
		t.Fatalf("pasted target=%q text=%q enter=%t", pastedTarget, pastedText, pastedEnter)
	}
	if !strings.Contains(out, "clear line and send command to %42: go test ./...") {
		t.Fatalf("output = %q", out)
	}
}

func TestSendValidation(t *testing.T) {
	tests := [][]string{
		{"send", "--command", "go test ./..."},
		{"-t", "%42", "send"},
		{"-t", "%42", "send", "--text", "hi", "--command", "go test ./..."},
		{"-t", "%42", "send", "--key", "Enter", "--enter"},
		{"-t", "%42", "send", "--keys", "C-u,"},
	}
	for _, args := range tests {
		if _, err := captureRun(t, args...); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

func TestLoopStatusPrintsRegisteredRuns(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "loop.jsonl")
	if err := os.WriteFile(logPath, []byte(`{"ts":"2026-05-12T08:00:00Z","type":"action","target":"work:0.0","action":"prompt","status":"ok"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runmeta.Write(dir, runmeta.Run{
		ID:         "loop-night-loop-123",
		Kind:       "loop",
		ConfigPath: "/repo/examples/night-loop.yaml",
		Target:     "work:0.0",
		LogPath:    logPath,
		PID:        os.Getpid(),
		StartedAt:  time.Date(2026, 5, 12, 8, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 5, 12, 8, 0, 0, 0, time.UTC),
		Status:     "running",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "loop", "status", "--run-dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"loop-night-loop-123", "running", "work:0.0", "/repo/examples/night-loop.yaml", "action:prompt"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestWorkflowStatusPrintsLocalState(t *testing.T) {
	t.Chdir(t.TempDir())
	changeDir := filepath.Join("openspec", "changes", "demo")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := "workflow.yaml"
	if err := os.WriteFile(configPath, []byte(`change: demo
agents_config: agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
  reviewer: reviewer-agent
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := workflow.WriteState(workflow.StatePath(changeDir), workflow.State{
		Change:      "demo",
		Status:      "running",
		Phase:       "review",
		Turn:        2,
		PendingRole: "qa",
		ChangeHash:  "sha256:abc",
		Gate:        workflow.GateResult{Reasons: []string{"missing_agreement"}},
	}); err != nil {
		t.Fatal(err)
	}

	out, err := captureRun(t, "workflow", "status", "--config", configPath, "--run-dir", filepath.Join(t.TempDir(), "runs"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"workflow_state: demo", "pending_role: qa", "change_hash: sha256:abc", "gate_reasons: missing_agreement"} {
		if !strings.Contains(out, want) {
			t.Fatalf("workflow status missing %q: %s", want, out)
		}
	}
}

func TestHelpCommandsPrintRicherGuidance(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "top level",
			args: []string{"--help"},
			want: []string{"tmact - local tmux automation", "tmact commands --json", "Safety:"},
		},
		{
			name: "loop",
			args: []string{"loop", "--help"},
			want: []string{"loop", "Subcommands:", "tmact loop status", "--dry-run", "permission prompts"},
		},
		{
			name: "nested loop status",
			args: []string{"loop", "status", "--help"},
			want: []string{"loop status", "Inspect registered loop run metadata", "--run-dir", "last event"},
		},
		{
			name: "workflow",
			args: []string{"workflow", "--help"},
			want: []string{"workflow", "OpenSpec artifact review", "PM -> SWE -> QA -> reviewer", "--execute"},
		},
		{
			name: "panels group",
			args: []string{"panels", "--help"},
			want: []string{"panels", "Subcommands:", "plan", "ensure", "--execute"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := captureRun(t, tt.args...)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Fatalf("help output missing %q: %s", want, out)
				}
			}
		})
	}
}

func TestCommandsJSONIsMachineReadable(t *testing.T) {
	out, err := captureRun(t, "commands", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest helpManifest
	if err := json.Unmarshal([]byte(out), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "tmact" || len(manifest.Commands) == 0 {
		t.Fatalf("manifest = %#v", manifest)
	}
	foundLoopStatus := false
	foundWorkflow := false
	for _, command := range manifest.Commands {
		if command.Command == "loop status" {
			foundLoopStatus = true
			if len(command.Examples) == 0 || len(command.Notes) == 0 {
				t.Fatalf("loop status help is too sparse: %#v", command)
			}
		}
		if command.Command == "workflow" {
			foundWorkflow = true
		}
	}
	if !foundLoopStatus {
		t.Fatalf("loop status missing from manifest: %#v", manifest.Commands)
	}
	if !foundWorkflow {
		t.Fatalf("workflow missing from manifest: %#v", manifest.Commands)
	}
}

func captureRun(t *testing.T, args ...string) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = write
	err = run(args)
	if closeErr := write.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	os.Stdout = oldStdout

	output, readErr := io.ReadAll(read)
	if readErr != nil && err == nil {
		err = readErr
	}
	return string(output), err
}

func stubCLIHooks(t *testing.T) func() {
	t.Helper()

	oldListAllTmuxPanes := listAllTmuxPanes
	oldListTargetTmuxPanes := listTargetTmuxPanes
	oldPasteTmuxText := pasteTmuxText
	oldSendTmuxKeys := sendTmuxKeys
	oldTmactNow := tmactNow

	return func() {
		listAllTmuxPanes = oldListAllTmuxPanes
		listTargetTmuxPanes = oldListTargetTmuxPanes
		pasteTmuxText = oldPasteTmuxText
		sendTmuxKeys = oldSendTmuxKeys
		tmactNow = oldTmactNow
	}
}
