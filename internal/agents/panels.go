package agents

import (
	"fmt"
	"strings"
	"time"

	"tmact/internal/tmux"
)

type PanelOptions struct {
	Session string
	Execute bool
}

type PanelReport struct {
	Timestamp  string           `json:"ts"`
	DryRun     bool             `json:"dry_run"`
	Operations []PanelOperation `json:"operations"`
}

type PanelOperation struct {
	Agent   string   `json:"agent"`
	Action  string   `json:"action"`
	Session string   `json:"session"`
	Window  string   `json:"window"`
	Target  string   `json:"target"`
	Repo    string   `json:"repo,omitempty"`
	Command []string `json:"command,omitempty"`
	Status  string   `json:"status"`
	Error   string   `json:"error,omitempty"`
}

func PlanPanels(cfg Config, opts PanelOptions) (PanelReport, error) {
	layout, err := tmux.ListLayout()
	if err != nil {
		return PanelReport{}, err
	}
	report, err := buildPanelReport(cfg, opts, layout)
	if err != nil {
		return PanelReport{}, err
	}
	return report, nil
}

func EnsurePanels(cfg Config, opts PanelOptions) (PanelReport, error) {
	layout, err := tmux.ListLayout()
	if err != nil {
		return PanelReport{}, err
	}
	report, err := buildPanelReport(cfg, opts, layout)
	if err != nil {
		return PanelReport{}, err
	}
	if !opts.Execute {
		return report, nil
	}

	report.DryRun = false
	for i := range report.Operations {
		op := &report.Operations[i]
		switch op.Action {
		case "exists":
			op.Status = "ok"
		case "new_session":
			setPanelStatus(op, tmux.NewSession(op.Session, op.Window, op.Repo, op.Command))
		case "new_window":
			setPanelStatus(op, tmux.NewWindow(op.Session, op.Window, op.Repo, op.Command))
		default:
			op.Status = "error"
			op.Error = "unknown panel action"
		}
	}
	return report, nil
}

func buildPanelReport(cfg Config, opts PanelOptions, layout tmux.Layout) (PanelReport, error) {
	report := PanelReport{
		Timestamp:  time.Now().Format(time.RFC3339),
		DryRun:     !opts.Execute,
		Operations: make([]PanelOperation, 0, len(cfg.Agents)),
	}
	for _, agent := range cfg.Agents {
		session, window, err := desiredPanel(agent, opts.Session)
		if err != nil {
			return PanelReport{}, err
		}
		command, err := launchCommand(agent)
		if err != nil {
			return PanelReport{}, fmt.Errorf("agent %q: %w", agent.Name, err)
		}

		action := "exists"
		switch {
		case !layout.Sessions[session]:
			action = "new_session"
			layout.Sessions[session] = true
			if layout.Windows[session] == nil {
				layout.Windows[session] = map[string]bool{}
			}
			layout.Windows[session][window] = true
		case !layout.Windows[session][window]:
			action = "new_window"
			if layout.Windows[session] == nil {
				layout.Windows[session] = map[string]bool{}
			}
			layout.Windows[session][window] = true
		}

		report.Operations = append(report.Operations, PanelOperation{
			Agent:   agent.Name,
			Action:  action,
			Session: session,
			Window:  window,
			Target:  session + ":" + window + ".0",
			Repo:    agent.Repo,
			Command: command,
			Status:  "planned",
		})
	}
	return report, nil
}

func desiredPanel(agent AgentConfig, overrideSession string) (string, string, error) {
	session := overrideSession
	if session == "" {
		session = agent.Session
	}
	if session == "" {
		session = targetSession(agent.Target)
	}
	if session == "" {
		return "", "", fmt.Errorf("agent %q: session is required for panel management", agent.Name)
	}

	window := agent.Window
	if window == "" {
		window = agent.Name
	}
	return session, window, nil
}

func targetSession(target string) string {
	if idx := strings.Index(target, ":"); idx >= 0 {
		return target[:idx]
	}
	return target
}

func launchCommand(agent AgentConfig) ([]string, error) {
	launcher := agentLauncher(agent)
	if launcher == "" {
		return nil, nil
	}
	switch launcher {
	case "codex", "claude", "gemini":
		return []string{launcher}, nil
	case "copilot":
		command := []string{"copilot"}
		if agent.AllowAllTools {
			command = append(command, "--allow-all-tools")
		}
		return command, nil
	default:
		return nil, fmt.Errorf("unsupported launcher %q", launcher)
	}
}

func setPanelStatus(op *PanelOperation, err error) {
	if err != nil {
		op.Status = "error"
		op.Error = err.Error()
		return
	}
	op.Status = "ok"
}
