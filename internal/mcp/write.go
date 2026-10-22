package mcp

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/statusd"
)

const (
	// Cloudflare drops a request with no origin response after ~100s, and a
	// timed-out tool call invites a duplicate retry, so writes stay well under.
	writeCallTimeout     = 80 * time.Second
	dispatchReadyTimeout = 45 * time.Second
	maxWaitTimeout       = 80 * time.Second
	defaultWaitTimeout   = 60 * time.Second
	defaultWaitSettle    = 2 * time.Second
	defaultPollInterval  = time.Second
	sendTextCheckDelay   = 1500 * time.Millisecond
	maxPromptBytes       = 32 << 10
)

var sessionNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// agentRuntimes may receive text; shell panes never do, since that would be
// arbitrary command execution.
var agentRuntimes = map[string]bool{"codex": true, "claude": true, "gemini": true}

// NormalizeRoots validates "~/..." dispatch roots and returns them cleaned.
func NormalizeRoots(roots []string) ([]string, error) {
	var out []string
	for _, r := range roots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		c := path.Clean(r)
		if !strings.HasPrefix(c, "~/") || hasDotDot(c) {
			return nil, fmt.Errorf("dispatch root %q must be a path under ~/", r)
		}
		out = append(out, c)
	}
	return out, nil
}

// allowedDir checks dir lexically against roots and returns its cleaned
// "~/..." form. It does not touch the filesystem, so it also applies to peers.
func allowedDir(dir string, roots []string) (string, error) {
	c := path.Clean(strings.TrimSpace(dir))
	if !strings.HasPrefix(c, "~/") || hasDotDot(c) {
		return "", fmt.Errorf("dir must look like ~/w/<repo> or another path under %s", strings.Join(roots, ", "))
	}
	for _, root := range roots {
		if c == root || strings.HasPrefix(c, root+"/") {
			return c, nil
		}
	}
	return "", fmt.Errorf("dir %q is outside the allowed roots %s", c, strings.Join(roots, ", "))
}

func hasDotDot(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// resolveLocalDir expands a lexically allowed dir on this machine and
// re-checks containment after following symlinks, so ~/w/link -> / is refused.
func resolveLocalDir(dir string, roots []string, home string) (string, error) {
	real, err := filepath.EvalSymlinks(filepath.Join(home, strings.TrimPrefix(dir, "~/")))
	if err != nil {
		return "", fmt.Errorf("dir %s: %w", dir, err)
	}
	for _, root := range roots {
		rootReal, err := filepath.EvalSymlinks(filepath.Join(home, strings.TrimPrefix(root, "~/")))
		if err != nil {
			continue
		}
		if real == rootReal || strings.HasPrefix(real, rootReal+string(filepath.Separator)) {
			return real, nil
		}
	}
	return "", fmt.Errorf("dir %s resolves outside the allowed roots", dir)
}

var writeAnnotations = &sdk.ToolAnnotations{
	ReadOnlyHint:    false,
	DestructiveHint: boolPtr(true),
	IdempotentHint:  false,
	OpenWorldHint:   boolPtr(false),
}

func addWriteTools(s *sdk.Server, t tools) {
	sdk.AddTool(s, &sdk.Tool{
		Name: "dispatch_work",
		Description: "Start work for a coding agent: create or reuse a tmux session in an allowed directory, launch the agent if needed, and submit the prompt. " +
			"Returns as soon as the prompt is accepted; follow up with wait_pane and capture_pane. " +
			"Reusing an idle session clears its conversation unless continue_conversation is true. Use for 派工 / 叫 agent 做 X / 開一個 agent.",
		Annotations: writeAnnotations,
	}, t.dispatchWork)
	sdk.AddTool(s, &sdk.Tool{
		Name:        "send_text",
		Description: "Type a follow-up message into an idle coding-agent pane and press Enter. Refused for shell panes, busy agents, and agents showing a prompt. Use for 跟它說 / 補一句 / 追問.",
		Annotations: writeAnnotations,
	}, t.sendText)
}

type dispatchIn struct {
	Session              string `json:"session" jsonschema:"tmux session name: letters, digits, _ or -"`
	Dir                  string `json:"dir" jsonschema:"working directory in ~/ form, e.g. ~/w/tmact"`
	Agent                string `json:"agent" jsonschema:"codex, claude, or gemini"`
	Prompt               string `json:"prompt" jsonschema:"the task for the agent"`
	Model                string `json:"model,omitempty" jsonschema:"optional model name"`
	Peer                 string `json:"peer,omitempty" jsonschema:"peer machine name from list_sessions; omit for the MCP host"`
	ContinueConversation bool   `json:"continue_conversation,omitempty" jsonschema:"reuse an idle same-agent pane without clearing it (MCP host only)"`
}

type dispatchOut struct {
	Peer            string   `json:"peer,omitempty"`
	Session         string   `json:"session"`
	Pane            string   `json:"pane,omitempty"`
	Dir             string   `json:"dir"`
	Agent           string   `json:"agent"`
	Model           string   `json:"model,omitempty"`
	SessionExisted  bool     `json:"session_existed"`
	AgentWasRunning bool     `json:"agent_was_running"`
	Steps           []string `json:"steps"`
	Next            string   `json:"next"`
}

func (t tools) dispatchWork(ctx context.Context, _ *sdk.CallToolRequest, in dispatchIn) (*sdk.CallToolResult, dispatchOut, error) {
	if !sessionNamePattern.MatchString(in.Session) {
		return nil, dispatchOut{}, fmt.Errorf("session must match %s", sessionNamePattern)
	}
	if !agentRuntimes[in.Agent] {
		return nil, dispatchOut{}, fmt.Errorf("agent must be one of %s", strings.Join(dispatch.SupportedAgents(), ", "))
	}
	if strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > maxPromptBytes {
		return nil, dispatchOut{}, fmt.Errorf("prompt must be non-empty and at most %d bytes", maxPromptBytes)
	}
	model, err := dispatch.ValidateModel(in.Agent, in.Model)
	if err != nil {
		return nil, dispatchOut{}, err
	}
	dir, err := allowedDir(in.Dir, t.roots)
	if err != nil {
		return nil, dispatchOut{}, err
	}
	if in.Peer != "" && in.ContinueConversation {
		return nil, dispatchOut{}, errors.New("continue_conversation is only supported on the MCP host; use send_text for a peer pane")
	}

	ctx, cancel := context.WithTimeout(ctx, writeCallTimeout)
	defer cancel()
	opts := dispatch.Options{
		Session:      in.Session,
		Agent:        in.Agent,
		Model:        model,
		Prompt:       in.Prompt,
		NoClear:      in.ContinueConversation,
		ReadyTimeout: dispatchReadyTimeout,
		ReadySettle:  dispatch.DefaultReadySettleDelay,
		Context:      ctx,
		// Fixed by design: MCP never dry-runs (ChatGPT confirms the call),
		// never accepts workspace trust (not even the empty-dir/worktree
		// auto-trust), and never blocks on the agent.
		Execute:     true,
		TrustFolder: false,
		NoAutoTrust: true,
		Wait:        false,
	}
	var report dispatch.Report
	if in.Peer == "" {
		home, err := t.deps.Home()
		if err != nil {
			return nil, dispatchOut{}, err
		}
		if opts.Dir, err = resolveLocalDir(dir, t.roots, home); err != nil {
			return nil, dispatchOut{}, err
		}
		report, err = t.deps.Dispatch(ctx, opts)
		if err != nil {
			return nil, dispatchOut{}, fmt.Errorf("dispatch: %w", err)
		}
	} else {
		opts.Dir = dir // the peer expands ~ against its own home
		report, err = t.deps.DispatchPeer(ctx, in.Peer, opts)
		if err != nil {
			return nil, dispatchOut{}, fmt.Errorf("dispatch on %s: %w", in.Peer, err)
		}
	}
	out := dispatchOut{
		Peer: in.Peer, Session: report.Session, Dir: report.Dir, Agent: report.Agent, Model: report.Model,
		SessionExisted: report.SessionExisted, AgentWasRunning: report.AgentWasRunning,
		Steps: []string{},
		Next:  "Call wait_pane with this pane, then capture_pane to read the result.",
	}
	if report.Target != "" {
		out.Pane = report.Target
		if in.Peer != "" {
			out.Pane = in.Peer + statusd.PeerSeparator + report.Target
		}
	}
	for _, st := range report.Steps {
		out.Steps = append(out.Steps, st.Name+": "+st.Status)
	}
	return nil, out, nil
}

type sendTextIn struct {
	Pane string `json:"pane" jsonschema:"pane id from list_panes"`
	Text string `json:"text" jsonschema:"message to type; Enter is pressed after it"`
}

type sendTextOut struct {
	Pane       string `json:"pane"`
	Accepted   bool   `json:"accepted"`
	StateAfter string `json:"state_after,omitempty"`
	Next       string `json:"next"`
}

func (t tools) sendText(ctx context.Context, _ *sdk.CallToolRequest, in sendTextIn) (*sdk.CallToolResult, sendTextOut, error) {
	if strings.TrimSpace(in.Text) == "" || len(in.Text) > maxPromptBytes {
		return nil, sendTextOut{}, fmt.Errorf("text must be non-empty and at most %d bytes", maxPromptBytes)
	}
	snap, err := t.snapshot(ctx)
	if err != nil {
		return nil, sendTextOut{}, err
	}
	key, pane, ok := findPane(snap, strings.TrimSpace(in.Pane))
	if !ok {
		return nil, sendTextOut{}, fmt.Errorf("pane %q is not in the statusd snapshot; use an id from list_panes", in.Pane)
	}
	switch {
	case !agentRuntimes[pane.Runtime]:
		return nil, sendTextOut{}, fmt.Errorf("pane %s runs %q, not a coding agent; send_text only types into codex/claude/gemini", key, pane.Runtime)
	case pane.Asking:
		return nil, sendTextOut{}, fmt.Errorf("pane %s is showing a prompt; it must be answered first", key)
	case pane.Running || !(pane.InputReady || pane.Idle):
		return nil, sendTextOut{}, fmt.Errorf("pane %s is busy (state %s); wait_pane until it is idle", key, pane.State)
	case pane.Stale:
		return nil, sendTextOut{}, fmt.Errorf("pane %s status is stale; refusing to type into it", key)
	}
	peer, paneID := statusd.SplitPeerTarget(pane.PaneID)
	if !localPaneIDPattern.MatchString(paneID) || peer != pane.Peer {
		return nil, sendTextOut{}, fmt.Errorf("pane %q has no tmux pane id in the snapshot", key)
	}
	ctx, cancel := context.WithTimeout(ctx, writeCallTimeout)
	defer cancel()
	if peer == "" {
		err = t.deps.SendLocal(ctx, paneID, in.Text)
	} else {
		err = t.deps.SendPeer(ctx, peer, paneID, in.Text)
	}
	if err != nil {
		return nil, sendTextOut{}, fmt.Errorf("send to %s: %w", key, err)
	}
	out := sendTextOut{Pane: key, Next: "Call wait_pane, then capture_pane to read the reply."}
	// PasteText has no Enter-swallow recovery, so report whether the agent
	// visibly picked the message up.
	if t.sleep(ctx, sendTextCheckDelay) == nil {
		if snap, err := t.snapshot(ctx); err == nil {
			if _, after, ok := findPane(snap, key); ok {
				out.StateAfter = after.State
				out.Accepted = after.Running || !after.InputReady || after.LastChangedAt != nil && pane.LastChangedAt != nil && after.LastChangedAt.After(*pane.LastChangedAt)
			}
		}
	}
	if !out.Accepted {
		out.Next = "The pane did not visibly start working yet. Check with capture_pane before sending again."
	}
	return nil, out, nil
}

type waitIn struct {
	Pane           string `json:"pane" jsonschema:"pane id from list_panes or dispatch_work"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"how long to wait (default 60, max 80)"`
	SettleSeconds  int    `json:"settle_seconds,omitempty" jsonschema:"how long the pane must stay idle (default 2)"`
}

type waitOut struct {
	Note             string     `json:"note"`
	Pane             string     `json:"pane"`
	Outcome          string     `json:"outcome" jsonschema:"idle, asking, gone, or timeout"`
	State            string     `json:"state,omitempty"`
	Runtime          string     `json:"runtime,omitempty"`
	Prompt           *promptOut `json:"prompt,omitempty"`
	ObservedLastLine string     `json:"observed_last_line,omitempty"`
	WaitedSeconds    float64    `json:"waited_seconds"`
}

func (t tools) waitPane(ctx context.Context, _ *sdk.CallToolRequest, in waitIn) (*sdk.CallToolResult, waitOut, error) {
	timeout := secondsOr(in.TimeoutSeconds, defaultWaitTimeout)
	if timeout > maxWaitTimeout {
		timeout = maxWaitTimeout
	}
	settle := secondsOr(in.SettleSeconds, defaultWaitSettle)
	start := t.now()
	deadline := start.Add(timeout)
	ref := strings.TrimSpace(in.Pane)
	var readySince time.Time
	for {
		snap, err := t.snapshot(ctx)
		if err != nil {
			return nil, waitOut{}, err
		}
		key, pane, ok := findPane(snap, ref)
		out := waitOut{Note: untrustedNote, Pane: ref, WaitedSeconds: t.now().Sub(start).Round(100 * time.Millisecond).Seconds()}
		if !ok {
			if ref == "" {
				return nil, waitOut{}, errors.New("pane is required")
			}
			out.Outcome = "gone"
			return nil, out, nil
		}
		ref = key
		out.Pane, out.State, out.Runtime, out.ObservedLastLine = key, pane.State, pane.Runtime, pane.LastLine
		out.Prompt = promptSummary(pane)
		now := t.now()
		switch {
		case pane.Asking:
			out.Outcome = "asking"
			return nil, out, nil
		case pane.InputReady && !pane.Running:
			if readySince.IsZero() {
				readySince = now
			}
			if now.Sub(readySince) >= settle {
				out.Outcome = "idle"
				return nil, out, nil
			}
		default:
			readySince = time.Time{}
		}
		if !now.Before(deadline) {
			out.Outcome = "timeout"
			return nil, out, nil
		}
		if err := t.sleep(ctx, t.pollInterval()); err != nil {
			return nil, waitOut{}, err
		}
	}
}

func secondsOr(n int, def time.Duration) time.Duration {
	if n <= 0 {
		return def
	}
	return time.Duration(n) * time.Second
}
