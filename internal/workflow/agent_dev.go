package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/prompt"
)

const (
	agentDevPlanning    = "planning"
	agentDevActive      = "active"
	agentDevFixPlanning = "fix_planning"
	agentDevComplete    = "complete"
)

type AgentDevPlan struct {
	Done       bool               `json:"done,omitempty"`
	PhaseID    string             `json:"phase_id,omitempty"`
	Title      string             `json:"title,omitempty"`
	Items      []AgentDevPlanItem `json:"items,omitempty"`
	ReviewItem *AgentDevPlanItem  `json:"review_item,omitempty"`
}

type AgentDevPlanItem struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

func (e *Engine) tickAgentDev(ctx context.Context, state State) (bool, bool, error) {
	for _, stage := range e.Loaded.Config.Stages {
		if stage.Type != "agent_dev" {
			continue
		}
		ss := state.Stages[stage.ID]
		if ss.Status == StageSucceeded || ss.Status == StageSkipped || ss.Status == StageFailed || ss.Status == StageBlocked {
			continue
		}
		ready, terminal := dependenciesReady(stage, state)
		if terminal {
			ss.Status = StageSkipped
			ss.Error = "dependency did not succeed"
			ss.FinishedAt = e.Now()
			state.Stages[stage.ID] = ss
			return true, updateRunStatus(&state, e.Loaded.Config, e.Now()), e.Store.Write(state)
		}
		if !ready {
			return false, false, nil
		}
		if ss.Status == StagePending || ss.Status == StageRunnable {
			ss.Status = StageRunning
			ss.StartedAt = e.Now()
			ss.Attempt++
			ss.AgentDev = &AgentDevState{Status: agentDevPlanning}
			state.Stages[stage.ID] = ss
			_ = e.Store.Event(Event{Type: "agent_dev_started", Stage: stage.ID, Attempt: ss.Attempt, Status: StageRunning})
		}
		if ss.AgentDev == nil {
			return true, false, fmt.Errorf("agent_dev stage %s has no durable state", stage.ID)
		}
		if ss.AgentDev.CurrentDispatchID != "" {
			last, ok, err := LastDispatch(e.Store, ss.AgentDev.CurrentDispatchID)
			if err != nil {
				return true, false, err
			}
			if ok && last.Status == "sent" && e.Now().Sub(last.Timestamp) >= stage.Timeout.Duration {
				ss.Status = StageBlocked
				ss.Error = "agent_dev dispatch timed out: " + ss.AgentDev.CurrentDispatchID
				state.Status = "needs_user"
				state.Stages[stage.ID] = ss
				_ = e.Store.Event(Event{Type: "agent_dev_timeout", Stage: stage.ID, Attempt: last.Attempt, Status: StageBlocked, Reason: ss.Error})
				return true, false, e.Store.Write(state)
			}
			if ok && last.Status == "sent" && last.Target != "" {
				raw, captureErr := e.CapturePane(last.Target, 200)
				if captureErr != nil {
					ss.Status = StageBlocked
					ss.Error = fmt.Sprintf("agent_dev target %s disappeared or cannot be captured: %v", last.Target, captureErr)
					state.Status = "needs_user"
					state.Stages[stage.ID] = ss
					return true, false, e.Store.Write(state)
				}
				if detected := prompt.Detect(raw); detected != nil {
					ss.Status = StageBlocked
					ss.Error = fmt.Sprintf("agent_dev target %s is waiting on %s prompt", last.Target, detected.Type)
					state.Status = "needs_user"
					state.Stages[stage.ID] = ss
					_ = e.Store.Event(Event{Type: "agent_dev_prompt_stop", Stage: stage.ID, Attempt: last.Attempt, Status: StageBlocked, Reason: ss.Error})
					return true, false, e.Store.Write(state)
				}
			}
			state.Stages[stage.ID] = ss
			return true, false, e.Store.Write(state)
		}
		return e.dispatchNextAgentDev(ctx, state, stage, ss)
	}
	return false, false, nil
}

func (e *Engine) dispatchNextAgentDev(ctx context.Context, state State, stage StageConfig, ss StageState) (bool, bool, error) {
	dev := ss.AgentDev
	cfg := *stage.AgentDev
	role, actor, workItemID, promptText := "", "", "", ""
	var contract *WorkItemConfig

	phase := currentPhase(dev)
	switch {
	case phase == nil || phase.Status == agentDevComplete:
		role, actor = "coordinator", cfg.Coordinator
		workItemID = fmt.Sprintf("phase-plan-%d", len(dev.Phases)+1)
		dev.Status = agentDevPlanning
	case phase.Status == agentDevFixPlanning:
		role, actor = "coordinator", cfg.Coordinator
		workItemID = fmt.Sprintf("%s-fix-plan-%d", phase.ID, dev.ReviewRound)
	case nextPendingItem(phase) != nil:
		item := nextPendingItem(phase)
		role, actor, workItemID = "implementer", cfg.Implementer, item.ID
		item.Status = "active"
		item.Attempt++
		item.StartedAt = e.Now()
		contract = &WorkItemConfig{ID: item.ID, CheckboxPath: cfg.QueuePath, CompleteOutcomes: []string{"complete"}}
	case allItemsComplete(phase) && phase.ReviewItem.Status != "complete":
		role, actor, workItemID = "reviewer", cfg.Reviewer, phase.ReviewItem.ID
		phase.ReviewItem.Status = "active"
		phase.ReviewItem.Attempt++
		phase.ReviewItem.StartedAt = e.Now()
		dev.ReviewRound++
		contract = &WorkItemConfig{ID: phase.ReviewItem.ID, CheckboxPath: cfg.QueuePath, CompleteOutcomes: []string{"approve"}}
	default:
		return true, false, fmt.Errorf("agent_dev stage %s cannot determine next action", stage.ID)
	}

	baseline, err := inspectGitBaseline(e.Loaded.Config)
	if err != nil {
		ss.Status = StageBlocked
		ss.Error = err.Error()
		state.Status = "needs_user"
		state.Stages[stage.ID] = ss
		return true, false, e.Store.Write(state)
	}
	if contract != nil {
		baseline, err = inspectWorkItemStart(e.Loaded.Config, *contract)
		if err != nil {
			ss.Status = StageBlocked
			ss.Error = err.Error()
			state.Status = "needs_user"
			state.Stages[stage.ID] = ss
			return true, false, e.Store.Write(state)
		}
	}
	dispatchID := fmt.Sprintf("%s.%s.agent-dev.%s.%d", state.RunID, stage.ID, sanitizeDispatchPart(workItemID), dispatchAttempt(dev, workItemID))
	switch role {
	case "coordinator":
		if phase != nil && phase.Status == agentDevFixPlanning {
			promptText = coordinatorFixPrompt(state, stage, *phase, dispatchID, e.Store.Root)
		} else {
			promptText = coordinatorPlanPrompt(state, stage, phase, dispatchID, e.Store.Root)
		}
	case "implementer":
		promptText = implementerPrompt(stage, *phase, *findPhaseItem(phase, workItemID), dispatchID, e.Store.Root)
	case "reviewer":
		promptText = reviewerPrompt(stage, *phase, dispatchID, e.Store.Root)
	}
	promptText += agentDevStopProtocol(state.RunID, e.Store.Root)
	dev.CurrentDispatchID = dispatchID
	dev.CurrentRole = role
	dev.CurrentWorkItem = workItemID
	ss.AgentDev = dev
	state.Stages[stage.ID] = ss
	if err := e.Store.Write(state); err != nil {
		return true, false, err
	}
	record := Dispatch{Timestamp: e.Now(), ID: dispatchID, RunID: state.RunID, Stage: stage.ID, Attempt: dispatchAttempt(dev, workItemID), Actor: actor, Status: "planned", WorkItem: workItemID, BaseHead: baseline.Head, Branch: baseline.Branch, Role: role}
	if err := e.sendAgentDevDispatch(ctx, actor, promptText, record, stage.Timeout.Duration); err != nil {
		_ = e.Store.Update(func(current *State) error {
			currentStage := current.Stages[stage.ID]
			if currentStage.AgentDev != nil && currentStage.AgentDev.CurrentDispatchID == dispatchID {
				if isDeferredDispatch(err) {
					resetActiveAgentDevItem(currentStage.AgentDev)
				}
				currentStage.AgentDev.CurrentDispatchID = ""
				currentStage.AgentDev.CurrentRole = ""
				currentStage.AgentDev.CurrentWorkItem = ""
			}
			if isDeferredDispatch(err) {
				currentStage.Error = err.Error()
			} else {
				currentStage.Status = StageBlocked
				currentStage.Error = err.Error()
				current.Status = "needs_user"
			}
			current.Stages[stage.ID] = currentStage
			return nil
		})
		return true, false, nil
	}
	_ = e.Store.Event(Event{Type: "agent_dev_dispatch", Stage: stage.ID, Attempt: record.Attempt, Status: StageWaitingReport, Details: record})
	return true, false, nil
}

func (e *Engine) sendAgentDevDispatch(ctx context.Context, actorName, promptText string, record Dispatch, timeout time.Duration) error {
	last, exists, err := LastDispatch(e.Store, record.ID)
	if err != nil {
		return err
	}
	if exists && (last.Status == "sending" || last.Status == "sent") {
		return nil
	}
	actor := e.Loaded.Config.Actors[actorName]
	target, runtime, session, trust, launch, err := e.resolveActor(actor)
	if err != nil {
		return err
	}
	if launch {
		if stopped, err := e.workflowStopRequested(); err != nil {
			return err
		} else if stopped {
			return needsUserError{"workflow stop requested before agent_dev dispatch"}
		}
		layout, layoutErr := e.ListLayout()
		if layoutErr != nil {
			return layoutErr
		}
		if actor.Launch.Reuse != nil && !*actor.Launch.Reuse && layout.Sessions[session] {
			return fmt.Errorf("actor %s session %s already exists and reuse is false", actorName, session)
		}
		if layout.Sessions[session] {
			if err := e.validateSessionCWD(session, e.Loaded.Config.Workspace.Root); err != nil {
				return err
			}
		}
		if !exists {
			if err := e.Store.Dispatch(record); err != nil {
				return err
			}
		}
		record.Timestamp, record.Runtime, record.Status = e.Now(), runtime, "sending"
		if err := e.Store.Dispatch(record); err != nil {
			return err
		}
		report, runErr := e.DispatchAgent(dispatch.Options{Session: session, Dir: e.Loaded.Config.Workspace.Root, Agent: runtime, Prompt: promptText, Execute: true, ReadyTimeout: timeout, ReadySettle: 1500 * time.Millisecond, TrustFolder: trust, WorkspaceLeaseOwner: record.RunID})
		if runErr != nil {
			record.Timestamp, record.Status = e.Now(), "failed"
			_ = e.Store.Dispatch(record)
			message := strings.ToLower(runErr.Error())
			if strings.Contains(message, "prompt") {
				return needsUserError{runErr.Error()}
			}
			if strings.Contains(message, "busy") || strings.Contains(message, "did not remain idle") || strings.Contains(message, "explicitly input-ready") {
				return deferredDispatchError{runErr.Error()}
			}
			return runErr
		}
		target = report.Target
	} else {
		if err := e.preflightAgent(target, runtime, e.Loaded.Config.Workspace.Root, e.Loaded.Config.Defaults.IdleAfter.Duration); err != nil {
			return err
		}
		record.Target, record.Runtime = target, runtime
		if !exists {
			if err := e.Store.Dispatch(record); err != nil {
				return err
			}
		}
		record.Timestamp, record.Status = e.Now(), "sending"
		if err := e.Store.Dispatch(record); err != nil {
			return err
		}
		if err := e.PasteText(target, "/clear", true); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		e.Sleep(2 * time.Second)
		if stopped, err := e.workflowStopRequested(); err != nil {
			return err
		} else if stopped {
			return needsUserError{"workflow stop requested after clear"}
		}
		if err := e.PasteText(target, promptText, true); err != nil {
			return err
		}
	}
	record.Timestamp, record.Target, record.Status = e.Now(), target, "sent"
	return e.Store.Dispatch(record)
}

func (e *Engine) workflowStopRequested() (bool, error) {
	state, err := e.Store.Read()
	if err != nil {
		return false, err
	}
	return state.Desired == "stopped", nil
}

func agentDevStopProtocol(runID, storeRoot string) string {
	return fmt.Sprintf(`

停止協議：執行任何有副作用的動作前，以及提交 plan/report 前，都必須執行 tmact workflow status --id %s --store-dir %q --json。若 desired 是 stopped，立即停止，不要再執行動作，也不要提交 plan/report。不得直接修改 workflow store 內的 state、events、dispatches、reports 或 evidence；workflow 狀態只能透過指定的 tmact workflow CLI 變更。`, runID, storeRoot)
}

func inspectGitBaseline(cfg Config) (gitWorkItemState, error) {
	status, err := gitBytes(cfg.Workspace.Root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return gitWorkItemState{}, err
	}
	if len(status) != 0 {
		return gitWorkItemState{}, errors.New("agent_dev requires a clean worktree before every dispatch")
	}
	head, err := gitText(cfg.Workspace.Root, "rev-parse", "HEAD")
	if err != nil {
		return gitWorkItemState{}, err
	}
	branch, err := gitText(cfg.Workspace.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return gitWorkItemState{}, err
	}
	return gitWorkItemState{Head: head, Branch: branch}, nil
}

func dispatchAttempt(dev *AgentDevState, itemID string) int {
	if phase := currentPhase(dev); phase != nil {
		for i := range phase.Items {
			if phase.Items[i].ID == itemID {
				return phase.Items[i].Attempt
			}
		}
		if phase.ReviewItem.ID == itemID {
			return phase.ReviewItem.Attempt
		}
	}
	return len(dev.Phases) + dev.ReviewRound + 1
}

func currentPhase(dev *AgentDevState) *PhaseState {
	if dev == nil || len(dev.Phases) == 0 {
		return nil
	}
	return &dev.Phases[len(dev.Phases)-1]
}

func nextPendingItem(phase *PhaseState) *WorkItemState {
	if phase == nil {
		return nil
	}
	for i := range phase.Items {
		if phase.Items[i].Status == "pending" {
			return &phase.Items[i]
		}
	}
	return nil
}

func allItemsComplete(phase *PhaseState) bool {
	if phase == nil || len(phase.Items) == 0 {
		return false
	}
	for _, item := range phase.Items {
		if item.Status != "complete" {
			return false
		}
	}
	return true
}

func sanitizeDispatchPart(value string) string {
	var out strings.Builder
	for _, r := range value {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			out.WriteRune(r)
		} else {
			out.WriteByte('-')
		}
	}
	return out.String()
}

func coordinatorPlanPrompt(state State, stage StageConfig, previous *PhaseState, dispatchID, storeRoot string) string {
	request, _ := Render(stage.ID+".agent_dev.request", stage.AgentDev.Request, templateData(state))
	contextText := "No phase has been completed yet."
	if previous != nil {
		contextText = fmt.Sprintf("The previous phase %s (%s) is complete. Plan the next phase, or report done if the full request is satisfied.", previous.ID, previous.Title)
	}
	return fmt.Sprintf(`Act as the Coordinator for an unattended agent-development workflow.

User request:
%s

%s

Plan exactly one bounded phase. Add one unchecked Markdown checkbox for every implementation work item and one review work item to %s. Use stable unique IDs. Commit only the queue changes, leave the worktree clean, then write a JSON plan outside the repository (for example under /tmp) with this schema:
{"phase_id":"P1","title":"...","items":[{"id":"P1-W1","title":"...","acceptance_criteria":["..."]}],"review_item":{"id":"P1-R1","title":"Review phase P1","acceptance_criteria":["approve only with no blocking findings"]}}

Submit it with:
tmact workflow plan-report --dispatch-id %s --file /tmp/tmact-agent-plan.json --store-dir %q

If all requested work is already complete, write {"done":true} and submit the same command without changing Git. Do not dispatch agents yourself.`, request, contextText, stage.AgentDev.QueuePath, dispatchID, storeRoot)
}

func coordinatorFixPrompt(state State, stage StageConfig, phase PhaseState, dispatchID, storeRoot string) string {
	findings, _ := json.MarshalIndent(state.Stages[stage.ID].AgentDev.Findings, "", "  ")
	return fmt.Sprintf(`Act as the Coordinator. Reviewer rejected phase %s (%s).

Confirmed findings:
%s

Add one or more unchecked fix work items to %s. Do not change the existing review checkbox. Commit only the queue changes and leave the worktree clean. Write the plan outside the repository:
{"phase_id":%q,"items":[{"id":"%s-F1","title":"...","acceptance_criteria":["..."]}]}

Submit it with:
tmact workflow plan-report --dispatch-id %s --file /tmp/tmact-agent-fix-plan.json --store-dir %q

Do not implement fixes or dispatch agents yourself.`, phase.ID, phase.Title, findings, stage.AgentDev.QueuePath, phase.ID, phase.ID, dispatchID, storeRoot)
}

func implementerPrompt(stage StageConfig, phase PhaseState, item WorkItemState, dispatchID, storeRoot string) string {
	criteria := strings.Join(item.AcceptanceCriteria, "\n- ")
	return fmt.Sprintf(`Act as the Implementer. Complete exactly one work item in phase %s.

Work item: %s — %s
Acceptance criteria:
- %s

Read repository instructions. Do not switch branches. Implement and validate only this item. Change only its checkbox in %s from [ ] to [x]. Commit implementation, tests, evidence, and checkbox together. Finish with a clean worktree, then run:
tmact workflow report --dispatch-id %s --outcome complete --body "summary" --store-dir %q

If blocked, make no commit, leave the worktree clean, and report outcome blocked.`, phase.ID, item.ID, item.Title, criteria, stage.AgentDev.QueuePath, dispatchID, storeRoot)
}

func reviewerPrompt(stage StageConfig, phase PhaseState, dispatchID, storeRoot string) string {
	phaseJSON, _ := json.MarshalIndent(phase, "", "  ")
	return fmt.Sprintf(`Act as the independent Reviewer for all completed work in phase %s — %s.

Durable phase state:
%s

Review every phase work item, commits, tests, and acceptance criteria. Verify every suspected finding. Do not modify source code.

If there are no blocking findings, add review evidence, change only review checkbox %s in %s to [x], commit evidence and checkbox, leave the worktree clean, then run:
tmact workflow report --dispatch-id %s --outcome approve --body "approved" --store-dir %q

If blocking findings remain, do not change Git. Write a JSON array outside the repository (for example /tmp/tmact-review-findings.json) where every finding has id, fingerprint, severity, file, optional line, description, and acceptance. Then run the same report command with --outcome request_changes --findings-file /tmp/tmact-review-findings.json. The worktree must remain clean.`, phase.ID, phase.Title, phaseJSON, phase.ReviewItem.ID, stage.AgentDev.QueuePath, dispatchID, storeRoot)
}

func validatePlanItems(items []AgentDevPlanItem, limit int) error {
	if len(items) == 0 || len(items) > limit {
		return fmt.Errorf("plan must contain 1..%d items", limit)
	}
	seen := map[string]bool{}
	for _, item := range items {
		if !idPattern.MatchString(item.ID) || strings.TrimSpace(item.Title) == "" || len(item.AcceptanceCriteria) == 0 {
			return fmt.Errorf("invalid plan item %q", item.ID)
		}
		if seen[item.ID] {
			return fmt.Errorf("duplicate plan item %q", item.ID)
		}
		seen[item.ID] = true
	}
	return nil
}

func validateFindings(findings []ReviewFinding) error {
	if len(findings) == 0 {
		return errors.New("request_changes requires at least one structured finding")
	}
	seen := map[string]bool{}
	for _, finding := range findings {
		if !idPattern.MatchString(finding.ID) || strings.TrimSpace(finding.Fingerprint) == "" || strings.TrimSpace(finding.Description) == "" || strings.TrimSpace(finding.Acceptance) == "" {
			return fmt.Errorf("invalid structured finding %q", finding.ID)
		}
		if !contains([]string{"blocking", "high", "medium", "low"}, finding.Severity) {
			return fmt.Errorf("finding %s has invalid severity %q", finding.ID, finding.Severity)
		}
		if seen[finding.Fingerprint] {
			return fmt.Errorf("duplicate finding fingerprint %q", finding.Fingerprint)
		}
		seen[finding.Fingerprint] = true
	}
	return nil
}

func ApplyAgentDevPlan(root, dispatchID string, plan AgentDevPlan) (Report, error) {
	store, dispatchRecord, err := FindDispatch(root, dispatchID)
	if err != nil {
		return Report{}, err
	}
	if dispatchRecord.Role != "coordinator" {
		return Report{}, fmt.Errorf("dispatch %s is not a coordinator plan dispatch", dispatchID)
	}
	if existing, ok, err := HasReport(store, dispatchID); err != nil {
		return Report{}, err
	} else if ok {
		return existing, nil
	}
	state, err := store.Read()
	if err != nil {
		return Report{}, err
	}
	if state.Desired == "stopped" || state.Status == "stopped" {
		return Report{}, fmt.Errorf("workflow %s is stopped; plan rejected", state.RunID)
	}
	loaded, err := LoadSnapshot(store, state)
	if err != nil {
		return Report{}, err
	}
	stage, ok := stageConfig(loaded.Config, dispatchRecord.Stage)
	if !ok || stage.Type != "agent_dev" || stage.AgentDev == nil {
		return Report{}, fmt.Errorf("dispatch %s does not belong to an agent_dev stage", dispatchID)
	}
	ss := state.Stages[stage.ID]
	if ss.Status != StageRunning || ss.AgentDev == nil || ss.AgentDev.CurrentDispatchID != dispatchID {
		return Report{}, fmt.Errorf("dispatch %s is not the active coordinator turn", dispatchID)
	}
	if plan.Done {
		if plan.PhaseID != "" || len(plan.Items) != 0 || plan.ReviewItem != nil {
			return Report{}, errors.New("done plan must not contain a phase or work items")
		}
		if phase := currentPhase(ss.AgentDev); phase != nil && phase.Status != agentDevComplete {
			return Report{}, errors.New("cannot finish while a phase is incomplete")
		}
		if err := verifyUnchangedGit(loaded.Config, dispatchRecord); err != nil {
			return Report{}, err
		}
	} else {
		fixPlan := ss.AgentDev.Status == agentDevFixPlanning
		if !idPattern.MatchString(plan.PhaseID) || (!fixPlan && strings.TrimSpace(plan.Title) == "") {
			return Report{}, errors.New("plan phase_id and title are required")
		}
		if err := validatePlanItems(plan.Items, stage.AgentDev.MaxItemsPerPhase); err != nil {
			return Report{}, err
		}
		if fixPlan {
			phase := currentPhase(ss.AgentDev)
			if phase == nil || phase.ID != plan.PhaseID || plan.ReviewItem != nil {
				return Report{}, errors.New("fix plan must target the active phase and must not replace its review item")
			}
			if len(phase.Items)+len(plan.Items) > stage.AgentDev.MaxItemsPerPhase {
				return Report{}, fmt.Errorf("phase %s would exceed max_items_per_phase=%d", phase.ID, stage.AgentDev.MaxItemsPerPhase)
			}
		} else {
			if len(ss.AgentDev.Phases) >= stage.AgentDev.MaxPhases {
				return Report{}, fmt.Errorf("agent_dev reached max_phases=%d", stage.AgentDev.MaxPhases)
			}
			if plan.ReviewItem == nil {
				return Report{}, errors.New("new phase plan requires review_item")
			}
			if err := validatePlanItems([]AgentDevPlanItem{*plan.ReviewItem}, 1); err != nil {
				return Report{}, fmt.Errorf("review_item: %w", err)
			}
		}
		if err := validateUniquePlanIDs(ss.AgentDev, plan); err != nil {
			return Report{}, err
		}
		ids := make([]string, 0, len(plan.Items)+1)
		for _, item := range plan.Items {
			ids = append(ids, item.ID)
		}
		if plan.ReviewItem != nil {
			ids = append(ids, plan.ReviewItem.ID)
		}
		if err := verifyPlanCommit(loaded.Config, *stage.AgentDev, dispatchRecord, ids); err != nil {
			return Report{}, err
		}
	}
	head, _ := gitText(loaded.Config.Workspace.Root, "rev-parse", "HEAD")
	report := Report{Timestamp: time.Now(), ID: sha256Bytes([]byte(dispatchID + "\x00plan")), DispatchID: dispatchID, RunID: state.RunID, Stage: stage.ID, Attempt: dispatchRecord.Attempt, Outcome: "planned", WorkItem: dispatchRecord.WorkItem, Commit: head}
	if plan.Done {
		report.Outcome = "done"
	}
	if err := store.Report(report); err != nil {
		return Report{}, err
	}
	err = store.Update(func(current *State) error {
		currentStage := current.Stages[stage.ID]
		if currentStage.AgentDev == nil || currentStage.AgentDev.CurrentDispatchID != dispatchID {
			return ErrStateConflict
		}
		dev := currentStage.AgentDev
		if plan.Done {
			dev.Status = agentDevComplete
			currentStage.Status = StageSucceeded
			currentStage.Outcome = "complete"
			currentStage.Disposition = "success"
			currentStage.FinishedAt = report.Timestamp
		} else if dev.Status == agentDevFixPlanning {
			phase := currentPhase(dev)
			for _, item := range plan.Items {
				phase.Items = append(phase.Items, plannedWorkItem(item, "fix"))
			}
			phase.Status = agentDevActive
			dev.Status = agentDevActive
		} else {
			phase := PhaseState{ID: plan.PhaseID, Title: plan.Title, Status: agentDevActive, StartedAt: report.Timestamp}
			for _, item := range plan.Items {
				phase.Items = append(phase.Items, plannedWorkItem(item, "implement"))
			}
			phase.ReviewItem = plannedWorkItem(*plan.ReviewItem, "review")
			dev.Phases = append(dev.Phases, phase)
			dev.Status = agentDevActive
		}
		clearAgentDevDispatch(dev)
		currentStage.AgentDev = dev
		currentStage.Error = ""
		current.Stages[stage.ID] = currentStage
		current.Status = "running"
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	_ = store.Event(Event{Type: "agent_dev_plan", Stage: stage.ID, Attempt: dispatchRecord.Attempt, Status: report.Outcome, Details: report})
	return report, nil
}

func applyAgentDevReport(store Store, dispatchRecord Dispatch, outcome, body string, findings []ReviewFinding) (Report, error) {
	if existing, ok, err := HasReport(store, dispatchRecord.ID); err != nil {
		return Report{}, err
	} else if ok {
		return existing, nil
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
	stage, ok := stageConfig(loaded.Config, dispatchRecord.Stage)
	if !ok || stage.Type != "agent_dev" || stage.AgentDev == nil {
		return Report{}, fmt.Errorf("dispatch %s does not belong to an agent_dev stage", dispatchRecord.ID)
	}
	ss := state.Stages[stage.ID]
	if ss.Status != StageRunning || ss.AgentDev == nil || ss.AgentDev.CurrentDispatchID != dispatchRecord.ID {
		return Report{}, fmt.Errorf("dispatch %s is not the active agent_dev turn", dispatchRecord.ID)
	}
	phase := currentPhase(ss.AgentDev)
	if phase == nil {
		return Report{}, errors.New("agent_dev report has no active phase")
	}
	allowed := map[string][]string{"implementer": {"complete", "blocked"}, "reviewer": {"approve", "request_changes", "blocked"}}
	if !contains(allowed[dispatchRecord.Role], outcome) {
		return Report{}, fmt.Errorf("outcome %q is not allowed for %s", outcome, dispatchRecord.Role)
	}
	contract := WorkItemConfig{ID: dispatchRecord.WorkItem, CheckboxPath: stage.AgentDev.QueuePath, CompleteOutcomes: []string{"complete"}}
	if dispatchRecord.Role == "reviewer" {
		contract.CompleteOutcomes = []string{"approve"}
	}
	if err := verifyWorkItemReport(loaded.Config, contract, dispatchRecord, outcome); err != nil {
		return Report{}, err
	}
	if outcome == "request_changes" {
		if err := validateFindings(findings); err != nil {
			return Report{}, err
		}
	} else if len(findings) > 0 {
		return Report{}, errors.New("structured findings are only valid with request_changes")
	}
	head, _ := gitText(loaded.Config.Workspace.Root, "rev-parse", "HEAD")
	report := Report{Timestamp: time.Now(), ID: sha256Bytes([]byte(dispatchRecord.ID + "\x00" + outcome + "\x00" + body)), DispatchID: dispatchRecord.ID, RunID: state.RunID, Stage: stage.ID, Attempt: dispatchRecord.Attempt, Outcome: outcome, Body: body, WorkItem: dispatchRecord.WorkItem, Commit: head, Findings: findings}
	if err := store.Report(report); err != nil {
		return Report{}, err
	}
	err = store.Update(func(current *State) error {
		currentStage := current.Stages[stage.ID]
		if currentStage.AgentDev == nil || currentStage.AgentDev.CurrentDispatchID != dispatchRecord.ID {
			return ErrStateConflict
		}
		dev := currentStage.AgentDev
		activePhase := currentPhase(dev)
		switch dispatchRecord.Role {
		case "implementer":
			item := findPhaseItem(activePhase, dispatchRecord.WorkItem)
			if item == nil {
				return fmt.Errorf("work item %s is not active", dispatchRecord.WorkItem)
			}
			if outcome == "blocked" {
				item.Status = "blocked"
				currentStage.Status = StageBlocked
				currentStage.Error = body
				current.Status = "needs_user"
			} else {
				item.Status = "complete"
				item.Commit = head
				item.FinishedAt = report.Timestamp
			}
		case "reviewer":
			if outcome == "blocked" {
				activePhase.ReviewItem.Status = "blocked"
				currentStage.Status = StageBlocked
				currentStage.Error = body
				current.Status = "needs_user"
			} else if outcome == "approve" {
				activePhase.ReviewItem.Status = "complete"
				activePhase.ReviewItem.Commit = head
				activePhase.ReviewItem.FinishedAt = report.Timestamp
				activePhase.Status = agentDevComplete
				activePhase.FinishedAt = report.Timestamp
				dev.Status = agentDevPlanning
				dev.Findings = nil
				dev.LastFindingKeys = nil
				dev.NoProgressReviews = 0
			} else {
				activePhase.ReviewItem.Status = "pending"
				keys := findingKeys(findings)
				if strings.Join(keys, "\x00") == strings.Join(dev.LastFindingKeys, "\x00") {
					dev.NoProgressReviews++
				} else {
					dev.NoProgressReviews = 0
				}
				dev.LastFindingKeys = keys
				dev.Findings = findings
				activePhase.Status = agentDevFixPlanning
				dev.Status = agentDevFixPlanning
				if dev.NoProgressReviews >= stage.AgentDev.MaxNoProgressReviews {
					currentStage.Status = StageBlocked
					currentStage.Error = "review findings made no progress"
					current.Status = "needs_user"
				}
			}
		}
		clearAgentDevDispatch(dev)
		currentStage.AgentDev = dev
		current.Stages[stage.ID] = currentStage
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	_ = store.Event(Event{Type: "agent_dev_report", Stage: stage.ID, Attempt: dispatchRecord.Attempt, Status: outcome, Details: report})
	return report, nil
}

func verifyUnchangedGit(cfg Config, dispatchRecord Dispatch) error {
	baseline, err := inspectGitBaseline(cfg)
	if err != nil {
		return err
	}
	if baseline.Head != dispatchRecord.BaseHead || baseline.Branch != dispatchRecord.Branch {
		return errors.New("coordinator done report requires unchanged HEAD and branch")
	}
	return nil
}

func verifyPlanCommit(cfg Config, agentDev AgentDevConfig, dispatchRecord Dispatch, itemIDs []string) error {
	baseline, err := inspectGitBaseline(cfg)
	if err != nil {
		return err
	}
	if baseline.Branch != dispatchRecord.Branch || baseline.Head == dispatchRecord.BaseHead {
		return errors.New("coordinator plan must advance HEAD on the same branch")
	}
	if _, err := gitBytes(cfg.Workspace.Root, "merge-base", "--is-ancestor", dispatchRecord.BaseHead, baseline.Head); err != nil {
		return errors.New("coordinator plan commit is not a descendant of the dispatch base")
	}
	names, err := gitText(cfg.Workspace.Root, "diff", "--name-only", dispatchRecord.BaseHead+".."+baseline.Head)
	if err != nil {
		return err
	}
	for _, name := range strings.Fields(names) {
		if filepath.Clean(name) != filepath.Clean(agentDev.QueuePath) {
			return fmt.Errorf("coordinator plan commit changed %s; only %s is allowed", name, agentDev.QueuePath)
		}
	}
	for _, id := range itemIDs {
		ok, err := checkboxState(cfg, WorkItemConfig{ID: id, CheckboxPath: agentDev.QueuePath}, true)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("planned work item %s does not have exactly one unchecked checkbox", id)
		}
		diff, err := gitBytes(cfg.Workspace.Root, "diff", "--unified=0", dispatchRecord.BaseHead+".."+baseline.Head, "--", agentDev.QueuePath)
		if err != nil {
			return err
		}
		if !addedUncheckedItem(diff, id) {
			return fmt.Errorf("planned work item %s was not added by the coordinator commit", id)
		}
	}
	diff, err := gitBytes(cfg.Workspace.Root, "diff", "--unified=0", dispatchRecord.BaseHead+".."+baseline.Head, "--", agentDev.QueuePath)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(diff), "\n") {
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			return errors.New("coordinator plan commit must append work items without removing queue content")
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") && strings.Contains(line, "[x]") {
			return errors.New("coordinator plan commit must not add checked work items")
		}
	}
	return nil
}

func addedUncheckedItem(diff []byte, id string) bool {
	for _, line := range strings.Split(string(diff), "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") && strings.Contains(line, "[ ]") && containsWorkItemID(line, id) {
			return true
		}
	}
	return false
}

func validateUniquePlanIDs(dev *AgentDevState, plan AgentDevPlan) error {
	seen := map[string]bool{}
	for _, phase := range dev.Phases {
		seen[phase.ID] = true
		seen[phase.ReviewItem.ID] = true
		for _, item := range phase.Items {
			seen[item.ID] = true
		}
	}
	if dev.Status != agentDevFixPlanning && seen[plan.PhaseID] {
		return fmt.Errorf("duplicate phase id %q", plan.PhaseID)
	}
	if dev.Status != agentDevFixPlanning {
		seen[plan.PhaseID] = true
	}
	for _, item := range plan.Items {
		if seen[item.ID] {
			return fmt.Errorf("duplicate work item id %q", item.ID)
		}
		seen[item.ID] = true
	}
	if plan.ReviewItem != nil && seen[plan.ReviewItem.ID] {
		return fmt.Errorf("duplicate review work item id %q", plan.ReviewItem.ID)
	}
	return nil
}

func plannedWorkItem(item AgentDevPlanItem, kind string) WorkItemState {
	return WorkItemState{ID: item.ID, Kind: kind, Title: item.Title, AcceptanceCriteria: append([]string{}, item.AcceptanceCriteria...), Status: "pending"}
}

func findPhaseItem(phase *PhaseState, id string) *WorkItemState {
	if phase == nil {
		return nil
	}
	for i := range phase.Items {
		if phase.Items[i].ID == id {
			return &phase.Items[i]
		}
	}
	return nil
}

func clearAgentDevDispatch(dev *AgentDevState) {
	dev.CurrentDispatchID = ""
	dev.CurrentRole = ""
	dev.CurrentWorkItem = ""
}

func resetActiveAgentDevItem(dev *AgentDevState) {
	phase := currentPhase(dev)
	if phase == nil {
		return
	}
	for i := range phase.Items {
		if phase.Items[i].ID == dev.CurrentWorkItem && phase.Items[i].Status == "active" {
			phase.Items[i].Status = "pending"
			phase.Items[i].StartedAt = time.Time{}
			return
		}
	}
	if phase.ReviewItem.ID == dev.CurrentWorkItem && phase.ReviewItem.Status == "active" {
		phase.ReviewItem.Status = "pending"
		phase.ReviewItem.StartedAt = time.Time{}
	}
}

func findingKeys(findings []ReviewFinding) []string {
	keys := make([]string, len(findings))
	for i, finding := range findings {
		keys[i] = finding.Fingerprint
	}
	sort.Strings(keys)
	return keys
}
