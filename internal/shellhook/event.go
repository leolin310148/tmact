// Package shellhook models opt-in shell preexec/precmd events that
// complement statusd's capture-based running/idle classification. Shells
// source the script from `tmact hook init <shell>` and emit structured
// events through `tmact hook emit`; statusd folds the per-pane command
// state into its snapshot without replacing the existing heuristics.
package shellhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// EventVersion is the wire version of Event. Emitters may omit the field;
// it defaults to this value on parse.
const EventVersion = 1

const (
	// TypePreexec marks a shell command that just started.
	TypePreexec = "preexec"
	// TypePrecmd marks the shell prompt returning after a command finished.
	TypePrecmd = "precmd"
)

// paneIDPattern matches canonical tmux pane ids like "%12". Events arrive
// from shell environments, so the strict format doubles as input hygiene.
var paneIDPattern = regexp.MustCompile(`^%[0-9]+$`)

// Event is one shell hook observation for a tmux pane.
type Event struct {
	Version   int    `json:"v"`
	Type      string `json:"type"`
	PaneID    string `json:"pane_id"`
	SessionID string `json:"session_id,omitempty"`
	// CommandID correlates a preexec with the precmd that ends it. The init
	// scripts generate it per command; empty is allowed (match falls back to
	// "any precmd ends the active command").
	CommandID string `json:"command_id,omitempty"`
	CWD       string `json:"cwd,omitempty"`
	Command   string `json:"command,omitempty"`
	// ExitCode is only meaningful on precmd events; nil when unknown.
	ExitCode  *int      `json:"exit_code,omitempty"`
	Timestamp time.Time `json:"ts,omitzero"`
}

const (
	maxCommandLen = 512
	maxFieldLen   = 256
)

// Validate checks the event is well-formed enough to store. It does not
// mutate the event; call Normalize first to apply defaults.
func (e Event) Validate() error {
	if e.Version != EventVersion {
		return fmt.Errorf("unsupported shell hook event version %d", e.Version)
	}
	switch e.Type {
	case TypePreexec, TypePrecmd:
	case "":
		return errors.New("shell hook event type is required")
	default:
		return fmt.Errorf("unsupported shell hook event type %q", e.Type)
	}
	if e.PaneID == "" {
		return errors.New("shell hook event pane_id is required")
	}
	if !paneIDPattern.MatchString(e.PaneID) {
		return fmt.Errorf("shell hook event pane_id %q is not a tmux pane id like %%5", e.PaneID)
	}
	if len(e.Command) > maxCommandLen {
		return fmt.Errorf("shell hook event command exceeds %d bytes", maxCommandLen)
	}
	if len(e.CommandID) > maxFieldLen || len(e.SessionID) > maxFieldLen || len(e.CWD) > maxFieldLen*4 {
		return errors.New("shell hook event field too long")
	}
	return nil
}

// Normalize fills defaults: version when omitted and timestamp when zero.
func (e Event) Normalize(now time.Time) Event {
	if e.Version == 0 {
		e.Version = EventVersion
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = now
	}
	return e
}

// ParseEvent decodes a JSON event, applies defaults, and validates it.
func ParseEvent(data []byte, now time.Time) (Event, error) {
	var e Event
	if err := json.Unmarshal(data, &e); err != nil {
		return Event{}, fmt.Errorf("decode shell hook event: %w", err)
	}
	e = e.Normalize(now)
	if err := e.Validate(); err != nil {
		return Event{}, err
	}
	return e, nil
}
