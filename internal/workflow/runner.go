package workflow

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

	"tmact/internal/agents"
	"tmact/internal/prompt"
	workflowstate "tmact/internal/state"
	"tmact/internal/tmux"
)

type Options struct {
	DryRun            bool
	Once              bool
	AssumeIdleOnStart bool
	StartStage        string
}

type Runner struct {
	cfg                Config
	options            Options
	now                func() time.Time
	capturePane        func(string, int) (string, error)
	pasteText          func(string, string, bool) error
	sendKeys           func(string, []string) error
	sleep              func(time.Duration)
	idleIgnorePatterns []*regexp.Regexp
	stageMatchers      [][]*regexp.Regexp
}

type paneState struct {
	Hash             string                  `json:"hash"`
	Idle             bool                    `json:"idle"`
	IdleFor          string                  `json:"idle_for"`
	AgentState       string                  `json:"agent_state"`
	RecentLines      []string                `json:"recent_lines,omitempty"`
	PermissionPrompt *prompt.DirectoryAccess `json:"permission_prompt,omitempty"`
}

type event struct {
	Timestamp string      `json:"ts"`
	Type      string      `json:"type"`
	Target    string      `json:"target"`
	Stage     string      `json:"stage,omitempty"`
	Cycle     int         `json:"cycle,omitempty"`
	DryRun    bool        `json:"dry_run,omitempty"`
	Status    string      `json:"status,omitempty"`
	Reason    string      `json:"reason,omitempty"`
	Details   interface{} `json:"details,omitempty"`
}

type runState struct {
	stageIndex        int
	stageStarted      bool
	stageRepeatsDone  int
	cyclesDone        int
	stopped           bool
	nextStageRun      time.Time
	nextCycleRun      time.Time
	lastHash          string
	lastChanged       time.Time
	targetStates      map[string]targetRunState
	defaultLastChange time.Time
}

type targetRunState struct {
	lastHash    string
	lastChanged time.Time
}

func (s *runState) stateForTarget(target string) targetRunState {
	if s.targetStates != nil {
		if state, ok := s.targetStates[target]; ok {
			return state
		}
	}

	lastChanged := s.defaultLastChange
	if lastChanged.IsZero() {
		lastChanged = s.lastChanged
	}
	lastHash := ""
	if len(s.targetStates) == 0 {
		lastHash = s.lastHash
	}
	return targetRunState{
		lastHash:    lastHash,
		lastChanged: lastChanged,
	}
}

func (s *runState) setStateForTarget(target string, targetState targetRunState) {
	if s.targetStates == nil {
		s.targetStates = map[string]targetRunState{}
	}
	s.targetStates[target] = targetState
	s.lastHash = targetState.lastHash
	s.lastChanged = targetState.lastChanged
}

func NewRunner(cfg Config, options Options) *Runner {
	compiledIgnore := make([]*regexp.Regexp, 0, len(cfg.IdleIgnorePatterns))
	for _, pattern := range cfg.IdleIgnorePatterns {
		compiledIgnore = append(compiledIgnore, regexp.MustCompile(pattern))
	}

	stageMatchers := make([][]*regexp.Regexp, 0, len(cfg.Stages))
	for _, stage := range cfg.Stages {
		matchers := make([]*regexp.Regexp, 0, len(stage.CompleteWhen.RecentOutputMatches))
		for _, pattern := range stage.CompleteWhen.RecentOutputMatches {
			matchers = append(matchers, regexp.MustCompile(pattern))
		}
		stageMatchers = append(stageMatchers, matchers)
	}

	return &Runner{
		cfg:                cfg,
		options:            options,
		now:                time.Now,
		capturePane:        tmux.CapturePane,
		pasteText:          tmux.PasteText,
		sendKeys:           tmux.SendKeys,
		sleep:              time.Sleep,
		idleIgnorePatterns: compiledIgnore,
		stageMatchers:      stageMatchers,
	}
}

func (r *Runner) Run(ctx context.Context) error {
	start := r.now()
	lastChanged := start
	if r.options.AssumeIdleOnStart {
		lastChanged = start.Add(-r.cfg.IdleAfter.Duration)
	}
	state := runState{
		nextCycleRun:      start,
		lastChanged:       lastChanged,
		defaultLastChange: lastChanged,
	}
	if r.options.StartStage != "" {
		index, err := r.stageIndex(r.options.StartStage)
		if err != nil {
			return err
		}
		state.stageIndex = index
	}

	ticker := time.NewTicker(r.cfg.PollInterval.Duration)
	defer ticker.Stop()

	for {
		now := r.now()
		if r.cfg.MaxRuntime.Duration > 0 && now.Sub(start) >= r.cfg.MaxRuntime.Duration {
			return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.currentStageTarget(state.stageIndex), Reason: "max_runtime"})
		}

		if err := r.runOnce(now, &state); err != nil {
			return err
		}
		if state.stopped {
			return nil
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

func (r *Runner) runOnce(now time.Time, state *runState) error {
	stageTarget := r.currentStageTarget(state.stageIndex)
	pane, err := r.observeCurrentTarget(now, state, stageTarget)
	if err != nil {
		_ = r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "error", Target: stageTarget, Status: "failed", Reason: err.Error()})
		return err
	}

	if pane.PermissionPrompt != nil && r.cfg.StopOnPermissionPrompt {
		state.stopped = true
		return r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "stop",
			Target:    stageTarget,
			Reason:    "permission_prompt",
			Details:   pane.PermissionPrompt,
		})
	}

	if pane.AgentState == agents.StateBlocked {
		return r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "blocked",
			Target:    stageTarget,
			Cycle:     state.cyclesDone + 1,
			Stage:     r.currentStageName(state.stageIndex),
			Details:   pane,
		})
	}

	if state.stageStarted {
		return r.maybeCompleteStage(now, state, pane)
	}
	return r.maybeStartStage(now, state, pane)
}

func (r *Runner) observeCurrentTarget(now time.Time, state *runState, target string) (paneState, error) {
	targetState := state.stateForTarget(target)
	pane, changed, err := r.observe(target, now, targetState.lastHash, targetState.lastChanged)
	if err != nil {
		return paneState{}, err
	}
	targetState.lastHash = pane.Hash
	if changed {
		targetState.lastChanged = now
		pane.Idle = false
		pane.IdleFor = "0s"
	}
	state.setStateForTarget(target, targetState)
	return pane, nil
}

func (r *Runner) observe(target string, now time.Time, previousHash string, lastChanged time.Time) (paneState, bool, error) {
	raw, err := r.capturePane(target, r.cfg.CaptureLines)
	if err != nil {
		return paneState{}, false, err
	}

	hash := hashText(r.idleText(raw))
	idleFor := now.Sub(lastChanged)
	agentState, detected := agents.ClassifyPane(raw)
	state := paneState{
		Hash:             hash,
		Idle:             idleFor >= r.cfg.IdleAfter.Duration,
		IdleFor:          idleFor.Truncate(time.Second).String(),
		AgentState:       agentState,
		RecentLines:      agents.LastMeaningfulLines(raw, 12),
		PermissionPrompt: detected,
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

func (r *Runner) maybeStartStage(now time.Time, state *runState, pane paneState) error {
	if r.cfg.MaxCycles > 0 && state.cyclesDone >= r.cfg.MaxCycles {
		state.stopped = true
		return r.emit(event{Timestamp: now.Format(time.RFC3339), Type: "stop", Target: r.currentStageTarget(state.stageIndex), Reason: "max_cycles"})
	}
	if now.Before(state.nextCycleRun) {
		return r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "skip",
			Target:    r.currentStageTarget(state.stageIndex),
			Status:    "not_due",
			Reason:    "cycle_every",
			Details:   map[string]interface{}{"next_cycle_run": state.nextCycleRun.Format(time.RFC3339)},
		})
	}
	if !state.nextStageRun.IsZero() && now.Before(state.nextStageRun) {
		return r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "skip",
			Target:    r.currentStageTarget(state.stageIndex),
			Stage:     r.currentStageName(state.stageIndex),
			Cycle:     state.cyclesDone + 1,
			Status:    "not_due",
			Reason:    "stage_every",
			Details:   map[string]interface{}{"next_stage_run": state.nextStageRun.Format(time.RFC3339)},
		})
	}
	if !pane.Idle || pane.AgentState == agents.StateWorking {
		return r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "skip",
			Target:    r.currentStageTarget(state.stageIndex),
			Stage:     r.currentStageName(state.stageIndex),
			Cycle:     state.cyclesDone + 1,
			Status:    "not_idle",
			Details:   pane,
		})
	}

	stage := r.cfg.Stages[state.stageIndex]
	if err := r.startStage(now, state.cyclesDone+1, state.stageRepeatsDone+1, stage); err != nil {
		return err
	}
	r.scheduleNextStageStart(now, state)
	state.stageStarted = true
	return nil
}

func (r *Runner) maybeCompleteStage(now time.Time, state *runState, pane paneState) error {
	stage := r.cfg.Stages[state.stageIndex]
	if !r.stageComplete(state.stageIndex, stage, pane) {
		return r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "skip",
			Target:    r.currentStageTarget(state.stageIndex),
			Stage:     stage.Name,
			Cycle:     state.cyclesDone + 1,
			Status:    "stage_incomplete",
			Details:   pane,
		})
	}

	if err := r.emit(event{
		Timestamp: now.Format(time.RFC3339),
		Type:      "stage_complete",
		Target:    r.currentStageTarget(state.stageIndex),
		Stage:     stage.Name,
		Cycle:     state.cyclesDone + 1,
		Status:    "ok",
		Details: map[string]interface{}{
			"pane":         pane,
			"repeat_index": state.stageRepeatsDone + 1,
			"repeat_total": r.stageRepeatTotal(stage),
		},
	}); err != nil {
		return err
	}

	state.stageStarted = false
	state.stageRepeatsDone++
	if state.stageRepeatsDone < r.stageRepeatTotal(stage) {
		return nil
	}
	state.stageRepeatsDone = 0
	state.stageIndex++
	if state.stageIndex >= len(r.cfg.Stages) {
		state.cyclesDone++
		state.stageIndex = 0
		state.nextCycleRun = now.Add(r.cfg.CycleEvery.Duration)
		details := map[string]interface{}{
			"next_cycle_run": state.nextCycleRun.Format(time.RFC3339),
		}
		if !state.nextStageRun.IsZero() {
			details["next_stage_run"] = state.nextStageRun.Format(time.RFC3339)
		}
		return r.emit(event{
			Timestamp: now.Format(time.RFC3339),
			Type:      "cycle_complete",
			Target:    r.currentStageTarget(state.stageIndex),
			Cycle:     state.cyclesDone,
			Status:    "ok",
			Details:   details,
		})
	}
	return nil
}

func (r *Runner) scheduleNextStageStart(now time.Time, state *runState) {
	if r.cfg.StageEvery.Duration <= 0 {
		state.nextStageRun = time.Time{}
		return
	}
	state.nextStageRun = now.Add(r.cfg.StageEvery.Duration)
}

func (r *Runner) startStage(now time.Time, cycle int, repeatIndex int, stage StageConfig) error {
	err := r.sendStagePrompt(stage)
	status := "ok"
	reason := ""
	if err != nil {
		status = "failed"
		reason = err.Error()
	}

	if emitErr := r.emit(event{
		Timestamp: now.Format(time.RFC3339),
		Type:      "stage_start",
		Target:    r.stageTarget(stage),
		Stage:     stage.Name,
		Cycle:     cycle,
		DryRun:    r.options.DryRun,
		Status:    status,
		Reason:    reason,
		Details: map[string]interface{}{
			"clear_before_prompt": r.cfg.ClearBeforePrompt,
			"post_delay":          stage.PostDelay.Duration.String(),
			"repeat_index":        repeatIndex,
			"repeat_total":        r.stageRepeatTotal(stage),
		},
	}); emitErr != nil && err == nil {
		err = emitErr
	}
	if err != nil {
		return err
	}

	if stage.PostDelay.Duration > 0 && !r.options.DryRun {
		r.sleep(stage.PostDelay.Duration)
	}
	return nil
}

func (r *Runner) sendStagePrompt(stage StageConfig) error {
	if r.options.DryRun {
		return nil
	}
	target := r.stageTarget(stage)
	if err := r.sendKeys(target, []string{"C-u"}); err != nil {
		return err
	}
	if r.cfg.ClearBeforePrompt {
		if err := r.pasteText(target, r.cfg.ClearCommand, true); err != nil {
			return err
		}
		if r.cfg.ClearPostDelay.Duration > 0 {
			r.sleep(r.cfg.ClearPostDelay.Duration)
		}
		if err := r.sendKeys(target, []string{"C-u"}); err != nil {
			return err
		}
	}
	return r.pasteText(target, stage.Prompt, true)
}

func (r *Runner) stageComplete(index int, stage StageConfig, pane paneState) bool {
	if r.stageStateComplete(stage) {
		return true
	}
	if pane.AgentState == agents.StateWorking {
		return false
	}
	if stage.CompleteWhen.Idle && !pane.Idle {
		return false
	}
	matchers := r.stageMatchers[index]
	if len(matchers) == 0 {
		return true
	}
	text := strings.Join(pane.RecentLines, "\n")
	for _, matcher := range matchers {
		if matcher.MatchString(text) {
			return true
		}
	}
	return false
}

func (r *Runner) stageStateComplete(stage StageConfig) bool {
	if stage.CompleteWhen.StatePath == "" || len(stage.CompleteWhen.StateIn) == 0 {
		return false
	}
	status, err := workflowstate.Load(r.resolveStatePath(stage.CompleteWhen.StatePath))
	if err != nil {
		return false
	}
	current := status.State()
	for _, accepted := range stage.CompleteWhen.StateIn {
		if current == accepted {
			return true
		}
	}
	return false
}

func (r *Runner) resolveStatePath(path string) string {
	if path == "" || filepath.IsAbs(path) || r.cfg.Repo == "" {
		return path
	}
	return filepath.Join(r.cfg.Repo, path)
}

func (r *Runner) currentStageName(index int) string {
	if index < 0 || index >= len(r.cfg.Stages) {
		return ""
	}
	return r.cfg.Stages[index].Name
}

func (r *Runner) currentStageTarget(index int) string {
	if index < 0 || index >= len(r.cfg.Stages) {
		return r.cfg.Target
	}
	return r.stageTarget(r.cfg.Stages[index])
}

func (r *Runner) stageTarget(stage StageConfig) string {
	if stage.Target != "" {
		return stage.Target
	}
	return r.cfg.Target
}

func (r *Runner) stageIndex(name string) (int, error) {
	for i, stage := range r.cfg.Stages {
		if stage.Name == name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("stage %q not found", name)
}

func (r *Runner) stageRepeatTotal(stage StageConfig) int {
	if stage.Repeat > 0 {
		return stage.Repeat
	}
	return 1
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
