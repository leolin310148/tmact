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

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
