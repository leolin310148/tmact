package agents

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAppliesDefaults(t *testing.T) {
	path := writeTempConfig(t, `
agents:
  - name: sample
    target: sample:0.0
`)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CaptureLines != 120 {
		t.Fatalf("capture_lines = %d", cfg.CaptureLines)
	}
	if cfg.Agents[0].CaptureLines != 120 {
		t.Fatalf("agent capture_lines = %d", cfg.Agents[0].CaptureLines)
	}
}

func TestLoadConfigRejectsDuplicateNames(t *testing.T) {
	path := writeTempConfig(t, `
agents:
  - name: sample
    target: sample:0.0
  - name: sample
    target: other:0.0
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadExampleAgentsConfig(t *testing.T) {
	if _, err := LoadConfig(filepath.Join("..", "..", "examples", "agents.yaml")); err != nil {
		t.Fatal(err)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "agents.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
