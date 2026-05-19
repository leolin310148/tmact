package agents

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/tmux"
)

type BroadcastOptions struct {
	Agent    string
	Role     string
	All      bool
	Text     string
	Enter    bool
	Execute  bool
	OnlyIdle bool
}

type BroadcastReport struct {
	Timestamp string            `json:"ts"`
	DryRun    bool              `json:"dry_run"`
	Results   []BroadcastResult `json:"results"`
}

type BroadcastResult struct {
	Agent  string `json:"agent"`
	Target string `json:"target"`
	State  string `json:"state,omitempty"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
}

func Broadcast(cfg Config, options BroadcastOptions) (BroadcastReport, error) {
	if err := validateBroadcastOptions(options); err != nil {
		return BroadcastReport{}, err
	}

	selected, err := selectBroadcastAgents(cfg, options)
	if err != nil {
		return BroadcastReport{}, err
	}

	report := BroadcastReport{
		Timestamp: time.Now().Format(time.RFC3339),
		DryRun:    !options.Execute,
		Results:   make([]BroadcastResult, 0, len(selected)),
	}
	for _, agent := range selected {
		report.Results = append(report.Results, broadcastToAgent(agent, options))
	}
	return report, nil
}

func validateBroadcastOptions(options BroadcastOptions) error {
	if strings.TrimSpace(options.Text) == "" {
		return errors.New("--text is required")
	}

	selectors := 0
	if options.Agent != "" {
		selectors++
	}
	if options.Role != "" {
		selectors++
	}
	if options.All {
		selectors++
	}
	if selectors == 0 {
		return errors.New("select one of --agent, --role, or --all")
	}
	if selectors > 1 {
		return errors.New("--agent, --role, and --all are mutually exclusive")
	}
	return nil
}

func selectBroadcastAgents(cfg Config, options BroadcastOptions) ([]AgentConfig, error) {
	var selected []AgentConfig
	for _, agent := range cfg.Agents {
		switch {
		case options.All:
			selected = append(selected, agent)
		case options.Agent != "" && agent.Name == options.Agent:
			selected = append(selected, agent)
		case options.Role != "" && agent.Role == options.Role:
			selected = append(selected, agent)
		}
	}
	if len(selected) == 0 {
		switch {
		case options.Agent != "":
			return nil, fmt.Errorf("agent %q not found", options.Agent)
		case options.Role != "":
			return nil, fmt.Errorf("no agents found for role %q", options.Role)
		default:
			return nil, errors.New("no agents selected")
		}
	}
	return selected, nil
}

func broadcastToAgent(agent AgentConfig, options BroadcastOptions) BroadcastResult {
	result := BroadcastResult{
		Agent:  agent.Name,
		Target: agent.Target,
	}

	if options.OnlyIdle {
		raw, err := tmux.CapturePane(agent.Target, agent.CaptureLines)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "capture_failed"
			result.Error = err.Error()
			return result
		}
		state, _ := ClassifyPane(raw)
		result.State = state
		if state != StateIdle {
			result.Status = "skipped"
			result.Reason = "not_idle"
			return result
		}
	}

	if !options.Execute {
		result.Status = "dry_run"
		return result
	}

	if err := tmux.PasteText(agent.Target, options.Text, options.Enter); err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		return result
	}
	result.Status = "sent"
	return result
}
