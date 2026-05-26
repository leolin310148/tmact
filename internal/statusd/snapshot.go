package statusd

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/tmux"
)

const maxSnapshotErrors = 32

// PeerSeparator splits a peer name from the embedded tmux identifier in
// merged snapshots (e.g. "z13@%0", "z13@probe:0.0"). The `@` character does
// not appear in tmux session names or pane ids, so the split is unambiguous.
const PeerSeparator = "@"

type Snapshot struct {
	Version      int                      `json:"version"`
	Timestamp    time.Time                `json:"ts"`
	GeneratedBy  string                   `json:"generated_by"`
	IntervalMS   int64                    `json:"interval_ms"`
	StaleAfterMS int64                    `json:"stale_after_ms"`
	Summary      Summary                  `json:"summary"`
	Sessions     map[string]SessionStatus `json:"sessions"`
	Panes        map[string]PaneStatus    `json:"panes"`
	Errors       []SnapshotError          `json:"errors,omitempty"`
}

type Summary struct {
	Sessions int `json:"sessions"`
	Panes    int `json:"panes"`
	Working  int `json:"working"`
	Asking   int `json:"asking"`
	Errors   int `json:"errors"`
}

type SessionStatus struct {
	Session      string    `json:"session"`
	SessionID    string    `json:"session_id,omitempty"`
	ActiveTarget string    `json:"active_target,omitempty"`
	Tag          string    `json:"tag"`
	Runtime      string    `json:"runtime"`
	State        string    `json:"state"`
	Running      bool      `json:"running"`
	Asking       bool      `json:"asking"`
	Stale        bool      `json:"stale"`
	RowBucket    int       `json:"row_bucket"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Peer is empty for local sessions and the peer name when this session
	// was merged in from a remote statusd via PeerFetcher.
	Peer string `json:"peer,omitempty"`
}

type PaneStatus struct {
	Target         string         `json:"target"`
	PaneID         string         `json:"pane_id,omitempty"`
	Session        string         `json:"session"`
	SessionID      string         `json:"session_id,omitempty"`
	WindowIndex    int            `json:"window_index"`
	Window         string         `json:"window,omitempty"`
	WindowActive   bool           `json:"-"`
	PaneIndex      int            `json:"pane_index"`
	Active         bool           `json:"-"`
	CWD            string         `json:"cwd,omitempty"`
	CurrentCommand string         `json:"current_command,omitempty"`
	Runtime        string         `json:"runtime"`
	Tag            string         `json:"tag"`
	State          string         `json:"state"`
	Idle           bool           `json:"idle"`
	InputReady     bool           `json:"input_ready"`
	Running        bool           `json:"running"`
	Asking         bool           `json:"asking"`
	Confidence     string         `json:"confidence,omitempty"`
	Signals        []string       `json:"signals,omitempty"`
	Prompt         *prompt.Prompt `json:"prompt,omitempty"`
	LastLine       string         `json:"last_line,omitempty"`
	LastChangedAt  *time.Time     `json:"last_changed_at,omitempty"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Error          string         `json:"error,omitempty"`
	// Peer is empty for local panes and the peer name when this pane was
	// merged in from a remote statusd via PeerFetcher.
	Peer string `json:"peer,omitempty"`
}

type SnapshotError struct {
	Scope  string `json:"scope"`
	Target string `json:"target,omitempty"`
	Error  string `json:"error"`
}

type paneMemory struct {
	Hash          string
	LastChangedAt time.Time
}

type Memory struct {
	panes map[string]paneMemory
}

func NewMemory() *Memory {
	return &Memory{panes: map[string]paneMemory{}}
}

func BuildSnapshot(ctx context.Context, cfg Config, mem *Memory) (Snapshot, error) {
	cfg = cfg.withDefaults()
	if mem == nil {
		mem = NewMemory()
	}
	now := cfg.Now()
	snapshot := newSnapshot(cfg, now)

	panes, err := cfg.ListPanes()
	if err != nil {
		snapshot.addError("tmux", "", err)
		return snapshot, err
	}
	panes = filterPanes(panes, cfg.IncludeSessions, cfg.ExcludeSessions)

	samples := 1
	if len(mem.panes) == 0 {
		samples = cfg.InitialSamples
	}
	report, err := panestatus.InspectPanes(panes, panestatus.Options{
		Lines:              cfg.CaptureLines,
		Samples:            samples,
		IdleIgnorePatterns: cfg.IdleIgnorePatterns,
		CaptureRuntimes: []string{
			panestatus.RuntimeClaude,
			panestatus.RuntimeCodex,
			panestatus.RuntimeCopilot,
			panestatus.RuntimeGemini,
		},
	}, cfg.CapturePane, cfg.Sleep)
	if err != nil {
		snapshot.addError("inspect", "", err)
		return snapshot, err
	}

	seen := map[string]bool{}
	for _, pane := range report.Panes {
		if err := ctx.Err(); err != nil {
			snapshot.addError("context", "", err)
			return snapshot, err
		}
		status := buildPaneStatus(pane, cfg, mem, now)
		snapshot.Panes[status.Target] = status
		seen[status.PaneID] = true
		if status.Error != "" {
			snapshot.addError("pane", status.Target, fmt.Errorf("%s", status.Error))
		}
	}
	for paneID := range mem.panes {
		if !seen[paneID] {
			delete(mem.panes, paneID)
		}
	}

	snapshot.Sessions = buildSessions(snapshot.Panes, now)
	snapshot.Summary = summarize(snapshot)
	return snapshot, nil
}

func newSnapshot(cfg Config, now time.Time) Snapshot {
	return Snapshot{
		Version:      1,
		Timestamp:    now,
		GeneratedBy:  "tmact statusd",
		IntervalMS:   cfg.Interval.Milliseconds(),
		StaleAfterMS: cfg.StaleAfter.Milliseconds(),
		Sessions:     map[string]SessionStatus{},
		Panes:        map[string]PaneStatus{},
	}
}

func buildPaneStatus(pane panestatus.PaneStatus, cfg Config, mem *Memory, now time.Time) PaneStatus {
	running, lastChangedAt, hasHistory := runningState(pane, cfg, mem, now)
	asking := isAsking(pane)
	state := pane.State
	if running && !asking {
		state = panestate.StateWorking
	}
	idle := !running && pane.Idle
	if hasHistory && !running && state == panestate.StateUnknown && !asking && pane.Error == "" {
		state = panestate.StateIdle
		idle = true
	}
	inputReady := !running && pane.InputReady

	var changedAt *time.Time
	if !lastChangedAt.IsZero() {
		changedAt = &lastChangedAt
	}

	return PaneStatus{
		Target:         pane.Target,
		PaneID:         pane.PaneID,
		Session:        pane.Session,
		SessionID:      pane.SessionID,
		WindowIndex:    pane.WindowIndex,
		Window:         pane.Window,
		WindowActive:   pane.WindowActive,
		PaneIndex:      pane.PaneIndex,
		Active:         pane.Active,
		CWD:            pane.CWD,
		CurrentCommand: pane.CurrentCommand,
		Runtime:        pane.Runtime,
		Tag:            RuntimeTag(pane.Runtime),
		State:          state,
		Idle:           idle,
		InputReady:     inputReady,
		Running:        running,
		Asking:         asking,
		Confidence:     pane.Confidence,
		Signals:        append([]string{}, pane.Signals...),
		Prompt:         pane.InteractivePrompt,
		LastLine:       pane.LastLine,
		LastChangedAt:  changedAt,
		UpdatedAt:      now,
		Error:          pane.Error,
	}
}

func runningState(pane panestatus.PaneStatus, cfg Config, mem *Memory, now time.Time) (bool, time.Time, bool) {
	if pane.PaneID == "" || pane.NormalizedHash == "" || pane.Error != "" {
		return pane.State == panestate.StateWorking, time.Time{}, false
	}

	previous, ok := mem.panes[pane.PaneID]
	lastChangedAt := previous.LastChangedAt
	if !ok {
		if pane.State == panestate.StateWorking {
			lastChangedAt = now
		}
		mem.panes[pane.PaneID] = paneMemory{Hash: pane.NormalizedHash, LastChangedAt: lastChangedAt}
		return pane.State == panestate.StateWorking, lastChangedAt, false
	}

	if previous.Hash != pane.NormalizedHash {
		lastChangedAt = now
	}
	mem.panes[pane.PaneID] = paneMemory{Hash: pane.NormalizedHash, LastChangedAt: lastChangedAt}

	if lastChangedAt.IsZero() {
		return pane.State == panestate.StateWorking, lastChangedAt, true
	}
	return now.Sub(lastChangedAt) <= cfg.RunningDebounce, lastChangedAt, true
}

func isAsking(pane panestatus.PaneStatus) bool {
	if pane.Asking || pane.Prompt != nil || pane.InteractivePrompt != nil || pane.State == panestate.StateWaitingPermission {
		return true
	}
	return false
}

func buildSessions(panes map[string]PaneStatus, now time.Time) map[string]SessionStatus {
	bySession := map[string][]PaneStatus{}
	for _, pane := range panes {
		bySession[pane.Session] = append(bySession[pane.Session], pane)
	}

	sessions := make([]string, 0, len(bySession))
	for session := range bySession {
		sessions = append(sessions, session)
	}
	sort.Strings(sessions)

	result := map[string]SessionStatus{}
	total := len(sessions)
	for idx, session := range sessions {
		group := bySession[session]
		sort.Slice(group, func(i, j int) bool {
			if group[i].WindowIndex != group[j].WindowIndex {
				return group[i].WindowIndex < group[j].WindowIndex
			}
			return group[i].PaneIndex < group[j].PaneIndex
		})
		active := activePane(group)
		status := SessionStatus{
			Session:      session,
			SessionID:    active.SessionID,
			ActiveTarget: active.Target,
			Tag:          active.Tag,
			Runtime:      active.Runtime,
			State:        active.State,
			Running:      anyRunning(group),
			Asking:       anyAsking(group),
			Stale:        false,
			RowBucket:    rowBucket(idx, total),
			UpdatedAt:    now,
		}
		result[session] = status
	}
	return result
}

func activePane(panes []PaneStatus) PaneStatus {
	if len(panes) == 0 {
		return PaneStatus{Runtime: panestatus.RuntimeUnknown, Tag: RuntimeTag(panestatus.RuntimeUnknown), State: panestate.StateUnknown}
	}
	for _, pane := range panes {
		if pane.WindowActive && pane.Active {
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

func anyRunning(panes []PaneStatus) bool {
	for _, pane := range panes {
		if pane.Running {
			return true
		}
	}
	return false
}

func anyAsking(panes []PaneStatus) bool {
	for _, pane := range panes {
		if pane.Asking {
			return true
		}
	}
	return false
}

func rowBucket(index int, total int) int {
	if total <= 0 {
		return 0
	}
	return index * 3 / total
}

func summarize(snapshot Snapshot) Summary {
	summary := Summary{
		Sessions: len(snapshot.Sessions),
		Panes:    len(snapshot.Panes),
		Errors:   len(snapshot.Errors),
	}
	for _, pane := range snapshot.Panes {
		if pane.State == panestate.StateWorking || pane.Running {
			summary.Working++
		}
		if pane.Asking {
			summary.Asking++
		}
	}
	return summary
}

func (s *Snapshot) addError(scope string, target string, err error) {
	if err == nil || len(s.Errors) >= maxSnapshotErrors {
		return
	}
	s.Errors = append(s.Errors, SnapshotError{
		Scope:  scope,
		Target: target,
		Error:  err.Error(),
	})
	s.Summary.Errors = len(s.Errors)
}

func (s Snapshot) IsStale(now time.Time) bool {
	staleAfter := time.Duration(s.StaleAfterMS) * time.Millisecond
	if staleAfter <= 0 {
		staleAfter = DefaultStaleAfter
	}
	return now.Sub(s.Timestamp) > staleAfter
}

// MergePeers folds remote peer snapshots into local. Each peer's sessions
// and panes are added with a "<name>@" prefix on their map keys, Target,
// Session, ActiveTarget, and PaneID; the Peer field marks the origin so the
// UI and send-router can tell remote from local. Peer fetch errors and
// errors reported in the remote snapshot are propagated into local.Errors
// with scope "peer:<name>" / "peer:<name>:<remote-scope>". Summary is
// recomputed after the merge.
func MergePeers(local Snapshot, peers map[string]PeerSnapshot) Snapshot {
	if len(peers) == 0 {
		return local
	}
	if local.Sessions == nil {
		local.Sessions = map[string]SessionStatus{}
	}
	if local.Panes == nil {
		local.Panes = map[string]PaneStatus{}
	}
	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		ps := peers[name]
		if ps.Err != nil {
			(&local).addError("peer:"+name, "", ps.Err)
			continue
		}
		if !ps.Reachable {
			continue
		}
		prefix := name + PeerSeparator
		for k, pane := range ps.Snapshot.Panes {
			pane.Peer = name
			if pane.Target != "" {
				pane.Target = prefix + pane.Target
			}
			if pane.Session != "" {
				pane.Session = prefix + pane.Session
			}
			if pane.PaneID != "" {
				pane.PaneID = prefix + pane.PaneID
			}
			local.Panes[prefix+k] = pane
		}
		for k, session := range ps.Snapshot.Sessions {
			session.Peer = name
			if session.Session != "" {
				session.Session = prefix + session.Session
			}
			if session.ActiveTarget != "" {
				session.ActiveTarget = prefix + session.ActiveTarget
			}
			local.Sessions[prefix+k] = session
		}
		for _, e := range ps.Snapshot.Errors {
			scope := "peer:" + name
			if e.Scope != "" {
				scope = scope + ":" + e.Scope
			}
			target := e.Target
			if target != "" {
				target = prefix + target
			}
			if len(local.Errors) >= maxSnapshotErrors {
				break
			}
			local.Errors = append(local.Errors, SnapshotError{
				Scope:  scope,
				Target: target,
				Error:  e.Error,
			})
		}
	}
	local.Summary = summarize(local)
	return local
}

// SplitPeerTarget returns (peerName, localTarget) for ids produced by
// MergePeers. For unprefixed ids (local panes) peerName is empty and the
// input is returned unchanged.
func SplitPeerTarget(id string) (peer string, rest string) {
	if i := strings.Index(id, PeerSeparator); i > 0 {
		return id[:i], id[i+len(PeerSeparator):]
	}
	return "", id
}

func filterPanes(panes []tmux.Pane, include []string, exclude []string) []tmux.Pane {
	if len(include) == 0 && len(exclude) == 0 {
		return panes
	}
	filtered := make([]tmux.Pane, 0, len(panes))
	for _, pane := range panes {
		if len(include) > 0 && !matchesAny(include, pane.Session) {
			continue
		}
		if matchesAny(exclude, pane.Session) {
			continue
		}
		filtered = append(filtered, pane)
	}
	return filtered
}

func matchesAny(patterns []string, value string) bool {
	for _, pattern := range patterns {
		matched, err := path.Match(pattern, value)
		if err == nil && matched {
			return true
		}
		if err != nil && pattern == value {
			return true
		}
	}
	return false
}
