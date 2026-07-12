package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/stt"
)

func TestSTTSetWritesProviderConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stt_provider.json")
	out, err := captureRun(t, "stt-set", "--config", path, "--provider", "openai", "--api-key", "sk-test", "--model", "whisper-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "wrote STT provider config") {
		t.Fatalf("output = %q", out)
	}
	if strings.Contains(out, "sk-test") {
		t.Fatalf("output leaked API key: %q", out)
	}
	cfg, err := stt.LoadProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "openai" || cfg.APIKey != "sk-test" || cfg.Model != "whisper-1" {
		t.Fatalf("config = %+v", cfg)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}

func TestApplyFileConfigSessionPersistence(t *testing.T) {
	save, restore, retention := false, true, 7
	cfg := statusd.Config{
		SessionSave:              true,
		SessionRestore:           false,
		SessionSaveInterval:      statusd.DefaultSessionSaveInterval,
		SessionSnapshotRetention: statusd.DefaultSessionSnapshotRetention,
		SessionSnapshotDir:       "/default",
	}
	webAddr := ""
	applyFileConfig(&cfg, &webAddr, statusd.FileConfig{
		SessionSave:              &save,
		SessionRestore:           &restore,
		SessionSaveInterval:      "10m",
		SessionSnapshotRetention: &retention,
		SessionSnapshotDir:       "/custom",
	}, map[string]bool{})
	if cfg.SessionSave || !cfg.SessionRestore {
		t.Fatalf("toggles = save:%v restore:%v", cfg.SessionSave, cfg.SessionRestore)
	}
	if cfg.SessionSaveInterval != 10*time.Minute || cfg.SessionSnapshotRetention != 7 || cfg.SessionSnapshotDir != "/custom" {
		t.Fatalf("session config = %+v", cfg)
	}
}

func TestValidateStatusdConfigRejectsUnsafeSessionPersistence(t *testing.T) {
	base := statusd.Config{
		Interval:                 time.Second,
		CaptureLines:             1,
		InitialSamples:           1,
		RunningDebounce:          time.Second,
		StaleAfter:               time.Second,
		SessionSave:              true,
		SessionSaveInterval:      time.Minute,
		SessionSnapshotRetention: 1,
		SessionSnapshotDir:       "/tmp/sessions",
	}
	tests := []statusd.Config{
		func() statusd.Config { c := base; c.SessionSaveInterval = 0; return c }(),
		func() statusd.Config { c := base; c.SessionSnapshotRetention = 0; return c }(),
		func() statusd.Config { c := base; c.SessionSnapshotDir = "relative"; return c }(),
	}
	for _, cfg := range tests {
		if err := validateStatusdConfig(cfg); err == nil {
			t.Fatalf("validateStatusdConfig(%+v) succeeded", cfg)
		}
	}
}

func TestValidatePeerNamesRejectsConflictingCostPeer(t *testing.T) {
	err := validatePeerNames(
		[]statusd.Peer{{Name: "z13", URL: "http://z13:7890"}},
		[]statusd.Peer{{Name: "z13", URL: "http://other:7890"}},
	)
	if err == nil || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("err = %v, want conflicting URL error", err)
	}
}
