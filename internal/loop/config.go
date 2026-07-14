package loop

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/statusd"
	"gopkg.in/yaml.v3"
)

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == 0 || value.Value == "" {
		d.Duration = 0
		return nil
	}

	parsed, err := time.ParseDuration(value.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", value.Value, err)
	}
	d.Duration = parsed
	return nil
}

type Config struct {
	Target                 string         `yaml:"target"`
	Peer                   string         `yaml:"peer"`
	StatusdConfig          string         `yaml:"statusd_config"`
	CaptureLines           int            `yaml:"capture_lines"`
	IdleIgnorePatterns     []string       `yaml:"idle_ignore_patterns"`
	PollInterval           Duration       `yaml:"poll_interval"`
	IdleAfter              Duration       `yaml:"idle_after"`
	AssumeIdleOnStart      bool           `yaml:"assume_idle_on_start"`
	MaxRuntime             Duration       `yaml:"max_runtime"`
	MaxActions             int            `yaml:"max_actions"`
	LogPath                string         `yaml:"log_path"`
	LogSkippedActions      bool           `yaml:"log_skipped_actions"`
	StopOnPermissionPrompt bool           `yaml:"stop_on_permission_prompt"`
	Quota                  *QuotaConfig   `yaml:"quota"`
	Actions                []ActionConfig `yaml:"actions"`
	Flows                  []FlowConfig   `yaml:"flows"`
}

// QuotaConfig makes the loop skip its scheduled actions/flows when the target
// agent's provider usage is too high, so an unattended loop never burns through
// a weekly limit or exhausts the hourly/session window. Absent (or enabled:
// false) means quota is never consulted and the loop behaves exactly as before.
type QuotaConfig struct {
	Enabled bool `yaml:"enabled"`
	// Provider selects which agent's quota to read: "codex" or "claude". It must
	// be set when enabled (there is no meaningful default — a loop drives one
	// agent, so the operator names it explicitly).
	Provider string `yaml:"provider"`
	// WeeklySkipAtPercent skips the cycle when the weekly window's used-percent
	// is >= this. Default 100 (skip only once the weekly limit is fully reached).
	WeeklySkipAtPercent float64 `yaml:"weekly_skip_at_percent"`
	// WeeklyRequireHeadroom runs a cycle only while weekly usage is below its
	// linear expected pace (expected percent - actual percent > 0). This uses
	// the weekly window's reset time and duration, so it adapts throughout the
	// week instead of relying on one fixed usage threshold. Default false.
	WeeklyRequireHeadroom bool `yaml:"weekly_require_headroom"`
	// SessionMinRemainingPercent skips the cycle when the session (hourly/5h)
	// window does not have strictly more than this percent remaining
	// (used-percent >= 100-this). Default 20.
	SessionMinRemainingPercent float64 `yaml:"session_min_remaining_percent"`
	// SessionGateEnabled controls whether the session window is required and
	// checked. It defaults to true; set it explicitly to false for providers or
	// loops that intentionally use only the weekly gates.
	SessionGateEnabled *bool `yaml:"session_gate_enabled"`
	// RefreshInterval bounds how often the rate-limited provider endpoint is
	// queried; the last snapshot is reused between refreshes. Default 5m.
	RefreshInterval Duration `yaml:"refresh_interval"`
	// FailClosed inverts the default fail-open behavior: when quota can't be
	// determined (missing/expired token, provider error, stale reading) the loop
	// skips instead of running. Default false — a broken token never silently
	// freezes the loop.
	FailClosed bool `yaml:"fail_closed"`
}

const (
	defaultWeeklySkipAtPercent        = 100
	defaultSessionMinRemainingPercent = 20
	defaultQuotaRefreshInterval       = 5 * time.Minute
)

type ActionConfig struct {
	Name         string   `yaml:"name"`
	Type         string   `yaml:"type"`
	Text         string   `yaml:"text"`
	Command      string   `yaml:"command"`
	Keys         []string `yaml:"keys"`
	Enter        *bool    `yaml:"enter"`
	Every        Duration `yaml:"every"`
	InitialDelay Duration `yaml:"initial_delay"`
	PostDelay    Duration `yaml:"post_delay"`
	OnlyWhenIdle bool     `yaml:"only_when_idle"`
	MaxRuns      int      `yaml:"max_runs"`
}

type FlowConfig struct {
	Name         string         `yaml:"name"`
	Every        Duration       `yaml:"every"`
	InitialDelay Duration       `yaml:"initial_delay"`
	OnlyWhenIdle bool           `yaml:"only_when_idle"`
	MaxRuns      int            `yaml:"max_runs"`
	Steps        []ActionConfig `yaml:"steps"`
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	applyDefaults(&cfg)
	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Peer != "" {
		peer, rest := statusd.SplitPeerTarget(cfg.Target)
		if peer == "" {
			cfg.Target = cfg.Peer + statusd.PeerSeparator + rest
		}
	}
	if cfg.CaptureLines <= 0 {
		cfg.CaptureLines = 120
	}
	if cfg.PollInterval.Duration <= 0 {
		cfg.PollInterval.Duration = 30 * time.Second
	}
	if cfg.IdleAfter.Duration <= 0 {
		cfg.IdleAfter.Duration = 2 * time.Minute
	}
	for i := range cfg.Actions {
		if cfg.Actions[i].Name == "" {
			cfg.Actions[i].Name = fmt.Sprintf("action-%d", i+1)
		}
	}
	if cfg.Quota != nil && cfg.Quota.Enabled {
		if cfg.Quota.WeeklySkipAtPercent == 0 {
			cfg.Quota.WeeklySkipAtPercent = defaultWeeklySkipAtPercent
		}
		if cfg.Quota.SessionMinRemainingPercent == 0 {
			cfg.Quota.SessionMinRemainingPercent = defaultSessionMinRemainingPercent
		}
		if cfg.Quota.RefreshInterval.Duration <= 0 {
			cfg.Quota.RefreshInterval.Duration = defaultQuotaRefreshInterval
		}
	}
	for i := range cfg.Flows {
		if cfg.Flows[i].Name == "" {
			cfg.Flows[i].Name = fmt.Sprintf("flow-%d", i+1)
		}
		for j := range cfg.Flows[i].Steps {
			if cfg.Flows[i].Steps[j].Name == "" {
				cfg.Flows[i].Steps[j].Name = fmt.Sprintf("step-%d", j+1)
			}
		}
	}
}

func validateConfig(cfg Config) error {
	if cfg.Target == "" {
		return errors.New("target is required")
	}
	if cfg.Peer != "" {
		peer, _ := statusd.SplitPeerTarget(cfg.Target)
		if peer != "" && peer != cfg.Peer {
			return fmt.Errorf("peer %q conflicts with target peer %q", cfg.Peer, peer)
		}
	}
	if len(cfg.Actions) == 0 && len(cfg.Flows) == 0 {
		return errors.New("at least one action or flow is required")
	}

	for _, action := range cfg.Actions {
		if err := validateAction(action); err != nil {
			return err
		}
	}
	for _, flow := range cfg.Flows {
		if len(flow.Steps) == 0 {
			return fmt.Errorf("flow %q: at least one step is required", flow.Name)
		}
		if flow.Every.Duration < 0 {
			return fmt.Errorf("flow %q: every cannot be negative", flow.Name)
		}
		if flow.InitialDelay.Duration < 0 {
			return fmt.Errorf("flow %q: initial_delay cannot be negative", flow.Name)
		}
		if flow.MaxRuns < 0 {
			return fmt.Errorf("flow %q: max_runs cannot be negative", flow.Name)
		}
		for _, step := range flow.Steps {
			if err := validateFlowStep(flow.Name, step); err != nil {
				return err
			}
		}
	}
	for _, pattern := range cfg.IdleIgnorePatterns {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("invalid idle_ignore_patterns entry %q: %w", pattern, err)
		}
	}
	if err := validateQuota(cfg.Quota); err != nil {
		return err
	}

	return nil
}

func validateQuota(q *QuotaConfig) error {
	if q == nil || !q.Enabled {
		return nil
	}
	if q.Provider == "" {
		return errors.New("quota: provider is required when quota.enabled")
	}
	known := false
	for _, name := range agentusage.Providers() {
		if name == q.Provider {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("quota: unknown provider %q (want one of %v)", q.Provider, agentusage.Providers())
	}
	if q.WeeklySkipAtPercent < 0 || q.WeeklySkipAtPercent > 100 {
		return fmt.Errorf("quota: weekly_skip_at_percent must be between 0 and 100, got %v", q.WeeklySkipAtPercent)
	}
	if q.SessionMinRemainingPercent < 0 || q.SessionMinRemainingPercent > 100 {
		return fmt.Errorf("quota: session_min_remaining_percent must be between 0 and 100, got %v", q.SessionMinRemainingPercent)
	}
	if q.RefreshInterval.Duration < 0 {
		return errors.New("quota: refresh_interval cannot be negative")
	}
	return nil
}

func validateAction(action ActionConfig) error {
	if err := validateActionBody(action, fmt.Sprintf("action %q", action.Name)); err != nil {
		return err
	}
	if action.Every.Duration < 0 {
		return fmt.Errorf("action %q: every cannot be negative", action.Name)
	}
	if action.InitialDelay.Duration < 0 {
		return fmt.Errorf("action %q: initial_delay cannot be negative", action.Name)
	}
	if action.PostDelay.Duration < 0 {
		return fmt.Errorf("action %q: post_delay cannot be negative", action.Name)
	}
	if action.MaxRuns < 0 {
		return fmt.Errorf("action %q: max_runs cannot be negative", action.Name)
	}
	return nil
}

func validateFlowStep(flowName string, step ActionConfig) error {
	context := fmt.Sprintf("flow %q step %q", flowName, step.Name)
	if err := validateActionBody(step, context); err != nil {
		return err
	}
	if step.Every.Duration != 0 {
		return fmt.Errorf("%s: every is only valid on the flow", context)
	}
	if step.InitialDelay.Duration != 0 {
		return fmt.Errorf("%s: initial_delay is only valid on the flow", context)
	}
	if step.OnlyWhenIdle {
		return fmt.Errorf("%s: only_when_idle is only valid on the flow", context)
	}
	if step.MaxRuns != 0 {
		return fmt.Errorf("%s: max_runs is only valid on the flow", context)
	}
	if step.PostDelay.Duration < 0 {
		return fmt.Errorf("%s: post_delay cannot be negative", context)
	}
	return nil
}

func validateActionBody(action ActionConfig, context string) error {
	switch action.Type {
	case "send_text":
		if action.Text == "" {
			return fmt.Errorf("%s: text is required", context)
		}
	case "send_keys":
		if len(action.Keys) == 0 {
			return fmt.Errorf("%s: keys are required", context)
		}
	case "clear":
	default:
		return fmt.Errorf("%s: unsupported type %q", context, action.Type)
	}
	return nil
}

func actionEnter(action ActionConfig) bool {
	if action.Enter != nil {
		return *action.Enter
	}
	switch action.Type {
	case "send_text", "clear":
		return true
	default:
		return false
	}
}
