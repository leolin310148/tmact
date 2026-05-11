package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigAppliesDefaults(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
rules:
  - type: directory_access_prompt
    allow_paths:
      - /tmp/project
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CaptureLines != 160 {
		t.Fatalf("capture_lines = %d", cfg.CaptureLines)
	}
	if cfg.PollInterval.Duration != 5*time.Second {
		t.Fatalf("poll_interval = %s", cfg.PollInterval.Duration)
	}
	if cfg.Rules[0].Name != "rule-1" {
		t.Fatalf("rule name = %q", cfg.Rules[0].Name)
	}
	if cfg.Rules[0].AcceptOption != "selected" {
		t.Fatalf("accept_option = %q", cfg.Rules[0].AcceptOption)
	}
	if cfg.Rules[0].Cooldown.Duration != 30*time.Second {
		t.Fatalf("cooldown = %s", cfg.Rules[0].Cooldown.Duration)
	}
}

func TestLoadConfigRejectsMissingAllowPaths(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
rules:
  - type: directory_access_prompt
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigAcceptsAllowPathPatterns(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
rules:
  - type: directory_access_prompt
    allow_path_patterns:
      - /tmp/sample-project-rn-*
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules[0].AllowPathPatterns[0] != "/tmp/sample-project-rn-*" {
		t.Fatalf("allow_path_patterns = %#v", cfg.Rules[0].AllowPathPatterns)
	}
}

func TestLoadConfigRejectsInvalidAllowPathPattern(t *testing.T) {
	path := writeTempConfig(t, `
target: sample:0.0
rules:
  - type: directory_access_prompt
    allow_path_patterns:
      - /tmp/sample-project-rn-[
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadExampleConfigs(t *testing.T) {
	if _, err := LoadConfig(filepath.Join("..", "..", "examples", "accept-question-watch.yaml")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"idll-roadmap-data-watch-codex.yaml",
		"idll-roadmap-data-watch-coordinator.yaml",
		"idll-roadmap-data-watch-copilot.yaml",
		"idll-roadmap-data-watch-gemini.yaml",
	} {
		if _, err := LoadConfig(filepath.Join("..", "..", "examples", name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
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
