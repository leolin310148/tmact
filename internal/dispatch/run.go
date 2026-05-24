package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/panestatus"
)

// Run dispatches work using the real tmux helpers.
func Run(opts Options) (Report, error) {
	return RunWithDeps(opts, DefaultDeps())
}

// RunWithDeps dispatches work using the supplied dependencies.
func RunWithDeps(opts Options, deps Deps) (Report, error) {
	report := Report{
		Session: opts.Session,
		Dir:     opts.Dir,
		Agent:   opts.Agent,
		Prompt:  opts.Prompt,
		Execute: opts.Execute,
	}

	if strings.TrimSpace(opts.Session) == "" {
		return report, fmt.Errorf("session name is required")
	}
	if !supportedAgents[opts.Agent] {
		return report, fmt.Errorf("unsupported agent %q; want one of %s", opts.Agent, strings.Join(SupportedAgents(), ", "))
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return report, fmt.Errorf("prompt is required")
	}
	if strings.TrimSpace(opts.Dir) == "" {
		return report, fmt.Errorf("dir is required")
	}
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return report, fmt.Errorf("resolve dir: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return report, fmt.Errorf("dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return report, fmt.Errorf("dir %s is not a directory", dir)
	}
	opts.Dir = dir
	report.Dir = dir
	if opts.ReadyTimeout <= 0 {
		opts.ReadyTimeout = defaultReadyTimeout
	}
	if opts.ReadySettle < 0 {
		return report, fmt.Errorf("ready settle cannot be negative")
	}

	layout, err := deps.ListLayout()
	if err != nil {
		return report, err
	}
	report.SessionExisted = layout.Sessions[opts.Session]

	if report.SessionExisted {
		return dispatchExisting(opts, deps, report)
	}
	return dispatchNew(opts, deps, report)
}

// dispatchNew creates the session running the user's default shell, then
// launches the agent into that shell as a keystroke. Running the agent on top
// of a shell (rather than as the session's own process) means quitting the
// agent drops back to a live shell instead of tearing the session down, so a
// human can take over and run git or other commands.
func dispatchNew(opts Options, deps Deps, report Report) (Report, error) {
	create := Step{
		Name:   "create-session",
		Detail: fmt.Sprintf("tmux new-session -d -s %s -c %s (default shell)", opts.Session, opts.Dir),
		Status: StatusPlanned,
	}
	launch := Step{Name: "launch-agent", Detail: fmt.Sprintf("send %q to the shell", opts.Agent), Status: StatusPlanned}
	ready := Step{Name: "wait-ready", Detail: readyDetail(opts), Status: StatusPlanned}
	send := Step{Name: "send-prompt", Detail: promptDetail(opts.Prompt), Status: StatusPlanned}
	steps := []Step{create, launch, ready, send}

	if !opts.Execute {
		report.Steps = steps
		return report, nil
	}

	if err := deps.NewSession(opts.Session, "", opts.Dir, nil); err != nil {
		steps[0].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("create session: %w", err)
	}
	steps[0].Status = StatusOK

	target, err := resolveSessionTarget(deps, opts.Session)
	if err != nil {
		report.Steps = steps
		return report, err
	}
	report.Target = target

	if err := deps.PasteText(target, opts.Agent, true); err != nil {
		steps[1].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("launch %s: %w", opts.Agent, err)
	}
	steps[1].Status = StatusOK

	if err := waitReady(opts, deps, target); err != nil {
		steps[2].Status = StatusFailed
		report.Steps = steps
		return report, err
	}
	steps[2].Status = StatusOK

	if err := submitPrompt(opts, deps, target); err != nil {
		steps[3].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("send prompt: %w", err)
	}
	steps[3].Status = StatusOK
	report.Steps = steps
	return report, nil
}

func dispatchExisting(opts Options, deps Deps, report Report) (Report, error) {
	panes, err := deps.ListSessionPanes(opts.Session)
	if err != nil {
		return report, err
	}
	if len(panes) == 0 {
		return report, fmt.Errorf("session %s has no panes", opts.Session)
	}
	pane := activePane(panes)
	target := paneTarget(pane)
	report.Target = target

	raw, err := deps.CapturePane(target, captureLines)
	if err != nil {
		return report, err
	}
	runtime := detectRuntime(deps, pane, raw)
	classified := panestate.Classify(raw)

	switch {
	case runtime == opts.Agent:
		if classified.State == panestate.StateWorking {
			return report, fmt.Errorf("session %s is already running %s but it is busy working; refusing to dispatch", opts.Session, opts.Agent)
		}
		if classified.Asking {
			return report, fmt.Errorf("session %s is running %s but it is waiting on a prompt (%s); resolve it first", opts.Session, opts.Agent, promptKind(classified))
		}
		report.AgentWasRunning = true
		return dispatchReuse(opts, deps, report, target)
	case runtime == panestatus.RuntimeShell:
		return dispatchLaunch(opts, deps, report, target)
	case isAgentRuntime(runtime):
		return report, fmt.Errorf("session %s is already running a different agent (%s); requested %s", opts.Session, runtime, opts.Agent)
	default:
		return report, fmt.Errorf("session %s active pane runtime is %q; refusing to dispatch (expected %s or an idle shell)", opts.Session, runtime, opts.Agent)
	}
}

func dispatchReuse(opts Options, deps Deps, report Report, target string) (Report, error) {
	clear := Step{Name: "clear", Detail: "/clear", Status: StatusPlanned}
	send := Step{Name: "send-prompt", Detail: promptDetail(opts.Prompt), Status: StatusPlanned}
	steps := []Step{clear, send}

	if !opts.Execute {
		report.Steps = steps
		return report, nil
	}

	if err := deps.PasteText(target, "/clear", true); err != nil {
		steps[0].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("send /clear: %w", err)
	}
	steps[0].Status = StatusOK
	deps.Sleep(clearDelay)

	if err := submitPrompt(opts, deps, target); err != nil {
		steps[1].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("send prompt: %w", err)
	}
	steps[1].Status = StatusOK
	report.Steps = steps
	return report, nil
}

func dispatchLaunch(opts Options, deps Deps, report Report, target string) (Report, error) {
	launch := Step{Name: "launch-agent", Detail: fmt.Sprintf("send %q to the shell", opts.Agent), Status: StatusPlanned}
	ready := Step{Name: "wait-ready", Detail: readyDetail(opts), Status: StatusPlanned}
	send := Step{Name: "send-prompt", Detail: promptDetail(opts.Prompt), Status: StatusPlanned}
	steps := []Step{launch, ready, send}

	if !opts.Execute {
		report.Steps = steps
		return report, nil
	}

	if err := deps.PasteText(target, opts.Agent, true); err != nil {
		steps[0].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("launch %s: %w", opts.Agent, err)
	}
	steps[0].Status = StatusOK

	if err := waitReady(opts, deps, target); err != nil {
		steps[1].Status = StatusFailed
		report.Steps = steps
		return report, err
	}
	steps[1].Status = StatusOK

	if err := submitPrompt(opts, deps, target); err != nil {
		steps[2].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("send prompt: %w", err)
	}
	steps[2].Status = StatusOK
	report.Steps = steps
	return report, nil
}
