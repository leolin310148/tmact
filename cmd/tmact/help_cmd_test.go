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
			want: []string{"loop", "Subcommands:", "tmact loop start", "--dry-run", "Permission", "Do not write nohup"},
		},
		{
			name: "loop start",
			args: []string{"loop", "start", "--help"},
			want: []string{"loop start", "Idempotently", "tmact-loops", "Do not put this command in nohup", "--timeout"},
		},
		{
			name: "loop stop",
			args: []string{"loop", "stop", "--help"},
			want: []string{"cooperative stop", "--wait", "--force", "Bash polling loop"},
		},
		{
			name: "nested loop status",
			args: []string{"loop", "status", "--help"},
			want: []string{"loop status", "Inspect registered loop run metadata", "--run-dir", "last event"},
		},
		{
			name: "trust folder",
			args: []string{"trust-folder", "--help"},
			want: []string{"trust-folder", "exact-directory", "--execute", "Default is dry-run", "refuses non-trust prompts"},
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
	foundLoopStart := false
	foundTrustFolder := false
	foundWorkflow := false
	foundLLM := false
	for _, command := range manifest.Commands {
		if command.Command == "loop status" {
			foundLoopStatus = true
			if len(command.Examples) == 0 || len(command.Notes) == 0 {
				t.Fatalf("loop status help is too sparse: %#v", command)
			}
		}
		if command.Command == "loop start" {
			foundLoopStart = true
			if len(command.Safety) == 0 || len(command.Notes) < 2 {
				t.Fatalf("loop start help is too sparse: %#v", command)
			}
		}
		if command.Command == "trust-folder" {
			foundTrustFolder = true
			if len(command.Safety) < 2 || len(command.Notes) < 2 {
				t.Fatalf("trust-folder help is too sparse: %#v", command)
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
	if !foundLoopStart {
		t.Fatalf("loop start missing from manifest: %#v", manifest.Commands)
	}
	if !foundTrustFolder {
		t.Fatalf("trust-folder missing from manifest: %#v", manifest.Commands)
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
	foundLoopLifecycle := false
	foundTrustWorkflow := false
	for _, note := range instructions.SafeDefaults {
		if strings.Contains(note, "untrusted") {
			foundUntrusted = true
			break
		}
	}
	for _, step := range instructions.RecommendedWorkflow {
		if strings.Contains(step, "tmact loop start") && strings.Contains(step, "tmact loop stop") {
			foundLoopLifecycle = true
		}
		if strings.Contains(step, "dispatch-work --trust-folder") && strings.Contains(step, "tmact trust-folder") {
			foundTrustWorkflow = true
		}
	}
	if !foundUntrusted {
		t.Fatalf("instructions missing untrusted-pane warning: %#v", instructions.SafeDefaults)
	}
	if !foundLoopLifecycle {
		t.Fatalf("instructions missing managed loop lifecycle: %#v", instructions.RecommendedWorkflow)
	}
	if !foundTrustWorkflow {
		t.Fatalf("instructions missing exact-directory trust workflow: %#v", instructions.RecommendedWorkflow)
	}
}
