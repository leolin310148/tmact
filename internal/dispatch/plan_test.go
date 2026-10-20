package dispatch_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/modelroute"
	"github.com/leolin310148/tmact/internal/tmux"
)

func TestPlanGuardStopsBeforeSideEffects(t *testing.T) {
	for _, mode := range []string{"session-created", "session-removed", "pane-replaced", "file-changed"} {
		t.Run(mode, func(t *testing.T) {
			rec, deps := baseDeps()
			o := baseOpts()
			o.Dir = t.TempDir()
			o.Execute = true
			o.PlanState = &dispatch.PlanState{}
			switch mode {
			case "session-created":
				deps.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{o.Session: true}}, nil }
			case "session-removed":
				o.PlanState.SessionExisted = true
			case "pane-replaced":
				o.PlanState = &dispatch.PlanState{SessionExisted: true, SessionID: "$old", PaneID: "%old"}
				deps.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{o.Session: true}}, nil }
				deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
					return []tmux.Pane{{Session: o.Session, SessionID: "$new", PaneID: "%new", CurrentPath: o.Dir, CurrentCommand: "zsh", Active: true}}, nil
				}
				deps.CapturePane = func(string, int) (string, error) { return "$ ", nil }
			case "file-changed":
				path := filepath.Join(o.Dir, "task.md")
				os.WriteFile(path, []byte("before"), 0600)
				o.Prompt = path
				files, _ := modelroute.FingerprintPromptFiles(o.Prompt, o.Dir)
				o.PlanFiles = &files
				os.WriteFile(path, []byte("after"), 0600)
			}
			_, err := dispatch.RunWithDeps(o, deps)
			if err == nil || !strings.Contains(err.Error(), "stale") {
				t.Fatalf("err=%v", err)
			}
			if rec.newSessions > 0 || len(rec.pastes) > 0 || len(rec.keys) > 0 {
				t.Fatal("stale plan mutated tmux")
			}
		})
	}
}

func TestPlanApplyNeverCallsDecideOrQuota(t *testing.T) {
	_, deps := baseDeps()
	o := baseOpts()
	o.Agent = ""
	o.AutoModel = true
	o.CapturePlan = true
	calls := 0
	deps.FetchUsage = healthyQuota
	deps.ChooseModel = func(context.Context, modelroute.Input) (*modelroute.Selection, error) {
		calls++
		return &modelroute.Selection{Status: "selected", Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium"}, nil
	}
	r, err := dispatch.RunWithDeps(o, deps)
	if err != nil {
		t.Fatal(err)
	}
	o.AutoModel = false
	o.Agent = r.Agent
	o.Model = r.Model
	o.Effort = r.Effort
	o.Selection = r.Selection
	o.PlanState = r.PlanState
	o.PlanFiles = &r.PlanFiles
	deps.FetchUsage = func(context.Context) agentusage.Snapshot {
		t.Fatal("apply fetched quota")
		return agentusage.Snapshot{}
	}
	_, err = dispatch.RunWithDeps(o, deps)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("decide calls=%d", calls)
	}
}
