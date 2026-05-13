package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tmact/internal/agents"

	"gopkg.in/yaml.v3"
)

var DefaultRoleOrder = []string{"pm", "swe", "qa", "reviewer"}

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
	Change       string            `yaml:"change"`
	AgentsConfig string            `yaml:"agents_config"`
	Roles        map[string]string `yaml:"roles"`
	Discussion   DiscussionConfig  `yaml:"discussion"`
	LogPath      string            `yaml:"log_path"`
}

type DiscussionConfig struct {
	RoleOrder             []string `yaml:"role_order"`
	MaxTurns              int      `yaml:"max_turns"`
	MaxRuntime            Duration `yaml:"max_runtime"`
	PollInterval          Duration `yaml:"poll_interval"`
	IdleAfter             Duration `yaml:"idle_after"`
	CaptureLines          int      `yaml:"capture_lines"`
	CreateMissingProposal bool     `yaml:"create_missing_proposal"`
}

type RoleBinding struct {
	Role  string
	Agent agents.AgentConfig
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
	if len(cfg.Discussion.RoleOrder) == 0 {
		cfg.Discussion.RoleOrder = append([]string(nil), DefaultRoleOrder...)
	}
	if cfg.Discussion.MaxTurns <= 0 {
		cfg.Discussion.MaxTurns = 24
	}
	if cfg.Discussion.MaxRuntime.Duration <= 0 {
		cfg.Discussion.MaxRuntime.Duration = 8 * time.Hour
	}
	if cfg.Discussion.PollInterval.Duration <= 0 {
		cfg.Discussion.PollInterval.Duration = 15 * time.Second
	}
	if cfg.Discussion.IdleAfter.Duration <= 0 {
		cfg.Discussion.IdleAfter.Duration = 30 * time.Second
	}
	if cfg.Discussion.CaptureLines <= 0 {
		cfg.Discussion.CaptureLines = 180
	}
	if cfg.LogPath == "" && cfg.Change != "" {
		cfg.LogPath = filepath.Join(".tmact", "workflow-"+cfg.Change+".jsonl")
	}
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.Change) == "" {
		return errors.New("change is required")
	}
	if _, err := ChangeDir(cfg.Change); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.AgentsConfig) == "" {
		return errors.New("agents_config is required")
	}
	if len(cfg.Roles) == 0 {
		return errors.New("roles are required")
	}
	seen := map[string]bool{}
	for _, role := range cfg.Discussion.RoleOrder {
		if role == "" {
			return errors.New("discussion.role_order cannot contain empty roles")
		}
		if seen[role] {
			return fmt.Errorf("discussion.role_order contains duplicate role %q", role)
		}
		seen[role] = true
		if strings.TrimSpace(cfg.Roles[role]) == "" {
			return fmt.Errorf("roles.%s is required", role)
		}
	}
	for _, role := range DefaultRoleOrder {
		if strings.TrimSpace(cfg.Roles[role]) == "" {
			return fmt.Errorf("roles.%s is required", role)
		}
	}
	if cfg.Discussion.MaxTurns <= 0 {
		return errors.New("discussion.max_turns must be positive")
	}
	if cfg.Discussion.MaxRuntime.Duration <= 0 {
		return errors.New("discussion.max_runtime must be positive")
	}
	if cfg.Discussion.PollInterval.Duration <= 0 {
		return errors.New("discussion.poll_interval must be positive")
	}
	if cfg.Discussion.IdleAfter.Duration <= 0 {
		return errors.New("discussion.idle_after must be positive")
	}
	if cfg.Discussion.CaptureLines <= 0 {
		return errors.New("discussion.capture_lines must be positive")
	}
	return nil
}

func ChangeDir(change string) (string, error) {
	if filepath.IsAbs(change) {
		return "", fmt.Errorf("change %q must be relative to openspec/changes", change)
	}
	clean := filepath.Clean(change)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("change %q escapes openspec/changes", change)
	}
	if strings.Contains(clean, string(filepath.Separator)+".."+string(filepath.Separator)) || strings.HasSuffix(clean, string(filepath.Separator)+"..") {
		return "", fmt.Errorf("change %q escapes openspec/changes", change)
	}
	base := filepath.Join("openspec", "changes")
	dir := filepath.Join(base, clean)
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absBase, absDir)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("change %q escapes openspec/changes", change)
	}
	return dir, nil
}

func ResolveRoles(cfg Config, agentCfg agents.Config) ([]RoleBinding, error) {
	byName := map[string]agents.AgentConfig{}
	for _, agent := range agentCfg.Agents {
		byName[agent.Name] = agent
	}
	bindings := make([]RoleBinding, 0, len(cfg.Discussion.RoleOrder))
	for _, role := range cfg.Discussion.RoleOrder {
		name := cfg.Roles[role]
		agent, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("role %q references unknown agent %q", role, name)
		}
		bindings = append(bindings, RoleBinding{Role: role, Agent: agent})
	}
	return bindings, nil
}

func TargetSummary(bindings []RoleBinding) string {
	parts := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		parts = append(parts, binding.Role+":"+binding.Agent.Target)
	}
	return strings.Join(parts, ",")
}
