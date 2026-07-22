package statusd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"
)

var ErrClosedSessionPersistenceDisabled = errors.New("closed-session persistence is disabled")

const (
	ClosedSessionsVersion     = 1
	DefaultClosedSessionsName = "closed-sessions.json"
	DefaultClosedSessionsMax  = 30
)

// ClosedSession is one recently-closed local tmux session shared by statusd,
// the web UI, and the CLI. Runtime is intent metadata; consumers must use an
// explicit allowlist before launching anything from it.
type ClosedSession struct {
	Session  string    `json:"session"`
	CWD      string    `json:"cwd,omitempty"`
	Runtime  string    `json:"runtime,omitempty"`
	ClosedAt time.Time `json:"closed_at"`
	// Peer is set only in merged /api/sessions/closed responses; entries are
	// always stored peer-less (each statusd records its own local sessions).
	Peer string `json:"peer,omitempty"`
}

type closedSessionsFile struct {
	Version  int             `json:"version"`
	Sessions []ClosedSession `json:"sessions"`
}

// ClosedSessionLog persists the most recently closed local tmux sessions,
// newest first, deduplicated by session name and capped at Max entries. All
// methods are safe for concurrent use (the daemon records on its poll loop
// while web handlers read and remove).
type ClosedSessionLog struct {
	path string
	max  int
	// Filesystem functions are stored per log so persistence failures remain
	// deterministic to test without global hooks.
	writeFile func(string, []byte, os.FileMode) error
	syncDir   func(string) error

	mu      sync.Mutex
	loaded  bool
	entries []ClosedSession
}

func NewClosedSessionLog(path string, max int) *ClosedSessionLog {
	if max <= 0 {
		max = DefaultClosedSessionsMax
	}
	return &ClosedSessionLog{
		path:      path,
		max:       max,
		writeFile: writeAtomicFile,
		syncDir:   syncDirectory,
	}
}

// DefaultClosedSessionsPath returns ~/.tmact/closed-sessions.json. Empty
// string on error (which disables persistence but keeps the in-memory log).
func DefaultClosedSessionsPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, DefaultFileConfigDir, DefaultClosedSessionsName)
}

// List returns a copy of the log, newest first.
func (l *ClosedSessionLog) List() []ClosedSession {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.load()
	out := make([]ClosedSession, len(l.entries))
	copy(out, l.entries)
	return out
}

// Record prepends the given closed sessions (newest first), dropping any older
// entry with the same session name, and persists the capped result. An empty
// path keeps the log in memory only.
func (l *ClosedSessionLog) Record(closed ...ClosedSession) error {
	return l.record(false, closed...)
}

// RecordDurable is Record with persistence required. It is used before a
// destructive close so success guarantees the reopen intent reached the
// configured atomic history file.
func (l *ClosedSessionLog) RecordDurable(closed ...ClosedSession) error {
	return l.record(true, closed...)
}

// StageDurable atomically records one entry and returns a rollback that
// restores the exact prior log. The rollback refuses to overwrite intervening
// mutations on this log instance.
func (l *ClosedSessionLog) StageDurable(closed ClosedSession) (func() error, error) {
	if closed.Session == "" {
		return nil, errors.New("closed-session name is empty")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.load()
	before := append([]ClosedSession(nil), l.entries...)
	staged := mergeClosedSessions([]ClosedSession{closed}, l.entries, l.max)
	if err := l.persist(staged, true); err != nil {
		return nil, err
	}
	l.entries = staged
	return func() error {
		l.mu.Lock()
		defer l.mu.Unlock()
		if !slices.Equal(l.entries, staged) {
			return errors.New("closed-session history changed after staging")
		}
		if err := l.persist(before, true); err != nil {
			return err
		}
		l.entries = before
		return nil
	}, nil
}

func (l *ClosedSessionLog) record(requirePersistence bool, closed ...ClosedSession) error {
	if len(closed) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.load()
	merged := mergeClosedSessions(closed, l.entries, l.max)
	if err := l.persist(merged, requirePersistence); err != nil {
		return err
	}
	l.entries = merged
	return nil
}

func mergeClosedSessions(closed, existing []ClosedSession, max int) []ClosedSession {
	seen := map[string]bool{}
	merged := make([]ClosedSession, 0, len(closed)+len(existing))
	for _, entry := range closed {
		if entry.Session == "" || seen[entry.Session] {
			continue
		}
		seen[entry.Session] = true
		entry.Peer = ""
		merged = append(merged, entry)
	}
	for _, entry := range existing {
		if seen[entry.Session] {
			continue
		}
		seen[entry.Session] = true
		merged = append(merged, entry)
	}
	if len(merged) > max {
		merged = merged[:max]
	}
	return merged
}

// Remove drops the entry for session, reporting whether one existed.
func (l *ClosedSessionLog) Remove(session string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.load()
	for i, entry := range l.entries {
		if entry.Session == session {
			kept := append([]ClosedSession(nil), l.entries[:i]...)
			kept = append(kept, l.entries[i+1:]...)
			if err := l.persist(kept, false); err != nil {
				return false, err
			}
			l.entries = kept
			return true, nil
		}
	}
	return false, nil
}

// PruneLive drops entries whose session name is currently live again (the user
// recreated it by hand), so the history only ever offers sessions that are
// actually gone. Persists only when something changed.
func (l *ClosedSessionLog) PruneLive(live map[string]bool) error {
	if len(live) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.load()
	kept := l.entries[:0:0]
	for _, entry := range l.entries {
		if !live[entry.Session] {
			kept = append(kept, entry)
		}
	}
	if len(kept) == len(l.entries) {
		return nil
	}
	if err := l.persist(kept, false); err != nil {
		return err
	}
	l.entries = kept
	return nil
}

// load reads the on-disk log once; a missing or corrupt file yields an empty
// log rather than an error (history is best-effort). Caller holds l.mu.
func (l *ClosedSessionLog) load() {
	if l.loaded {
		return
	}
	l.loaded = true
	if l.path == "" {
		return
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		return
	}
	var file closedSessionsFile
	if err := json.Unmarshal(data, &file); err != nil || file.Version != ClosedSessionsVersion {
		return
	}
	entries := file.Sessions
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].ClosedAt.After(entries[j].ClosedAt) })
	if len(entries) > l.max {
		entries = entries[:l.max]
	}
	l.entries = entries
}

// persist writes entries atomically and syncs the containing directory. Caller
// holds l.mu; l.entries is not changed until this succeeds.
func (l *ClosedSessionLog) persist(entries []ClosedSession, requirePersistence bool) error {
	if l.path == "" {
		if requirePersistence {
			return ErrClosedSessionPersistenceDisabled
		}
		return nil
	}
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("prepare closed-session history directory: %w", err)
	}
	data, err := json.MarshalIndent(closedSessionsFile{
		Version:  ClosedSessionsVersion,
		Sessions: entries,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode closed-session history: %w", err)
	}
	writeFile := l.writeFile
	if writeFile == nil {
		writeFile = writeAtomicFile
	}
	if err := writeFile(l.path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write closed-session history: %w", err)
	}
	syncDir := l.syncDir
	if syncDir == nil {
		syncDir = syncDirectory
	}
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("sync closed-session history directory: %w", err)
	}
	return nil
}
