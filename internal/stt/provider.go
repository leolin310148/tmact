package stt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultProvider = "openai"
	DefaultModel    = "gpt-4o-transcribe"
	DefaultEndpoint = "https://api.openai.com/v1/audio/transcriptions"
)

// ProviderConfig is the local speech-to-text provider configuration stored at
// ~/.tmact/stt_provider.json.
type ProviderConfig struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

// DefaultProviderPath returns ~/.tmact/stt_provider.json for the current user.
func DefaultProviderPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tmact", "stt_provider.json"), nil
}

// LoadProvider reads and validates an STT provider config.
func LoadProvider(path string) (ProviderConfig, error) {
	if path == "" {
		var err error
		path, err = DefaultProviderPath()
		if err != nil {
			return ProviderConfig{}, err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ProviderConfig{}, fmt.Errorf("STT provider config not found at %s; run tmact stt-set --provider openai --api-key <key>", path)
		}
		return ProviderConfig{}, err
	}
	var cfg ProviderConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ProviderConfig{}, fmt.Errorf("parse STT provider config %s: %w", path, err)
	}
	if err := cfg.NormalizeAndValidate(); err != nil {
		return ProviderConfig{}, err
	}
	return cfg, nil
}

// SaveProvider validates and writes an STT provider config with user-only file
// permissions because it contains an API key.
func SaveProvider(path string, cfg ProviderConfig) error {
	if path == "" {
		var err error
		path, err = DefaultProviderPath()
		if err != nil {
			return err
		}
	}
	if err := cfg.NormalizeAndValidate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// NormalizeAndValidate fills safe defaults and enforces supported providers.
func (c *ProviderConfig) NormalizeAndValidate() error {
	c.Provider = strings.TrimSpace(c.Provider)
	c.APIKey = strings.TrimSpace(c.APIKey)
	c.Model = strings.TrimSpace(c.Model)
	c.Endpoint = strings.TrimSpace(c.Endpoint)

	if c.Provider == "" {
		c.Provider = DefaultProvider
	}
	if c.Provider != DefaultProvider {
		return fmt.Errorf("unsupported STT provider %q; supported provider: %s", c.Provider, DefaultProvider)
	}
	if c.APIKey == "" {
		return errors.New("STT provider config is missing api_key")
	}
	if c.Model == "" {
		c.Model = DefaultModel
	}
	if c.Endpoint == "" {
		c.Endpoint = DefaultEndpoint
	}
	return nil
}
