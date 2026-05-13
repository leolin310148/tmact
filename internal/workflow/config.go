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
var DefaultImplementationStageOrder = []string{"swe_apply", "qa_verify", "pm_archive"}

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
	Change         string               `yaml:"change"`
	AgentsConfig   string               `yaml:"agents_config"`
	Roles          map[string]string    `yaml:"roles"`
	PromptDispatch PromptDispatchConfig `yaml:"prompt_dispatch"`
	Discussion     DiscussionConfig     `yaml:"discussion"`
	Implementation ImplementationConfig `yaml:"implementation"`
	LogPath        string               `yaml:"log_path"`
}

type PromptDispatchConfig struct {
	ClearBeforePrompt    *bool    `yaml:"clear_before_prompt"`
	ClearCommand         string   `yaml:"clear_command"`
	ClearDelay           Duration `yaml:"clear_delay"`
	LegacyMarkerFallback *bool    `yaml:"legacy_marker_fallback"`
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

type ImplementationConfig struct {
	StageOrder               []string        `yaml:"stage_order"`
	MaxTurns                 int             `yaml:"max_turns"`
	MaxRuntime               Duration        `yaml:"max_runtime"`
	PollInterval             Duration        `yaml:"poll_interval"`
	IdleAfter                Duration        `yaml:"idle_after"`
	CaptureLines             int             `yaml:"capture_lines"`
	RequirePhase1Agreed      *bool           `yaml:"require_phase1_agreed"`
	AllowDryRunWithoutPhase1 bool            `yaml:"allow_dry_run_without_phase1"`
	ApplyInstructions        CommandConfig   `yaml:"apply_instructions"`
	VerifyCommands           []CommandConfig `yaml:"verify_commands"`
	ArchiveCommand           CommandConfig   `yaml:"archive_command"`
}

type CommandConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
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

func LoadImplementationConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	applyImplementationDefaults(&cfg)
	if err := validateImplementationConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	applyPromptDispatchDefaults(&cfg.PromptDispatch)
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

func applyImplementationDefaults(cfg *Config) {
	applyPromptDispatchDefaults(&cfg.PromptDispatch)
	if len(cfg.Implementation.StageOrder) == 0 {
		cfg.Implementation.StageOrder = append([]string(nil), DefaultImplementationStageOrder...)
	}
	if cfg.Implementation.MaxTurns <= 0 {
		cfg.Implementation.MaxTurns = 12
	}
	if cfg.Implementation.MaxRuntime.Duration <= 0 {
		cfg.Implementation.MaxRuntime.Duration = 8 * time.Hour
	}
	if cfg.Implementation.PollInterval.Duration <= 0 {
		cfg.Implementation.PollInterval.Duration = 15 * time.Second
	}
	if cfg.Implementation.IdleAfter.Duration <= 0 {
		cfg.Implementation.IdleAfter.Duration = 30 * time.Second
	}
	if cfg.Implementation.CaptureLines <= 0 {
		cfg.Implementation.CaptureLines = 180
	}
	if cfg.Implementation.RequirePhase1Agreed == nil {
		require := true
		cfg.Implementation.RequirePhase1Agreed = &require
	}
	if cfg.Implementation.ApplyInstructions.Command == "" && len(cfg.Implementation.ApplyInstructions.Args) == 0 {
		cfg.Implementation.ApplyInstructions = CommandConfig{Command: "openspec", Args: []string{"instructions", "apply", "--change", "{{change}}"}}
	}
	if len(cfg.Implementation.VerifyCommands) == 0 {
		cfg.Implementation.VerifyCommands = []CommandConfig{
			{Command: "openspec", Args: []string{"validate", "{{change}}", "--strict"}},
			{Command: "go", Args: []string{"test", "./..."}},
		}
	}
	if cfg.Implementation.ArchiveCommand.Command == "" && len(cfg.Implementation.ArchiveCommand.Args) == 0 {
		cfg.Implementation.ArchiveCommand = CommandConfig{Command: "openspec", Args: []string{"archive", "{{change}}", "--yes"}}
	}
	if cfg.LogPath == "" && cfg.Change != "" {
		cfg.LogPath = filepath.Join(".tmact", "implementation-"+cfg.Change+".jsonl")
	}
}

func applyPromptDispatchDefaults(cfg *PromptDispatchConfig) {
	if cfg.ClearBeforePrompt == nil {
		value := true
		cfg.ClearBeforePrompt = &value
	}
	if cfg.ClearCommand == "" {
		cfg.ClearCommand = "/clear"
	}
	if cfg.ClearDelay.Duration == 0 {
		cfg.ClearDelay.Duration = 5 * time.Second
	}
	if cfg.LegacyMarkerFallback == nil {
		value := false
		cfg.LegacyMarkerFallback = &value
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
	if err := validatePromptDispatchConfig(cfg.PromptDispatch); err != nil {
		return err
	}
	return nil
}

func validateImplementationConfig(cfg Config) error {
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
	for _, role := range []string{"swe", "qa", "pm"} {
		if strings.TrimSpace(cfg.Roles[role]) == "" {
			return fmt.Errorf("roles.%s is required", role)
		}
	}
	seen := map[string]bool{}
	for _, stage := range cfg.Implementation.StageOrder {
		if _, ok := implementationStageRole(stage); !ok {
			return fmt.Errorf("implementation.stage_order contains unknown stage %q", stage)
		}
		if seen[stage] {
			return fmt.Errorf("implementation.stage_order contains duplicate stage %q", stage)
		}
		seen[stage] = true
	}
	for _, stage := range DefaultImplementationStageOrder {
		if !seen[stage] {
			return fmt.Errorf("implementation.stage_order missing stage %q", stage)
		}
	}
	if cfg.Implementation.MaxTurns <= 0 {
		return errors.New("implementation.max_turns must be positive")
	}
	if cfg.Implementation.MaxRuntime.Duration <= 0 {
		return errors.New("implementation.max_runtime must be positive")
	}
	if cfg.Implementation.PollInterval.Duration <= 0 {
		return errors.New("implementation.poll_interval must be positive")
	}
	if cfg.Implementation.IdleAfter.Duration <= 0 {
		return errors.New("implementation.idle_after must be positive")
	}
	if cfg.Implementation.CaptureLines <= 0 {
		return errors.New("implementation.capture_lines must be positive")
	}
	if err := validateCommand("implementation.apply_instructions", cfg.Implementation.ApplyInstructions); err != nil {
		return err
	}
	if len(cfg.Implementation.VerifyCommands) == 0 {
		return errors.New("implementation.verify_commands are required")
	}
	for i, command := range cfg.Implementation.VerifyCommands {
		if err := validateCommand(fmt.Sprintf("implementation.verify_commands[%d]", i), command); err != nil {
			return err
		}
	}
	if err := validateCommand("implementation.archive_command", cfg.Implementation.ArchiveCommand); err != nil {
		return err
	}
	if err := validatePromptDispatchConfig(cfg.PromptDispatch); err != nil {
		return err
	}
	return nil
}

func validatePromptDispatchConfig(cfg PromptDispatchConfig) error {
	if promptDispatchClearEnabled(cfg) && strings.TrimSpace(cfg.ClearCommand) == "" {
		return errors.New("prompt_dispatch.clear_command is required when clear_before_prompt is enabled")
	}
	if cfg.ClearDelay.Duration < 0 {
		return errors.New("prompt_dispatch.clear_delay cannot be negative")
	}
	return nil
}

func promptDispatchClearEnabled(cfg PromptDispatchConfig) bool {
	return cfg.ClearBeforePrompt == nil || *cfg.ClearBeforePrompt
}

func promptDispatchLegacyMarkerFallback(cfg PromptDispatchConfig) bool {
	return cfg.LegacyMarkerFallback != nil && *cfg.LegacyMarkerFallback
}

func validateCommand(path string, command CommandConfig) error {
	if strings.TrimSpace(command.Command) == "" {
		return fmt.Errorf("%s.command is required", path)
	}
	return nil
}

func implementationStageRole(stage string) (string, bool) {
	switch stage {
	case "swe_apply":
		return "swe", true
	case "qa_verify":
		return "qa", true
	case "pm_archive":
		return "pm", true
	default:
		return "", false
	}
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
