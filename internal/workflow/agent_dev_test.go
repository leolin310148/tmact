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
