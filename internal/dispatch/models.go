package dispatch

import (
	"fmt"
	"slices"
	"strings"

	"github.com/leolin310148/tmact/internal/panestatus"
)

// supportedModels is an explicit allowlist of model names accepted by each
// launcher. Keep this list intentionally small and user-facing: adding a newly
// released model requires a tmact update instead of silently accepting a typo.
var supportedModels = map[string][]string{
	panestatus.RuntimeClaude: {
		"fable",
		"opus",
		"sonnet",
		"haiku",
		"claude-fable-5-1",
		"claude-fable-5",
		"claude-opus-5-5",
		"claude-sonnet-5-5",
		"claude-haiku-5-5",
		"claude-opus-5",
		"claude-opus-4-8",
		"claude-sonnet-4-6",
		"claude-haiku-4-5",
	},
	panestatus.RuntimeCodex: {
		"gpt-6.1-sol",
		"gpt-6-astra",
		"gpt-6-sol",
		"gpt-6-luna",
		"gpt-5.6-sol",
		"gpt-5.6-terra",
		"gpt-5.6-luna",
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.3-codex-spark",
	},
}

// SupportedModels lists the model names dispatch-work accepts for an agent.
// The returned slice is a copy and is safe for callers to modify.
func SupportedModels(agent string) []string {
	return slices.Clone(supportedModels[agent])
}

// ValidateModel trims and validates a requested model against the agent's
// explicit allowlist. An empty model means to use the launcher's default.
func ValidateModel(agent, model string) (string, error) {
	if model == "" {
		return "", nil
	}

	model = strings.TrimSpace(model)
	if model == "" {
		return "", fmt.Errorf("model cannot be blank")
	}

	models := supportedModels[agent]
	if len(models) == 0 {
		return "", fmt.Errorf("model selection only supports claude or codex, got %q", agent)
	}
	if !slices.Contains(models, model) {
		return "", fmt.Errorf("unsupported model %q for %s; want one of %s", model, agent, strings.Join(models, ", "))
	}
	return model, nil
}

// ValidateEffort validates launcher-specific effort overrides. Automatic
// selection uses each catalog model's complete supported effort set.
func ValidateEffort(agent, effort string) error {
	if effort == "" {
		return nil
	}
	levels := []string{"low", "medium", "high", "xhigh", "max"}
	if agent == "codex" {
		levels = append(levels, "ultra")
	}
	if agent != "claude" && agent != "codex" || !slices.Contains(levels, effort) {
		return fmt.Errorf("unsupported effort %q for %s", effort, agent)
	}
	return nil
}

// ValidateModelEffort checks known per-model limitations after launcher-level
// validation. Luna supports up to max, while Sol/Astra also support ultra.
func ValidateModelEffort(agent, model, effort string) error {
	if err := ValidateEffort(agent, effort); err != nil {
		return err
	}
	if agent == "codex" && strings.Contains(model, "luna") && effort == "ultra" {
		return fmt.Errorf("model %s does not support ultra effort", model)
	}
	return nil
}
