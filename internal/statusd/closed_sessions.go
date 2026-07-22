package statusd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

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

	mu      sync.Mutex
	loaded  bool
	entries []ClosedSession
}

func NewClosedSessionLog(path string, max int) *ClosedSessionLog {
	if max <= 0 {
		max = DefaultClosedSessionsMax
	}
	return &ClosedSessionLog{path: path, max: max}
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
// entry with the same session name, and persists the capped result.
func (l *ClosedSessionLog) Record(closed ...ClosedSession) {
	if len(closed) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.load()
	seen := map[string]bool{}
	merged := make([]ClosedSession, 0, len(closed)+len(l.entries))
	for _, entry := range closed {
		if entry.Session == "" || seen[entry.Session] {
			continue
		}
		seen[entry.Session] = true
		entry.Peer = ""
		merged = append(merged, entry)
	}
	for _, entry := range l.entries {
		if seen[entry.Session] {
			continue
		}
		seen[entry.Session] = true
		merged = append(merged, entry)
	}
	if len(merged) > l.max {
		merged = merged[:l.max]
	}
	l.entries = merged
	l.persist()
}

// Remove drops the entry for session, reporting whether one existed.
func (l *ClosedSessionLog) Remove(session string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.load()
	for i, entry := range l.entries {
		if entry.Session == session {
			l.entries = append(l.entries[:i:i], l.entries[i+1:]...)
			l.persist()
			return true
		}
	}
	return false
}

// PruneLive drops entries whose session name is currently live again (the user
// recreated it by hand), so the history only ever offers sessions that are
// actually gone. Persists only when something changed.
func (l *ClosedSessionLog) PruneLive(live map[string]bool) {
	if len(live) == 0 {
		return
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
		return
	}
	l.entries = kept
	l.persist()
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

// persist writes the current entries atomically; best-effort. Caller holds l.mu.
func (l *ClosedSessionLog) persist() {
	if l.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return
	}
	data, err := json.MarshalIndent(closedSessionsFile{
		Version:  ClosedSessionsVersion,
		Sessions: l.entries,
	}, "", "  ")
	if err != nil {
		return
	}
	_ = writeAtomicFile(l.path, append(data, '\n'), 0o600)
}
