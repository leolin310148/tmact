package statusd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultFileConfigDir  = ".tmact"
	DefaultFileConfigName = "statusd.json"
	DefaultWebAddr        = "127.0.0.1:7890"
	DefaultLogPath        = "/tmp/tmact-statusd.jsonl"
	DefaultFileInterval   = 500 * time.Millisecond
)

// FileConfig is the on-disk shape of ~/.tmact/statusd.json. Pointer / string
// fields let us tell "absent" from "explicit zero" so CLI flags can override
// only the keys the user actually set.
type FileConfig struct {
	WebAddr     string `json:"web_addr,omitempty"`
	Interval    string `json:"interval,omitempty"`
	SocketPath  string `json:"socket_path,omitempty"`
	LogPath     string `json:"log_path,omitempty"`
	TmuxOptions *bool  `json:"tmux_options,omitempty"`
}

// DefaultFileConfig is the seed written when statusd.json is missing.
func DefaultFileConfig() FileConfig {
	t := true
	return FileConfig{
		WebAddr:     DefaultWebAddr,
		Interval:    DefaultFileInterval.String(),
		SocketPath:  DefaultSocketPath,
		LogPath:     DefaultLogPath,
		TmuxOptions: &t,
	}
}

// DefaultFileConfigPath returns ~/.tmact/statusd.json. Empty string on error.
func DefaultFileConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, DefaultFileConfigDir, DefaultFileConfigName)
}

// LoadFileConfig parses path. Returns os.ErrNotExist if the file is missing
// so callers can decide whether to seed defaults.
func LoadFileConfig(path string) (FileConfig, error) {
	if path == "" {
		return FileConfig{}, errors.New("statusd config path is empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, err
	}
	var cfg FileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return FileConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.Interval != "" {
		if _, err := time.ParseDuration(cfg.Interval); err != nil {
			return FileConfig{}, fmt.Errorf("parse %s: invalid interval %q: %w", path, cfg.Interval, err)
		}
	}
	return cfg, nil
}

// WriteFileConfig writes cfg as pretty JSON, creating parent dirs as needed.
func WriteFileConfig(path string, cfg FileConfig) error {
	if path == "" {
		return errors.New("statusd config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// LoadOrCreateFileConfig returns the parsed file at path. If the file is
// missing it writes DefaultFileConfig() to path and returns those defaults.
// The bool reports whether the seed file was just created.
func LoadOrCreateFileConfig(path string) (FileConfig, bool, error) {
	cfg, err := LoadFileConfig(path)
	if err == nil {
		return cfg, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return FileConfig{}, false, err
	}
	seed := DefaultFileConfig()
	if writeErr := WriteFileConfig(path, seed); writeErr != nil {
		return FileConfig{}, false, writeErr
	}
	return seed, true, nil
}

// IntervalDuration returns the parsed interval, or zero if unset.
func (c FileConfig) IntervalDuration() time.Duration {
	if c.Interval == "" {
		return 0
	}
	d, err := time.ParseDuration(c.Interval)
	if err != nil {
		return 0
	}
	return d
}
