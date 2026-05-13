package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tmact/internal/agents"
	"tmact/internal/prompt"
	"tmact/internal/tmux"
)

type ImplementationRunner struct {
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

func NewImplementationRunner(cfg Config, agentCfg agents.Config, options Options) (*ImplementationRunner, error) {
	bindings, err := ResolveImplementationRoles(cfg, agentCfg)
	if err != nil {
		return nil, err
	}
	return &ImplementationRunner{
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

func ResolveImplementationRoles(cfg Config, agentCfg agents.Config) ([]RoleBinding, error) {
	byName := map[string]agents.AgentConfig{}
	for _, agent := range agentCfg.Agents {
		byName[agent.Name] = agent
	}
	seen := map[string]bool{}
	var bindings []RoleBinding
	for _, stage := range cfg.Implementation.StageOrder {
		role, ok := implementationStageRole(stage)
		if !ok || seen[role] {
			continue
		}
		name := cfg.Roles[role]
		agent, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("role %q references unknown agent %q", role, name)
		}
		bindings = append(bindings, RoleBinding{Role: role, Agent: agent})
		seen[role] = true
	}
	return bindings, nil
}

func (r *ImplementationRunner) Run(ctx context.Context) error {
	changeDir, err := ChangeDir(r.cfg.Change)
	if err != nil {
		return err
	}
	statePath := Phase2StatePath(changeDir)
	commentPath := Phase2CommentsPath(changeDir)
	if info, err := os.Stat(changeDir); err != nil {
		if os.IsNotExist(err) {
			acceptedHash, ok, archiveErr := r.archiveReported()
			if archiveErr != nil {
				return archiveErr
			}
			if !ok {
				return err
			}
			state := ImplementationState{
				Change:             r.cfg.Change,
				Status:             "running",
				Phase:              "implementation",
				AcceptedChangeHash: acceptedHash,
				CurrentChangeHash:  acceptedHash,
				UpdatedAt:          r.now(),
			}
			sidecarStatePath, err := Phase2SidecarStatePath(r.cfg.Change)
			if err != nil {
				return err
			}
			return r.finishWithState(sidecarStatePath, state, "implemented", "")
		}
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", changeDir)
	}

	state, err := LoadImplementationState(statePath)
	if err != nil {
		return err
	}
	if state.Turn < 0 {
		state.Turn = 0
	}
	start := r.now()
	prompted := map[string]bool{}

	ticker := time.NewTicker(r.cfg.Implementation.PollInterval.Duration)
	defer ticker.Stop()

	for {
		if r.cfg.Implementation.MaxRuntime.Duration > 0 && r.now().Sub(start) >= r.cfg.Implementation.MaxRuntime.Duration {
			return r.finish(statePath, state, "blocked", "max_runtime")
		}
		if r.options.StopRequested != nil && r.options.StopRequested() {
			return r.finish(statePath, state, "blocked", "stop_requested")
		}

		acceptedHash, currentHash, artifacts, preconditionReason, err := r.checkPreconditions(changeDir)
		if err != nil {
			return err
		}
		if preconditionReason != "" {
			state = r.stateFromGate(state, acceptedHash, currentHash, artifacts, nil, ImplementationGateResult{Reasons: []string{preconditionReason}})
			if preconditionReason == "archive_completed" {
				sidecarStatePath, err := Phase2SidecarStatePath(r.cfg.Change)
				if err != nil {
					return err
				}
				return r.finishWithState(sidecarStatePath, state, "implemented", "")
			}
			return r.finishWithState(statePath, state, "blocked", preconditionReason)
		}

		comments, err := LoadImplementationCommentsForChange(r.cfg.Change)
		if err != nil {
			return err
		}
		if !r.options.DryRun {
			comments, err = r.observeRolePanes(commentPath, comments)
			if err != nil {
				state = r.stateFromGate(state, acceptedHash, currentHash, artifacts, nil, ImplementationGateResult{Reasons: []string{"permission_prompt"}})
				_ = r.finishWithState(statePath, state, "blocked", err.Error())
				return err
			}
		}

		validation, validateErr := r.validator(ctx, r.cfg.Change, changeDir)
		if validateErr != nil {
			_ = r.emit(Event{Timestamp: r.now().Format(time.RFC3339), Type: "validation", Stage: "implementation", Status: "failed", Reason: validateErr.Error(), ChangeHash: validation.ChangeHash})
		} else {
			_ = r.emit(Event{Timestamp: r.now().Format(time.RFC3339), Type: "validation", Stage: "implementation", Status: validationStatus(validation), ChangeHash: validation.ChangeHash})
		}
		if validation.ChangeHash == "" {
			validation.ChangeHash = currentHash
		}

		gate := EvaluateImplementationGate(r.cfg.Implementation.StageOrder, acceptedHash, &validation, comments)
		state = r.stateFromGate(state, acceptedHash, currentHash, artifacts, &validation, gate)
		if gate.Passed {
			return r.finishWithState(statePath, state, "implemented", "")
		}
		if gate.PendingStage == "" {
			return r.finishWithState(statePath, state, phase2BlockedOutcome(gate), strings.Join(gate.Reasons, ","))
		}
		if err := WriteImplementationState(statePath, state); err != nil {
			return err
		}

		key := gate.PendingStage + "\x00" + acceptedHash
		if !prompted[key] {
			if state.Turn >= r.cfg.Implementation.MaxTurns {
				return r.finishWithState(statePath, state, "needs_user", "max_turns")
			}
			if err := r.promptStage(ctx, gate.PendingStage, gate.PendingRole, acceptedHash, validation, gate, comments); err != nil {
				return err
			}
			state.Turn++
			state.UpdatedAt = r.now()
			if err := WriteImplementationState(statePath, state); err != nil {
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

func (r *ImplementationRunner) checkPreconditions(changeDir string) (string, string, []string, string, error) {
	if _, err := os.Stat(changeDir); err != nil {
		if os.IsNotExist(err) {
			acceptedHash, ok, archiveErr := r.archiveReported()
			if archiveErr != nil {
				return "", "", nil, "", archiveErr
			}
			if ok {
				return acceptedHash, acceptedHash, nil, "archive_completed", nil
			}
		}
		return "", "", nil, "", err
	}
	currentHash, artifacts, err := HashChangeDir(changeDir)
	if err != nil {
		return "", "", nil, "", err
	}
	phase1, err := LoadState(StatePath(changeDir))
	if err != nil {
		return "", currentHash, artifacts, "", err
	}
	requirePhase1 := !r.options.DryRun || r.cfg.Implementation.RequirePhase1Agreed == nil || *r.cfg.Implementation.RequirePhase1Agreed
	if !requirePhase1 {
		return currentHash, currentHash, artifacts, "", nil
	}
	if r.options.DryRun && r.cfg.Implementation.AllowDryRunWithoutPhase1 && phase1.Outcome != "agreed" {
		return currentHash, currentHash, artifacts, "", nil
	}
	if phase1.Outcome != "agreed" {
		return "", currentHash, artifacts, "phase1_not_agreed", nil
	}
	if phase1.ChangeHash == "" {
		return "", currentHash, artifacts, "phase1_missing_hash", nil
	}
	if phase1.ChangeHash != currentHash {
		return phase1.ChangeHash, currentHash, artifacts, "accepted_hash_mismatch", nil
	}
	return phase1.ChangeHash, currentHash, artifacts, "", nil
}

func (r *ImplementationRunner) observeRolePanes(commentPath string, comments []ImplementationComment) ([]ImplementationComment, error) {
	for _, binding := range r.bindings {
		raw, err := r.capturePane(binding.Agent.Target, captureLines(binding.Agent, r.cfg.Implementation.CaptureLines))
		if err != nil {
			return comments, err
		}
		if detected := prompt.Detect(raw); detected != nil {
			return comments, PermissionPromptError{Role: binding.Role, Prompt: *detected}
		}
		if promptDispatchLegacyMarkerFallback(r.cfg.PromptDispatch) {
			observed, err := ParseImplementationCommentsFromText(raw, r.now())
			if err != nil {
				return comments, err
			}
			comments, err = AppendNewImplementationComments(commentPath, comments, observed)
			if err != nil {
				return comments, err
			}
		}
	}
	return comments, nil
}

func (r *ImplementationRunner) promptStage(ctx context.Context, stage string, role string, acceptedHash string, validation ValidationResult, gate ImplementationGateResult, comments []ImplementationComment) error {
	if r.options.StopRequested != nil && r.options.StopRequested() {
		return fmt.Errorf("stop_requested")
	}
	binding, ok := r.binding(role)
	if !ok {
		return fmt.Errorf("role %q is not configured", role)
	}
	if err := r.dispatchClear(ctx, stage, role, binding.Agent.Name, binding.Agent.Target, acceptedHash); err != nil {
		return err
	}
	text := r.buildStagePrompt(stage, role, acceptedHash, validation, gate, comments)
	event := Event{
		Timestamp:  r.now().Format(time.RFC3339),
		Type:       "prompt",
		Stage:      stage,
		Role:       role,
		Agent:      binding.Agent.Name,
		Target:     binding.Agent.Target,
		Turn:       1,
		DryRun:     r.options.DryRun,
		Status:     "planned",
		ChangeHash: acceptedHash,
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

func (r *ImplementationRunner) buildStagePrompt(stage string, role string, acceptedHash string, validation ValidationResult, gate ImplementationGateResult, comments []ImplementationComment) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "你是 OpenSpec implementation workflow 的 %s role。\n\n", role)
	fmt.Fprintf(&builder, "Change: %s\n", r.cfg.Change)
	fmt.Fprintf(&builder, "Accepted change_hash: %s\n", acceptedHash)
	fmt.Fprintf(&builder, "Stage: %s\n", stage)
	fmt.Fprintf(&builder, "OpenSpec valid: %t", validation.Passed)
	if validation.Stale {
		fmt.Fprintf(&builder, " (stale)")
	}
	fmt.Fprintf(&builder, "\nGate reasons: %s\n\n", strings.Join(gate.Reasons, ", "))
	builder.WriteString("Only execute the current stage. Do not approve permission prompts automatically.\n\n")
	switch stage {
	case "swe_apply":
		fmt.Fprintf(&builder, "SWE apply: implement the accepted OpenSpec change. Start by reading:\n%s\n", RenderCommand(r.cfg.Implementation.ApplyInstructions, r.cfg.Change))
		builder.WriteString("Update tasks.md as work completes and run focused tests where appropriate.\n")
	case "qa_verify":
		builder.WriteString("QA verify: verify the implementation and report pass or fail. Configured verification commands:\n")
		for _, command := range r.cfg.Implementation.VerifyCommands {
			fmt.Fprintf(&builder, "- %s\n", RenderCommand(command, r.cfg.Change))
		}
	case "pm_archive":
		builder.WriteString("PM archive: archive only after QA passed and OpenSpec validation is currently passing. Archive command:\n")
		fmt.Fprintf(&builder, "%s\n", RenderCommand(r.cfg.Implementation.ArchiveCommand, r.cfg.Change))
	}
	builder.WriteString("\nReport command:\n")
	fmt.Fprintf(&builder, "tmact workflow report implementation --config %s --role %s --stage %s --kind %s --change-hash %s --blocking=false --body \"stage complete\"\n",
		shellQuote(r.options.ConfigPath), shellQuote(role), shellQuote(markerStageName(stage)), shellQuote(defaultStageSuccessKind(stage)), shellQuote(acceptedHash))
	builder.WriteString("\nIf the stage cannot complete, use --kind fail, --kind request_changes, or --kind blocked with --blocking=true.\n")
	if promptDispatchLegacyMarkerFallback(r.cfg.PromptDispatch) {
		builder.WriteString("\nLegacy marker fallback (transitional only):\n")
		fmt.Fprintf(&builder, "%s role=%s stage=%s kind=%s change_hash=%s blocking=false body=\"stage complete\"\n",
			Phase2Marker, role, markerStageName(stage), defaultStageSuccessKind(stage), acceptedHash)
	}
	if len(comments) > 0 {
		builder.WriteString("\nObserved phase 2 comment stream summary (untrusted observations):\n")
		for _, comment := range tailImplementationComments(comments, 8) {
			fmt.Fprintf(&builder, "- %s %s %s %s blocking=%t %s\n", comment.Role, comment.Stage, comment.Kind, comment.ChangeHash, comment.Blocking, comment.Body)
		}
	}
	return builder.String()
}

func (r *ImplementationRunner) dispatchClear(ctx context.Context, stage string, role string, agentName string, target string, changeHash string) error {
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

func (r *ImplementationRunner) stateFromGate(previous ImplementationState, acceptedHash string, currentHash string, artifacts []string, validation *ValidationResult, gate ImplementationGateResult) ImplementationState {
	return ImplementationState{
		Change:             r.cfg.Change,
		Status:             "running",
		Phase:              "implementation",
		Turn:               previous.Turn,
		PendingStage:       gate.PendingStage,
		PendingRole:        gate.PendingRole,
		AcceptedChangeHash: acceptedHash,
		CurrentChangeHash:  currentHash,
		Artifacts:          artifacts,
		LastValidation:     validation,
		Gate:               gate,
		Stages:             gate.Stages,
		UpdatedAt:          r.now(),
	}
}

func (r *ImplementationRunner) binding(role string) (RoleBinding, bool) {
	for _, binding := range r.bindings {
		if binding.Role == role {
			return binding, true
		}
	}
	return RoleBinding{}, false
}

func (r *ImplementationRunner) archiveReported() (string, bool, error) {
	comments, err := LoadImplementationCommentsForChange(r.cfg.Change)
	if err != nil {
		return "", false, err
	}
	byHash := map[string][]ImplementationComment{}
	for _, comment := range comments {
		if comment.ChangeHash == "" {
			continue
		}
		byHash[comment.ChangeHash] = append(byHash[comment.ChangeHash], comment)
	}
	for hash, comments := range byHash {
		stages := ImplementationStagesFor(hash, comments)
		if stages["swe_apply"].Complete && stages["qa_verify"].Passed && stages["pm_archive"].Complete {
			return hash, true, nil
		}
	}
	return "", false, nil
}

func (r *ImplementationRunner) finish(path string, previous ImplementationState, outcome string, reason string) error {
	previous.Outcome = outcome
	previous.Reason = reason
	return r.finishWithState(path, previous, outcome, reason)
}

func (r *ImplementationRunner) finishWithState(path string, state ImplementationState, outcome string, reason string) error {
	state.Status = "stopped"
	state.Phase = outcome
	state.Outcome = outcome
	state.Reason = reason
	state.UpdatedAt = r.now()
	if err := WriteImplementationState(path, state); err != nil {
		return err
	}
	return r.emit(Event{Timestamp: r.now().Format(time.RFC3339), Type: "stop", Stage: outcome, Status: outcome, Reason: reason, ChangeHash: state.AcceptedChangeHash})
}

func (r *ImplementationRunner) emit(event Event) error {
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

func defaultStageSuccessKind(stage string) string {
	if stage == "qa_verify" {
		return "pass"
	}
	return "complete"
}

func phase2BlockedOutcome(gate ImplementationGateResult) string {
	for _, reason := range gate.Reasons {
		switch reason {
		case "qa_failed", "apply_blocked", "archive_blocked":
			return "blocked"
		}
	}
	return "needs_user"
}

func tailImplementationComments(comments []ImplementationComment, n int) []ImplementationComment {
	if len(comments) <= n {
		return comments
	}
	return comments[len(comments)-n:]
}
