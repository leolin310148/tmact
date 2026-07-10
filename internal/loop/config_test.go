package loop

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigAppliesDefaults(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
actions:
  - type: clear
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CaptureLines != 120 {
		t.Fatalf("capture_lines = %d", cfg.CaptureLines)
	}
	if cfg.PollInterval.Duration != 30*time.Second {
		t.Fatalf("poll_interval = %s", cfg.PollInterval.Duration)
	}
	if cfg.IdleAfter.Duration != 2*time.Minute {
		t.Fatalf("idle_after = %s", cfg.IdleAfter.Duration)
	}
	if cfg.Actions[0].Name != "action-1" {
		t.Fatalf("action name = %q", cfg.Actions[0].Name)
	}
	if !actionEnter(cfg.Actions[0]) {
		t.Fatal("clear should enter by default")
	}
}

func TestLoadConfigParsesPostDelay(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
actions:
  - type: clear
    post_delay: 5s
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Actions[0].PostDelay.Duration != 5*time.Second {
		t.Fatalf("post_delay = %s", cfg.Actions[0].PostDelay.Duration)
	}
}

func TestLoadConfigParsesQuotaPaceGates(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
quota:
  enabled: true
  provider: codex
  weekly_require_headroom: true
  session_min_remaining_percent: 20
actions:
  - type: clear
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Quota.WeeklyRequireHeadroom {
		t.Fatal("weekly_require_headroom should be enabled")
	}
	if cfg.Quota.SessionMinRemainingPercent != 20 {
		t.Fatalf("session_min_remaining_percent = %v", cfg.Quota.SessionMinRemainingPercent)
	}
}

func TestLoadConfigParsesPeerTarget(t *testing.T) {
	path := writeTempConfig(t, `
peer: peer-a
target: "%7"
statusd_config: /tmp/statusd.json
actions:
  - type: clear
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Target != "peer-a@%7" || cfg.Peer != "peer-a" || cfg.StatusdConfig != "/tmp/statusd.json" {
		t.Fatalf("cfg = %#v", cfg)
	}
}

func TestLoadConfigRejectsConflictingPeerTarget(t *testing.T) {
	path := writeTempConfig(t, `
peer: peer-a
target: peer-b@%7
actions:
  - type: clear
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigParsesFlows(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
flows:
  - name: maintenance-cycle
    every: 20m
    initial_delay: 1m
    only_when_idle: true
    max_runs: 3
    steps:
      - type: send_keys
        keys: ["C-u"]
        post_delay: 500ms
      - name: clear-context
        type: clear
        post_delay: 5s
      - type: send_text
        text: continue
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	flow := cfg.Flows[0]
	if flow.Name != "maintenance-cycle" {
		t.Fatalf("flow name = %q", flow.Name)
	}
	if flow.Every.Duration != 20*time.Minute {
		t.Fatalf("flow every = %s", flow.Every.Duration)
	}
	if flow.InitialDelay.Duration != time.Minute {
		t.Fatalf("flow initial_delay = %s", flow.InitialDelay.Duration)
	}
	if !flow.OnlyWhenIdle {
		t.Fatal("flow should be idle-gated")
	}
	if flow.MaxRuns != 3 {
		t.Fatalf("flow max_runs = %d", flow.MaxRuns)
	}
	if flow.Steps[0].Name != "step-1" {
		t.Fatalf("first step name = %q", flow.Steps[0].Name)
	}
	if flow.Steps[1].Name != "clear-context" {
		t.Fatalf("second step name = %q", flow.Steps[1].Name)
	}
	if flow.Steps[0].PostDelay.Duration != 500*time.Millisecond {
		t.Fatalf("first step post_delay = %s", flow.Steps[0].PostDelay.Duration)
	}
}

func TestLoadConfigRejectsFlowStepSchedule(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
flows:
  - name: invalid-cycle
    steps:
      - type: clear
        every: 5m
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsNegativeActionMaxRuns(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
actions:
  - type: clear
    max_runs: -1
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsNegativeFlowMaxRuns(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
flows:
  - type: clear
    max_runs: -1
    steps:
      - type: clear
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsInvalidIdleIgnorePattern(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
idle_ignore_patterns:
  - "["
actions:
  - type: clear
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsMissingText(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
actions:
  - name: missing-text
    type: send_text
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadExampleConfigs(t *testing.T) {
	examples := []string{
		"night-loop.yaml",
		"maintenance-loop.yaml",
		"frontend-review-loop.yaml",
		"quota-aware-loop.yaml",
	}

	for _, name := range examples {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadConfig(filepath.Join("..", "..", "examples", name)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
