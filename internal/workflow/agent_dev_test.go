package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/tmux"
)

func initAgentDevTest(t *testing.T) (Loaded, *Engine, string) {
	t.Helper()
	dir := t.TempDir()
	runAgentDevGit(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "WORK_ITEMS.md"), []byte("# Work Items\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runAgentDevGit(t, dir, "add", "WORK_ITEMS.md")
	runAgentDevGit(t, dir, "commit", "-qm", "initialize work queue")
	path := writeConfig(t, dir, `version: 2
workspace:
  root: .
  git: {lease: true}
actors:
  coordinator: {launch: {runtime: codex, session: coordinator}}
  implementer: {launch: {runtime: claude, session: implementer}}
  reviewer: {launch: {runtime: codex, session: reviewer}}
defaults: {timeout: 5m, max_parallel: 1}
stages:
  - id: delivery
    type: agent_dev
    agent_dev:
      coordinator: coordinator
      implementer: implementer
      reviewer: reviewer
      request: deliver the requested feature
      queue_path: WORK_ITEMS.md
`)
	runAgentDevGit(t, dir, "add", "workflow.yaml")
	runAgentDevGit(t, dir, "commit", "-qm", "add agent dev workflow")
	loaded, err := Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(loaded, filepath.Join(t.TempDir(), "runs"), true)
	if err != nil {
		t.Fatal(err)
	}
	return loaded, engine, dir
}

func runAgentDevGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func appendQueueAndCommit(t *testing.T, dir, text, message string) {
	t.Helper()
	path := filepath.Join(dir, "WORK_ITEMS.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, []byte(text)...), 0o644); err != nil {
		t.Fatal(err)
	}
	runAgentDevGit(t, dir, "add", "WORK_ITEMS.md")
	runAgentDevGit(t, dir, "commit", "-qm", message)
}

func replaceQueueAndCommit(t *testing.T, dir, old, replacement, message string, extraFile bool) {
	t.Helper()
	path := filepath.Join(dir, "WORK_ITEMS.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := []byte(string(raw))
	updated = []byte(replaceOnce(t, string(updated), old, replacement))
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	runAgentDevGit(t, dir, "add", "WORK_ITEMS.md")
	if extraFile {
		if err := os.WriteFile(filepath.Join(dir, message+".txt"), []byte("evidence\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		runAgentDevGit(t, dir, "add", message+".txt")
	}
	runAgentDevGit(t, dir, "commit", "-qm", message)
}

func replaceOnce(t *testing.T, text, old, replacement string) string {
	t.Helper()
	index := -1
	for i := 0; i+len(old) <= len(text); i++ {
		if text[i:i+len(old)] == old {
			if index >= 0 {
				t.Fatalf("%q occurs more than once", old)
			}
			index = i
		}
	}
	if index < 0 {
		t.Fatalf("%q not found in %q", old, text)
	}
	return text[:index] + replacement + text[index+len(old):]
}

func activateAgentDevDispatch(t *testing.T, engine *Engine, role, itemID string, attempt int) Dispatch {
	t.Helper()
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := inspectGitBaseline(engine.Loaded.Config)
	if err != nil {
		t.Fatal(err)
	}
	id := state.RunID + ".delivery.test." + sanitizeDispatchPart(itemID) + "." + fmt.Sprint(attempt)
	ss := state.Stages["delivery"]
	if ss.AgentDev == nil {
		ss.AgentDev = &AgentDevState{Status: agentDevPlanning}
	}
	ss.Status = StageRunning
	ss.AgentDev.CurrentDispatchID = id
	ss.AgentDev.CurrentRole = role
	ss.AgentDev.CurrentWorkItem = itemID
	if phase := currentPhase(ss.AgentDev); phase != nil {
		if role == "implementer" {
			item := findPhaseItem(phase, itemID)
			item.Status = "active"
			item.Attempt = attempt
		} else if role == "reviewer" {
			phase.ReviewItem.Status = "active"
			phase.ReviewItem.Attempt = attempt
		}
	}
	state.Stages["delivery"] = ss
	if err := engine.Store.Write(state); err != nil {
		t.Fatal(err)
	}
	record := Dispatch{Timestamp: engine.Now(), ID: id, RunID: state.RunID, Stage: "delivery", Attempt: attempt, Actor: role, Status: "sent", WorkItem: itemID, BaseHead: baseline.Head, Branch: baseline.Branch, Role: role}
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	return record
}

func TestAgentDevPlanImplementReviewFixConverges(t *testing.T) {
	_, engine, dir := initAgentDevTest(t)
	coordinator := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-1", 1)
	appendQueueAndCommit(t, dir, "- [ ] P1-W1 implement feature\n- [ ] P1-R1 review phase\n", "plan phase one")
	plan := AgentDevPlan{PhaseID: "P1", Title: "Feature", Items: []AgentDevPlanItem{{ID: "P1-W1", Title: "Implement feature", AcceptanceCriteria: []string{"tests pass"}}}, ReviewItem: &AgentDevPlanItem{ID: "P1-R1", Title: "Review phase", AcceptanceCriteria: []string{"no blockers"}}}
	if _, err := ApplyAgentDevPlan(engine.Store.Root, coordinator.ID, plan); err != nil {
		t.Fatal(err)
	}

	implement := activateAgentDevDispatch(t, engine, "implementer", "P1-W1", 1)
	replaceQueueAndCommit(t, dir, "[ ] P1-W1", "[x] P1-W1", "implement-feature", true)
	if _, err := ApplyReportDetailed(engine.Store.Root, implement.ID, "complete", "implemented", nil); err != nil {
		t.Fatal(err)
	}

	review := activateAgentDevDispatch(t, engine, "reviewer", "P1-R1", 1)
	findings := []ReviewFinding{{ID: "F1", Fingerprint: "missing-edge-test", Severity: "blocking", File: "feature.go", Line: 10, Description: "missing edge test", Acceptance: "add the regression test"}}
	if _, err := ApplyReportDetailed(engine.Store.Root, review.ID, "request_changes", "needs a fix", findings); err != nil {
		t.Fatal(err)
	}

	fixCoordinator := activateAgentDevDispatch(t, engine, "coordinator", "P1-fix-plan-1", 2)
	appendQueueAndCommit(t, dir, "- [ ] P1-F1 add edge regression\n", "plan phase one fix")
	fixPlan := AgentDevPlan{PhaseID: "P1", Items: []AgentDevPlanItem{{ID: "P1-F1", Title: "Add edge regression", AcceptanceCriteria: []string{"edge test passes"}}}}
	if _, err := ApplyAgentDevPlan(engine.Store.Root, fixCoordinator.ID, fixPlan); err != nil {
		t.Fatal(err)

	}
	fix := activateAgentDevDispatch(t, engine, "implementer", "P1-F1", 1)
	replaceQueueAndCommit(t, dir, "[ ] P1-F1", "[x] P1-F1", "fix-edge-regression", true)
	if _, err := ApplyReportDetailed(engine.Store.Root, fix.ID, "complete", "fixed", nil); err != nil {
		t.Fatal(err)
	}

	approval := activateAgentDevDispatch(t, engine, "reviewer", "P1-R1", 2)
	replaceQueueAndCommit(t, dir, "[ ] P1-R1", "[x] P1-R1", "approve-phase-one", true)
	if _, err := ApplyReportDetailed(engine.Store.Root, approval.ID, "approve", "approved", nil); err != nil {
		t.Fatal(err)
	}

	done := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-2", 3)
	if _, err := ApplyAgentDevPlan(engine.Store.Root, done.ID, AgentDevPlan{Done: true}); err != nil {
		t.Fatal(err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	ss := state.Stages["delivery"]
	if ss.Status != StageSucceeded || ss.AgentDev.Status != agentDevComplete || len(ss.AgentDev.Phases) != 1 {
		t.Fatalf("stage=%#v", ss)
	}
	phase := ss.AgentDev.Phases[0]
	if phase.Status != agentDevComplete || phase.ReviewItem.Status != "complete" || len(phase.Items) != 2 {
		t.Fatalf("phase=%#v", phase)
	}
}

func TestAgentDevConfigValidationAndDefaults(t *testing.T) {
	loaded, _, _ := initAgentDevTest(t)
	stage := loaded.Config.Stages[0]
	if stage.AgentDev.MaxPhases != 20 || stage.AgentDev.MaxItemsPerPhase != 100 || stage.AgentDev.MaxNoProgressReviews != 2 {
		t.Fatalf("agent_dev defaults=%#v", stage.AgentDev)
	}
}

func TestAgentDevTickFireAndForgetCoordinator(t *testing.T) {
	_, engine, _ := initAgentDevTest(t)
	engine.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{}}, nil }
	var sent dispatch.Options
	engine.DispatchAgent = func(options dispatch.Options) (dispatch.Report, error) {
		sent = options
		return dispatch.Report{Target: "%42"}, nil
	}
	engine.Now = func() time.Time { return time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC) }
	done, err := engine.Tick(context.Background())
	if err != nil || done {
		t.Fatalf("done=%t err=%v", done, err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	dev := state.Stages["delivery"].AgentDev
	if dev == nil || dev.CurrentRole != "coordinator" || dev.CurrentDispatchID == "" {
		t.Fatalf("agent_dev=%#v", dev)
	}
	if sent.Session != "coordinator" || !strings.Contains(sent.Prompt, "workflow plan-report") || !strings.Contains(sent.Prompt, dev.CurrentDispatchID) || !strings.Contains(sent.Prompt, "停止協議") {
		t.Fatalf("dispatch=%#v", sent)
	}
	last, ok, err := LastDispatch(engine.Store, dev.CurrentDispatchID)
	if err != nil || !ok || last.Status != "sent" || last.Role != "coordinator" {
		t.Fatalf("last=%#v ok=%t err=%v", last, ok, err)
	}
}

func TestAgentDevWaitingDispatchStopsOnApprovalPrompt(t *testing.T) {
	_, engine, _ := initAgentDevTest(t)
	record := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-1", 1)
	record.Target = "%42"
	record.Timestamp = engine.Now()
	record.Status = "sent"
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	engine.CapturePane = func(string, int) (string, error) {
		return "Do you want to proceed?\n  1. Yes\n❯ 2. No\n", nil
	}
	done, err := engine.Tick(context.Background())
	if err != nil || done {
		t.Fatalf("done=%t err=%v", done, err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	ss := state.Stages["delivery"]
	if ss.Status != StageBlocked || state.Status != "needs_user" || !strings.Contains(ss.Error, "prompt") {
		t.Fatalf("state=%#v stage=%#v", state, ss)
	}
}

func TestAgentDevQuotaWaitPersistsAndResumesSameDispatch(t *testing.T) {
	_, engine, _ := initAgentDevTest(t)
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 3, 23, 0, 0, 0, location)
	engine.Now = func() time.Time { return now }
	record := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-1", 1)
	record.Target = "%42"
	record.Timestamp = now
	record.Status = "sent"
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	raw := "You've hit your session limit · resets 2am (Asia/Taipei)\nWhat do you want to do?\n❯ 1. Stop and wait for limit to reset\n  2. Upgrade your plan\n"
	captures := 0
	engine.CapturePane = func(string, int) (string, error) {
		captures++
		return raw, nil
	}
	var keys []string
	engine.SendKeys = func(target string, sent []string) error {
		if target != "%42" {
			t.Fatalf("target=%q", target)
		}
		keys = append(keys, sent...)
		return nil
	}
	if done, err := engine.Tick(context.Background()); err != nil || done {
		t.Fatalf("done=%t err=%v", done, err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	dev := state.Stages["delivery"].AgentDev
	if state.Status != agentDevWaitingQuota || dev.QuotaWait == nil || !dev.QuotaWait.PromptAnswered {
		t.Fatalf("state=%#v dev=%#v", state, dev)
	}
	if len(keys) != 1 || keys[0] != "Enter" {
		t.Fatalf("keys=%#v", keys)
	}
	if want := time.Date(2026, 8, 4, 2, 1, 0, 0, location); !dev.QuotaWait.NextCheckAt.Equal(want) {
		t.Fatalf("next_check_at=%s want=%s", dev.QuotaWait.NextCheckAt, want)
	}

	if err := engine.Store.Update(func(interrupted *State) error {
		interrupted.Status = "stopped"
		interrupted.Desired = "running"
		interrupted.Reason = "runner_context_canceled"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err = engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != agentDevWaitingQuota || state.Stages["delivery"].AgentDev.QuotaWait == nil {
		t.Fatalf("quota wait did not survive runner restart state: %#v", state)
	}
	if !state.FinishedAt.IsZero() {
		t.Fatalf("waiting workflow retained terminal finished_at: %s", state.FinishedAt)
	}
	if captures != 1 {
		t.Fatalf("captured before reset: %d", captures)
	}

	now = state.Stages["delivery"].AgentDev.QuotaWait.NextCheckAt
	engine.CapturePane = func(string, int) (string, error) { return "Claude Code ready\n❯", nil }
	var resumed dispatch.Options
	engine.DispatchAgent = func(options dispatch.Options) (dispatch.Report, error) {
		resumed = options
		return dispatch.Report{Target: "%42"}, nil
	}
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err = engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	dev = state.Stages["delivery"].AgentDev
	if state.Status != "running" || dev.QuotaWait != nil || dev.CurrentDispatchID != record.ID {
		t.Fatalf("state=%#v dev=%#v", state, dev)
	}
	if resumed.Session != "coordinator" || resumed.Prompt == "" || !strings.Contains(resumed.Prompt, record.ID) {
		t.Fatalf("resumed=%#v", resumed)
	}
	last, ok, err := LastDispatch(engine.Store, record.ID)
	if err != nil || !ok || !last.Timestamp.Equal(now) || last.Attempt != 1 {
		t.Fatalf("last=%#v ok=%t err=%v", last, ok, err)
	}
}

func TestAgentDevCodexQuotaWaitSurvivesRunnerRestartAndResumesSameAttempt(t *testing.T) {
	loaded, engine, _ := initAgentDevTest(t)
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 13, 11, 55, 0, 0, location)
	engine.Now = func() time.Time { return now }
	record := activateAgentDevDispatch(t, engine, "coordinator", "P3-fix-plan-48", 52)
	record.Target = "%169"
	record.Runtime = "codex"
	record.Timestamp = now
	record.Status = "sent"
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Update(func(state *State) error {
		stage := state.Stages["delivery"]
		stage.Error = "session coordinator is busy; refusing to dispatch"
		state.Stages["delivery"] = stage
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	raw := "› Act as the Coordinator.\n■ You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 18th, 2026 9:34\nAM.\n› Summarize recent commits\n~/w/ndt/mxcp-flow · main · Context 0% used\n"
	captures := 0
	engine.CapturePane = func(target string, _ int) (string, error) {
		captures++
		if target != "%169" {
			t.Fatalf("target=%q", target)
		}
		return raw, nil
	}
	keys := 0
	engine.SendKeys = func(string, []string) error { keys++; return nil }
	if done, err := engine.Tick(context.Background()); err != nil || done {
		t.Fatalf("done=%t err=%v", done, err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	wait := state.Stages["delivery"].AgentDev.QuotaWait
	wantReset := time.Date(2026, 8, 18, 9, 34, 0, 0, location)
	if state.Status != agentDevWaitingQuota || !strings.Contains(state.Reason, "Codex quota exhausted") || strings.Contains(state.Reason, "busy") || wait == nil || wait.Provider != "codex" || !wait.ResetAt.Equal(wantReset) || wait.DispatchID != record.ID || wait.Session != "coordinator" || wait.Attempt != 52 || wait.Role != "coordinator" || wait.WorkItem != "P3-fix-plan-48" || !wait.PromptAnswered {
		t.Fatalf("state=%#v wait=%#v", state, wait)
	}
	if wait.Timezone != "Asia/Taipei" || keys != 0 || captures != 1 {
		t.Fatalf("timezone=%q keys=%d captures=%d", wait.Timezone, keys, captures)
	}

	restarted, err := NewEngine(loaded, engine.Store.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = func() time.Time { return now }
	restarted.CapturePane = func(string, int) (string, error) {
		captures++
		return raw, nil
	}
	if _, err := restarted.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if captures != 1 {
		t.Fatalf("runner restart captured before deadline: %d", captures)
	}

	now = wait.NextCheckAt
	var resumed dispatch.Options
	resumeCalls := 0
	restarted.DispatchAgent = func(options dispatch.Options) (dispatch.Report, error) {
		resumeCalls++
		resumed = options
		return dispatch.Report{Target: "%169"}, nil
	}
	if _, err := restarted.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err = restarted.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	dev := state.Stages["delivery"].AgentDev
	if state.Status != "running" || dev.QuotaWait != nil || dev.CurrentDispatchID != record.ID || dev.CurrentRole != "coordinator" || dev.CurrentWorkItem != "P3-fix-plan-48" {
		t.Fatalf("state=%#v dev=%#v", state, dev)
	}
	if resumed.Session != "coordinator" || resumed.QuotaResume == nil || resumed.QuotaResume.Provider != "codex" || !resumed.QuotaResume.ResetAt.Equal(wantReset) || !resumed.QuotaResume.ResumeAt.Equal(now) || !strings.Contains(resumed.Prompt, record.ID) {
		t.Fatalf("resumed=%#v", resumed)
	}
	last, ok, err := LastDispatch(restarted.Store, record.ID)
	if err != nil || !ok || last.Attempt != 52 || last.Status != "sent" || !last.Timestamp.Equal(now) {
		t.Fatalf("last=%#v ok=%t err=%v", last, ok, err)
	}
	restarted.CapturePane = func(string, int) (string, error) { return "OpenAI Codex\nWorking (1s · esc to interrupt)\n", nil }
	if _, err := restarted.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if resumeCalls != 1 {
		t.Fatalf("same dispatch resumed %d times", resumeCalls)
	}
	events, err := os.ReadFile(restarted.Store.EventsPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(events), `"type":"agent_dev_quota_wait"`) || !strings.Contains(string(events), `"type":"agent_dev_quota_resumed"`) {
		t.Fatalf("quota lifecycle events missing: %s", events)
	}
}

func TestAgentDevCodexQuotaResumeCrashDoesNotRedispatch(t *testing.T) {
	loaded, engine, _ := initAgentDevTest(t)
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 18, 9, 35, 0, 0, location)
	engine.Now = func() time.Time { return now }
	record := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-1", 7)
	record.Target = "%169"
	record.Runtime = "codex"
	record.Status = "sent"
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	stage := state.Stages["delivery"]
	stage.AgentDev.QuotaWait = &AgentDevQuotaWait{Provider: "codex", Target: "%169", Session: "coordinator", DispatchID: record.ID, Attempt: 7, Role: "coordinator", WorkItem: "phase-plan-1", Timezone: "Asia/Taipei", ObservedAt: now.Add(-5 * 24 * time.Hour), ResetAt: now.Add(-time.Minute), NextCheckAt: now, PromptAnswered: true}
	stage.Error = "Codex quota exhausted"
	state.Status = agentDevWaitingQuota
	state.Reason = stage.Error
	state.Stages["delivery"] = stage
	if err := engine.Store.Write(state); err != nil {
		t.Fatal(err)
	}
	engine.DispatchAgent = func(dispatch.Options) (dispatch.Report, error) {
		panic("simulated runner crash after durable sending marker")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected simulated crash")
			}
		}()
		_, _ = engine.Tick(context.Background())
	}()
	last, ok, err := LastDispatch(engine.Store, record.ID)
	if err != nil || !ok || last.Status != "sending" {
		t.Fatalf("last=%#v ok=%t err=%v", last, ok, err)
	}

	restarted, err := NewEngine(loaded, engine.Store.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	dispatches := 0
	restarted.DispatchAgent = func(dispatch.Options) (dispatch.Report, error) {
		dispatches++
		return dispatch.Report{}, nil
	}
	state, err = restarted.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	stage = state.Stages["delivery"]
	if state.Status != "needs_user" || stage.Status != StageBlocked || !strings.Contains(stage.Error, "indeterminate dispatch") || dispatches != 0 {
		t.Fatalf("state=%#v stage=%#v dispatches=%d", state, stage, dispatches)
	}
}

func TestAgentDevCodexQuotaFormatDriftFailsClosedWithoutInput(t *testing.T) {
	_, engine, _ := initAgentDevTest(t)
	record := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-1", 1)
	record.Target = "%169"
	record.Runtime = "codex"
	record.Status = "sent"
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	engine.CapturePane = func(string, int) (string, error) {
		return "■ You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again later.\n› Suggestion\n/work · Context 0% used\n", nil
	}
	inputs := 0
	engine.SendKeys = func(string, []string) error { inputs++; return nil }
	engine.PasteText = func(string, string, bool) error { inputs++; return nil }
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	stage := state.Stages["delivery"]
	if state.Status != "needs_user" || stage.Status != StageBlocked || stage.AgentDev.QuotaWait != nil || !strings.Contains(stage.Error, "unrecognized usage-limit") || inputs != 0 {
		t.Fatalf("state=%#v stage=%#v inputs=%d", state, stage, inputs)
	}
}

func TestAgentDevClaudeQuotaMalformedResetFailsClosedWithoutSelection(t *testing.T) {
	_, engine, _ := initAgentDevTest(t)
	record := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-1", 1)
	record.Target = "%42"
	record.Status = "sent"
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	engine.CapturePane = func(string, int) (string, error) {
		return "You've hit your session limit · resets whenever (Asia/Taipei)\nWhat do you want to do?\n❯ 1. Stop and wait for limit to reset\n  2. Upgrade your plan\n", nil
	}
	keys := 0
	engine.SendKeys = func(string, []string) error { keys++; return nil }
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	stage := state.Stages["delivery"]
	if state.Status != "needs_user" || stage.Status != StageBlocked || stage.AgentDev.QuotaWait != nil || keys != 0 {
		t.Fatalf("state=%#v stage=%#v keys=%d", state, stage, keys)
	}
}

func TestAgentDevQuotaResetExtendsWhenProviderStillLimits(t *testing.T) {
	_, engine, _ := initAgentDevTest(t)
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 4, 2, 1, 0, 0, location)
	engine.Now = func() time.Time { return now }
	record := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-1", 1)
	record.Target = "%42"
	record.Status = "sent"
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	ss := state.Stages["delivery"]
	ss.AgentDev.QuotaWait = &AgentDevQuotaWait{Provider: "claude", Target: "%42", ResetAt: now.Add(-time.Minute), NextCheckAt: now, PromptAnswered: true}
	ss.Error = "waiting quota"
	state.Stages["delivery"] = ss
	state.Status = agentDevWaitingQuota
	if err := engine.Store.Write(state); err != nil {
		t.Fatal(err)
	}
	quotaRaw := "You've hit your session limit · resets 3am (Asia/Taipei)\nWhat do you want to do?\n❯ 1. Stop and wait for limit to reset\n  2. Upgrade your plan\n"
	limited := false
	engine.CapturePane = func(string, int) (string, error) {
		if limited {
			return quotaRaw, nil
		}
		return "Claude Code ready\n❯", nil
	}
	engine.DispatchAgent = func(dispatch.Options) (dispatch.Report, error) {
		limited = true
		return dispatch.Report{Target: "%42"}, errors.New("target is waiting on choice_prompt prompt")
	}
	answered := 0
	engine.SendKeys = func(string, []string) error { answered++; return nil }
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err = engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	wait := state.Stages["delivery"].AgentDev.QuotaWait
	if state.Status != agentDevWaitingQuota || wait == nil || answered != 1 {
		t.Fatalf("state=%#v wait=%#v answered=%d", state, wait, answered)
	}
	want := time.Date(2026, 8, 4, 3, 1, 0, 0, location)
	if !wait.NextCheckAt.Equal(want) {
		t.Fatalf("next_check_at=%s want=%s", wait.NextCheckAt, want)
	}
}

func TestAgentDevRecoversPreviouslyBlockedQuotaPrompt(t *testing.T) {
	_, engine, _ := initAgentDevTest(t)
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 3, 10, 3, 0, 0, location)
	engine.Now = func() time.Time { return now }
	record := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-1", 1)
	record.Target = "%42"
	record.Status = "sent"
	record.Timestamp = time.Date(2026, 8, 3, 0, 53, 0, 0, location)
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	ss := state.Stages["delivery"]
	ss.Status = StageBlocked
	ss.Error = "agent_dev target %42 is waiting on choice_prompt prompt"
	state.Stages["delivery"] = ss
	state.Status = "needs_user"
	state.Reason = ss.Error
	if err := engine.Store.Write(state); err != nil {
		t.Fatal(err)
	}
	engine.CapturePane = func(string, int) (string, error) {
		return "You've hit your session limit · resets 2am (Asia/Taipei)\nWhat do you want to do?\n❯ 1. Stop and wait for limit to reset\n  2. Upgrade your plan\n", nil
	}
	answered := 0
	engine.SendKeys = func(string, []string) error { answered++; return nil }
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err = engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != agentDevWaitingQuota || state.Stages["delivery"].Status != StageRunning || answered != 1 {
		t.Fatalf("state=%#v answered=%d", state, answered)
	}
	wait := state.Stages["delivery"].AgentDev.QuotaWait
	wantReset := time.Date(2026, 8, 3, 2, 0, 0, 0, location)
	if wait == nil || !wait.ObservedAt.Equal(record.Timestamp) || !wait.ResetAt.Equal(wantReset) || !wait.NextCheckAt.Equal(now) {
		t.Fatalf("wait=%#v want reset=%s next=%s", wait, wantReset, now)
	}
}

func TestAgentDevCorrectsLegacyQuotaDateAndResumes(t *testing.T) {
	_, engine, _ := initAgentDevTest(t)
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 3, 16, 20, 0, 0, location)
	engine.Now = func() time.Time { return now }
	record := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-1", 1)
	record.Target = "%42"
	record.Status = "sent"
	record.Timestamp = time.Date(2026, 8, 3, 0, 53, 0, 0, location)
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	ss := state.Stages["delivery"]
	ss.AgentDev.QuotaWait = &AgentDevQuotaWait{
		Provider:       "claude",
		Target:         "%42",
		ResetAt:        time.Date(2026, 8, 4, 2, 0, 0, 0, location),
		NextCheckAt:    time.Date(2026, 8, 4, 2, 1, 0, 0, location),
		PromptAnswered: true,
	}
	state.Stages["delivery"] = ss
	state.Status = agentDevWaitingQuota
	if err := engine.Store.Write(state); err != nil {
		t.Fatal(err)
	}
	engine.CapturePane = func(string, int) (string, error) { return "Claude Code ready\n❯", nil }
	engine.DispatchAgent = func(dispatch.Options) (dispatch.Report, error) {
		return dispatch.Report{Target: "%42"}, nil
	}
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err = engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	dev := state.Stages["delivery"].AgentDev
	if state.Status != "running" || dev.QuotaWait != nil || dev.CurrentDispatchID != record.ID {
		t.Fatalf("legacy quota did not resume: state=%#v dev=%#v", state, dev)
	}
}

func TestAgentDevDeferredDispatchRestoresPendingItem(t *testing.T) {
	_, engine, _ := initAgentDevTest(t)
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	state.Stages["delivery"] = StageState{
		Status: StageRunning,
		AgentDev: &AgentDevState{Status: agentDevActive, Phases: []PhaseState{{
			ID:         "P1",
			Title:      "Feature",
			Status:     agentDevActive,
			Items:      []WorkItemState{{ID: "P1-W1", Title: "Implement", Status: "pending"}},
			ReviewItem: WorkItemState{ID: "P1-R1", Kind: "review", Title: "Review", Status: "pending"},
		}}},
	}
	if err := engine.Store.Write(state); err != nil {
		t.Fatal(err)
	}
	appendQueueAndCommit(t, engine.Loaded.Config.Workspace.Root, "- [ ] P1-W1 implement\n- [ ] P1-R1 review\n", "seed queue")
	engine.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{}}, nil }
	engine.DispatchAgent = func(dispatch.Options) (dispatch.Report, error) {
		return dispatch.Report{}, errors.New("agent did not remain idle")
	}
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err = engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	item := state.Stages["delivery"].AgentDev.Phases[0].Items[0]
	if item.Status != "pending" || state.Stages["delivery"].AgentDev.CurrentDispatchID != "" {
		t.Fatalf("item=%#v stage=%#v", item, state.Stages["delivery"])
	}
}

func TestAgentDevRestartBlocksIndeterminateSendingDispatch(t *testing.T) {
	loaded, engine, _ := initAgentDevTest(t)
	record := activateAgentDevDispatch(t, engine, "coordinator", "phase-plan-1", 1)
	record.Status = "sending"
	if err := engine.Store.Dispatch(record); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewEngine(loaded, engine.Store.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	state, err := restarted.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "needs_user" || state.Stages["delivery"].Status != StageBlocked || !strings.Contains(state.Reason, "indeterminate dispatch") {
		t.Fatalf("state=%#v stage=%#v", state, state.Stages["delivery"])
	}
}
