package statusd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/leolin310148/tmact/internal/shellhook"
)

type Daemon struct {
	cfg             Config
	mem             *Memory
	store           *Store
	optionCache     *TmuxOptionCache
	peers           *PeerFetcher
	hooks           *shellhook.Store
	sessions        SessionPersistence
	nextSessionSave time.Time

	closed *ClosedSessionLog
	// prevSessions is the last poll's local session set (name → reopen info)
	// used to detect sessions that disappeared; nil until the first poll so a
	// daemon restart never mass-records sessions it simply hasn't seen yet.
	prevSessions map[string]ClosedSession
}

func NewDaemon(cfg Config) *Daemon {
	cfg = cfg.withDefaults()
	d := &Daemon{
		cfg:         cfg,
		mem:         NewMemory(),
		store:       NewStore(),
		optionCache: NewTmuxOptionCache(),
		hooks:       shellhook.NewStore(),
		sessions: SessionPersistence{
			Store: SessionSnapshotStore{
				Dir:       cfg.SessionSnapshotDir,
				Retention: cfg.SessionSnapshotRetention,
			},
			Capture:   cfg.ListSessionState,
			Client:    cfg.RestoreClient,
			Now:       cfg.Now,
			HomeDir:   cfg.HomeDir,
			DirExists: cfg.DirExists,
		},
	}
	d.closed = NewClosedSessionLog(cfg.ClosedSessionsPath, cfg.ClosedSessionsMax)
	if len(cfg.Peers) > 0 {
		d.peers = NewPeerFetcher(cfg.Peers, cfg.PeerInterval, cfg.PeerTimeout)
		d.peers.SetLogger(cfg.Logf)
	}
	return d
}

// Peers returns the peer fetcher, or nil when no peers are configured.
func (d *Daemon) Peers() *PeerFetcher { return d.peers }

// Hooks returns the shell hook store the web ingest endpoint records into.
func (d *Daemon) Hooks() *shellhook.Store { return d.hooks }

// Store returns the in-memory snapshot store the daemon publishes to. The web
// server and IPC handlers read from this; nothing is written to disk.
func (d *Daemon) Store() *Store {
	return d.store
}

// ClosedSessions returns the recently-closed-session log the web UI reads for
// its reopen history.
func (d *Daemon) ClosedSessions() *ClosedSessionLog {
	return d.closed
}

// trackClosedSessions diffs the local sessions in snapshot against the
// previous poll: sessions that disappeared are recorded as recently closed
// (with the cwd of their last active pane, so they can be reopened in place),
// and entries whose name is live again are pruned. Runs only from the Start
// loop — one-shot scans must not mutate the shared history file.
func (d *Daemon) trackClosedSessions(snapshot Snapshot, scanErr error) {
	if d.closed == nil {
		return
	}
	// A failed scan can report a partial (or empty) session set; diffing
	// against it would record sessions that are still alive.
	if scanErr != nil {
		return
	}
	current := make(map[string]ClosedSession, len(snapshot.Sessions))
	live := make(map[string]bool, len(snapshot.Sessions))
	for name, sess := range snapshot.Sessions {
		if sess.Peer != "" {
			continue
		}
		live[name] = true
		current[name] = ClosedSession{
			Session: name,
			Runtime: sess.Runtime,
			CWD:     sessionCWD(snapshot, sess),
		}
	}
	if d.prevSessions != nil {
		var closedNow []ClosedSession
		for name, entry := range d.prevSessions {
			if !live[name] {
				entry.ClosedAt = d.cfg.Now()
				closedNow = append(closedNow, entry)
			}
		}
		if len(closedNow) > 0 {
			sort.Slice(closedNow, func(i, j int) bool {
				return closedNow[i].Session < closedNow[j].Session
			})
			d.closed.Record(closedNow...)
		}
	}
	d.closed.PruneLive(live)
	d.prevSessions = current
}

// sessionCWD picks the reopen cwd for a session: its active pane's cwd, else
// the first pane (by window/pane index) that reports one.
func sessionCWD(snapshot Snapshot, sess SessionStatus) string {
	if pane, ok := snapshot.Panes[sess.ActiveTarget]; ok && pane.CWD != "" {
		return pane.CWD
	}
	best := ""
	bestWindow, bestPane := 0, 0
	for _, pane := range snapshot.Panes {
		if pane.Peer != "" || pane.Session != sess.Session || pane.CWD == "" {
			continue
		}
		if best == "" || pane.WindowIndex < bestWindow ||
			(pane.WindowIndex == bestWindow && pane.PaneIndex < bestPane) {
			best = pane.CWD
			bestWindow, bestPane = pane.WindowIndex, pane.PaneIndex
		}
	}
	return best
}

func (d *Daemon) RunOnce(ctx context.Context) (Snapshot, error) {
	hookStates := d.hooks.States()
	forceCapturePaneIDs := make(map[string]bool)
	for paneID, state := range hookStates {
		if state.Active != nil {
			forceCapturePaneIDs[paneID] = true
		}
	}
	snapshot, scanErr := buildSnapshot(ctx, d.cfg, d.mem, forceCapturePaneIDs)
	if scanErr != nil {
		if d.cfg.LogPath != "" {
			_ = appendLog(d.cfg.LogPath, snapshot)
		}
		return snapshot, scanErr
	}
	// Shell hook state merges before the tmux-option publish so the status
	// line reflects it too, and before MergePeers because hooks are local-only.
	seen := make(map[string]string, len(snapshot.Panes))
	for _, pane := range snapshot.Panes {
		if pane.PaneID != "" {
			seen[pane.PaneID] = pane.SessionID
		}
	}
	d.hooks.Prune(seen, snapshot.Timestamp.Add(-shellHookPruneGrace))
	hookStates = d.hooks.States()
	snapshot = ApplyShellHooks(snapshot, hookStates)
	for _, pane := range snapshot.Panes {
		if hasSignal(pane.Signals, SignalShellHookActiveStale) {
			if state, ok := hookStates[pane.PaneID]; ok {
				d.hooks.DeleteIfUpdatedAt(pane.PaneID, state.UpdatedAt)
			}
		}
	}
	if d.cfg.TmuxOptions {
		if err := PublishTmuxOptions(d.cfg, snapshot, d.optionCache); err != nil {
			snapshot.addError("tmux_options", "", err)
			if scanErr == nil {
				scanErr = err
			}
		}
	}
	if _, err := EnforcePaneSize(d.cfg); err != nil {
		snapshot.addError("pane_size", "", err)
	}
	// PublishTmuxOptions / EnforcePaneSize run before the merge so they only
	// ever touch local tmux. Merging remote peers afterwards keeps /api/snapshot
	// and the web UI in sync without confusing the local tmux integration.
	if d.peers != nil {
		snapshot = MergePeers(snapshot, d.peers.Latest())
	}
	d.store.Publish(snapshot)
	if d.cfg.LogPath != "" {
		_ = appendLog(d.cfg.LogPath, snapshot)
	}
	return snapshot, scanErr
}

func (d *Daemon) Start(ctx context.Context) error {
	d.restoreSessionsAtStartup()
	if d.peers != nil {
		d.peers.Start(ctx)
	}
	for {
		snapshot, scanErr := d.RunOnce(ctx)
		d.trackClosedSessions(snapshot, scanErr)
		d.maybeSaveSessions()

		timer := time.NewTimer(d.cfg.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (d *Daemon) restoreSessionsAtStartup() {
	if !d.cfg.SessionRestore {
		return
	}
	outcome, path, fallbacks, err := d.sessions.RestoreIfEmpty()
	if err != nil {
		d.logf("session restore skipped: %v", err)
		return
	}
	switch outcome {
	case SessionRestoreCompleted:
		d.logf("restored tmux sessions from %s (cwd fallbacks: %d)", path, fallbacks)
	case SessionRestoreSkippedNoSnapshot:
		d.logf("session restore skipped: no valid snapshot")
	}
}

func (d *Daemon) maybeSaveSessions() {
	if !d.cfg.SessionSave {
		return
	}
	now := d.cfg.Now()
	if !d.nextSessionSave.IsZero() && now.Before(d.nextSessionSave) {
		return
	}
	d.nextSessionSave = now.Add(d.cfg.SessionSaveInterval)
	path, err := d.sessions.Save()
	if err != nil {
		if errors.Is(err, ErrNoSessions) {
			d.logf("session save skipped: 0 sessions")
		} else {
			d.logf("session save skipped: %v", err)
		}
		return
	}
	d.logf("saved tmux sessions to %s", path)
}

func (d *Daemon) logf(format string, args ...any) {
	if d.cfg.Logf != nil {
		d.cfg.Logf(format, args...)
	}
}

func appendLog(path string, snapshot Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	return encoder.Encode(snapshot)
}
