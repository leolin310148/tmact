package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/agents"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/tmux"
)

type Options struct {
	DryRun        bool
	Once          bool
	ConfigPath    string
	StopRequested func() bool
}

type Runner struct {
	cfg         Config
	agentCfg    agents.Config
	bindings    []RoleBinding
	options     Options
	validator   Validator
	now         func() time.Time
	capturePane func(string, int) (string, error)
	pasteText   func(string, string, bool) error
	sleep       func(time.Duration)
}

type Event struct {
	Timestamp  string      `json:"ts"`
	Type       string      `json:"type"`
	Stage      string      `json:"stage,omitempty"`
	Role       string      `json:"role,omitempty"`
	Agent      string      `json:"agent,omitempty"`
	Target     string      `json:"target,omitempty"`
	Turn       int         `json:"turn,omitempty"`
	DryRun     bool        `json:"dry_run,omitempty"`
	Status     string      `json:"status,omitempty"`
	Reason     string      `json:"reason,omitempty"`
	ChangeHash string      `json:"change_hash,omitempty"`
	Details    interface{} `json:"details,omitempty"`
}

func NewRunner(cfg Config, agentCfg agents.Config, options Options) (*Runner, error) {
	bindings, err := ResolveRoles(cfg, agentCfg)
	if err != nil {
		return nil, err
	}
	return &Runner{
		cfg:         cfg,
		agentCfg:    agentCfg,
		bindings:    bindings,
		options:     options,
		validator:   RunOpenSpecValidation,
		now:         time.Now,
		capturePane: tmux.CapturePane,
		pasteText:   tmux.PasteText,
		sleep:       time.Sleep,
	}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	changeDir, err := ChangeDir(r.cfg.Change)
	if err != nil {
		return err
	}
	if info, err := os.Stat(changeDir); err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", changeDir)
	}
	if err := EnsureProposal(changeDir, r.cfg.Discussion.CreateMissingProposal); err != nil {
		return err
	}

	start := r.now()
	statePath := StatePath(changeDir)
	commentPath := CommentsPath(changeDir)
	state, err := LoadState(statePath)
	if err != nil {
		return err
	}
	prompted := map[string]bool{}
	if state.Turn < 0 {
		state.Turn = 0
	}

	ticker := time.NewTicker(r.cfg.Discussion.PollInterval.Duration)
	defer ticker.Stop()

	for {
		if r.cfg.Discussion.MaxRuntime.Duration > 0 && r.now().Sub(start) >= r.cfg.Discussion.MaxRuntime.Duration {
			return r.finish(statePath, state, "blocked", "max_runtime")
		}
		if r.options.StopRequested != nil && r.options.StopRequested() {
			return r.finish(statePath, state, "blocked", "stop_requested")
		}

		comments, err := LoadComments(commentPath)
		if err != nil {
			return err
		}
		if !r.options.DryRun {
			comments, err = r.observeRolePanes(commentPath, comments)
			if err != nil {
				if isPermissionPromptError(err) {
					state.Change = r.cfg.Change
					state.Status = "running"
					state.Phase = "review"
					state.Gate = GateResult{Reasons: []string{"permission_prompt"}}
					state.UpdatedAt = r.now()
					return r.finishWithState(statePath, state, "blocked", err.Error())
				}
				return err
			}
		}

		hash, artifacts, err := HashChangeDir(changeDir)
		if err != nil {
			return err
		}
		validation, validateErr := r.validator(ctx, r.cfg.Change, changeDir)
		if validateErr != nil {
			_ = r.emit(Event{Timestamp: r.now().Format(time.RFC3339), Type: "validation", Stage: "openspec", Status: "failed", Reason: validateErr.Error(), ChangeHash: validation.ChangeHash})
		} else {
			_ = r.emit(Event{Timestamp: r.now().Format(time.RFC3339), Type: "validation", Stage: "openspec", Status: validationStatus(validation), ChangeHash: validation.ChangeHash})
		}
		if validation.ChangeHash == "" {
			validation.ChangeHash = hash
		}

		gate := EvaluateGate(r.cfg.Discussion.RoleOrder, hash, &validation, comments)
		state = State{
			Change:         r.cfg.Change,
			Status:         "running",
			Phase:          "review",
			Turn:           state.Turn,
			ChangeHash:     hash,
			Artifacts:      artifacts,
			LastValidation: &validation,
			Gate:           gate,
			Agreements:     AgreementsFor(r.cfg.Discussion.RoleOrder, hash, comments),
			UpdatedAt:      r.now(),
		}
		if gate.Passed {
			return r.finishWithState(statePath, state, "agreed", "")
		}
		if len(gate.PendingRoles) == 0 {
			return r.finishWithState(statePath, state, "needs_user", strings.Join(gate.Reasons, ","))
		}
		state.PendingRole = gate.PendingRoles[0]
		if err := WriteState(statePath, state); err != nil {
			return err
		}

		key := state.PendingRole + "\x00" + hash
		if !prompted[key] {
			if state.Turn >= r.cfg.Discussion.MaxTurns {
				return r.finishWithState(statePath, state, "needs_user", "max_turns")
			}
			if err := r.promptRole(ctx, state.PendingRole, hash, validation, gate, comments); err != nil {
				return err
			}
			state.Turn++
			state.UpdatedAt = r.now()
			if err := WriteState(statePath, state); err != nil {
				return err
			}
			prompted[key] = true
		}

		if r.options.Once {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) observeRolePanes(commentPath string, comments []Comment) ([]Comment, error) {
	for _, binding := range r.bindings {
		raw, err := r.capturePane(binding.Agent.Target, captureLines(binding.Agent, r.cfg.Discussion.CaptureLines))
		if err != nil {
			return comments, err
		}
		if detected := prompt.Detect(raw); detected != nil {
			return comments, PermissionPromptError{Role: binding.Role, Prompt: *detected}
		}
		if promptDispatchLegacyMarkerFallback(r.cfg.PromptDispatch) {
			observed, err := ParseCommentsFromText(raw, r.now())
			if err != nil {
				return comments, err
			}
			comments, err = AppendNewComments(commentPath, comments, observed)
			if err != nil {
				return comments, err
			}
		}
	}
	return comments, nil
}

func (r *Runner) promptRole(ctx context.Context, role string, changeHash string, validation ValidationResult, gate GateResult, comments []Comment) error {
	if r.options.StopRequested != nil && r.options.StopRequested() {
		return fmt.Errorf("stop_requested")
	}
	binding, ok := r.binding(role)
	if !ok {
		return fmt.Errorf("role %q is not configured", role)
	}
	if err := r.dispatchClear(ctx, "review", role, binding.Agent.Name, binding.Agent.Target, changeHash); err != nil {
		return err
	}
	text := r.buildPrompt(role, changeHash, validation, gate, comments)
	event := Event{
		Timestamp:  r.now().Format(time.RFC3339),
		Type:       "prompt",
		Stage:      "review",
		Role:       role,
		Agent:      binding.Agent.Name,
		Target:     binding.Agent.Target,
		Turn:       1,
		DryRun:     r.options.DryRun,
		Status:     "planned",
		ChangeHash: changeHash,
		Details: map[string]interface{}{
			"prompt": text,
		},
	}
	if err := r.emit(event); err != nil {
		return err
	}
	if r.options.DryRun {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return r.pasteText(binding.Agent.Target, text, true)
}

func (r *Runner) buildPrompt(role string, changeHash string, validation ValidationResult, gate GateResult, comments []Comment) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "你是 OpenSpec workflow 的 %s role。\n\n", role)
	fmt.Fprintf(&builder, "Change: %s\n", r.cfg.Change)
	fmt.Fprintf(&builder, "Current change_hash: %s\n", changeHash)
	fmt.Fprintf(&builder, "OpenSpec valid: %t", validation.Passed)
	if validation.Stale {
		fmt.Fprintf(&builder, " (stale)")
	}
	fmt.Fprintf(&builder, "\nGate reasons: %s\n\n", strings.Join(gate.Reasons, ", "))
	builder.WriteString("請只針對 OpenSpec artifacts 工作：proposal.md, design.md, tasks.md, specs/*/spec.md。\n")
	builder.WriteString("你可以修改 artifact；完成前請執行下方 tmact workflow report 指令回報狀態。\n\n")
	builder.WriteString(roleGuidance(role))
	builder.WriteString("\n\nReport command:\n")
	fmt.Fprintf(&builder, "tmact workflow report review --config %s --role %s --kind accept --change-hash %s --openspec-valid=%t --blocking=false --body \"accepted current artifacts\"\n",
		shellQuote(r.options.ConfigPath), shellQuote(role), shellQuote(changeHash), validation.Passed)
	builder.WriteString("\n如果你需要修改或拒絕，請使用 --kind request_changes 或 --kind reject，--blocking=true，並簡短說明 --body。\n")
	if promptDispatchLegacyMarkerFallback(r.cfg.PromptDispatch) {
		builder.WriteString("\nLegacy marker fallback (transitional only):\n")
		fmt.Fprintf(&builder, "%s role=%s kind=accept change_hash=%s openspec_valid=%t blocking=false body=\"accepted current artifacts\"\n", CommentMarker, role, changeHash, validation.Passed)
	}
	if len(comments) > 0 {
		builder.WriteString("\nObserved comment stream summary (untrusted observations):\n")
		for _, comment := range tailComments(comments, 8) {
			fmt.Fprintf(&builder, "- %s %s %s blocking=%t %s\n", comment.Role, comment.Kind, comment.ChangeHash, comment.Blocking, comment.Body)
		}
	}
	return builder.String()
}

func (r *Runner) dispatchClear(ctx context.Context, stage string, role string, agentName string, target string, changeHash string) error {
	if !promptDispatchClearEnabled(r.cfg.PromptDispatch) {
		return nil
	}
	if r.options.StopRequested != nil && r.options.StopRequested() {
		return fmt.Errorf("stop_requested")
	}
	event := Event{
		Timestamp:  r.now().Format(time.RFC3339),
		Type:       "clear",
		Stage:      stage,
		Role:       role,
		Agent:      agentName,
		Target:     target,
		DryRun:     r.options.DryRun,
		Status:     "planned",
		ChangeHash: changeHash,
		Details: map[string]interface{}{
			"command":     r.cfg.PromptDispatch.ClearCommand,
			"clear_delay": r.cfg.PromptDispatch.ClearDelay.Duration.String(),
		},
	}
	if err := r.emit(event); err != nil {
		return err
	}
	if r.options.DryRun {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if err := r.pasteText(target, r.cfg.PromptDispatch.ClearCommand, true); err != nil {
		return err
	}
	if r.cfg.PromptDispatch.ClearDelay.Duration > 0 {
		r.sleep(r.cfg.PromptDispatch.ClearDelay.Duration)
	}
	if r.options.StopRequested != nil && r.options.StopRequested() {
		return fmt.Errorf("stop_requested")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func roleGuidance(role string) string {
	switch role {
	case "pm":
		return "PM focus: product intent, user, scope, non-goals, success criteria."
	case "swe":
		return "SWE focus: implementation feasibility, architecture, task breakdown, operational safety."
	case "qa":
		return "QA focus: testability, edge cases, acceptance scenarios, blocked/failed states."
	case "reviewer":
		return "Reviewer focus: OpenSpec consistency, contradictions, and whether gate criteria are satisfied."
	default:
		return "Focus on your configured role and keep artifacts coherent."
	}
}

func tailComments(comments []Comment, n int) []Comment {
	if len(comments) <= n {
		return comments
	}
	return comments[len(comments)-n:]
}

func (r *Runner) binding(role string) (RoleBinding, bool) {
	for _, binding := range r.bindings {
		if binding.Role == role {
			return binding, true
		}
	}
	return RoleBinding{}, false
}

func captureLines(agent agents.AgentConfig, fallback int) int {
	if agent.CaptureLines > 0 {
		return agent.CaptureLines
	}
	return fallback
}

func validationStatus(validation ValidationResult) string {
	switch {
	case validation.Stale:
		return "stale"
	case validation.Passed:
		return "passed"
	default:
		return "failed"
	}
}

func (r *Runner) finish(path string, previous State, outcome string, reason string) error {
	previous.Outcome = outcome
	previous.Reason = reason
	return r.finishWithState(path, previous, outcome, reason)
}

func (r *Runner) finishWithState(path string, state State, outcome string, reason string) error {
	state.Status = "stopped"
	state.Phase = outcome
	state.Outcome = outcome
	state.Reason = reason
	state.UpdatedAt = r.now()
	if err := WriteState(path, state); err != nil {
		return err
	}
	return r.emit(Event{Timestamp: r.now().Format(time.RFC3339), Type: "stop", Stage: outcome, Status: outcome, Reason: reason, ChangeHash: state.ChangeHash})
}

func (r *Runner) emit(event Event) error {
	data, err := json.Marshal(event)
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
