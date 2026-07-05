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
	DefaultFileInterval   = 500 * time.Millisecond
)

// FileConfig is the on-disk shape of ~/.tmact/statusd.json. Pointer / string
// fields let us tell "absent" from "explicit zero" so CLI flags can override
// only the keys the user actually set.
type FileConfig struct {
	WebAddr     string           `json:"web_addr,omitempty"`
	Interval    string           `json:"interval,omitempty"`
	SocketPath  string           `json:"socket_path,omitempty"`
	LogPath     string           `json:"log_path,omitempty"`
	TmuxOptions *bool            `json:"tmux_options,omitempty"`
	PaneCols    *int             `json:"pane_cols,omitempty"`
	PaneRows    *int             `json:"pane_rows,omitempty"`
	Peers       []PeerFileConfig `json:"peers,omitempty"`
	CostPeers   []PeerFileConfig `json:"cost_peers,omitempty"`
	// DispatchPeers are remote statusd instances that dispatch-work --peer can
	// call without also merging their snapshots into this daemon.
	DispatchPeers []PeerFileConfig `json:"dispatch_peers,omitempty"`
	PeerInterval  string           `json:"peer_interval,omitempty"`
	PeerTimeout   string           `json:"peer_timeout,omitempty"`
	// AgentUsage enables the web UI's agent quota / rate-limit usage panel,
	// which reads each agent's local OAuth credentials read-only and polls the
	// provider usage endpoints on a slow ticker. Defaults to true when absent.
	AgentUsage *bool `json:"agent_usage,omitempty"`
	// AgentCost enables the token-spend (cost) computation shown in that panel:
	// a local session-log scan priced at API rates, plus peer roll-up. Defaults
	// to true when absent. Set false on a machine (e.g. a peer) that should
	// neither compute its own cost nor contribute spend to a hub's total.
	AgentCost *bool `json:"agent_cost,omitempty"`
	// UsageInterval / SpendInterval tune the agent-usage panel refresh cadences
	// independently (Go duration strings, e.g. "5m", "30s"). UsageInterval
	// drives the rate-limited quota fetch; SpendInterval the local token-spend
	// scan + peer roll-up. Empty uses defaults (5m / 60s).
	UsageInterval string `json:"usage_interval,omitempty"`
	SpendInterval string `json:"spend_interval,omitempty"`
	// WebPush configures the same-origin PWA Web Push service. VAPIDPrivateKey
	// is supported for local config, but production should prefer the
	// TMACT_WEBPUSH_VAPID_PRIVATE_KEY environment variable so secrets are not
	// accidentally copied around with general statusd config.
	WebPushVAPIDPublicKey    string `json:"webpush_vapid_public_key,omitempty"`
	WebPushVAPIDPrivateKey   string `json:"webpush_vapid_private_key,omitempty"`
	WebPushVAPIDSubject      string `json:"webpush_vapid_subject,omitempty"`
	WebPushSubscriptionsPath string `json:"webpush_subscriptions_path,omitempty"`
}

// PeerFileConfig is the on-disk shape of one entry in FileConfig.Peers.
type PeerFileConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// DefaultFileConfig is the seed written when statusd.json is missing.
//
// LogPath is deliberately left empty: when set, the daemon appends a full
// snapshot to it every tick (500ms) with no rotation, which grows without
// bound (gigabytes within a day). Diagnostics/errors go to stderr (captured
// by launchd/systemd), so the snapshot log is opt-in via --log-path or an
// explicit log_path in the config.
func DefaultFileConfig() FileConfig {
	t := true
	usage := true
	cols, rows := DefaultPaneCols, DefaultPaneRows
	return FileConfig{
		WebAddr:     DefaultWebAddr,
		Interval:    DefaultFileInterval.String(),
		SocketPath:  DefaultSocketPath,
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
	if cfg.UsageInterval != "" {
		if _, err := time.ParseDuration(cfg.UsageInterval); err != nil {
			return FileConfig{}, fmt.Errorf("parse %s: invalid usage_interval %q: %w", path, cfg.UsageInterval, err)
		}
	}
	if cfg.SpendInterval != "" {
		if _, err := time.ParseDuration(cfg.SpendInterval); err != nil {
			return FileConfig{}, fmt.Errorf("parse %s: invalid spend_interval %q: %w", path, cfg.SpendInterval, err)
		}
	}
	return cfg, nil
}

// UsageIntervalDuration returns the parsed quota refresh interval, or zero if unset.
func (c FileConfig) UsageIntervalDuration() time.Duration {
	d, err := time.ParseDuration(c.UsageInterval)
	if c.UsageInterval == "" || err != nil {
		return 0
	}
	return d
}

// SpendIntervalDuration returns the parsed token-spend refresh interval, or zero if unset.
func (c FileConfig) SpendIntervalDuration() time.Duration {
	d, err := time.ParseDuration(c.SpendInterval)
	if c.SpendInterval == "" || err != nil {
		return 0
	}
	return d
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
