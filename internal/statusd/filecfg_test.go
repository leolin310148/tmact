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
	if cfg.SocketPath != DefaultSocketPath {
		t.Errorf("SocketPath = %q, want %q", cfg.SocketPath, DefaultSocketPath)
	}
	if cfg.LogPath != "" {
		t.Errorf("LogPath = %q, want empty (snapshot log opt-in)", cfg.LogPath)
	}
	if cfg.IntervalDuration() != DefaultFileInterval {
		t.Errorf("Interval = %v, want %v", cfg.IntervalDuration(), DefaultFileInterval)
	}
	if cfg.TmuxOptions == nil || !*cfg.TmuxOptions {
		t.Errorf("TmuxOptions should default to true")
	}
	if cfg.AgentUsage == nil || !*cfg.AgentUsage {
		t.Errorf("AgentUsage should default to true")
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
  "socket_path": "/tmp/x.sock",
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
	if cfg.SocketPath != "/tmp/x.sock" || cfg.LogPath != "/tmp/x.jsonl" {
		t.Errorf("paths not parsed: %+v", cfg)
	}
	if cfg.TmuxOptions == nil || *cfg.TmuxOptions {
		t.Errorf("TmuxOptions = %v, want pointer to false", cfg.TmuxOptions)
	}
}

func TestLoadFileConfig_CostPeers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "statusd.json")
	body := `{
  "cost_peers": [
    { "name": "z13", "url": "http://z13:7890" }
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadFileConfig(path)
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if len(cfg.CostPeers) != 1 {
		t.Fatalf("CostPeers = %d, want 1", len(cfg.CostPeers))
	}
	if got := cfg.CostPeers[0]; got.Name != "z13" || got.URL != "http://z13:7890" {
		t.Fatalf("CostPeers[0] = %+v", got)
	}
}

func TestLoadFileConfig_DispatchPeers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "statusd.json")
	body := `{
  "dispatch_peers": [
    { "name": "hub", "url": "http://hub:7890" }
  ]
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadFileConfig(path)
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if len(cfg.DispatchPeers) != 1 {
		t.Fatalf("DispatchPeers = %d, want 1", len(cfg.DispatchPeers))
	}
	if got := cfg.DispatchPeers[0]; got.Name != "hub" || got.URL != "http://hub:7890" {
		t.Fatalf("DispatchPeers[0] = %+v", got)
	}
}

func TestLoadFileConfig_UsageAndSpendIntervals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "statusd.json")
	body := `{"usage_interval": "10m", "spend_interval": "30s"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadFileConfig(path)
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if d := cfg.UsageIntervalDuration(); d != 10*time.Minute {
		t.Errorf("UsageInterval = %v, want 10m", d)
	}
	if d := cfg.SpendIntervalDuration(); d != 30*time.Second {
		t.Errorf("SpendInterval = %v, want 30s", d)
	}
	// Unset → zero so the web package defaults apply.
	if d := (FileConfig{}).SpendIntervalDuration(); d != 0 {
		t.Errorf("unset SpendInterval = %v, want 0", d)
	}
}

func TestLoadFileConfig_AgentCostToggle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "statusd.json")
	if err := os.WriteFile(path, []byte(`{"agent_cost": false}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := LoadFileConfig(path)
	if err != nil {
		t.Fatalf("LoadFileConfig: %v", err)
	}
	if cfg.AgentCost == nil || *cfg.AgentCost {
		t.Errorf("AgentCost = %v, want pointer to false", cfg.AgentCost)
	}
	// Absent → nil so the CLI default (enabled) applies.
	other := filepath.Join(dir, "other.json")
	if err := os.WriteFile(other, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if c, _ := LoadFileConfig(other); c.AgentCost != nil {
		t.Errorf("absent AgentCost = %v, want nil", c.AgentCost)
	}
}

func TestLoadFileConfig_BadSpendInterval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "statusd.json")
	if err := os.WriteFile(path, []byte(`{"spend_interval": "nope"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadFileConfig(path); err == nil {
		t.Fatal("expected error for invalid spend_interval")
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
