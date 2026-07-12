package dispatch

import (
	"sort"
	"time"

	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	StatusPlanned = "planned"
	StatusOK      = "ok"
	StatusFailed  = "failed"

	defaultReadyTimeout = 30 * time.Second
	pollInterval        = time.Second
	captureLines        = 200
	clearDelay          = 2 * time.Second
	// DefaultReadySettleDelay gives cold-starting terminal UIs, especially
	// Codex, a short stable idle window before the first prompt is pasted.
	DefaultReadySettleDelay = 1500 * time.Millisecond

	// submitSettleDelay is how long to wait after pasting/Enter before
	// checking whether the prompt actually left the input box.
	submitSettleDelay = 750 * time.Millisecond
	// submitRetries is how many recovery attempts to make when the prompt
	// has not started working yet — either re-sending Enter (cold start
	// swallowed it) or re-pasting (the agent UI dropped the paste).
	submitRetries = 5
)

var supportedAgents = map[string]bool{
	panestatus.RuntimeClaude: true,
	panestatus.RuntimeCodex:  true,
	panestatus.RuntimeGemini: true,
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
	Model        string
	Prompt       string
	Execute      bool
	ReadyTimeout time.Duration
	ReadySettle  time.Duration
	TrustFolder  bool
}

// Step is one planned or executed operation in a dispatch.
type Step struct {
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status"`
}

// Report is the outcome of a dispatch-work run.
type Report struct {
	Peer            string `json:"peer,omitempty"`
	Session         string `json:"session"`
	Target          string `json:"target,omitempty"`
	Dir             string `json:"dir"`
	Agent           string `json:"agent"`
	Model           string `json:"model,omitempty"`
	Prompt          string `json:"prompt"`
	SessionExisted  bool   `json:"session_existed"`
	AgentWasRunning bool   `json:"agent_was_running"`
	Execute         bool   `json:"execute"`
	TrustFolder     bool   `json:"trust_folder,omitempty"`
	TrustedFolder   bool   `json:"trusted_folder,omitempty"`
	Steps           []Step `json:"steps"`
}

// Deps holds the tmux side effects so callers can be tested without a live session.
type Deps struct {
	ListLayout       func() (tmux.Layout, error)
	ListSessionPanes func(string) ([]tmux.Pane, error)
	CapturePane      func(string, int) (string, error)
	CapturePaneANSI  func(string, int) (string, error)
	NewSession       func(session, window, cwd string, command []string) error
	PasteText        func(target, text string, enter bool) error
	SendKeys         func(target string, keys []string) error
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
		CapturePaneANSI:  tmux.CapturePaneANSI,
		NewSession:       tmux.NewSession,
		PasteText:        tmux.PasteText,
		SendKeys:         tmux.SendKeys,
		ProcessRuntime:   panestatus.DetectChildProcessRuntime,
		Sleep:            time.Sleep,
		Now:              time.Now,
	}
}
