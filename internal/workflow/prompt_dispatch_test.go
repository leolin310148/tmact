package workflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agents"
)

func TestPromptRoleDryRunEmitsClearPlanAndReportCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	logPath := filepath.Join(t.TempDir(), "workflow.jsonl")
	cfg := promptDispatchTestConfig(logPath)
	agentCfg := promptDispatchAgentConfig()
	runner, err := NewRunner(cfg, agentCfg, Options{DryRun: true, ConfigPath: "workflow.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	runner.pasteText = func(string, string, bool) error {
		t.Fatal("dry-run should not paste")
		return nil
	}
	if err := runner.promptRole(context.Background(), "qa", "sha256:abc", ValidationResult{Passed: true}, GateResult{}, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	for _, want := range []string{`"type":"clear"`, `"/clear"`, `"type":"prompt"`, "tmact workflow report review", "--config workflow.yaml"} {
		if !strings.Contains(output, want) {
			t.Fatalf("log missing %q: %s", want, output)
		}
	}
}

func TestPromptRoleLiveClearsBeforePrompt(t *testing.T) {
	cfg := promptDispatchTestConfig("")
	agentCfg := promptDispatchAgentConfig()
	runner, err := NewRunner(cfg, agentCfg, Options{DryRun: false, ConfigPath: "workflow.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	runner.sleep = func(time.Duration) {}
	var calls []string
	runner.pasteText = func(target string, text string, enter bool) error {
		if target != "%qa" || !enter {
			t.Fatalf("paste target=%q enter=%t", target, enter)
		}
		calls = append(calls, text)
		return nil
	}
	if err := runner.promptRole(context.Background(), "qa", "sha256:abc", ValidationResult{Passed: true}, GateResult{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "/clear" || !strings.Contains(calls[1], "tmact workflow report review") {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestPromptRoleStopBeforeClearSendsNothing(t *testing.T) {
	cfg := promptDispatchTestConfig("")
	agentCfg := promptDispatchAgentConfig()
	runner, err := NewRunner(cfg, agentCfg, Options{
		DryRun:        false,
		ConfigPath:    "workflow.yaml",
		StopRequested: func() bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	runner.pasteText = func(string, string, bool) error {
		t.Fatal("stop before clear should not paste")
		return nil
	}
	if err := runner.promptRole(context.Background(), "qa", "sha256:abc", ValidationResult{Passed: true}, GateResult{}, nil); err == nil {
		t.Fatal("expected stop_requested error")
	}
}

func TestPromptRoleStopAfterClearSkipsPrompt(t *testing.T) {
	cfg := promptDispatchTestConfig("")
	agentCfg := promptDispatchAgentConfig()
	stopChecks := 0
	runner, err := NewRunner(cfg, agentCfg, Options{
		DryRun:     false,
		ConfigPath: "workflow.yaml",
		StopRequested: func() bool {
			stopChecks++
			return stopChecks >= 3
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner.sleep = func(time.Duration) {}
	var calls []string
	runner.pasteText = func(_ string, text string, _ bool) error {
		calls = append(calls, text)
		return nil
	}
	if err := runner.promptRole(context.Background(), "qa", "sha256:abc", ValidationResult{Passed: true}, GateResult{}, nil); err == nil {
		t.Fatal("expected stop_requested error")
	}
	if len(calls) != 1 || calls[0] != "/clear" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestImplementationPromptIncludesReportCommand(t *testing.T) {
	cfg := promptDispatchImplementationTestConfig("")
	agentCfg := promptDispatchAgentConfig()
	runner, err := NewImplementationRunner(cfg, agentCfg, Options{DryRun: true, ConfigPath: "workflow.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	prompt := runner.buildStagePrompt("qa_verify", "qa", "sha256:abc", ValidationResult{Passed: true}, ImplementationGateResult{}, nil)
	for _, want := range []string{"tmact workflow report implementation", "--stage verify", "--kind pass", "--change-hash sha256:abc"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q: %s", want, prompt)
		}
	}
}

func TestObserveRolePanesOnlyParsesMarkersWhenFallbackEnabled(t *testing.T) {
	t.Chdir(t.TempDir())
	commentPath := filepath.Join(t.TempDir(), "comments.jsonl")
	cfg := promptDispatchTestConfig("")
	cfg.PromptDispatch.LegacyMarkerFallback = boolPtr(false)
	runner, err := NewRunner(cfg, promptDispatchAgentConfig(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	runner.capturePane = func(string, int) (string, error) {
		return `TMAct-OpenSpec-Comment: role=qa kind=accept change_hash=sha256:abc openspec_valid=true blocking=false`, nil
	}
	comments, err := runner.observeRolePanes(commentPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 0 {
		t.Fatalf("fallback off should not parse markers: %#v", comments)
	}

	cfg.PromptDispatch.LegacyMarkerFallback = boolPtr(true)
	runner, err = NewRunner(cfg, promptDispatchAgentConfig(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	runner.capturePane = func(string, int) (string, error) {
		return `TMAct-OpenSpec-Comment: role=qa kind=accept change_hash=sha256:abc openspec_valid=true blocking=false`, nil
	}
	comments, err = runner.observeRolePanes(commentPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("fallback on should parse marker: %#v", comments)
	}
}

func promptDispatchTestConfig(logPath string) Config {
	return Config{
		Change: "demo",
		Roles:  map[string]string{"pm": "pm-agent", "swe": "swe-agent", "qa": "qa-agent", "reviewer": "reviewer-agent"},
		PromptDispatch: PromptDispatchConfig{
			ClearBeforePrompt: boolPtr(true),
			ClearCommand:      "/clear",
		},
		Discussion: DiscussionConfig{
			RoleOrder: []string{"pm", "swe", "qa", "reviewer"},
		},
		LogPath: logPath,
	}
}

func promptDispatchImplementationTestConfig(logPath string) Config {
	cfg := promptDispatchTestConfig(logPath)
	cfg.Implementation = ImplementationConfig{
		StageOrder: []string{"swe_apply", "qa_verify", "pm_archive"},
	}
	return cfg
}

func promptDispatchAgentConfig() agents.Config {
	return agents.Config{Agents: []agents.AgentConfig{
		{Name: "pm-agent", Target: "%pm"},
		{Name: "swe-agent", Target: "%swe"},
		{Name: "qa-agent", Target: "%qa"},
		{Name: "reviewer-agent", Target: "%reviewer"},
	}}
}
