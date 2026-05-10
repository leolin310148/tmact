package workflow

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
	Target                 string        `yaml:"target"`
	Repo                   string        `yaml:"repo"`
	CaptureLines           int           `yaml:"capture_lines"`
	IdleIgnorePatterns     []string      `yaml:"idle_ignore_patterns"`
	PollInterval           Duration      `yaml:"poll_interval"`
	IdleAfter              Duration      `yaml:"idle_after"`
	StageEvery             Duration      `yaml:"stage_every"`
	CycleEvery             Duration      `yaml:"cycle_every"`
	MaxRuntime             Duration      `yaml:"max_runtime"`
	MaxCycles              int           `yaml:"max_cycles"`
	LogPath                string        `yaml:"log_path"`
	StopOnPermissionPrompt bool          `yaml:"stop_on_permission_prompt"`
	Stages                 []StageConfig `yaml:"stages"`
}

type StageConfig struct {
	Name         string             `yaml:"name"`
	Target       string             `yaml:"target"`
	Prompt       string             `yaml:"prompt"`
	CompleteWhen CompleteWhenConfig `yaml:"complete_when"`
	PostDelay    Duration           `yaml:"post_delay"`
	Repeat       int                `yaml:"repeat"`
}

type CompleteWhenConfig struct {
	Idle                bool     `yaml:"idle"`
	RecentOutputMatches []string `yaml:"recent_output_matches"`
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
	if cfg.PollInterval.Duration == 0 {
		cfg.PollInterval.Duration = 20 * time.Second
	}
	if cfg.IdleAfter.Duration == 0 {
		cfg.IdleAfter.Duration = 2 * time.Minute
	}
	for i := range cfg.Stages {
		if cfg.Stages[i].Name == "" {
			cfg.Stages[i].Name = fmt.Sprintf("stage-%d", i+1)
		}
		if cfg.Stages[i].Repeat == 0 {
			cfg.Stages[i].Repeat = 1
		}
	}
}

func validateConfig(cfg Config) error {
	if len(cfg.Stages) == 0 {
		return errors.New("at least one stage is required")
	}
	if cfg.Target == "" {
		for _, stage := range cfg.Stages {
			if stage.Target == "" {
				return errors.New("target is required when a stage does not set target")
			}
		}
	}
	if cfg.PollInterval.Duration < 0 {
		return errors.New("poll_interval cannot be negative")
	}
	if cfg.IdleAfter.Duration < 0 {
		return errors.New("idle_after cannot be negative")
	}
	if cfg.CycleEvery.Duration < 0 {
		return errors.New("cycle_every cannot be negative")
	}
	if cfg.StageEvery.Duration < 0 {
		return errors.New("stage_every cannot be negative")
	}
	if cfg.MaxRuntime.Duration < 0 {
		return errors.New("max_runtime cannot be negative")
	}
	if cfg.MaxCycles < 0 {
		return errors.New("max_cycles cannot be negative")
	}
	for _, pattern := range cfg.IdleIgnorePatterns {
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("invalid idle_ignore_patterns entry %q: %w", pattern, err)
		}
	}
	for _, stage := range cfg.Stages {
		if err := validateStage(stage); err != nil {
			return err
		}
	}
	return nil
}

func validateStage(stage StageConfig) error {
	context := fmt.Sprintf("stage %q", stage.Name)
	if stage.Prompt == "" {
		return fmt.Errorf("%s: prompt is required", context)
	}
	if stage.PostDelay.Duration < 0 {
		return fmt.Errorf("%s: post_delay cannot be negative", context)
	}
	if stage.Repeat < 0 {
		return fmt.Errorf("%s: repeat cannot be negative", context)
	}
	if !stage.CompleteWhen.Idle && len(stage.CompleteWhen.RecentOutputMatches) == 0 {
		return fmt.Errorf("%s: complete_when is required", context)
	}
	for _, pattern := range stage.CompleteWhen.RecentOutputMatches {
		if pattern == "" {
			return fmt.Errorf("%s: recent_output_matches cannot contain empty entries", context)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("%s: invalid recent_output_matches entry %q: %w", context, pattern, err)
		}
	}
	return nil
}
