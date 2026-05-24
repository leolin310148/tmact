package main

import (
	"encoding/json"
	"strings"
	"testing"
)

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
			want: []string{"workflow", "OpenSpec review and implementation", "workflow example", "SWE apply -> QA verify -> PM archive", "--execute"},
		},
		{
			name: "workflow example",
			args: []string{"workflow", "example", "--help"},
			want: []string{"workflow example", "combined OpenSpec workflow YAML", "tmact workflow example"},
		},
		{
			name: "workflow implement",
			args: []string{"workflow", "implement", "--help"},
			want: []string{"workflow implement", "OpenSpec implementation", "--config", "--execute"},
		},
		{
			name: "workflow report",
			args: []string{"workflow", "report", "--help"},
			want: []string{"workflow report", "durable JSONL reports", "workflow report review", "workflow report implementation"},
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
