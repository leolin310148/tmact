package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	loopcfg "github.com/leolin310148/tmact/internal/loop"
)

func TestLoopExamplePrintsValidYAML(t *testing.T) {
	for _, tt := range []struct {
		name      string
		args      []string
		wantQuota bool
	}{
		{name: "basic"},
		{name: "quota", args: []string{"--quota"}, wantQuota: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"loop", "example"}, tt.args...)
			out, err := captureRun(t, args...)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"target: sample-agent:0.0", "flows:", "only_when_idle: true", "type: clear", "type: send_text"} {
				if !strings.Contains(out, want) {
					t.Fatalf("loop example missing %q:\n%s", want, out)
				}
			}
			path := filepath.Join(t.TempDir(), "loop.yaml")
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := loopcfg.LoadConfig(path)
			if err != nil {
				t.Fatalf("generated example must validate: %v\n%s", err, out)
			}
			if tt.wantQuota {
				if cfg.Quota == nil || !cfg.Quota.Enabled || !cfg.Quota.WeeklyRequireHeadroom || cfg.Quota.SessionMinRemainingPercent != 20 {
					t.Fatalf("quota example parsed incorrectly: %#v", cfg.Quota)
				}
			} else if cfg.Quota != nil {
				t.Fatalf("basic example unexpectedly enabled quota: %#v", cfg.Quota)
			}
		})
	}
}

func TestLoopExampleRejectsPositionalArguments(t *testing.T) {
	if _, err := captureRun(t, "loop", "example", "unexpected"); err == nil {
		t.Fatal("expected positional argument error")
	}
}
