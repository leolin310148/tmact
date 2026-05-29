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
	WebAddr      string           `json:"web_addr,omitempty"`
	Interval     string           `json:"interval,omitempty"`
	SocketPath   string           `json:"socket_path,omitempty"`
	LogPath      string           `json:"log_path,omitempty"`
	TmuxOptions  *bool            `json:"tmux_options,omitempty"`
	PaneCols     *int             `json:"pane_cols,omitempty"`
	PaneRows     *int             `json:"pane_rows,omitempty"`
	Peers        []PeerFileConfig `json:"peers,omitempty"`
	PeerInterval string           `json:"peer_interval,omitempty"`
	PeerTimeout  string           `json:"peer_timeout,omitempty"`
	// AgentUsage enables the web UI's agent quota / rate-limit usage panel,
	// which reads each agent's local OAuth credentials read-only and polls the
	// provider usage endpoints on a slow ticker. Defaults to true when absent.
	AgentUsage *bool `json:"agent_usage,omitempty"`
}

// PeerFileConfig is the on-disk shape of one entry in FileConfig.Peers.
type PeerFileConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// DefaultFileConfig is the seed written when statusd.json is missing.
func DefaultFileConfig() FileConfig {
	t := true
	usage := true
	cols, rows := DefaultPaneCols, DefaultPaneRows
	return FileConfig{
		WebAddr:     DefaultWebAddr,
		Interval:    DefaultFileInterval.String(),
		SocketPath:  DefaultSocketPath,
		LogPath:     DefaultLogPath,
		TmuxOptions: &t,
		PaneCols:    &cols,
		PaneRows:    &rows,
		AgentUsage:  &usage,
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
	if cfg.PeerInterval != "" {
		if _, err := time.ParseDuration(cfg.PeerInterval); err != nil {
			return FileConfig{}, fmt.Errorf("parse %s: invalid peer_interval %q: %w", path, cfg.PeerInterval, err)
		}
	}
	if cfg.PeerTimeout != "" {
		if _, err := time.ParseDuration(cfg.PeerTimeout); err != nil {
			return FileConfig{}, fmt.Errorf("parse %s: invalid peer_timeout %q: %w", path, cfg.PeerTimeout, err)
		}
	}
	return cfg, nil
}

// PeerIntervalDuration returns the parsed peer poll interval, or zero if unset.
func (c FileConfig) PeerIntervalDuration() time.Duration {
	if c.PeerInterval == "" {
		return 0
	}
	d, err := time.ParseDuration(c.PeerInterval)
	if err != nil {
		return 0
	}
	return d
}

// PeerTimeoutDuration returns the parsed per-fetch timeout, or zero if unset.
func (c FileConfig) PeerTimeoutDuration() time.Duration {
	if c.PeerTimeout == "" {
		return 0
	}
	d, err := time.ParseDuration(c.PeerTimeout)
	if err != nil {
		return 0
	}
	return d
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
