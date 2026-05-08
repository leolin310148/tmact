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

func TestResolveConfigPathUsesExplicitPath(t *testing.T) {
	got, err := ResolveConfigPath("custom.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got != "custom.yaml" {
		t.Fatalf("path = %q", got)
	}
}

func TestResolveConfigPathFindsDefaultAgentsFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("agents.yaml", []byte("agents: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveConfigPath("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "agents.yaml" {
		t.Fatalf("path = %q", got)
	}
}

func TestFilterConfigByAgent(t *testing.T) {
	cfg := Config{
		Agents: []AgentConfig{
			{Name: "one", Target: "one:0.0", Role: "frontend"},
			{Name: "two", Target: "two:0.0", Role: "backend"},
		},
	}

	filtered, err := FilterConfig(cfg, Filter{Agent: "two"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Agents) != 1 || filtered.Agents[0].Name != "two" {
		t.Fatalf("agents = %#v", filtered.Agents)
	}
}

func TestFilterConfigByRole(t *testing.T) {
	cfg := Config{
		Agents: []AgentConfig{
			{Name: "one", Target: "one:0.0", Role: "frontend"},
			{Name: "two", Target: "two:0.0", Role: "backend"},
			{Name: "three", Target: "three:0.0", Role: "frontend"},
		},
	}

	filtered, err := FilterConfig(cfg, Filter{Role: "frontend"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Agents) != 2 {
		t.Fatalf("agents = %#v", filtered.Agents)
	}
}

func TestFilterConfigRejectsAgentAndRole(t *testing.T) {
	cfg := Config{Agents: []AgentConfig{{Name: "one", Target: "one:0.0", Role: "frontend"}}}

	_, err := FilterConfig(cfg, Filter{Agent: "one", Role: "frontend"})
	if err == nil {
		t.Fatal("expected error")
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
