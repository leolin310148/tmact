package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
