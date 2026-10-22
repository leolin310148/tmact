package dispatch

import (
	"context"
	"sort"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/modelroute"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/panewait"
	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	StatusPlanned = "planned"
	StatusOK      = "ok"
	StatusFailed  = "failed"

	defaultReadyTimeout     = 30 * time.Second
	pollInterval            = time.Second
	defaultWaitPollInterval = 500 * time.Millisecond
	captureLines            = 200
	clearDelay              = 2 * time.Second
	// DefaultReadySettleDelay gives cold-starting terminal UIs, especially
	// Codex, a short stable idle window before the first prompt is pasted.
	DefaultReadySettleDelay = 1500 * time.Millisecond
	// DefaultWaitTimeout bounds opt-in post-dispatch waiting.
	DefaultWaitTimeout = 5 * time.Minute
	// DefaultWaitSettle requires the result prompt to remain input-ready before
	// dispatch-work reports it.
	DefaultWaitSettle = time.Second
	// DefaultResultLines controls the final pane capture included after waiting.
	DefaultResultLines = 200

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
type PlanState struct {
	SessionExisted bool   `json:"session_existed"`
	SessionID      string `json:"session_id,omitempty"`
	PaneID         string `json:"pane_id,omitempty"`
	Runtime        string `json:"runtime,omitempty"`
	Dir            string `json:"dir,omitempty"`
}

type Options struct {
	PlanState   *PlanState
	CapturePlan bool
	PlanFiles   *[]modelroute.TaskFile

	Session   string
	Target    string
	Dir       string
	Agent     string
	Model     string
	Effort    string
	AutoModel bool
	// RoutingTask keeps ask capability IDs and reply instructions out of decide.
	RoutingTask      string
	RoutingDir       string
	RoutingTrace     modelroute.Trace
	RoutingPreflight bool
	// Selection is carried from an explicit read-only preflight (ask).
	Selection *modelroute.Selection
	// ProbeAutoModel requests read-only routing context for a peer caller.
	ProbeAutoModel bool
	// RestartAgent is set after routing preflight to replace an idle agent.
	RestartAgent bool
	Prompt       string
	Execute      bool
	ReadyTimeout time.Duration
	ReadySettle  time.Duration
	TrustFolder  bool
	// NoAutoTrust disables the implicit TrustFolder opt-in for an empty dir or
	// a linked git worktree. MCP sets it: workspace trust stays off that wire.
	NoAutoTrust bool
	// NoClear reuses an idle same-agent pane without sending /clear first, so
	// the new prompt continues the pane's current conversation.
	NoClear bool
	// Queue lets a prompt be typed into a same-agent pane that is busy
	// working, so Claude queues it and Codex holds it for its next tool call
	// or steers it into the current turn.
	// It requires NoClear, is limited to Claude and Codex, and never applies
	// to a pane showing a prompt.
	Queue               bool
	Wait                bool
	WaitTimeout         time.Duration
	WaitSettle          time.Duration
	ResultLines         int
	Context             context.Context
	WorkspaceLeaseOwner string
	QuotaResume         *QuotaResume
}

// QuotaResume is an internal capability used by durable workflows to reuse a
// Codex pane only after the exact persisted usage-limit deadline. The CLI does
// not expose it.
type QuotaResume struct {
	Provider string
	ResetAt  time.Time
	ResumeAt time.Time
}

// Step is one planned or executed operation in a dispatch.
type Step struct {
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
	Status string `json:"status"`
}

// WaitBaseline records the pane observation that proved the prompt left the
// input box. It is deliberately metadata-only; pane contents are captured only
// in the final, explicitly requested result.
type WaitBaseline struct {
	Accepted bool     `json:"accepted"`
	Evidence string   `json:"evidence"`
	State    string   `json:"state"`
	RawState string   `json:"raw_state"`
	LastLine string   `json:"last_line,omitempty"`
	Signals  []string `json:"signals,omitempty"`
}

// WaitOutcome is the terminal pane-state observation from bounded waiting.
// ConditionMet means input-ready was observed, not that the task succeeded.
type WaitOutcome struct {
	Target             string   `json:"target"`
	PaneID             string   `json:"pane_id,omitempty"`
	State              string   `json:"state"`
	RawState           string   `json:"raw_state"`
	Reason             string   `json:"reason"`
	ConditionMet       bool     `json:"condition_met"`
	TransitionObserved bool     `json:"transition_observed"`
	Samples            int      `json:"samples"`
	LastLine           string   `json:"last_line,omitempty"`
	Signals            []string `json:"signals,omitempty"`
	Tail               []string `json:"tail,omitempty"`
	Elapsed            string   `json:"elapsed"`
}

// WaitReport describes the opt-in post-submit wait. Outcome is absent in a
// dry-run, where Status remains planned.
type WaitReport struct {
	Status      string        `json:"status"`
	Timeout     string        `json:"timeout"`
	Settle      string        `json:"settle"`
	Baseline    *WaitBaseline `json:"baseline,omitempty"`
	Outcome     *WaitOutcome  `json:"outcome,omitempty"`
	ResultLines int           `json:"result_lines"`
}

// ResultReport is the final bounded pane capture after a wait. It is omitted
// when the pane disappeared before a result could be captured.
type ResultReport struct {
	Lines int    `json:"lines"`
	Text  string `json:"text"`
}

// Report is the outcome of a dispatch-work run.
type Report struct {
	ErrorCode    string                `json:"error_code,omitempty"`
	RoutingError *modelroute.Failure   `json:"routing_error,omitempty"`
	ApplyCommand string                `json:"apply_command,omitempty"`
	PlanFiles    []modelroute.TaskFile `json:"plan_files,omitempty"`
	PlanFile     string                `json:"plan_file,omitempty"`
	PlanState    *PlanState            `json:"plan_state,omitempty"`

	DispatchID           string                `json:"dispatch_id,omitempty"`
	Peer                 string                `json:"peer,omitempty"`
	Session              string                `json:"session"`
	Target               string                `json:"target,omitempty"`
	Dir                  string                `json:"dir"`
	Agent                string                `json:"agent"`
	Model                string                `json:"model,omitempty"`
	Effort               string                `json:"effort,omitempty"`
	Selection            *modelroute.Selection `json:"selection,omitempty"`
	RoutingInput         *modelroute.Input     `json:"routing_input,omitempty"`
	AutoModelSupported   bool                  `json:"auto_model_supported,omitempty"`
	LaunchOptionsVersion int                   `json:"launch_options_version,omitempty"`
	Prompt               string                `json:"prompt"`
	SessionExisted       bool                  `json:"session_existed"`
	AgentWasRunning      bool                  `json:"agent_was_running"`
	Execute              bool                  `json:"execute"`
	TrustFolder          bool                  `json:"trust_folder,omitempty"`
	TrustedFolder        bool                  `json:"trusted_folder,omitempty"`
	AutoTrust            string                `json:"auto_trust,omitempty"`
	Steps                []Step                `json:"steps"`
	Wait                 *WaitReport           `json:"wait,omitempty"`
	Result               *ResultReport         `json:"result,omitempty"`
	// Error is set only by --json callers reporting a failed dispatch.
	Error string `json:"error,omitempty"`
}

// Deps holds the tmux side effects so callers can be tested without a live session.
type Deps struct {
	FetchUsage         func(context.Context) agentusage.Snapshot
	ChooseModel        func(context.Context, modelroute.Input) (*modelroute.Selection, error)
	ListLayout         func() (tmux.Layout, error)
	ListSessionPanes   func(string) ([]tmux.Pane, error)
	CapturePane        func(string, int) (string, error)
	CapturePaneContext func(context.Context, string, int) (string, error)
	CapturePaneANSI    func(string, int) (string, error)
	NewSession         func(session, window, cwd string, command []string) error
	PasteText          func(target, text string, enter bool) error
	SendKeys           func(target string, keys []string) error
	ProcessRuntime     func(int) panestatus.RuntimeDetection
	Sleep              func(time.Duration)
	Now                func() time.Time
	WaitPane           func(context.Context, panewait.Options) (panewait.Report, error)
}

// DefaultDeps wires Deps to the real tmux helpers.
func DefaultDeps() Deps {
	return Deps{
		FetchUsage:         func(ctx context.Context) agentusage.Snapshot { return agentusage.Fetch(ctx) },
		ChooseModel:        modelroute.Choose,
		ListLayout:         tmux.ListLayout,
		ListSessionPanes:   tmux.ListSessionPanes,
		CapturePane:        tmux.CapturePane,
		CapturePaneContext: tmux.CapturePaneContext,
		CapturePaneANSI:    tmux.CapturePaneANSI,
		NewSession:         tmux.NewSession,
		PasteText:          tmux.PasteText,
		SendKeys:           tmux.SendKeys,
		ProcessRuntime:     panestatus.DetectChildProcessRuntime,
		Sleep:              time.Sleep,
		Now:                time.Now,
		WaitPane:           panewait.Run,
	}
}
