package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigAppliesDefaults(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
stages:
  - prompt: implement
    complete_when:
      idle: true
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CaptureLines != 160 {
		t.Fatalf("capture_lines = %d", cfg.CaptureLines)
	}
	if cfg.PollInterval.Duration != 20*time.Second {
		t.Fatalf("poll_interval = %s", cfg.PollInterval.Duration)
	}
	if cfg.IdleAfter.Duration != 2*time.Minute {
		t.Fatalf("idle_after = %s", cfg.IdleAfter.Duration)
	}
	if cfg.StageEvery.Duration != 0 {
		t.Fatalf("stage_every = %s", cfg.StageEvery.Duration)
	}
	if cfg.Stages[0].Name != "stage-1" {
		t.Fatalf("stage name = %q", cfg.Stages[0].Name)
	}
	if cfg.Stages[0].Repeat != 1 {
		t.Fatalf("stage repeat = %d", cfg.Stages[0].Repeat)
	}
}

func TestLoadConfigParsesStageEvery(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
stage_every: 20m
stages:
  - prompt: implement
    complete_when:
      idle: true
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.StageEvery.Duration != 20*time.Minute {
		t.Fatalf("stage_every = %s", cfg.StageEvery.Duration)
	}
}

func TestLoadConfigRejectsMissingTarget(t *testing.T) {
	path := writeTempConfig(t, `
stages:
  - prompt: implement
    complete_when:
      idle: true
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigAllowsStageTargetsWithoutDefaultTarget(t *testing.T) {
	path := writeTempConfig(t, `
stages:
  - target: planner:0.0
    prompt: plan
    complete_when:
      idle: true
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Stages[0].Target != "planner:0.0" {
		t.Fatalf("stage target = %q", cfg.Stages[0].Target)
	}
}

func TestLoadConfigRejectsMissingStages(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsMissingPrompt(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
stages:
  - name: implement
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsMissingCompleteWhen(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
stages:
  - name: implement
    prompt: implement
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsInvalidCompletionRegex(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
stages:
  - name: implement
    prompt: implement
    complete_when:
      recent_output_matches:
        - "["
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsNegativeDuration(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
poll_interval: -1s
stages:
  - name: implement
    prompt: implement
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsNegativeStageEvery(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
stage_every: -1s
stages:
  - name: implement
    prompt: implement
    complete_when:
      idle: true
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsNegativeRepeat(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
stages:
  - name: implement
    repeat: -1
    prompt: implement
    complete_when:
      idle: true
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadExampleWorkflowConfig(t *testing.T) {
	if _, err := LoadConfig(filepath.Join("..", "..", "examples", "implement-review-workflow.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(filepath.Join("..", "..", "examples", "simple-improvement-workflow.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(filepath.Join("..", "..", "examples", "five-improvement-review-workflow.yaml")); err != nil {
		t.Fatal(err)
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
