package shellhook

import (
	"sync"
	"time"
)

// CommandRecord is one shell command observed via hooks.
type CommandRecord struct {
	CommandID string    `json:"command_id,omitempty"`
	Command   string    `json:"command,omitempty"`
	CWD       string    `json:"cwd,omitempty"`
	StartedAt time.Time `json:"started_at,omitzero"`
	EndedAt   time.Time `json:"ended_at,omitzero"`
	ExitCode  *int      `json:"exit_code,omitempty"`
	// Matched reports whether the precmd that ended this command carried the
	// same command_id as its preexec. False also covers bare precmd events
	// that never had a preexec.
	Matched bool `json:"matched,omitempty"`
}

// PaneState is the per-pane view the store keeps: the still-running command
// (if any) and the most recently finished one.
type PaneState struct {
	PaneID    string         `json:"pane_id"`
	SessionID string         `json:"session_id,omitempty"`
	Active    *CommandRecord `json:"active,omitempty"`
	Completed *CommandRecord `json:"completed,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Store keeps the latest shell hook state per tmux pane. Safe for concurrent
// use: the web ingest handler writes while the daemon scan loop reads.
type Store struct {
	mu    sync.Mutex
	panes map[string]PaneState
	now   func() time.Time
}

func NewStore() *Store {
	return &Store{panes: map[string]PaneState{}, now: time.Now}
}

// SetNow overrides the clock; test hook.
func (s *Store) SetNow(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// Record validates and applies one event. A preexec becomes the pane's
// active command; a precmd completes the active command and records whether
// the ids matched. A precmd with no active command still records a
// completion so the pane counts as input-ready.
//
// Emits are backgrounded shell jobs, so delivery order is not guaranteed: a
// delayed precmd for command A can arrive after command B's preexec. A
// precmd whose command_id mismatches the active command AND whose timestamp
// predates the active command's start is that stale case and is dropped
// instead of clearing the newer active command.
func (s *Store) Record(e Event) error {
	e = e.Normalize(s.now())
	if err := e.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.panes[e.PaneID]
	state.PaneID = e.PaneID
	if e.SessionID != "" {
		state.SessionID = e.SessionID
	}
	state.UpdatedAt = e.Timestamp

	switch e.Type {
	case TypePreexec:
		state.Active = &CommandRecord{
			CommandID: e.CommandID,
			Command:   e.Command,
			CWD:       e.CWD,
			StartedAt: e.Timestamp,
		}
	case TypePrecmd:
		if active := state.Active; active != nil &&
			e.CommandID != "" && active.CommandID != "" && e.CommandID != active.CommandID &&
			e.Timestamp.Before(active.StartedAt) {
			return nil
		}
		completed := CommandRecord{
			CommandID: e.CommandID,
			CWD:       e.CWD,
			EndedAt:   e.Timestamp,
			ExitCode:  e.ExitCode,
		}
		if active := state.Active; active != nil {
			completed.Command = active.Command
			completed.StartedAt = active.StartedAt
			if completed.CommandID == "" {
				completed.CommandID = active.CommandID
			}
			if completed.CWD == "" {
				completed.CWD = active.CWD
			}
			completed.Matched = e.CommandID != "" && e.CommandID == active.CommandID
		}
		state.Active = nil
		state.Completed = &completed
	}
	s.panes[e.PaneID] = state
	return nil
}

// State returns a copy of one pane's hook state.
func (s *Store) State(paneID string) (PaneState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.panes[paneID]
	if !ok {
		return PaneState{}, false
	}
	return copyPaneState(state), true
}

// States returns a copy of every pane's hook state, keyed by pane id.
func (s *Store) States() map[string]PaneState {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]PaneState, len(s.panes))
	for id, state := range s.panes {
		out[id] = copyPaneState(state)
	}
	return out
}

// Prune drops state for panes that are gone. Entries updated at or after
// keepAfter survive even when unseen, so an event racing the pane scan (a
// brand-new pane whose first prompt fired before statusd noticed the pane)
// is not lost.
func (s *Store) Prune(seen map[string]bool, keepAfter time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, state := range s.panes {
		if seen[id] || !state.UpdatedAt.Before(keepAfter) {
			continue
		}
		delete(s.panes, id)
	}
}

func copyPaneState(state PaneState) PaneState {
	if state.Active != nil {
		active := *state.Active
		state.Active = &active
	}
	if state.Completed != nil {
		completed := *state.Completed
		state.Completed = &completed
	}
	return state
}
