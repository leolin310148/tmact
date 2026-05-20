package statusd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadOrCreateFileConfig_SeedsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "statusd.json")

	cfg, created, err := LoadOrCreateFileConfig(path)
	if err != nil {
		t.Fatalf("LoadOrCreateFileConfig: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true for missing file")
	}
	if cfg.WebAddr != DefaultWebAddr {
		t.Errorf("WebAddr = %q, want %q", cfg.WebAddr, DefaultWebAddr)
	}
	if cfg.StatePath != DefaultStatePath {
		t.Errorf("StatePath = %q, want %q", cfg.StatePath, DefaultStatePath)
	}
	if cfg.LogPath != DefaultLogPath {
		t.Errorf("LogPath = %q, want %q", cfg.LogPath, DefaultLogPath)
	}
	if cfg.IntervalDuration() != DefaultFileInterval {
		t.Errorf("Interval = %v, want %v", cfg.IntervalDuration(), DefaultFileInterval)
	}
	if cfg.TmuxOptions == nil || !*cfg.TmuxOptions {
		t.Errorf("TmuxOptions should default to true")
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("seed file not written: %v", err)
	}

	cfg2, created2, err := LoadOrCreateFileConfig(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if created2 {
		t.Fatalf("expected created=false on second load")
	}
	if cfg2.WebAddr != cfg.WebAddr {
		t.Errorf("round-trip WebAddr drift: got %q want %q", cfg2.WebAddr, cfg.WebAddr)
	}
}

func TestLoadFileConfig_ParsesValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "statusd.json")
	body := `{
  "web_addr": "0.0.0.0:7890",
  "interval": "5s",
  "state_path": "/tmp/x.json",
  "log_path": "/tmp/x.jsonl",
  "tmux_options": false
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadFileConfig(path)
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if cfg.WebAddr != "0.0.0.0:7890" {
		t.Errorf("WebAddr = %q", cfg.WebAddr)
	}
	if d := cfg.IntervalDuration(); d != 5*time.Second {
		t.Errorf("Interval = %v, want 5s", d)
	}
	if cfg.StatePath != "/tmp/x.json" || cfg.LogPath != "/tmp/x.jsonl" {
		t.Errorf("paths not parsed: %+v", cfg)
	}
	if cfg.TmuxOptions == nil || *cfg.TmuxOptions {
		t.Errorf("TmuxOptions = %v, want pointer to false", cfg.TmuxOptions)
	}
}

func TestLoadFileConfig_BadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "statusd.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadFileConfig(path); err == nil {
		t.Fatalf("expected error parsing bad JSON")
	}
}

func TestLoadFileConfig_BadInterval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "statusd.json")
	if err := os.WriteFile(path, []byte(`{"interval":"banana"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadFileConfig(path); err == nil {
		t.Fatalf("expected error parsing bad interval")
	}
}

func TestLoadFileConfig_MissingFileReportsNotExist(t *testing.T) {
	_, err := LoadFileConfig(filepath.Join(t.TempDir(), "nope.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}
