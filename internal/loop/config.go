package loop

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

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
	Actions                []ActionConfig `yaml:"actions"`
}

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
}

func validateConfig(cfg Config) error {
	if cfg.Target == "" {
		return errors.New("target is required")
	}
	if len(cfg.Actions) == 0 {
		return errors.New("at least one action is required")
	}

	for _, action := range cfg.Actions {
		switch action.Type {
		case "send_text":
			if action.Text == "" {
				return fmt.Errorf("action %q: text is required", action.Name)
			}
		case "send_keys":
			if len(action.Keys) == 0 {
				return fmt.Errorf("action %q: keys are required", action.Name)
			}
		case "clear":
		default:
			return fmt.Errorf("action %q: unsupported type %q", action.Name, action.Type)
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
	}
	for _, pattern := range cfg.IdleIgnorePatterns {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("invalid idle_ignore_patterns entry %q: %w", pattern, err)
		}
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
