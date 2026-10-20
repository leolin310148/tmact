package dispatch_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/modelroute"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/tmux"
)

func healthyQuota(context.Context) agentusage.Snapshot {
	return agentusage.Snapshot{GeneratedAt: time.Now(), Providers: []agentusage.ProviderUsage{{Provider: "codex", Windows: []agentusage.RateWindow{{Name: "weekly", UsedPercent: 20}}}}}
}

func TestAutoModelDryRunAndEffortLaunch(t *testing.T) {
	rec, deps := baseDeps()
	opts := baseOpts()
	opts.Agent = ""
	opts.AutoModel = true
	deps.FetchUsage = healthyQuota
	deps.ChooseModel = func(_ context.Context, in modelroute.Input) (*modelroute.Selection, error) {
		if in.Task != opts.Prompt || len(in.Candidates) != 3 {
			t.Fatalf("input=%+v", in)
		}
		return &modelroute.Selection{Status: "selected", Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium", Confidence: .9}, nil
	}
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if report.Agent != "codex" || report.Selection == nil || report.Effort != "medium" {
		t.Fatalf("report=%+v", report)
	}
	detail := stepDetail(t, report, "launch-agent")
	if !strings.Contains(detail, "model_reasoning_effort") || !strings.Contains(detail, "gpt-6.1-sol") {
		t.Fatal(detail)
	}
	if len(rec.pastes) > 0 || rec.newSessions > 0 {
		t.Fatal("dry-run mutated tmux")
	}
}

func TestAutoModelFailureNeverCreatesSession(t *testing.T) {
	for _, mode := range []string{"decide-error", "invalid-profile"} {
		t.Run(mode, func(t *testing.T) {
			rec, deps := baseDeps()
			opts := baseOpts()
			opts.AutoModel = true
			opts.Agent = ""
			opts.Execute = true
			opts.Dir = t.TempDir()
			deps.FetchUsage = healthyQuota
			deps.ChooseModel = func(context.Context, modelroute.Input) (*modelroute.Selection, error) {
				if mode == "decide-error" {
					return nil, errors.New("decision unavailable")
				}
				return &modelroute.Selection{Agent: "codex", Model: "bad; shell code", Effort: "medium"}, nil
			}
			if _, err := dispatch.RunWithDeps(opts, deps); err == nil {
				t.Fatal("expected failure")
			}
			if rec.newSessions != 0 || len(rec.pastes) > 0 {
				t.Fatal("failed selection mutated tmux")
			}
		})
	}
}

func TestAutoModelReusesExistingAgentWithoutDecide(t *testing.T) {
	rec, deps := baseDeps()
	opts := baseOpts()
	opts.Agent = ""
	opts.AutoModel = true
	opts.NoClear = true
	deps.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil }
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{claudePane()}, nil }
	deps.CapturePane = func(string, int) (string, error) {
		return "Claude Code\n❯\n⏵⏵ auto mode on (shift+tab to cycle) · ← for agents", nil
	}
	deps.FetchUsage = func(context.Context) agentusage.Snapshot {
		t.Fatal("reuse must not fetch quota")
		return agentusage.Snapshot{}
	}
	deps.ChooseModel = func(context.Context, modelroute.Input) (*modelroute.Selection, error) {
		t.Fatal("reuse must not decide")
		return nil, nil
	}
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if report.Agent != "claude" || !report.AgentWasRunning || report.Selection == nil || report.Selection.Status != "reused" {
		t.Fatalf("report=%+v", report)
	}
	if len(rec.pastes) > 0 {
		t.Fatal("dry-run pasted")
	}
}

func TestAutoModelProbeIsReadOnlyAndNeverRunsDecide(t *testing.T) {
	rec, deps := baseDeps()
	opts := baseOpts()
	opts.Agent = ""
	opts.ProbeAutoModel = true
	deps.FetchUsage = healthyQuota
	deps.ChooseModel = func(context.Context, modelroute.Input) (*modelroute.Selection, error) {
		t.Fatal("peer probe ran decide")
		return nil, nil
	}
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if !report.AutoModelSupported || report.RoutingInput == nil {
		t.Fatalf("report=%+v", report)
	}
	if len(rec.pastes) > 0 || rec.newSessions > 0 {
		t.Fatal("probe mutated tmux")
	}
	opts.Execute = true
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil {
		t.Fatal("executing probe must be rejected")
	}
}

func TestManualEffortLaunchAndValidation(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		t.Run(agent, func(t *testing.T) {
			_, deps := baseDeps()
			opts := baseOpts()
			opts.Agent = agent
			opts.Effort = "high"
			report, err := dispatch.RunWithDeps(opts, deps)
			if err != nil {
				t.Fatal(err)
			}
			detail := stepDetail(t, report, "launch-agent")
			if agent == "claude" && !strings.Contains(detail, "--effort 'high'") || agent == "codex" && !strings.Contains(detail, "model_reasoning_effort") {
				t.Fatal(detail)
			}
			opts.Effort = "high; echo unsafe"
			if _, err := dispatch.RunWithDeps(opts, deps); err == nil {
				t.Fatal("invalid effort accepted")
			}
		})
	}
}

func TestAutoModelReplacesIdleAgentAfterShellConfirmation(t *testing.T) {
	for _, oldAgent := range []string{"claude", "codex"} {
		t.Run(oldAgent, func(t *testing.T) {
			newAgent := "codex"
			model := "gpt-6.1-sol"
			if oldAgent == "codex" {
				newAgent = "claude"
				model = "claude-sonnet-5-5"
			}
			rec, deps := baseDeps()
			opts := baseOpts()
			opts.Dir = t.TempDir()
			opts.Agent = ""
			opts.AutoModel = true
			opts.Execute = true
			opts.ReadySettle = 0
			stage := "old"
			now := time.Now()
			deps.Now = func() time.Time { return now }
			deps.Sleep = func(d time.Duration) { now = now.Add(d) }
			deps.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil }
			deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
				p := claudePane()
				if stage == "shell" {
					p.CurrentCommand = "zsh"
				}
				return []tmux.Pane{p}, nil
			}
			deps.ProcessRuntime = func(int) panestatus.RuntimeDetection {
				runtime := oldAgent
				if stage == "shell" {
					runtime = "shell"
				} else if stage == "new" || stage == "working" {
					runtime = newAgent
				}
				return panestatus.RuntimeDetection{Runtime: runtime}
			}
			raw := func() (string, error) {
				if stage == "shell" {
					return "$ ", nil
				}
				agent := oldAgent
				if stage == "new" || stage == "working" {
					agent = newAgent
				}
				if stage == "working" {
					return "Working... esc to interrupt", nil
				}
				suffix := ""
				if stage == "exit-draft" {
					suffix = "/exit"
				}
				if agent == "claude" {
					return "Claude Code\n❯ " + suffix + "\n⏵⏵ auto mode on (shift+tab to cycle) · ← for agents", nil
				}
				return "OpenAI Codex\n› " + suffix + "\n/work · Context 0% used\n", nil
			}
			deps.CapturePane = func(string, int) (string, error) { return raw() }
			deps.CapturePaneANSI = deps.CapturePane
			deps.FetchUsage = func(context.Context) agentusage.Snapshot {
				return agentusage.Snapshot{GeneratedAt: time.Now(), Providers: []agentusage.ProviderUsage{{Provider: "claude", Windows: []agentusage.RateWindow{{Name: "weekly", UsedPercent: 1}, {Name: "session", UsedPercent: 1}}}, {Provider: "codex", Windows: []agentusage.RateWindow{{Name: "weekly", UsedPercent: 1}}}}}
			}
			deps.ChooseModel = func(context.Context, modelroute.Input) (*modelroute.Selection, error) {
				if stage != "old" {
					t.Fatal("decision ran after exit")
				}
				return &modelroute.Selection{Status: "selected", Agent: newAgent, Model: model, Effort: "medium", Confidence: .9}, nil
			}
			deps.PasteText = func(target, text string, enter bool) error {
				rec.pastes = append(rec.pastes, paste{target, text, enter})
				switch {
				case text == "/exit":
					if stage != "old" || enter {
						t.Fatal("unguarded exit")
					}
					stage = "exit-draft"
				case strings.HasPrefix(text, newAgent+" --model"):
					if stage != "shell" {
						t.Fatal("launched before old process exited")
					}
					stage = "new"
				case text == opts.Prompt:
					if stage != "new" {
						t.Fatal("prompt before launch")
					}
					stage = "working"
				default:
					t.Fatalf("unexpected paste %q", text)
				}
				return nil
			}
			deps.SendKeys = func(string, []string) error {
				if stage != "exit-draft" {
					t.Fatal("unexpected Enter")
				}
				stage = "shell"
				return nil
			}
			report, err := dispatch.RunWithDeps(opts, deps)
			if err != nil {
				t.Fatal(err)
			}
			if report.Agent != newAgent || stepStatus(t, report, "exit-agent") != "ok" || stepStatus(t, report, "wait-shell") != "ok" {
				t.Fatalf("report=%+v", report)
			}
			for _, p := range rec.pastes {
				if p.text == "/clear" {
					t.Fatal("restart must replace process instead of clearing")
				}
			}
		})
	}
}

func TestAutoModelRestartRefusesBusyAgentBeforeDeciding(t *testing.T) {
	rec, deps := baseDeps()
	opts := baseOpts()
	opts.AutoModel = true
	opts.Agent = ""
	deps.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil }
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{claudePane()}, nil }
	deps.CapturePane = func(string, int) (string, error) { return "Claude Code\nWorking... esc to interrupt", nil }
	deps.FetchUsage = func(context.Context) agentusage.Snapshot {
		t.Fatal("busy agent must not trigger routing")
		return agentusage.Snapshot{}
	}
	if _, err := dispatch.RunWithDeps(opts, deps); err == nil {
		t.Fatal("busy agent replaced")
	}
	if len(rec.pastes) > 0 {
		t.Fatal("busy agent received keys")
	}
}

func TestAutoRoutingUsesTargetCWDForReferencedFiles(t *testing.T) {
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Dir = t.TempDir()
	opts.Agent = ""
	opts.AutoModel = true
	opts.Prompt = "Read task.md and implement it."
	targetDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(targetDir, "task.md"), []byte("Actual task in target cwd"), 0600); err != nil {
		t.Fatal(err)
	}
	deps.ListLayout = func() (tmux.Layout, error) { return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil }
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) {
		return []tmux.Pane{{Session: "work", PaneID: "%1", CurrentPath: targetDir, CurrentCommand: "zsh", Active: true, WindowActive: true}}, nil
	}
	deps.CapturePane = func(string, int) (string, error) { return "$ ", nil }
	deps.FetchUsage = healthyQuota
	deps.ChooseModel = func(_ context.Context, in modelroute.Input) (*modelroute.Selection, error) {
		if in.Task != opts.Prompt || len(in.Files) != 1 || in.Files[0].Content != "Actual task in target cwd" {
			t.Fatalf("input=%+v", in)
		}
		return &modelroute.Selection{Status: "selected", Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium"}, nil
	}
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if report.Prompt != opts.Prompt {
		t.Fatal("file expansion changed the delivered prompt")
	}
}

func TestDispatchOutcomeLogCorrelatesWithoutCapabilityOrPrompt(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "routing.jsonl")
	t.Setenv("TMACT_ROUTING_LOG", logPath)
	_, deps := baseDeps()
	opts := baseOpts()
	opts.Prompt = "private instruction q_capability_should_not_be_logged"
	opts.RoutingTrace = modelroute.Trace{DispatchID: "d_test", Command: "ask", Session: opts.Session, Execute: false}
	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if report.DispatchID != "d_test" {
		t.Fatal(report.DispatchID)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), opts.Prompt) || strings.Contains(string(raw), "q_capability") {
		t.Fatal("capability leaked into outcome log")
	}
	entries, err := modelroute.ReadLog("d_test", 50, true)
	if err != nil || len(entries) != 1 || entries[0].Outcome.Status != "planned" {
		t.Fatalf("records=%+v err=%v", entries, err)
	}
}
