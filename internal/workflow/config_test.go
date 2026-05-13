package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigDefaultsAndRequiredRoles(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(filepath.Join("openspec", "changes", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := "workflow.yaml"
	data := []byte(`change: demo
agents_config: agents.yaml
roles:
  pm: pm-agent
  swe: swe-agent
  qa: qa-agent
  reviewer: reviewer-agent
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.Discussion.RoleOrder, ",") != "pm,swe,qa,reviewer" {
		t.Fatalf("role order = %#v", cfg.Discussion.RoleOrder)
	}
	if cfg.Discussion.MaxTurns != 24 || cfg.Discussion.CaptureLines != 180 {
		t.Fatalf("defaults not applied: %#v", cfg.Discussion)
	}
}

func TestChangeDirRejectsEscapes(t *testing.T) {
	for _, change := range []string{"../demo", "/tmp/demo", "demo/../../escape"} {
		t.Run(change, func(t *testing.T) {
			if _, err := ChangeDir(change); err == nil {
				t.Fatalf("expected %q to fail", change)
			}
		})
	}
}
