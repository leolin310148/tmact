package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/tmux"
)

func loadTestWorkflow(t *testing.T, body string) Loaded {
	t.Helper()
	dir := t.TempDir()
	path := writeConfig(t, dir, body)
	loaded, err := Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestSchedulerCommandHumanLifecycle(t *testing.T) {
	loaded := loadTestWorkflow(t, minimalConfig("", `  - id: machine
    type: command
    argv: [/usr/bin/true]
  - id: approve
    type: human
    needs: [machine]
    outcomes: {yes: success, no: failed}
    input:
      note: {type: string, required: true}
`))
	root := filepath.Join(t.TempDir(), "runs")
	engine, err := NewEngine(loaded, root, true)
	if err != nil {
		t.Fatal(err)
	}
	done, err := engine.Tick(context.Background())
	if err != nil || done {
		t.Fatalf("tick1 done=%t err=%v", done, err)
	}
	state, _ := engine.Store.Read()
	if state.Stages["machine"].Status != StageSucceeded {
		t.Fatalf("state=%#v", state.Stages)
	}
	done, err = engine.Tick(context.Background())
	if err != nil || done {
		t.Fatalf("tick2 done=%t err=%v", done, err)
	}
	state, _ = engine.Store.Read()
	if state.Status != "needs_user" || state.Stages["approve"].Status != StageWaitingHuman {
		t.Fatalf("state=%#v", state)
	}
	if err := ResolveHuman(root, state.RunID, "", "approve", "yes", map[string]string{"note": "ship"}); err != nil {
		t.Fatal(err)
	}
	done, err = engine.Tick(context.Background())
	if err != nil || !done {
		t.Fatalf("tick3 done=%t err=%v", done, err)
	}
	state, _ = engine.Store.Read()
	if state.Status != "succeeded" {
		t.Fatalf("status=%s", state.Status)
	}
}

func TestCommandRetryAndEvidenceEnvironment(t *testing.T) {
	t.Setenv("WORKFLOW_ALLOWED", "visible")
	t.Setenv("WORKFLOW_SECRET", "hidden")
	loaded := loadTestWorkflow(t, `version: 2
workspace: {root: .}
defaults: {timeout: 5s, retry: {max_attempts: 2}}
stages:
  - id: env
    type: command
    argv: [/usr/bin/env]
    inherit_env: [WORKFLOW_ALLOWED]
  - id: fail
    type: command
    needs: [env]
    argv: [/usr/bin/false]
`)
	root := filepath.Join(t.TempDir(), "runs")
	engine, err := NewEngine(loaded, root, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, _ := engine.Store.Read()
	ev := state.Stages["env"].Evidence
	if ev == nil || !strings.Contains(ev.Stdout, "WORKFLOW_ALLOWED=visible") || strings.Contains(ev.Stdout, "WORKFLOW_SECRET") {
		t.Fatalf("stdout=%q", ev.Stdout)
	}
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, _ = engine.Store.Read()
	if state.Stages["fail"].Status != StagePending || state.Stages["fail"].Attempt != 1 {
		t.Fatalf("fail=%#v", state.Stages["fail"])
	}
	done, err := engine.Tick(context.Background())
	if err != nil || !done {
		t.Fatalf("done=%t err=%v", done, err)
	}
	state, _ = engine.Store.Read()
	if state.Status != "failed" || state.Stages["fail"].Attempt != 2 {
		t.Fatalf("state=%#v", state)
	}
}

func TestProducerBindsNewRevision(t *testing.T) {
	loaded := loadTestWorkflow(t, `version: 2
workspace: {root: .}
revisions:
  files: {files: {paths: [.]}}
defaults: {timeout: 5s}
stages:
  - id: produce
    type: command
    argv: [/usr/bin/touch, generated.txt]
    bind_revisions: [files]
    produces_revisions: [files]
`)
	root := filepath.Join(t.TempDir(), "runs")
	engine, err := NewEngine(loaded, root, true)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := engine.Store.Read()
	done, err := engine.Tick(context.Background())
	if err != nil || !done {
		t.Fatalf("done=%t err=%v", done, err)
	}
	after, _ := engine.Store.Read()
	if before.Revisions["files"] == after.Revisions["files"] {
		t.Fatal("producer did not update revision")
	}
	if after.Stages["produce"].BoundRevisions["files"] != after.Revisions["files"] {
		t.Fatalf("bound=%v current=%v", after.Stages["produce"].BoundRevisions, after.Revisions)
	}
}

func TestReportDurabilityStaleAndRecovery(t *testing.T) {
	loaded := loadTestWorkflow(t, `version: 2
workspace: {root: .}
agents_config: agents.yaml
actors: {reviewer: {agent: reviewer}}
revisions:
  spec: {files: {paths: [spec.txt]}}
defaults: {timeout: 5s}
stages:
  - id: review
    type: agent
    actor: reviewer
    prompt: review
    bind_revisions: [spec]
    outcomes: {accept: success, revise: retry}
`)
	if err := os.WriteFile(filepath.Join(loaded.Config.Workspace.Root, "spec.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "runs")
	engine, err := NewEngine(loaded, root, true)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := engine.Store.Read()
	current, err := ComputeRevisions(loaded.Config, templateData(state))
	if err != nil {
		t.Fatal(err)
	}
	state.Revisions = current
	ss := state.Stages["review"]
	ss.Status = StageWaitingReport
	ss.Attempt = 1
	ss.DispatchID = state.RunID + ".review.1"
	ss.BoundRevisions = bindValues(current, []string{"spec"})
	state.Stages["review"] = ss
	if err := engine.Store.Write(state); err != nil {
		t.Fatal(err)
	}
	d := Dispatch{ID: ss.DispatchID, RunID: state.RunID, Stage: "review", Attempt: 1, Actor: "reviewer", Status: "sent", Revisions: ss.BoundRevisions}
	if err := engine.Store.Dispatch(d); err != nil {
		t.Fatal(err)
	}
	report, err := ApplyReport(root, d.ID, "accept", "looks good")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := ApplyReport(root, d.ID, "revise", "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.ID != report.ID {
		t.Fatalf("duplicate report changed: %#v %#v", report, duplicate)
	}
	state, _ = engine.Store.Read()
	ss = state.Stages["review"]
	ss.Status = StageWaitingReport
	ss.Attempt = 2
	ss.DispatchID = state.RunID + ".review.2"
	ss.BoundRevisions = bindValues(state.Revisions, []string{"spec"})
	state.Stages["review"] = ss
	if err := engine.Store.Write(state); err != nil {
		t.Fatal(err)
	}
	d = Dispatch{ID: ss.DispatchID, RunID: state.RunID, Stage: "review", Attempt: 2, Actor: "reviewer", Status: "sent", Revisions: ss.BoundRevisions}
	if err := engine.Store.Dispatch(d); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(loaded.Config.Workspace.Root, "spec.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyReport(root, d.ID, "accept", "stale"); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("error=%v", err)
	}
	state, _ = engine.Store.Read()
	ss = state.Stages["review"]
	ss.Status = StageRunning
	ss.Attempt = 3
	ss.DispatchID = state.RunID + ".review.3"
	state.Stages["review"] = ss
	if err := engine.Store.Write(state); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Dispatch(Dispatch{ID: ss.DispatchID, RunID: state.RunID, Stage: "review", Attempt: 3, Status: "sending"}); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewEngine(loaded, root, true)
	if err != nil {
		t.Fatal(err)
	}
	state, _ = recovered.Store.Read()
	if state.Stages["review"].Status != StageWaitingReport {
		t.Fatalf("recovered=%#v", state.Stages["review"])
	}
}

func TestTemplateUsesLowercaseStateFields(t *testing.T) {
	state := State{Stages: map[string]StageState{"verify": {ID: "verify", Evidence: &Evidence{Summary: "passed"}}}}
	got, err := Render("test", "{{ .stages.verify.evidence.summary }}", templateData(state))
	if err != nil {
		t.Fatal(err)
	}
	if got != "passed" {
		t.Fatalf("got=%q", got)
	}
}

func TestAgentExecutorPreflightAndDurablePrompt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agents.yaml"), []byte("agents:\n  - {name: reviewer, target: work:0.0, type: codex}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := writeConfig(t, dir, `version: 2
workspace: {root: .}
agents_config: agents.yaml
actors: {reviewer: {agent: reviewer}}
defaults: {timeout: 5s, idle_after: 1ms}
stages:
  - id: review
    type: agent
    actor: reviewer
    prompt: Review this.
    outcomes: {accept: success}
`)
	loaded, err := Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(loaded, filepath.Join(t.TempDir(), "runs"), true)
	if err != nil {
		t.Fatal(err)
	}
	engine.ListPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{{CurrentPath: dir, PanePID: 42}}, nil }
	engine.CapturePane = func(string, int) (string, error) { return "ready", nil }
	engine.ProcessRuntime = func(int) panestatus.RuntimeDetection {
		return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeCodex}
	}
	engine.Sleep = func(time.Duration) {}
	var sent []string
	engine.PasteText = func(_ string, text string, _ bool) error { sent = append(sent, text); return nil }
	done, err := engine.Tick(context.Background())
	if err != nil || done {
		t.Fatalf("done=%t err=%v", done, err)
	}
	state, _ := engine.Store.Read()
	ss := state.Stages["review"]
	if ss.Status != StageWaitingReport || ss.DispatchID == "" {
		t.Fatalf("stage=%#v", ss)
	}
	if len(sent) != 2 || sent[0] != "/clear" || !strings.Contains(sent[1], "--dispatch-id "+ss.DispatchID) {
		t.Fatalf("sent=%#v", sent)
	}
	for _, want := range []string{"tmact workflow status --id " + state.RunID, "--store-dir \"" + engine.Store.Root + "\" --json", "`desired` 是 `stopped`", "不要回報"} {
		if !strings.Contains(sent[1], want) {
			t.Fatalf("prompt missing %q: %s", want, sent[1])
		}
	}
	last, ok, err := LastDispatch(engine.Store, ss.DispatchID)
	if err != nil || !ok || last.Status != "sent" {
		t.Fatalf("dispatch=%#v ok=%t err=%v", last, ok, err)
	}
}

func TestAgentPermissionPromptNeedsUserWithoutSending(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agents.yaml"), []byte("agents:\n  - {name: reviewer, target: work:0.0, type: codex}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := writeConfig(t, dir, `version: 2
workspace: {root: .}
agents_config: agents.yaml
actors: {reviewer: {agent: reviewer}}
defaults: {timeout: 5s}
stages:
  - {id: review, type: agent, actor: reviewer, prompt: review, outcomes: {accept: success}}
`)
	loaded, err := Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(loaded, filepath.Join(t.TempDir(), "runs"), true)
	if err != nil {
		t.Fatal(err)
	}
	engine.ListPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{{CurrentPath: dir, PanePID: 42}}, nil }
	engine.CapturePane = func(string, int) (string, error) { return "Allow this command?\n  1. Yes\n❯ 2. No\n", nil }
	engine.ProcessRuntime = func(int) panestatus.RuntimeDetection {
		return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeCodex}
	}
	called := false
	engine.PasteText = func(string, string, bool) error { called = true; return nil }
	if done, err := engine.Tick(context.Background()); err != nil || done {
		t.Fatalf("done=%t err=%v", done, err)
	}
	state, _ := engine.Store.Read()
	if state.Status != "needs_user" || state.Stages["review"].Status != StageBlocked || called {
		t.Fatalf("state=%#v called=%t", state, called)
	}
}

func TestNeedsUserDoesNotScheduleAdditionalStages(t *testing.T) {
	loaded := loadTestWorkflow(t, minimalConfig("", `  - id: approve
    type: human
    outcomes: {yes: success}
  - id: should_wait
    type: command
    argv: [/usr/bin/touch, should-not-run]
`))
	engine, err := NewEngine(loaded, filepath.Join(t.TempDir(), "runs"), true)
	if err != nil {
		t.Fatal(err)
	}
	if done, err := engine.Tick(context.Background()); err != nil || done {
		t.Fatalf("done=%t err=%v", done, err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "needs_user" || state.Stages["approve"].Status != StageWaitingHuman {
		t.Fatalf("state=%#v", state)
	}
	if state.Stages["should_wait"].Status != StagePending {
		t.Fatalf("additional stage scheduled while waiting for user: %#v", state.Stages["should_wait"])
	}
	if _, err := os.Stat(filepath.Join(loaded.Config.Workspace.Root, "should-not-run")); !os.IsNotExist(err) {
		t.Fatalf("command ran while waiting for user: %v", err)
	}
	if done, err := engine.Tick(context.Background()); err != nil || done {
		t.Fatalf("second tick done=%t err=%v", done, err)
	}
	state, err = engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Stages["should_wait"].Status != StagePending {
		t.Fatalf("additional stage scheduled on a later needs_user tick: %#v", state.Stages["should_wait"])
	}
}

func TestFailedDependencySkipPropagatesTransitively(t *testing.T) {
	loaded := loadTestWorkflow(t, minimalConfig("", `  - id: apply
    type: command
    argv: [/usr/bin/true]
  - id: validate
    type: command
    needs: [apply]
    argv: [/usr/bin/true]
  - id: qa
    type: command
    needs: [validate]
    argv: [/usr/bin/touch, qa-must-not-run]
`))
	engine, err := NewEngine(loaded, filepath.Join(t.TempDir(), "runs"), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Update(func(state *State) error {
		ss := state.Stages["apply"]
		ss.Status = StageBlocked
		ss.Error = "implementation needs user"
		state.Stages["apply"] = ss
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	done, err := engine.Tick(context.Background())
	if err != nil || !done {
		t.Fatalf("done=%t err=%v", done, err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if state.Stages["validate"].Status != StageSkipped || state.Stages["qa"].Status != StageSkipped {
		t.Fatalf("dependency failure did not propagate: %#v", state.Stages)
	}
	if _, err := os.Stat(filepath.Join(loaded.Config.Workspace.Root, "qa-must-not-run")); !os.IsNotExist(err) {
		t.Fatalf("downstream command ran after dependency failure: %v", err)
	}
}

func TestUnavailableLaunchActorDefersWithoutNeedsUser(t *testing.T) {
	cases := map[string]string{
		"busy":          "session work is already running codex but it is busy working; refusing to dispatch",
		"unknown state": "session work is running codex but pane state is unknown; refusing to clear or dispatch until it is explicitly input-ready",
	}
	for name, message := range cases {
		t.Run(name, func(t *testing.T) {
			loaded := loadTestWorkflow(t, `version: 2
workspace: {root: .}
actors:
  reviewer:
    launch: {runtime: codex, session: work, reuse: true}
defaults: {timeout: 5s, retry: {max_attempts: 2, backoff: 1ms}}
stages:
  - {id: review, type: agent, actor: reviewer, prompt: review, outcomes: {accept: success}}
`)
			engine, err := NewEngine(loaded, filepath.Join(t.TempDir(), "runs"), true)
			if err != nil {
				t.Fatal(err)
			}
			engine.ListLayout = func() (tmux.Layout, error) {
				return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
			}
			engine.ListSessionPanes = func(string) ([]tmux.Pane, error) {
				return []tmux.Pane{{CurrentPath: loaded.Config.Workspace.Root}}, nil
			}
			engine.DispatchAgent = func(dispatch.Options) (dispatch.Report, error) {
				return dispatch.Report{}, errors.New(message)
			}
			if done, err := engine.Tick(context.Background()); err != nil || done {
				t.Fatalf("done=%t err=%v", done, err)
			}
			state, err := engine.Store.Read()
			if err != nil {
				t.Fatal(err)
			}
			ss := state.Stages["review"]
			if state.Status != "running" || ss.Status != StagePending || ss.Attempt != 0 || ss.NextAttemptAt.IsZero() {
				t.Fatalf("unavailable actor should defer without consuming an attempt: state=%#v stage=%#v", state, ss)
			}
		})
	}
}

func TestStoppedWorkflowRejectsAgentReport(t *testing.T) {
	loaded := loadTestWorkflow(t, `version: 2
workspace: {root: .}
actors:
  reviewer:
    launch: {runtime: codex, session: work}
defaults: {timeout: 5s}
stages:
  - {id: review, type: agent, actor: reviewer, prompt: review, outcomes: {accept: success}}
`)
	root := filepath.Join(t.TempDir(), "runs")
	engine, err := NewEngine(loaded, root, true)
	if err != nil {
		t.Fatal(err)
	}
	state, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	ss := state.Stages["review"]
	ss.Status = StageWaitingReport
	ss.Attempt = 1
	ss.DispatchID = state.RunID + ".review.0.1"
	state.Stages["review"] = ss
	state.Desired = "stopped"
	if err := engine.Store.Write(state); err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Dispatch(Dispatch{ID: ss.DispatchID, RunID: state.RunID, Stage: "review", Attempt: 1, Actor: "reviewer", Status: "sent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyReport(root, ss.DispatchID, "accept", "late report"); err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("error=%v", err)
	}
}

func TestSameTargetActorsAreMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agents.yaml"), []byte("agents:\n  - {name: shared, target: work:0.0, type: codex}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := writeConfig(t, dir, `version: 2
workspace: {root: .}
agents_config: agents.yaml
actors:
  first: {agent: shared}
  second: {agent: shared}
defaults: {timeout: 5s, idle_after: 1ms, max_parallel: 2}
stages:
  - {id: one, type: agent, actor: first, prompt: one, outcomes: {ok: success}}
  - {id: two, type: agent, actor: second, prompt: two, outcomes: {ok: success}}
`)
	loaded, err := Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(loaded, filepath.Join(t.TempDir(), "runs"), true)
	if err != nil {
		t.Fatal(err)
	}
	engine.ListPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{{CurrentPath: dir, PanePID: 1}}, nil }
	engine.CapturePane = func(string, int) (string, error) { return "ready", nil }
	engine.ProcessRuntime = func(int) panestatus.RuntimeDetection {
		return panestatus.RuntimeDetection{Runtime: panestatus.RuntimeCodex}
	}
	engine.Sleep = func(time.Duration) {}
	engine.PasteText = func(string, string, bool) error { return nil }
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, _ := engine.Store.Read()
	waiting := 0
	for _, id := range []string{"one", "two"} {
		if state.Stages[id].Status == StageWaitingReport {
			waiting++
		}
	}
	if waiting != 1 {
		t.Fatalf("same target dispatched %d stages: %#v", waiting, state.Stages)
	}
}

func TestStoreRejectsStaleWriterAndRunnerLockIsExclusive(t *testing.T) {
	loaded := loadTestWorkflow(t, minimalConfig("", "  - {id: ok, type: command, argv: [/usr/bin/true]}\n"))
	engine, err := NewEngine(loaded, filepath.Join(t.TempDir(), "runs"), true)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := engine.Store.Read()
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Store.Update(func(s *State) error { s.Status = "paused"; return nil }); err != nil {
		t.Fatal(err)
	}
	stale.Status = "running"
	if err := engine.Store.Write(stale); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("error=%v", err)
	}
	release, err := engine.Store.AcquireRunnerLock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Store.AcquireRunnerLock(); !errors.Is(err, ErrRunnerActive) {
		t.Fatalf("duplicate runner lock error=%v", err)
	}
	active, err := engine.Store.RunnerActive()
	if err != nil || !active {
		t.Fatalf("active=%t err=%v", active, err)
	}
	if _, err := NewEngine(loaded, engine.Store.Root, true); !errors.Is(err, ErrRunnerActive) {
		t.Fatalf("NewEngine while runner active error=%v", err)
	}
	release()
	active, err = engine.Store.RunnerActive()
	if err != nil || active {
		t.Fatalf("after release active=%t err=%v", active, err)
	}
}
