package agents

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

var DefaultConfigPaths = []string{
	"tmact.agents.yaml",
	"agents.yaml",
}

type Config struct {
	CaptureLines int           `yaml:"capture_lines"`
	Agents       []AgentConfig `yaml:"agents"`
}

type AgentConfig struct {
	Name          string `yaml:"name"`
	Target        string `yaml:"target"`
	Session       string `yaml:"session"`
	Window        string `yaml:"window"`
	Repo          string `yaml:"repo"`
	Type          string `yaml:"type"`
	Role          string `yaml:"role"`
	Launcher      string `yaml:"launcher"`
	AllowAllTools bool   `yaml:"allow_all_tools"`
	TrustFolder   bool   `yaml:"trust_folder"`
	CaptureLines  int    `yaml:"capture_lines"`
}

type Filter struct {
	Agent string
	Role  string
}

func ResolveConfigPath(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	for _, candidate := range DefaultConfigPaths {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("agent config not found; pass --config or create one of: %s", joinConfigPaths(DefaultConfigPaths))
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

func FilterConfig(cfg Config, filter Filter) (Config, error) {
	if filter.Agent != "" && filter.Role != "" {
		return Config{}, errors.New("--agent and --role are mutually exclusive")
	}
	if filter.Agent == "" && filter.Role == "" {
		return cfg, nil
	}

	var filtered []AgentConfig
	for _, agent := range cfg.Agents {
		switch {
		case filter.Agent != "" && agent.Name == filter.Agent:
			filtered = append(filtered, agent)
		case filter.Role != "" && agent.Role == filter.Role:
			filtered = append(filtered, agent)
		}
	}
	if len(filtered) == 0 {
		if filter.Agent != "" {
			return Config{}, fmt.Errorf("agent %q not found", filter.Agent)
		}
		return Config{}, fmt.Errorf("no agents found for role %q", filter.Role)
	}
	cfg.Agents = filtered
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.CaptureLines <= 0 {
		cfg.CaptureLines = 120
	}
	for i := range cfg.Agents {
		if cfg.Agents[i].CaptureLines <= 0 {
			cfg.Agents[i].CaptureLines = cfg.CaptureLines
		}
		if cfg.Agents[i].Target == "" && cfg.Agents[i].Session != "" && cfg.Agents[i].Window != "" {
			cfg.Agents[i].Target = cfg.Agents[i].Session + ":" + cfg.Agents[i].Window + ".0"
		}
	}
}

func validateConfig(cfg Config) error {
	if len(cfg.Agents) == 0 {
		return errors.New("at least one agent is required")
	}

	names := map[string]bool{}
	for i, agent := range cfg.Agents {
		context := fmt.Sprintf("agent %d", i+1)
		if agent.Name == "" {
			return fmt.Errorf("%s: name is required", context)
		}
		if names[agent.Name] {
			return fmt.Errorf("%s: duplicate name %q", context, agent.Name)
		}
		names[agent.Name] = true
		if agent.Target == "" && (agent.Session == "" || agent.Window == "") {
			return fmt.Errorf("%s %q: target is required unless session and window are set", context, agent.Name)
		}
		if agent.CaptureLines < 0 {
			return fmt.Errorf("%s %q: capture_lines cannot be negative", context, agent.Name)
		}
		if err := validateLauncher(agent); err != nil {
			return fmt.Errorf("%s %q: %w", context, agent.Name, err)
		}
	}
	return nil
}

func validateLauncher(agent AgentConfig) error {
	launcher := agentLauncher(agent)
	if launcher == "" {
		if agent.AllowAllTools {
			return errors.New("allow_all_tools requires launcher or type")
		}
		if agent.TrustFolder {
			return errors.New("trust_folder requires a claude or codex launcher")
		}
		return nil
	}
	if !isSupportedLauncher(launcher) {
		return fmt.Errorf("unsupported launcher %q", launcher)
	}
	if agent.AllowAllTools && launcher != "copilot" {
		return fmt.Errorf("allow_all_tools is only supported for copilot, not %s", launcher)
	}
	if agent.TrustFolder && launcher != "claude" && launcher != "codex" {
		return fmt.Errorf("trust_folder is only supported for claude or codex, not %s", launcher)
	}
	if agent.TrustFolder && agent.Repo == "" {
		return errors.New("trust_folder requires repo as the exact allowed directory")
	}
	return nil
}

func agentLauncher(agent AgentConfig) string {
	if agent.Launcher != "" {
		return agent.Launcher
	}
	if isSupportedLauncher(agent.Type) {
		return agent.Type
	}
	return ""
}

func isSupportedLauncher(launcher string) bool {
	switch launcher {
	case "codex", "claude", "copilot", "gemini":
		return true
	default:
		return false
	}
}

func joinConfigPaths(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	result := paths[0]
	for _, path := range paths[1:] {
		result += ", " + path
	}
	return result
}
