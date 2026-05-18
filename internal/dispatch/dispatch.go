package dispatch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"tmact/internal/panestate"
	"tmact/internal/panestatus"
	"tmact/internal/tmux"
)

const (
	StatusPlanned = "planned"
	StatusOK      = "ok"
	StatusFailed  = "failed"

	defaultReadyTimeout = 30 * time.Second
	pollInterval        = time.Second
	captureLines        = 200
	clearDelay          = 2 * time.Second
)

var supportedAgents = map[string]bool{
	panestatus.RuntimeClaude:  true,
	panestatus.RuntimeCodex:   true,
	panestatus.RuntimeGemini:  true,
	panestatus.RuntimeCopilot: true,
}

// SupportedAgents lists the agent launchers dispatch-work understands.
func SupportedAgents() []string {
	names := make([]string, 0, len(supportedAgents))
	for name := range supportedAgents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Options configures a single dispatch-work run.
type Options struct {
	Session      string
	Dir          string
	Agent        string
	Prompt       string
	Execute      bool
	ReadyTimeout time.Duration
}

// Step is one planned or executed operation in a dispatch.
type Step struct {
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status"`
}

// Report is the outcome of a dispatch-work run.
type Report struct {
	Session         string `json:"session"`
	Target          string `json:"target,omitempty"`
	Dir             string `json:"dir"`
	Agent           string `json:"agent"`
	Prompt          string `json:"prompt"`
	SessionExisted  bool   `json:"session_existed"`
	AgentWasRunning bool   `json:"agent_was_running"`
	Execute         bool   `json:"execute"`
	Steps           []Step `json:"steps"`
}

// Deps holds the tmux side effects so callers can be tested without a live session.
type Deps struct {
	ListLayout       func() (tmux.Layout, error)
	ListSessionPanes func(string) ([]tmux.Pane, error)
	CapturePane      func(string, int) (string, error)
	NewSession       func(session, window, cwd string, command []string) error
	PasteText        func(target, text string, enter bool) error
	ProcessRuntime   func(int) panestatus.RuntimeDetection
	Sleep            func(time.Duration)
	Now              func() time.Time
}

// DefaultDeps wires Deps to the real tmux helpers.
func DefaultDeps() Deps {
	return Deps{
		ListLayout:       tmux.ListLayout,
		ListSessionPanes: tmux.ListSessionPanes,
		CapturePane:      tmux.CapturePane,
		NewSession:       tmux.NewSession,
		PasteText:        tmux.PasteText,
		ProcessRuntime:   panestatus.DetectChildProcessRuntime,
		Sleep:            time.Sleep,
		Now:              time.Now,
	}
}

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

func dispatchNew(opts Options, deps Deps, report Report) (Report, error) {
	create := Step{
		Name:   "create-session",
		Detail: fmt.Sprintf("tmux new-session -d -s %s -c %s %s", opts.Session, opts.Dir, opts.Agent),
		Status: StatusPlanned,
	}
	ready := Step{Name: "wait-ready", Detail: readyDetail(opts), Status: StatusPlanned}
	send := Step{Name: "send-prompt", Detail: promptDetail(opts.Prompt), Status: StatusPlanned}
	steps := []Step{create, ready, send}

	if !opts.Execute {
		report.Steps = steps
		return report, nil
	}

	if err := deps.NewSession(opts.Session, "", opts.Dir, []string{opts.Agent}); err != nil {
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

	if err := waitReady(opts, deps, target); err != nil {
		steps[1].Status = StatusFailed
		report.Steps = steps
		return report, err
	}
	steps[1].Status = StatusOK

	if err := deps.PasteText(target, opts.Prompt, true); err != nil {
		steps[2].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("send prompt: %w", err)
	}
	steps[2].Status = StatusOK
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

	if err := deps.PasteText(target, opts.Prompt, true); err != nil {
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

	if err := deps.PasteText(target, opts.Prompt, true); err != nil {
		steps[2].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("send prompt: %w", err)
	}
	steps[2].Status = StatusOK
	report.Steps = steps
	return report, nil
}

func waitReady(opts Options, deps Deps, target string) error {
	deadline := deps.Now().Add(opts.ReadyTimeout)
	for {
		panes, err := deps.ListSessionPanes(opts.Session)
		if err != nil {
			return fmt.Errorf("wait for %s: %w", opts.Agent, err)
		}
		pane, ok := findPane(panes, target)
		if !ok {
			return fmt.Errorf("wait for %s: pane %s disappeared", opts.Agent, target)
		}
		raw, err := deps.CapturePane(target, captureLines)
		if err != nil {
			return fmt.Errorf("wait for %s: %w", opts.Agent, err)
		}
		classified := panestate.Classify(raw)
		if classified.Asking {
			return fmt.Errorf("%s startup is waiting on a prompt (%s); refusing to auto-confirm", opts.Agent, promptKind(classified))
		}
		runtime := detectRuntime(deps, pane, raw)
		if runtime == opts.Agent && classified.State != panestate.StateWorking {
			return nil
		}
		if !deps.Now().Before(deadline) {
			return fmt.Errorf("%s did not become ready within %s (runtime=%s state=%s)", opts.Agent, opts.ReadyTimeout, runtime, classified.State)
		}
		deps.Sleep(pollInterval)
	}
}

func detectRuntime(deps Deps, pane tmux.Pane, raw string) string {
	if deps.ProcessRuntime != nil {
		detected := deps.ProcessRuntime(pane.PanePID)
		if detected.Runtime != panestatus.RuntimeUnknown && detected.Runtime != "" {
			return detected.Runtime
		}
	}
	return panestatus.ClassifyRuntime(pane, raw).Runtime
}

func resolveSessionTarget(deps Deps, session string) (string, error) {
	panes, err := deps.ListSessionPanes(session)
	if err != nil {
		return "", err
	}
	if len(panes) == 0 {
		return "", fmt.Errorf("session %s has no panes", session)
	}
	return paneTarget(activePane(panes)), nil
}

func activePane(panes []tmux.Pane) tmux.Pane {
	for _, pane := range panes {
		if pane.Active && pane.WindowActive {
			return pane
		}
	}
	for _, pane := range panes {
		if pane.Active {
			return pane
		}
	}
	return panes[0]
}

func findPane(panes []tmux.Pane, target string) (tmux.Pane, bool) {
	for _, pane := range panes {
		if paneTarget(pane) == target {
			return pane, true
		}
	}
	return tmux.Pane{}, false
}

func paneTarget(pane tmux.Pane) string {
	if pane.PaneID != "" {
		return pane.PaneID
	}
	return fmt.Sprintf("%s:%d.%d", pane.Session, pane.WindowIndex, pane.PaneIndex)
}

func isAgentRuntime(runtime string) bool {
	switch runtime {
	case panestatus.RuntimeClaude, panestatus.RuntimeCodex, panestatus.RuntimeGemini, panestatus.RuntimeCopilot:
		return true
	default:
		return false
	}
}

func promptKind(classified panestate.Result) string {
	if classified.InteractivePrompt != nil && classified.InteractivePrompt.Type != "" {
		return classified.InteractivePrompt.Type
	}
	return "interactive prompt"
}

func promptDetail(prompt string) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if len(prompt) > 60 {
		return prompt[:57] + "..."
	}
	return prompt
}

func readyDetail(opts Options) string {
	return fmt.Sprintf("wait up to %s for %s to be ready", opts.ReadyTimeout, opts.Agent)
}
