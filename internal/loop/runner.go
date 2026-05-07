package loop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"tmact/internal/prompt"
	"tmact/internal/tmux"
)

type Options struct {
	DryRun bool
	Once   bool
}

type Runner struct {
	cfg                Config
	options            Options
	now                func() time.Time
	idleIgnorePatterns []*regexp.Regexp
}

type actionState struct {
	config  ActionConfig
	nextRun time.Time
	runs    int
}

type flowState struct {
	config  FlowConfig
	nextRun time.Time
	runs    int
}

type paneState struct {
	Hash             string                  `json:"hash"`
	Idle             bool                    `json:"idle"`
	IdleFor          string                  `json:"idle_for"`
	PermissionPrompt *prompt.DirectoryAccess `json:"permission_prompt,omitempty"`
}

type event struct {
	Timestamp string      `json:"ts"`
	Type      string      `json:"type"`
	Target    string      `json:"target"`
	Action    string      `json:"action,omitempty"`
	DryRun    bool        `json:"dry_run,omitempty"`
	Status    string      `json:"status,omitempty"`
	Reason    string      `json:"reason,omitempty"`
	Details   interface{} `json:"details,omitempty"`
}

func NewRunner(cfg Config, options Options) *Runner {
	compiled := make([]*regexp.Regexp, 0, len(cfg.IdleIgnorePatterns))
	for _, pattern := range cfg.IdleIgnorePatterns {
		compiled = append(compiled, regexp.MustCompile(pattern))
	}
	return &Runner{
		cfg:                cfg,
		options:            options,
		now:                time.Now,
		idleIgnorePatterns: compiled,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	start := r.now()
	lastChangedAt := start
	if r.cfg.AssumeIdleOnStart {
		lastChangedAt = start.Add(-r.cfg.IdleAfter.Duration)
	}

	actions := make([]actionState, 0, len(r.cfg.Actions))
	for _, cfg := range r.cfg.Actions {
		actions = append(actions, actionState{
			config:  cfg,
			nextRun: start.Add(cfg.InitialDelay.Duration),
		})
	}
	flows := make([]flowState, 0, len(r.cfg.Flows))
	for _, cfg := range r.cfg.Flows {
		flows = append(flows, flowState{
			config:  cfg,
			nextRun: start.Add(cfg.InitialDelay.Duration),
		})
	}

	var lastHash string
	actionCount := 0
	ticker := time.NewTicker(r.cfg.PollInterval.Duration)
	defer ticker.Stop()

	for {
		now := r.now()
		if r.cfg.MaxRuntime.Duration > 0 && now.Sub(start) >= r.cfg.MaxRuntime.Duration {
			return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "max_runtime"})
		}

		state, changed, err := r.observe(now, lastHash, lastChangedAt)
		if err != nil {
			_ = r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "error", Target: r.cfg.Target, Status: "failed", Reason: err.Error()})
			return err
		}
		if lastHash == "" {
			lastHash = state.Hash
		} else if changed {
			lastHash = state.Hash
			lastChangedAt = now
			state.Idle = false
			state.IdleFor = "0s"
		}

		if state.PermissionPrompt != nil && r.cfg.StopOnPermissionPrompt {
			return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "permission_prompt", Details: state.PermissionPrompt})
		}

		for i := range actions {
			if r.cfg.MaxActions > 0 && actionCount >= r.cfg.MaxActions {
				return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "max_actions"})
			}

			executed, err := r.maybeRunAction(now, state, &actions[i])
			if err != nil {
				return err
			}
			if executed {
				actionCount++
			}
		}
		for i := range flows {
			if r.cfg.MaxActions > 0 && actionCount >= r.cfg.MaxActions {
				return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.cfg.Target, Reason: "max_actions"})
			}

			executed, err := r.maybeRunFlow(now, state, &flows[i])
			if err != nil {
				return err
			}
			actionCount += executed
		}

		if r.options.Once {
			return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "state", Target: r.cfg.Target, Details: state})
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) observe(now time.Time, previousHash string, lastChangedAt time.Time) (paneState, bool, error) {
	raw, err := tmux.CapturePane(r.cfg.Target, r.cfg.CaptureLines)
	if err != nil {
		return paneState{}, false, err
	}

	hash := hashText(r.idleText(raw))
	idleFor := now.Sub(lastChangedAt)
	state := paneState{
		Hash:             hash,
		Idle:             idleFor >= r.cfg.IdleAfter.Duration,
		IdleFor:          idleFor.Truncate(time.Second).String(),
		PermissionPrompt: prompt.DetectDirectoryAccess(raw),
	}
	return state, previousHash != "" && hash != previousHash, nil
}

func (r *Runner) idleText(raw string) string {
	if len(r.idleIgnorePatterns) == 0 {
		return raw
	}

	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		ignore := false
		for _, pattern := range r.idleIgnorePatterns {
			if pattern.MatchString(line) {
				ignore = true
				break
			}
		}
		if !ignore {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func (r *Runner) maybeRunAction(now time.Time, state paneState, action *actionState) (bool, error) {
	if now.Before(action.nextRun) {
		return false, nil
	}
	if action.config.MaxRuns > 0 && action.runs >= action.config.MaxRuns {
		return false, nil
	}
	if action.config.OnlyWhenIdle && !state.Idle {
		if r.cfg.LogSkippedActions {
			return false, r.emit(event{
				Timestamp: now.Format(time.RFC3339),
				Type:      "skip",
				Target:    r.cfg.Target,
				Action:    action.config.Name,
				Status:    "not_idle",
				Details:   state,
			})
		}
		return false, nil
	}

	if err := r.runAction(now, action.config.Name, action.config); err != nil {
		return false, err
	}

	action.runs++
	if action.config.Every.Duration > 0 {
		action.nextRun = now.Add(action.config.Every.Duration)
	} else {
		action.nextRun = time.Time{}
		action.config.MaxRuns = action.runs
	}
	return true, nil
}

func (r *Runner) maybeRunFlow(now time.Time, state paneState, flow *flowState) (int, error) {
	if now.Before(flow.nextRun) {
		return 0, nil
	}
	if flow.config.MaxRuns > 0 && flow.runs >= flow.config.MaxRuns {
		return 0, nil
	}
	if flow.config.OnlyWhenIdle && !state.Idle {
		if r.cfg.LogSkippedActions {
			return 0, r.emit(event{
				Timestamp: now.Format(time.RFC3339),
				Type:      "skip",
				Target:    r.cfg.Target,
				Action:    flow.config.Name,
				Status:    "not_idle",
				Details:   state,
			})
		}
		return 0, nil
	}

	if err := r.emit(event{
		Timestamp: now.Format(time.RFC3339),
		Type:      "flow",
		Target:    r.cfg.Target,
		Action:    flow.config.Name,
		DryRun:    r.options.DryRun,
		Status:    "start",
		Details: map[string]interface{}{
			"steps": len(flow.config.Steps),
		},
	}); err != nil {
		return 0, err
	}

	executed := 0
	for _, step := range flow.config.Steps {
		stepName := flow.config.Name + "." + step.Name
		if err := r.runAction(now, stepName, step); err != nil {
			return executed, err
		}
		executed++
	}

	if err := r.emit(event{
		Timestamp: now.Format(time.RFC3339),
		Type:      "flow",
		Target:    r.cfg.Target,
		Action:    flow.config.Name,
		DryRun:    r.options.DryRun,
		Status:    "ok",
		Details: map[string]interface{}{
			"steps": executed,
		},
	}); err != nil {
		return executed, err
	}

	flow.runs++
	if flow.config.Every.Duration > 0 {
		flow.nextRun = now.Add(flow.config.Every.Duration)
	} else {
		flow.nextRun = time.Time{}
		flow.config.MaxRuns = flow.runs
	}
	return executed, nil
}

func (r *Runner) runAction(now time.Time, name string, action ActionConfig) error {
	err := r.executeAction(action)
	status := "ok"
	reason := ""
	if err != nil {
		status = "failed"
		reason = err.Error()
	}

	if emitErr := r.emit(event{
		Timestamp: now.Format(time.RFC3339),
		Type:      "action",
		Target:    r.cfg.Target,
		Action:    name,
		DryRun:    r.options.DryRun,
		Status:    status,
		Reason:    reason,
		Details: map[string]interface{}{
			"type":       action.Type,
			"post_delay": action.PostDelay.Duration.String(),
		},
	}); emitErr != nil && err == nil {
		err = emitErr
	}
	if err != nil {
		return err
	}

	if action.PostDelay.Duration > 0 && !r.options.DryRun {
		time.Sleep(action.PostDelay.Duration)
	}
	return nil
}

func (r *Runner) executeAction(action ActionConfig) error {
	if r.options.DryRun {
		return nil
	}

	switch action.Type {
	case "send_text":
		return tmux.PasteText(r.cfg.Target, action.Text, actionEnter(action))
	case "send_keys":
		return tmux.SendKeys(r.cfg.Target, action.Keys)
	case "clear":
		command := action.Command
		if command == "" {
			command = "/clear"
		}
		return tmux.PasteText(r.cfg.Target, command, actionEnter(action))
	default:
		return fmt.Errorf("unsupported action type %q", action.Type)
	}
}

func (r *Runner) emit(e event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	if r.cfg.LogPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.cfg.LogPath), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(r.cfg.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(append(data, '\n'))
	return err
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
