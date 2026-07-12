package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/leolin310148/tmact/internal/agents"
	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/tmux"
	"gopkg.in/yaml.v3"
)

type Engine struct {
	Loaded           Loaded
	Store            Store
	Execute          bool
	Now              func() time.Time
	Sleep            func(time.Duration)
	ListLayout       func() (tmux.Layout, error)
	ListPanes        func(string) ([]tmux.Pane, error)
	ListSessionPanes func(string) ([]tmux.Pane, error)
	CapturePane      func(string, int) (string, error)
	CapturePaneANSI  func(string, int) (string, error)
	PasteText        func(string, string, bool) error
	DispatchAgent    func(dispatch.Options) (dispatch.Report, error)
	ProcessRuntime   func(int) panestatus.RuntimeDetection
	KillSession      func(string) error
	ActorKeys        map[string]string
}

type Plan struct {
	RunID      string            `json:"run_id"`
	ConfigHash string            `json:"config_hash"`
	Workspace  string            `json:"workspace"`
	Variables  map[string]any    `json:"variables"`
	Revisions  map[string]string `json:"revisions"`
	Stages     []PlanStage       `json:"stages"`
	Execute    bool              `json:"execute"`
}
type PlanStage struct {
	ID                string   `json:"id"`
	Type              string   `json:"type"`
	Needs             []string `json:"needs,omitempty"`
	Actor             string   `json:"actor,omitempty"`
	BindRevisions     []string `json:"bind_revisions,omitempty"`
	ProducesRevisions []string `json:"produces_revisions,omitempty"`
}

func BuildPlan(loaded Loaded) (Plan, error) {
	data := TemplateData{Vars: loaded.Variables, Stages: map[string]any{}, Revisions: map[string]string{}}
	revs, err := ComputeRevisions(loaded.Config, data)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{RunID: RunID(loaded.Hash), ConfigHash: loaded.Hash, Workspace: loaded.Config.Workspace.Root, Variables: loaded.Variables, Revisions: revs}
	for _, s := range loaded.Config.Stages {
		plan.Stages = append(plan.Stages, PlanStage{ID: s.ID, Type: s.Type, Needs: s.Needs, Actor: s.Actor, BindRevisions: s.BindRevisions, ProducesRevisions: s.ProducesRevisions})
	}
	return plan, nil
}

func NewEngine(loaded Loaded, storeRoot string, execute bool) (*Engine, error) {
	id := RunID(loaded.Hash)
	e := &Engine{Loaded: loaded, Store: NewStore(storeRoot, id), Execute: execute, Now: time.Now, Sleep: time.Sleep, ListLayout: tmux.ListLayout, ListPanes: tmux.ListPanes, ListSessionPanes: tmux.ListSessionPanes, CapturePane: tmux.CapturePane, CapturePaneANSI: tmux.CapturePaneANSI, PasteText: tmux.PasteText, DispatchAgent: dispatch.Run, ProcessRuntime: panestatus.DetectChildProcessRuntime, KillSession: tmux.KillSession}
	e.ActorKeys = e.buildActorKeys()
	release, err := e.Store.AcquireRunnerLock()
	if err != nil {
		return nil, err
	}
	defer release()
	if state, err := e.Store.Read(); err == nil {
		if state.ConfigHash != loaded.Hash {
			return nil, errors.New("active run config snapshot hash does not match")
		}
		recovered := false
		for _, stage := range loaded.Config.Stages {
			ss := state.Stages[stage.ID]
			if ss.Status != StageRunning {
				continue
			}
			if stage.Type == "agent" && ss.DispatchID != "" {
				last, ok, readErr := LastDispatch(e.Store, ss.DispatchID)
				if readErr != nil {
					return nil, readErr
				}
				if ok && (last.Status == "sending" || last.Status == "sent") {
					ss.Status = StageWaitingReport
					state.Stages[stage.ID] = ss
					recovered = true
					continue
				}
			}
			ss.Status = StagePending
			if ss.Attempt > 0 {
				ss.Attempt--
			}
			ss.DispatchID = ""
			state.Stages[stage.ID] = ss
			recovered = true
		}
		if recovered {
			state.PID = os.Getpid()
			state.HeartbeatAt = e.Now()
			if err := e.Store.Write(state); err != nil {
				return nil, err
			}
		}
		return e, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	plan, err := BuildPlan(loaded)
	if err != nil {
		return nil, err
	}
	now := e.Now()
	state := State{RunID: id, Status: "running", Desired: "running", ConfigPath: loaded.Config.ConfigPath, ConfigHash: loaded.Hash, Workspace: loaded.Config.Workspace.Root, Variables: loaded.Variables, Revisions: plan.Revisions, Stages: map[string]StageState{}, StartedAt: now, UpdatedAt: now, HeartbeatAt: now, PID: os.Getpid()}
	for _, stage := range loaded.Config.Stages {
		state.Stages[stage.ID] = StageState{ID: stage.ID, Status: StagePending}
	}
	if err := e.Store.Init(loaded, state); err != nil {
		return nil, err
	}
	_ = e.Store.Event(Event{Type: "run_started", Status: "running"})
	return e, nil
}

func (e *Engine) Run(ctx context.Context, once bool) error {
	if !e.Execute {
		return errors.New("engine execution requires execute=true")
	}
	release, err := e.Store.AcquireRunnerLock()
	if err != nil {
		return err
	}
	defer release()
	for {
		done, err := e.Tick(ctx)
		if err != nil {
			if errors.Is(err, ErrStateConflict) {
				continue
			}
			return err
		}
		if done || once {
			return nil
		}
		select {
		case <-ctx.Done():
			_ = e.Store.Update(func(s *State) error {
				s.Status = "stopped"
				s.Reason = ctx.Err().Error()
				s.FinishedAt = e.Now()
				return nil
			})
			return nil
		case <-time.After(e.Loaded.Config.Defaults.PollInterval.Duration):
		}
	}
}

type executionResult struct {
	stage    StageConfig
	evidence *Evidence
	outcome  string
	err      error
}

func (e *Engine) Tick(ctx context.Context) (bool, error) {
	state, err := e.Store.Read()
	if err != nil {
		return false, err
	}
	now := e.Now()
	state.HeartbeatAt = now
	state.PID = os.Getpid()
	if state.Desired == "stopped" {
		state.Status = "stopped"
		state.FinishedAt = now
		state.Reason = "operator_request"
		_ = e.Store.Write(state)
		_ = e.Store.Event(Event{Type: "run_stopped", Status: "stopped"})
		if err := e.cleanupActors(); err != nil {
			return true, err
		}
		return true, nil
	}
	if state.Desired == "paused" {
		state.Status = "paused"
		return false, e.Store.Write(state)
	}
	if state.Status == "paused" {
		state.Status = "running"
	}
	if state.Status == "needs_user" {
		return false, e.Store.Write(state)
	}
	data := templateData(state)
	current, err := ComputeRevisions(e.Loaded.Config, data)
	if err != nil {
		return false, err
	}
	if revisionsChanged(state.Revisions, current) {
		old := state.Revisions
		state.Revisions = current
		invalidateDrift(&state, e.Loaded.Config, old, current, now)
		_ = e.Store.Event(Event{Type: "revisions_changed", Details: map[string]any{"before": old, "after": current}})
	}
	advanceStale(&state, e.Loaded.Config)
	for _, stage := range e.Loaded.Config.Stages {
		ss := state.Stages[stage.ID]
		if ss.Status == StageWaitingReport && !ss.StartedAt.IsZero() && now.Sub(ss.StartedAt) >= stage.Timeout.Duration {
			finishDisposition(&ss, stage, "timeout", &Evidence{Result: "timeout", Summary: "agent report timed out", FinishedAt: now}, now)
			state.Stages[stage.ID] = ss
			_ = e.Store.Event(Event{Type: "stage_timeout", Stage: stage.ID, Attempt: ss.Attempt, Status: ss.Status})
		}
	}
	active := activeCount(state)
	available := e.Loaded.Config.Defaults.MaxParallel - active
	if available < 0 {
		available = 0
	}
	busyActors := e.activeActors(state)
	var selected []StageConfig
	for _, stage := range e.Loaded.Config.Stages {
		ss := state.Stages[stage.ID]
		if ss.Status != StagePending && ss.Status != StageRunnable {
			continue
		}
		if !ss.NextAttemptAt.IsZero() && now.Before(ss.NextAttemptAt) {
			continue
		}
		ready, terminal := dependenciesReady(stage, state)
		if terminal {
			ss.Status = StageSkipped
			ss.FinishedAt = now
			ss.Error = "dependency did not succeed"
			state.Stages[stage.ID] = ss
			continue
		}
		if !ready {
			continue
		}
		ok, err := EvaluateCondition(stage.When, state)
		if err != nil {
			return false, err
		}
		if !ok {
			ss.Status = StageSkipped
			ss.FinishedAt = now
			state.Stages[stage.ID] = ss
			_ = e.Store.Event(Event{Type: "stage_skipped", Stage: stage.ID, Status: StageSkipped})
			continue
		}
		ss.Status = StageRunnable
		state.Stages[stage.ID] = ss
		if stage.Type == "human" {
			ss.Attempt++
			ss.Status = StageWaitingHuman
			ss.StartedAt = now
			ss.BoundRevisions = bindValues(state.Revisions, stage.BindRevisions)
			state.Stages[stage.ID] = ss
			state.Status = "needs_user"
			_ = e.Store.Event(Event{Type: "human_wait", Stage: stage.ID, Status: StageWaitingHuman})
			for _, selectedStage := range selected {
				selectedState := state.Stages[selectedStage.ID]
				if selectedState.Status == StageRunnable {
					selectedState.Status = StagePending
					state.Stages[selectedStage.ID] = selectedState
				}
			}
			selected = nil
			break
		}
		if available <= 0 {
			continue
		}
		actorKey := e.actorMutexKey(stage.Actor)
		if stage.Actor != "" && busyActors[actorKey] {
			continue
		}
		selected = append(selected, stage)
		available--
		if stage.Actor != "" {
			busyActors[actorKey] = true
		}
	}
	results := make(chan executionResult, len(selected))
	var wg sync.WaitGroup
	for _, stage := range selected {
		ss := state.Stages[stage.ID]
		ss.Attempt++
		ss.Status = StageRunning
		ss.StartedAt = now
		ss.FinishedAt = time.Time{}
		ss.Error = ""
		ss.Outcome = ""
		ss.Disposition = ""
		ss.BoundRevisions = bindValues(state.Revisions, stage.BindRevisions)
		if stage.Type == "agent" {
			ss.DispatchID = fmt.Sprintf("%s.%s.%d.%d", state.RunID, stage.ID, ss.Generation, ss.Attempt)
		}
		state.Stages[stage.ID] = ss
		_ = e.Store.Event(Event{Type: "stage_started", Stage: stage.ID, Attempt: ss.Attempt, Status: StageRunning})
	}
	if err := e.Store.Write(state); err != nil {
		return false, err
	}
	for _, stage := range selected {
		stage := stage
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch stage.Type {
			case "command":
				ev, outcome, err := e.executeCommand(ctx, stage, state)
				results <- executionResult{stage: stage, evidence: ev, outcome: outcome, err: err}
			case "gate":
				ok, err := EvaluateCondition(stage.Condition, state)
				outcome := "success"
				if !ok {
					outcome = "blocked"
				}
				results <- executionResult{stage: stage, evidence: &Evidence{Result: outcome, Summary: fmt.Sprintf("condition=%t", ok)}, outcome: outcome, err: err}
			case "agent":
				ev, err := e.executeAgent(ctx, stage, state)
				results <- executionResult{stage: stage, evidence: ev, err: err}
			}
		}()
	}
	wg.Wait()
	close(results)
	for result := range results {
		latest, readErr := e.Store.Read()
		if readErr != nil {
			return false, readErr
		}
		ss := latest.Stages[result.stage.ID]
		fresh, computeErr := ComputeRevisions(e.Loaded.Config, templateData(latest))
		if computeErr != nil {
			return false, computeErr
		}
		latest.Revisions = fresh
		if attemptInputsDrifted(ss, result.stage, fresh) {
			ss.Status = StageStale
			ss.Error = "revision changed during attempt"
			ss.FinishedAt = e.Now()
			_ = e.Store.Event(Event{Type: "stage_stale", Stage: result.stage.ID, Attempt: ss.Attempt, Status: StageStale, Reason: ss.Error})
			if err := e.Store.Update(func(current *State) error {
				current.Revisions = fresh
				current.Stages[result.stage.ID] = ss
				return nil
			}); err != nil {
				return false, err
			}
			continue
		}
		if result.err != nil {
			ss.Error = result.err.Error()
			if isDeferredDispatch(result.err) {
				ss.Status = StagePending
				if ss.Attempt > 0 {
					ss.Attempt--
				}
				ss.DispatchID = ""
				ss.StartedAt = time.Time{}
				delay := result.stage.Retry.Backoff.Duration
				if delay <= 0 {
					delay = e.Loaded.Config.Defaults.PollInterval.Duration
				}
				ss.NextAttemptAt = e.Now().Add(delay)
				latest.Status = "running"
				_ = e.Store.Event(Event{Type: "stage_deferred", Stage: result.stage.ID, Attempt: ss.Attempt, Status: ss.Status, Reason: result.err.Error()})
			} else if isNeedsUser(result.err) {
				ss.Status = StageBlocked
				latest.Status = "needs_user"
			} else {
				finishDisposition(&ss, result.stage, "failed", result.evidence, e.Now())
			}
			if !isDeferredDispatch(result.err) {
				_ = e.Store.Event(Event{Type: "stage_error", Stage: result.stage.ID, Attempt: ss.Attempt, Status: ss.Status, Reason: result.err.Error()})
			}
		} else if result.stage.Type == "agent" {
			ss.Status = StageWaitingReport
			ss.Evidence = result.evidence
			latest.Status = "running"
			_ = e.Store.Event(Event{Type: "dispatch_sent", Stage: result.stage.ID, Attempt: ss.Attempt, Status: StageWaitingReport, Details: map[string]any{"dispatch_id": ss.DispatchID}})
		} else {
			finishDisposition(&ss, result.stage, result.outcome, result.evidence, e.Now())
			if ss.Status == StageSucceeded && len(result.stage.ProducesRevisions) > 0 {
				fresh, computeErr := ComputeRevisions(e.Loaded.Config, templateData(latest))
				if computeErr != nil {
					return false, computeErr
				}
				latest.Revisions = fresh
				ss.BoundRevisions = bindValues(fresh, appendUnique(result.stage.BindRevisions, result.stage.ProducesRevisions...))
			}
			_ = e.writeEvidence(result.stage.ID, ss.Attempt, result.evidence)
			_ = e.Store.Event(Event{Type: "stage_finished", Stage: result.stage.ID, Attempt: ss.Attempt, Status: ss.Status, Details: result.evidence})
		}
		if err := e.Store.Update(func(current *State) error {
			current.Revisions = latest.Revisions
			current.Stages[result.stage.ID] = ss
			if latest.Status == "needs_user" {
				current.Status = "needs_user"
			}
			return nil
		}); err != nil {
			return false, err
		}
	}
	state, err = e.Store.Read()
	if err != nil {
		return false, err
	}
	finished := updateRunStatus(&state, e.Loaded.Config, e.Now())
	if err := e.Store.Write(state); err != nil {
		return false, err
	}
	if finished {
		if err := e.cleanupActors(); err != nil {
			return true, err
		}
	}
	return finished, nil
}

func (e *Engine) cleanupActors() error {
	records, err := readJSONLines[Dispatch](e.Store.DispatchesPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	launched := map[string]bool{}
	for _, record := range records {
		if record.Status == "sending" || record.Status == "sent" {
			launched[record.Actor] = true
		}
	}
	seen := map[string]bool{}
	for name, actor := range e.Loaded.Config.Actors {
		if !launched[name] {
			continue
		}
		if actor.Launch == nil || actor.Launch.OnFinish != "stop" || seen[actor.Launch.Session] {
			continue
		}
		seen[actor.Launch.Session] = true
		layout, err := e.ListLayout()
		if err != nil {
			return err
		}
		if !layout.Sessions[actor.Launch.Session] {
			continue
		}
		if err := e.KillSession(actor.Launch.Session); err != nil {
			return fmt.Errorf("stop actor session %s: %w", actor.Launch.Session, err)
		}
	}
	return nil
}

func finishDisposition(ss *StageState, stage StageConfig, outcome string, evidence *Evidence, now time.Time) {
	disp := stage.Outcomes[outcome]
	if disp == "" {
		if outcome == "success" {
			disp = "success"
		} else if outcome == "blocked" {
			disp = "blocked"
		} else {
			disp = "failed"
		}
	}
	ss.Outcome = outcome
	ss.Disposition = disp
	ss.Evidence = evidence
	ss.FinishedAt = now
	switch disp {
	case "success":
		ss.Status = StageSucceeded
	case "retry":
		if ss.Attempt < stage.Retry.MaxAttempts {
			ss.Status = StagePending
			ss.NextAttemptAt = now.Add(stage.Retry.Backoff.Duration)
		} else {
			ss.Status = StageFailed
			ss.Error = "maximum attempts reached"
		}
	case "blocked":
		ss.Status = StageBlocked
	case "failed":
		if ss.Attempt < stage.Retry.MaxAttempts {
			ss.Status = StagePending
			ss.NextAttemptAt = now.Add(stage.Retry.Backoff.Duration)
		} else {
			ss.Status = StageFailed
		}
	}
}

func updateRunStatus(state *State, cfg Config, now time.Time) bool {
	all := true
	waitingHuman := false
	for _, stage := range cfg.Stages {
		s := state.Stages[stage.ID]
		switch s.Status {
		case StageFailed:
			state.Status = "failed"
			state.Reason = "stage " + stage.ID + " failed"
			state.FinishedAt = now
			return true
		case StageBlocked:
			if state.Status == "needs_user" {
				all = false
				continue
			}
			state.Status = "blocked"
			state.Reason = "stage " + stage.ID + " blocked"
			state.FinishedAt = now
			return true
		case StageWaitingHuman:
			waitingHuman = true
			all = false
		case StageSucceeded, StageSkipped:
		default:
			all = false
		}
	}
	if all {
		state.Status = "succeeded"
		state.FinishedAt = now
		state.Reason = ""
		return true
	}
	if waitingHuman {
		state.Status = "needs_user"
		return false
	}
	if state.Status != "needs_user" {
		state.Status = "running"
	}
	return false
}
func dependenciesReady(stage StageConfig, state State) (bool, bool) {
	for _, id := range stage.Needs {
		s := state.Stages[id]
		if s.Status == StageFailed || s.Status == StageBlocked || (s.Status == StageSkipped && s.Error != "") {
			return false, true
		}
		if s.Status != StageSucceeded && s.Status != StageSkipped {
			return false, false
		}
	}
	return true, false
}
func activeCount(state State) int {
	n := 0
	for _, s := range state.Stages {
		if s.Status == StageRunning || s.Status == StageWaitingReport {
			n++
		}
	}
	return n
}
func (e *Engine) activeActors(state State) map[string]bool {
	out := map[string]bool{}
	byID := map[string]StageConfig{}
	for _, s := range e.Loaded.Config.Stages {
		byID[s.ID] = s
	}
	for id, s := range state.Stages {
		if s.Status == StageRunning || s.Status == StageWaitingReport {
			if actor := byID[id].Actor; actor != "" {
				out[e.actorMutexKey(actor)] = true
			}
		}
	}
	return out
}
func (e *Engine) actorMutexKey(name string) string {
	if key := e.ActorKeys[name]; key != "" {
		return key
	}
	return "actor:" + name
}
func (e *Engine) buildActorKeys() map[string]string {
	out := map[string]string{}
	for name, actor := range e.Loaded.Config.Actors {
		if actor.Launch != nil {
			out[name] = "target-session:" + actor.Launch.Session
			continue
		}
		target, _, _, _, _, err := e.resolveActor(actor)
		if err == nil && target != "" {
			out[name] = "target:" + target
		} else {
			out[name] = "agent:" + actor.Agent
		}
	}
	return out
}
func revisionsChanged(a, b map[string]string) bool {
	if len(a) != len(b) {
		return true
	}
	for k, v := range a {
		if b[k] != v {
			return true
		}
	}
	return false
}
func bindValues(values map[string]string, names []string) map[string]string {
	if len(names) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, n := range names {
		out[n] = values[n]
	}
	return out
}
func appendUnique(base []string, values ...string) []string {
	seen := map[string]bool{}
	out := append([]string{}, base...)
	for _, v := range out {
		seen[v] = true
	}
	for _, v := range values {
		if !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out
}
func revisionInputs(stage StageConfig) []string {
	var out []string
	for _, name := range stage.BindRevisions {
		if !contains(stage.ProducesRevisions, name) {
			out = append(out, name)
		}
	}
	return out
}
func templateData(state State) TemplateData {
	stages := map[string]any{}
	raw, _ := json.Marshal(state.Stages)
	_ = json.Unmarshal(raw, &stages)
	return TemplateData{Vars: state.Variables, Stages: stages, Revisions: state.Revisions, Run: map[string]any{"id": state.RunID, "workspace": state.Workspace}}
}

func invalidateDrift(state *State, cfg Config, old, current map[string]string, now time.Time) {
	changed := map[string]bool{}
	for name, value := range old {
		if current[name] != value {
			changed[name] = true
		}
	}
	protected := protectedProducerChains(*state, cfg, changed)
	invalid := map[string]bool{}
	for _, stage := range cfg.Stages {
		ss := state.Stages[stage.ID]
		if !contains([]string{StageRunning, StageWaitingReport, StageSucceeded}, ss.Status) {
			continue
		}
		for name, bound := range ss.BoundRevisions {
			if protected[name][stage.ID] {
				continue
			}
			if changed[name] && current[name] != bound {
				ss.Status = StageStale
				ss.Error = "revision changed"
				ss.FinishedAt = now
				state.Stages[stage.ID] = ss
				invalid[stage.ID] = true
				break
			}
		}
	}
	for {
		progress := false
		for _, stage := range cfg.Stages {
			if invalid[stage.ID] {
				continue
			}
			for _, need := range stage.Needs {
				if invalid[need] {
					ss := state.Stages[stage.ID]
					if contains([]string{StageSucceeded, StageSkipped, StageFailed, StageBlocked, StageWaitingReport, StageRunning}, ss.Status) {
						ss.Status = StageStale
						ss.Error = "upstream stage became stale"
						ss.FinishedAt = now
						state.Stages[stage.ID] = ss
					}
					invalid[stage.ID] = true
					progress = true
					break
				}
			}
		}
		if !progress {
			break
		}
	}
}

func protectedProducerChains(state State, cfg Config, changed map[string]bool) map[string]map[string]bool {
	stages := map[string]StageConfig{}
	for _, stage := range cfg.Stages {
		stages[stage.ID] = stage
	}
	protected := map[string]map[string]bool{}
	for _, producer := range cfg.Stages {
		ss := state.Stages[producer.ID]
		if ss.Status != StageRunning && ss.Status != StageWaitingReport {
			continue
		}
		for _, revision := range producer.ProducesRevisions {
			if !changed[revision] {
				continue
			}
			if protected[revision] == nil {
				protected[revision] = map[string]bool{}
			}
			protectStageAndAncestors(producer.ID, stages, protected[revision])
		}
	}
	return protected
}

func protectStageAndAncestors(stageID string, stages map[string]StageConfig, protected map[string]bool) {
	if protected[stageID] {
		return
	}
	protected[stageID] = true
	for _, need := range stages[stageID].Needs {
		protectStageAndAncestors(need, stages, protected)
	}
}

func attemptInputsDrifted(ss StageState, stage StageConfig, current map[string]string) bool {
	for name, bound := range ss.BoundRevisions {
		if contains(stage.ProducesRevisions, name) {
			continue
		}
		if current[name] != bound {
			return true
		}
	}
	return false
}
func advanceStale(state *State, cfg Config) {
	for _, stage := range cfg.Stages {
		ss := state.Stages[stage.ID]
		if ss.Status == StageStale {
			ss.Status = StagePending
			ss.Generation++
			ss.Attempt = 0
			ss.Outcome = ""
			ss.Disposition = ""
			ss.DispatchID = ""
			ss.Evidence = nil
			ss.NextAttemptAt = time.Time{}
			state.Stages[stage.ID] = ss
		}
	}
}

func (e *Engine) executeCommand(parent context.Context, stage StageConfig, state State) (*Evidence, string, error) {
	data := templateData(state)
	argv, err := renderList(stage.ID+".argv", stage.Argv, data)
	if err != nil {
		return nil, "failed", err
	}
	cwdText, err := Render(stage.ID+".cwd", stage.Cwd, data)
	if err != nil {
		return nil, "failed", err
	}
	cwd, err := safeWorkspacePath(e.Loaded.Config.Workspace.Root, cwdText)
	if err != nil {
		return nil, "failed", err
	}
	ctx, cancel := context.WithTimeout(parent, stage.Timeout.Duration)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = cwd
	env := []string{}
	for _, name := range stage.InheritEnv {
		if strings.Contains(name, "=") {
			return nil, "failed", fmt.Errorf("invalid inherited env name %q", name)
		}
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	for _, name := range sortedKeys(stage.Env) {
		value, err := Render(stage.ID+".env."+name, stage.Env[name], data)
		if err != nil {
			return nil, "failed", err
		}
		env = append(env, name+"="+value)
	}
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	started := e.Now()
	runErr := cmd.Run()
	finished := e.Now()
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	ev := &Evidence{Result: "failed", Argv: argv, Cwd: cwd, StartedAt: started, FinishedAt: finished, ExitCode: &exitCode, Stdout: summarize(stdout.String()), Stderr: summarize(stderr.String())}
	if ctx.Err() == context.DeadlineExceeded {
		ev.Summary = "command timed out"
		return ev, "timeout", nil
	}
	if containsInt(stage.SuccessExitCodes, exitCode) {
		ev.Result = "success"
		ev.Summary = "command exited successfully"
		return ev, "success", nil
	}
	ev.Summary = fmt.Sprintf("command exited with code %d", exitCode)
	if runErr != nil && exitCode < 0 {
		return ev, "failed", runErr
	}
	return ev, "failed", nil
}
func summarize(text string) string {
	const limit = 16 * 1024
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "\n...[truncated]"
}
func containsInt(values []int, want int) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func (e *Engine) writeEvidence(stage string, attempt int, ev *Evidence) error {
	if ev == nil {
		return nil
	}
	raw, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(e.Store.Dir, "evidence", fmt.Sprintf("%s-%d.json", stage, attempt)), append(raw, '\n'), 0o644)
}

type needsUserError struct{ message string }

func (e needsUserError) Error() string { return e.message }
func isNeedsUser(err error) bool       { var target needsUserError; return errors.As(err, &target) }

type deferredDispatchError struct{ message string }

func (e deferredDispatchError) Error() string { return e.message }
func isDeferredDispatch(err error) bool {
	var target deferredDispatchError
	return errors.As(err, &target)
}

func (e *Engine) executeAgent(ctx context.Context, stage StageConfig, state State) (*Evidence, error) {
	ss := state.Stages[stage.ID]
	actor := e.Loaded.Config.Actors[stage.Actor]
	promptText, err := Render(stage.ID+".prompt", stage.Prompt, templateData(state))
	if err != nil {
		return nil, err
	}
	outcomes := sortedKeys(stage.Outcomes)
	statusCommand := fmt.Sprintf("tmact workflow status --id %s --store-dir %q --json", state.RunID, e.Store.Root)
	promptText += fmt.Sprintf("\n\n停止協議：執行任何有副作用的動作前，以及回報前，都必須執行 `%s`。若輸出的 `desired` 是 `stopped`，立即停止，不要再執行動作，也不要回報。\n完成後請回報：tmact workflow report --dispatch-id %s --outcome OUTCOME --body \"summary\"\nOUTCOME 必須是：%s", statusCommand, ss.DispatchID, strings.Join(outcomes, ", "))
	dispatchRecord := Dispatch{Timestamp: e.Now(), ID: ss.DispatchID, RunID: state.RunID, Stage: stage.ID, Attempt: ss.Attempt, Actor: stage.Actor, Status: "planned", Revisions: bindValues(ss.BoundRevisions, revisionInputs(stage))}
	last, exists, err := LastDispatch(e.Store, ss.DispatchID)
	if err != nil {
		return nil, err
	}
	if exists && (last.Status == "sending" || last.Status == "sent") {
		return &Evidence{Result: "dispatched", Summary: "recovered durable dispatch " + ss.DispatchID}, nil
	}
	target, runtime, session, trust, launch, err := e.resolveActor(actor)
	if err != nil {
		return nil, err
	}
	if launch {
		layout, layoutErr := e.ListLayout()
		if layoutErr != nil {
			return nil, layoutErr
		}
		if actor.Launch.Reuse != nil && !*actor.Launch.Reuse {
			if layout.Sessions[session] {
				return nil, fmt.Errorf("actor %s session %s already exists and reuse is false", stage.Actor, session)
			}
		}
		if layout.Sessions[session] {
			if err := e.validateSessionCWD(session, e.Loaded.Config.Workspace.Root); err != nil {
				return nil, err
			}
		}
		if !exists {
			if err := e.Store.Dispatch(dispatchRecord); err != nil {
				return nil, err
			}
		}
		dispatchRecord.Timestamp = e.Now()
		dispatchRecord.Runtime = runtime
		dispatchRecord.Status = "sending"
		if err := e.Store.Dispatch(dispatchRecord); err != nil {
			return nil, err
		}
		report, err := e.DispatchAgent(dispatch.Options{Session: session, Dir: e.Loaded.Config.Workspace.Root, Agent: runtime, Prompt: promptText, Execute: true, ReadyTimeout: stage.Timeout.Duration, ReadySettle: 1500 * time.Millisecond, TrustFolder: trust})
		if err != nil {
			dispatchRecord.Timestamp = e.Now()
			dispatchRecord.Status = "failed"
			_ = e.Store.Dispatch(dispatchRecord)
			message := strings.ToLower(err.Error())
			if strings.Contains(message, "prompt") {
				return nil, needsUserError{err.Error()}
			}
			if strings.Contains(message, "busy") || strings.Contains(message, "did not remain idle") || strings.Contains(message, "explicitly input-ready") {
				return nil, deferredDispatchError{err.Error()}
			}
			return nil, err
		}
		target = report.Target
	} else {
		if err := e.preflightAgent(target, runtime, e.Loaded.Config.Workspace.Root, e.Loaded.Config.Defaults.IdleAfter.Duration); err != nil {
			return nil, err
		}
		dispatchRecord.Target = target
		dispatchRecord.Runtime = runtime
		if !exists {
			if err := e.Store.Dispatch(dispatchRecord); err != nil {
				return nil, err
			}
		}
		dispatchRecord.Timestamp = e.Now()
		dispatchRecord.Status = "sending"
		if err := e.Store.Dispatch(dispatchRecord); err != nil {
			return nil, err
		}
		if err := e.PasteText(target, "/clear", true); err != nil {
			dispatchRecord.Timestamp = e.Now()
			dispatchRecord.Status = "failed"
			_ = e.Store.Dispatch(dispatchRecord)
			return nil, err
		}
		select {
		case <-ctx.Done():
			dispatchRecord.Timestamp = e.Now()
			dispatchRecord.Status = "failed"
			_ = e.Store.Dispatch(dispatchRecord)
			return nil, ctx.Err()
		default:
		}
		e.Sleep(2 * time.Second)
		if err := e.PasteText(target, promptText, true); err != nil {
			dispatchRecord.Timestamp = e.Now()
			dispatchRecord.Status = "failed"
			_ = e.Store.Dispatch(dispatchRecord)
			return nil, err
		}
	}
	dispatchRecord.Timestamp = e.Now()
	dispatchRecord.Target = target
	dispatchRecord.Status = "sent"
	if err := e.Store.Dispatch(dispatchRecord); err != nil {
		return nil, err
	}
	return &Evidence{Result: "dispatched", Summary: "dispatch " + ss.DispatchID + " sent"}, nil
}

func (e *Engine) resolveActor(actor ActorConfig) (target, runtime, session string, trust, launch bool, err error) {
	if actor.Launch != nil {
		return "", actor.Launch.Runtime, actor.Launch.Session, actor.Launch.TrustFolder, true, nil
	}
	path := e.Loaded.Config.AgentsConfig
	if path == "" {
		err = errors.New("agents_config is required by named actors")
		return
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(e.Loaded.Config.ConfigPath), path)
	}
	cfg, loadErr := agents.LoadConfig(path)
	if loadErr != nil {
		err = loadErr
		return
	}
	for _, agent := range cfg.Agents {
		if agent.Name == actor.Agent {
			runtime = agent.Launcher
			if runtime == "" {
				runtime = agent.Type
			}
			target = agent.Target
			session = agent.Session
			if runtime == "" {
				err = fmt.Errorf("agent %q does not declare a runtime", actor.Agent)
			}
			return
		}
	}
	err = fmt.Errorf("agent %q not found in %s", actor.Agent, path)
	return
}
func (e *Engine) validateSessionCWD(session, workspace string) error {
	panes, err := e.ListSessionPanes(session)
	if err != nil {
		return err
	}
	if len(panes) == 0 {
		return fmt.Errorf("session %s has no panes", session)
	}
	pane := panes[0]
	for _, candidate := range panes {
		if candidate.Active {
			pane = candidate
			break
		}
	}
	cwd, err := filepath.EvalSymlinks(pane.CurrentPath)
	if err != nil {
		return err
	}
	if filepath.Clean(cwd) != filepath.Clean(workspace) {
		return fmt.Errorf("session %s cwd %s does not equal workspace %s", session, cwd, workspace)
	}
	return nil
}
func (e *Engine) preflightAgent(target, runtime, workspace string, idleAfter time.Duration) error {
	panes, err := e.ListPanes(target)
	if err != nil {
		return err
	}
	if len(panes) != 1 {
		return fmt.Errorf("target %s resolved to %d panes", target, len(panes))
	}
	pane := panes[0]
	cwd, err := filepath.EvalSymlinks(pane.CurrentPath)
	if err != nil {
		return err
	}
	if filepath.Clean(cwd) != filepath.Clean(workspace) {
		return fmt.Errorf("target %s cwd %s does not equal workspace %s", target, cwd, workspace)
	}
	raw, err := e.CapturePane(target, 200)
	if err != nil {
		return err
	}
	if detected := prompt.Detect(raw); detected != nil {
		return needsUserError{fmt.Sprintf("target %s is waiting on %s prompt", target, detected.Type)}
	}
	det := e.ProcessRuntime(pane.PanePID)
	if det.Runtime == "" || det.Runtime == panestatus.RuntimeUnknown {
		det = panestatus.ClassifyRuntime(pane, raw)
	}
	if runtime != "" && det.Runtime != runtime {
		return fmt.Errorf("target %s runtime is %s, expected %s", target, det.Runtime, runtime)
	}
	classified, err := e.classifyAgentPane(target, raw)
	if err != nil {
		return err
	}
	if classified.State == panestate.StateWorking {
		return deferredDispatchError{fmt.Sprintf("target %s is busy", target)}
	}
	if classified.State == panestate.StateDraftInput {
		return deferredDispatchError{fmt.Sprintf("target %s has unsent operator input", target)}
	}
	if classified.State != panestate.StateWaitingInput && classified.State != panestate.StateIdle {
		return deferredDispatchError{fmt.Sprintf("target %s is not explicitly input-ready (state=%s)", target, classified.State)}
	}
	if idleAfter > 0 {
		e.Sleep(idleAfter)
		second, err := e.CapturePane(target, 200)
		if err != nil {
			return err
		}
		secondClassified, classifyErr := e.classifyAgentPane(target, second)
		if classifyErr != nil {
			return classifyErr
		}
		if second != raw || (secondClassified.State != panestate.StateWaitingInput && secondClassified.State != panestate.StateIdle) {
			return deferredDispatchError{fmt.Sprintf("target %s did not remain idle for %s", target, idleAfter)}
		}
	}
	return nil
}

func (e *Engine) classifyAgentPane(target, raw string) (panestate.Result, error) {
	classified := panestate.Classify(raw)
	if e.CapturePaneANSI == nil {
		return classified, nil
	}
	ansi, err := e.CapturePaneANSI(target, 200)
	if err != nil {
		return classified, fmt.Errorf("capture styled pane %s: %w", target, err)
	}
	return panestate.ClassifyANSI(raw, ansi), nil
}

func ApplyReport(root, dispatchID, outcome, body string) (Report, error) {
	store, d, err := FindDispatch(root, dispatchID)
	if err != nil {
		return Report{}, err
	}
	existing, hasReport, err := HasReport(store, dispatchID)
	if err != nil {
		return Report{}, err
	}
	if hasReport {
		outcome = existing.Outcome
		body = existing.Body
	}
	state, err := store.Read()
	if err != nil {
		return Report{}, err
	}
	if state.Desired == "stopped" || state.Status == "stopped" {
		return Report{}, fmt.Errorf("workflow %s is stopped; report rejected", state.RunID)
	}
	loaded, err := LoadSnapshot(store, state)
	if err != nil {
		return Report{}, err
	}
	stage, ok := stageConfig(loaded.Config, d.Stage)
	if !ok {
		return Report{}, fmt.Errorf("stage %q not found in snapshot", d.Stage)
	}
	disp, ok := stage.Outcomes[outcome]
	if !ok {
		return Report{}, fmt.Errorf("outcome %q is not allowed for stage %q", outcome, d.Stage)
	}
	current, err := ComputeRevisions(loaded.Config, templateData(state))
	if err != nil {
		return Report{}, err
	}
	for name, bound := range d.Revisions {
		if current[name] != bound {
			_ = store.Update(func(s *State) error {
				ss := s.Stages[d.Stage]
				ss.Status = StageStale
				ss.Error = "report rejected: revision changed"
				s.Stages[d.Stage] = ss
				s.Revisions = current
				return nil
			})
			return Report{}, fmt.Errorf("dispatch %s is stale: revision %s changed", dispatchID, name)
		}
	}
	ss := state.Stages[d.Stage]
	if hasReport && (ss.DispatchID != dispatchID || ss.Status != StageWaitingReport) {
		return existing, nil
	}
	if ss.DispatchID != dispatchID || ss.Attempt != d.Attempt || ss.Status != StageWaitingReport {
		return Report{}, fmt.Errorf("dispatch %s is not the active waiting attempt", dispatchID)
	}
	report := existing
	if !hasReport {
		report = Report{Timestamp: time.Now(), ID: sha256Bytes([]byte(dispatchID + "\x00" + outcome + "\x00" + body)), DispatchID: dispatchID, RunID: d.RunID, Stage: d.Stage, Attempt: d.Attempt, Outcome: outcome, Body: body, Revisions: current}
		if err := store.Report(report); err != nil {
			return Report{}, err
		}
	}
	err = store.Update(func(s *State) error {
		invalidateDrift(s, loaded.Config, s.Revisions, current, report.Timestamp)
		stageState := s.Stages[d.Stage]
		ev := &Evidence{Result: outcome, Summary: body, Body: body, FinishedAt: report.Timestamp}
		finishDisposition(&stageState, stage, outcome, ev, report.Timestamp)
		stageState.Disposition = disp
		stageState.BoundRevisions = bindValues(current, appendUnique(stage.BindRevisions, stage.ProducesRevisions...))
		s.Revisions = current
		s.Stages[d.Stage] = stageState
		s.Status = "running"
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	if updated, readErr := store.Read(); readErr == nil {
		if raw, marshalErr := json.MarshalIndent(updated.Stages[d.Stage].Evidence, "", "  "); marshalErr == nil {
			_ = os.WriteFile(filepath.Join(store.Dir, "evidence", fmt.Sprintf("%s-%d.json", d.Stage, d.Attempt)), append(raw, '\n'), 0o644)
		}
	}
	_ = store.Event(Event{Type: "report", Stage: d.Stage, Attempt: d.Attempt, Status: disp, Details: report})
	return report, nil
}

func ResolveHuman(root, id, configPath, stageID, outcome string, input map[string]string) error {
	store, state, err := Find(root, id, configPath)
	if err != nil {
		return err
	}
	if state.Desired == "stopped" || state.Status == "stopped" {
		return fmt.Errorf("workflow %s is stopped; resolution rejected", state.RunID)
	}
	loaded, err := LoadSnapshot(store, state)
	if err != nil {
		return err
	}
	stage, ok := stageConfig(loaded.Config, stageID)
	if !ok || stage.Type != "human" {
		return fmt.Errorf("stage %q is not a human stage", stageID)
	}
	if _, ok := stage.Outcomes[outcome]; !ok {
		return fmt.Errorf("outcome %q is not allowed for stage %q", outcome, stageID)
	}
	current, err := ComputeRevisions(loaded.Config, templateData(state))
	if err != nil {
		return err
	}
	ss := state.Stages[stageID]
	if attemptInputsDrifted(ss, stage, current) {
		_ = store.Update(func(s *State) error {
			stageState := s.Stages[stageID]
			stageState.Status = StageStale
			stageState.Error = "human resolution rejected: revision changed"
			s.Stages[stageID] = stageState
			s.Revisions = current
			return nil
		})
		return fmt.Errorf("human stage %s is stale: a bound revision changed", stageID)
	}
	values := map[string]any{}
	for key, schema := range stage.Input {
		raw, provided := input[key]
		if !provided {
			if schema.Required {
				return fmt.Errorf("input %q is required", key)
			}
			continue
		}
		kind := schema.Type
		if kind == "" {
			kind = "string"
		}
		value, err := parseScalar(kind, raw)
		if err != nil {
			return err
		}
		values[key] = value
	}
	for key := range input {
		if _, ok := stage.Input[key]; !ok {
			return fmt.Errorf("unknown input %q", key)
		}
	}
	return store.Update(func(s *State) error {
		ss := s.Stages[stageID]
		if ss.Status != StageWaitingHuman {
			return fmt.Errorf("stage %q is not waiting for human input", stageID)
		}
		ss.Input = values
		ss.BoundRevisions = bindValues(current, appendUnique(stage.BindRevisions, stage.ProducesRevisions...))
		finishDisposition(&ss, stage, outcome, &Evidence{Result: outcome, Summary: "resolved by human", FinishedAt: time.Now()}, time.Now())
		s.Stages[stageID] = ss
		s.Revisions = current
		s.Status = "running"
		return nil
	})
}

func RetryStage(root, id, stageID string) error {
	store, state, err := Find(root, id, "")
	if err != nil {
		return err
	}
	if state.Desired == "stopped" || state.Status == "stopped" {
		return fmt.Errorf("workflow %s is stopped; resume it before retrying", state.RunID)
	}
	loaded, err := LoadSnapshot(store, state)
	if err != nil {
		return err
	}
	if _, ok := stageConfig(loaded.Config, stageID); !ok {
		return fmt.Errorf("unknown stage %q", stageID)
	}
	return store.Update(func(s *State) error {
		reset := map[string]bool{stageID: true}
		for {
			changed := false
			for _, stage := range loaded.Config.Stages {
				for _, need := range stage.Needs {
					if reset[need] && !reset[stage.ID] {
						reset[stage.ID] = true
						changed = true
					}
				}
			}
			if !changed {
				break
			}
		}
		for id := range reset {
			ss := s.Stages[id]
			ss.Status = StagePending
			ss.Generation++
			ss.Attempt = 0
			ss.Outcome = ""
			ss.Disposition = ""
			ss.DispatchID = ""
			ss.Evidence = nil
			ss.Error = ""
			ss.NextAttemptAt = time.Time{}
			s.Stages[id] = ss
		}
		s.Status = "running"
		s.Reason = ""
		s.FinishedAt = time.Time{}
		return nil
	})
}

func LoadSnapshot(store Store, state State) (Loaded, error) {
	raw, err := os.ReadFile(filepath.Join(store.Dir, "config.yaml"))
	if err != nil {
		return Loaded{}, err
	}
	tmp := filepath.Join(store.Dir, "config.snapshot.yaml")
	_ = tmp
	var cfg Config
	if err := decodeStrict(raw, &cfg); err != nil {
		return Loaded{}, err
	}
	cfg.ConfigPath = state.ConfigPath
	applyDefaults(&cfg)
	cfg.Workspace.Root = state.Workspace
	if err := Validate(cfg, state.Variables); err != nil {
		return Loaded{}, err
	}
	return Loaded{Config: cfg, Variables: state.Variables, Raw: raw, Hash: state.ConfigHash}, nil
}
func decodeStrict(raw []byte, cfg *Config) error {
	if err := rejectDuplicateKeys(raw); err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("workflow config must contain exactly one YAML document")
		}
		return err
	}
	return nil
}
func stageConfig(cfg Config, id string) (StageConfig, bool) {
	for _, s := range cfg.Stages {
		if s.ID == id {
			return s, true
		}
	}
	return StageConfig{}, false
}

func SortedStageStates(state State) []StageState {
	out := make([]StageState, 0, len(state.Stages))
	for _, s := range state.Stages {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
