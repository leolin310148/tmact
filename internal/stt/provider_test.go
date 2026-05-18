package stt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tmact", "stt_provider.json")
	if err := SaveProvider(path, ProviderConfig{
		Provider: "openai",
		APIKey:   "sk-test",
		Model:    "whisper-1",
		Endpoint: "https://example.test/transcribe",
	}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}

	cfg, err := LoadProvider(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "openai" || cfg.APIKey != "sk-test" || cfg.Model != "whisper-1" || cfg.Endpoint != "https://example.test/transcribe" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestSaveProviderTightensExistingFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stt_provider.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveProvider(path, ProviderConfig{APIKey: "sk-test"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}

func TestProviderDefaults(t *testing.T) {
	cfg := ProviderConfig{APIKey: "sk-test"}
	if err := cfg.NormalizeAndValidate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != DefaultProvider {
		t.Fatalf("provider = %q, want %q", cfg.Provider, DefaultProvider)
	}
	if cfg.Model != DefaultModel {
		t.Fatalf("model = %q, want %q", cfg.Model, DefaultModel)
	}
	if cfg.Endpoint != DefaultEndpoint {
		t.Fatalf("endpoint = %q, want %q", cfg.Endpoint, DefaultEndpoint)
	}
}

func TestLoadProviderMissingFileGuidesUser(t *testing.T) {
	_, err := LoadProvider(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected missing config error")
	}
	if !strings.Contains(err.Error(), "tmact stt-set --provider openai --api-key") {
		t.Fatalf("error = %q, want stt-set guidance", err.Error())
	}
}

func TestProviderRejectsMissingAPIKey(t *testing.T) {
	cfg := ProviderConfig{Provider: "openai"}
	if err := cfg.NormalizeAndValidate(); err == nil {
		t.Fatal("expected missing api_key error")
	}
}

func TestProviderRejectsUnknownProvider(t *testing.T) {
	cfg := ProviderConfig{Provider: "other", APIKey: "key"}
	if err := cfg.NormalizeAndValidate(); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}
