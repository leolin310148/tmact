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
		"claude-fable-5",
		"claude-opus-4-8",
		"claude-sonnet-4-6",
		"claude-haiku-4-5",
	},
	panestatus.RuntimeCodex: {
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
