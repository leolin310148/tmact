package watch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	Target       string       `yaml:"target"`
	CaptureLines int          `yaml:"capture_lines"`
	PollInterval Duration     `yaml:"poll_interval"`
	MaxRuntime   Duration     `yaml:"max_runtime"`
	LogPath      string       `yaml:"log_path"`
	Rules        []RuleConfig `yaml:"rules"`
}

type RuleConfig struct {
	Name              string   `yaml:"name"`
	Type              string   `yaml:"type"`
	AllowPaths        []string `yaml:"allow_paths"`
	AllowPathPatterns []string `yaml:"allow_path_patterns"`
	AcceptOption      string   `yaml:"accept_option"`
	Cooldown          Duration `yaml:"cooldown"`
	MaxRuns           int      `yaml:"max_runs"`
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
		cfg.CaptureLines = 160
	}
	if cfg.PollInterval.Duration <= 0 {
		cfg.PollInterval.Duration = 5 * time.Second
	}
	for i := range cfg.Rules {
		if cfg.Rules[i].Name == "" {
			cfg.Rules[i].Name = fmt.Sprintf("rule-%d", i+1)
		}
		if cfg.Rules[i].AcceptOption == "" {
			cfg.Rules[i].AcceptOption = "selected"
		}
		if cfg.Rules[i].Cooldown.Duration <= 0 {
			cfg.Rules[i].Cooldown.Duration = 30 * time.Second
		}
	}
}

func validateConfig(cfg Config) error {
	if cfg.Target == "" {
		return errors.New("target is required")
	}
	if len(cfg.Rules) == 0 {
		return errors.New("at least one rule is required")
	}
	for _, rule := range cfg.Rules {
		if rule.Type != "directory_access_prompt" {
			return fmt.Errorf("rule %q: unsupported type %q", rule.Name, rule.Type)
		}
		if len(rule.AllowPaths) == 0 && len(rule.AllowPathPatterns) == 0 {
			return fmt.Errorf("rule %q: at least one allow_paths or allow_path_patterns entry is required", rule.Name)
		}
		if rule.AcceptOption != "selected" {
			return fmt.Errorf("rule %q: unsupported accept_option %q", rule.Name, rule.AcceptOption)
		}
		if rule.Cooldown.Duration < 0 {
			return fmt.Errorf("rule %q: cooldown cannot be negative", rule.Name)
		}
		if rule.MaxRuns < 0 {
			return fmt.Errorf("rule %q: max_runs cannot be negative", rule.Name)
		}
		for _, allowed := range rule.AllowPaths {
			if allowed == "" {
				return fmt.Errorf("rule %q: allow_paths cannot contain empty entries", rule.Name)
			}
		}
		for _, pattern := range rule.AllowPathPatterns {
			if pattern == "" {
				return fmt.Errorf("rule %q: allow_path_patterns cannot contain empty entries", rule.Name)
			}
			if err := validatePathPattern(pattern); err != nil {
				return fmt.Errorf("rule %q: invalid allow_path_patterns entry %q: %w", rule.Name, pattern, err)
			}
		}
	}
	return nil
}

func validatePathPattern(pattern string) error {
	normalized, err := normalizePath(pattern)
	if err != nil {
		return err
	}
	_, err = filepath.Match(normalized, normalized)
	return err
}
