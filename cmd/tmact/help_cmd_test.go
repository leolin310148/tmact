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
			want: []string{"tmact - local tmux automation", "tmact commands --json", "tmact llm instructions", "Safety:"},
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
		{
			name: "llm group",
			args: []string{"llm", "--help"},
			want: []string{"llm", "LLMs and tools", "instructions", "tmact commands --json"},
		},
		{
			name: "llm instructions",
			args: []string{"llm", "instructions", "--help"},
			want: []string{"llm instructions", "LLM-facing operating instructions", "--json", "permission"},
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
	foundLLM := false
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
		if command.Command == "llm instructions" {
			foundLLM = true
			if len(command.Safety) == 0 {
				t.Fatalf("llm instructions help is missing safety notes: %#v", command)
			}
		}
	}
	if !foundLoopStatus {
		t.Fatalf("loop status missing from manifest: %#v", manifest.Commands)
	}
	if !foundWorkflow {
		t.Fatalf("workflow missing from manifest: %#v", manifest.Commands)
	}
	if !foundLLM {
		t.Fatalf("llm instructions missing from manifest: %#v", manifest.Commands)
	}
}

func TestLLMInstructionsJSONIncludesPolicyAndCatalog(t *testing.T) {
	out, err := captureRun(t, "llm", "instructions", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var instructions llmInstructions
	if err := json.Unmarshal([]byte(out), &instructions); err != nil {
		t.Fatal(err)
	}
	if instructions.Name == "" || len(instructions.SafeDefaults) == 0 || len(instructions.CommandCatalog.Commands) == 0 {
		t.Fatalf("instructions too sparse: %#v", instructions)
	}
	foundUntrusted := false
	for _, note := range instructions.SafeDefaults {
		if strings.Contains(note, "untrusted") {
			foundUntrusted = true
			break
		}
	}
	if !foundUntrusted {
		t.Fatalf("instructions missing untrusted-pane warning: %#v", instructions.SafeDefaults)
	}
}
