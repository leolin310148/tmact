package statusd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var errTestScan = errors.New("scan failed")

func closedAt(sec int) time.Time {
	return time.Date(2026, 7, 21, 12, 0, sec, 0, time.UTC)
}

func TestClosedSessionLogRecordDedupesAndCaps(t *testing.T) {
	log := NewClosedSessionLog("", 3)
	log.Record(ClosedSession{Session: "a", CWD: "/a", ClosedAt: closedAt(1)})
	log.Record(ClosedSession{Session: "b", CWD: "/b", ClosedAt: closedAt(2)})
	// Re-closing "a" replaces the older entry and moves it to the front.
	log.Record(ClosedSession{Session: "a", CWD: "/a2", ClosedAt: closedAt(3)})
	log.Record(
		ClosedSession{Session: "c", ClosedAt: closedAt(4)},
		ClosedSession{Session: "d", ClosedAt: closedAt(4)},
	)

	got := log.List()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (capped): %#v", len(got), got)
	}
	if got[0].Session != "c" || got[1].Session != "d" || got[2].Session != "a" {
		t.Fatalf("order = %s %s %s", got[0].Session, got[1].Session, got[2].Session)
	}
	if got[2].CWD != "/a2" {
		t.Fatalf("re-closed cwd = %q, want /a2", got[2].CWD)
	}
}

func TestClosedSessionLogPersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed-sessions.json")
	log := NewClosedSessionLog(path, 10)
	log.Record(ClosedSession{Session: "work", CWD: "/repo", Runtime: "claude", ClosedAt: closedAt(1)})

	reloaded := NewClosedSessionLog(path, 10)
	got := reloaded.List()
	if len(got) != 1 || got[0].Session != "work" || got[0].CWD != "/repo" || got[0].Runtime != "claude" {
		t.Fatalf("reloaded = %#v", got)
	}

	removed, err := reloaded.Remove("work")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("Remove returned false for existing entry")
	}
	removed, err = reloaded.Remove("work")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("Remove returned true for missing entry")
	}
	if got := NewClosedSessionLog(path, 10).List(); len(got) != 0 {
		t.Fatalf("after remove, reloaded = %#v", got)
	}
}

func TestClosedSessionLogRecordDurableRequiresPath(t *testing.T) {
	log := NewClosedSessionLog("", 10)
	err := log.RecordDurable(ClosedSession{Session: "work", ClosedAt: closedAt(1)})
	if !errors.Is(err, ErrClosedSessionPersistenceDisabled) {
		t.Fatalf("error = %v, want ErrClosedSessionPersistenceDisabled", err)
	}
	if got := log.List(); len(got) != 0 {
		t.Fatalf("failed durable record changed memory: %#v", got)
	}
}

func TestClosedSessionLogRecordSurfacesWriteFailures(t *testing.T) {
	t.Run("unwritable path", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		log := NewClosedSessionLog(filepath.Join(parent, "closed.json"), 10)
		err := log.Record(ClosedSession{Session: "work", ClosedAt: closedAt(1)})
		if err == nil || !strings.Contains(err.Error(), "prepare closed-session history directory") {
			t.Fatalf("error = %v", err)
		}
		if got := log.List(); len(got) != 0 {
			t.Fatalf("failed record changed memory: %#v", got)
		}
	})

	t.Run("write", func(t *testing.T) {
		log := NewClosedSessionLog(filepath.Join(t.TempDir(), "closed.json"), 10)
		log.writeFile = func(string, []byte, os.FileMode) error {
			return errors.New("disk full")
		}
		err := log.Record(ClosedSession{Session: "work", ClosedAt: closedAt(1)})
		if err == nil || !strings.Contains(err.Error(), "disk full") {
			t.Fatalf("error = %v", err)
		}
		if got := log.List(); len(got) != 0 {
			t.Fatalf("failed record changed memory: %#v", got)
		}
	})

	t.Run("rename", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "closed.json")
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		log := NewClosedSessionLog(path, 10)
		err := log.Record(ClosedSession{Session: "work", ClosedAt: closedAt(1)})
		if err == nil || !strings.Contains(err.Error(), "write closed-session history") {
			t.Fatalf("error = %v", err)
		}
		if got := log.List(); len(got) != 0 {
			t.Fatalf("failed record changed memory: %#v", got)
		}
	})
}

func TestClosedSessionLogRemoveSurfacesWriteFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed.json")
	log := NewClosedSessionLog(path, 10)
	entry := ClosedSession{Session: "work", ClosedAt: closedAt(1)}
	if err := log.Record(entry); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	removed, err := log.Remove("work")
	if err == nil || removed {
		t.Fatalf("removed=%v error=%v", removed, err)
	}
	if got := log.List(); len(got) != 1 || got[0] != entry {
		t.Fatalf("failed remove changed memory: %#v", got)
	}
}

func TestClosedSessionLogPruneLive(t *testing.T) {
	log := NewClosedSessionLog("", 10)
	log.Record(
		ClosedSession{Session: "gone", ClosedAt: closedAt(1)},
		ClosedSession{Session: "back", ClosedAt: closedAt(1)},
	)
	if err := log.PruneLive(map[string]bool{"back": true, "other": true}); err != nil {
		t.Fatal(err)
	}
	got := log.List()
	if len(got) != 1 || got[0].Session != "gone" {
		t.Fatalf("after prune = %#v", got)
	}
}

func TestDaemonTrackClosedSessions(t *testing.T) {
	now := closedAt(30)
	cfg := Config{
		ClosedSessionsPath: filepath.Join(t.TempDir(), "closed.json"),
		Now:                func() time.Time { return now },
	}
	d := NewDaemon(cfg)

	snap := func(sessions map[string]SessionStatus, panes map[string]PaneStatus) Snapshot {
		return Snapshot{Sessions: sessions, Panes: panes}
	}
	first := snap(map[string]SessionStatus{
		"work":       {Session: "work", Runtime: "claude", ActiveTarget: "work:1.0"},
		"idle":       {Session: "idle"},
		"z13@remote": {Session: "z13@remote", Peer: "z13"},
	}, map[string]PaneStatus{
		"work:1.0": {Target: "work:1.0", Session: "work", CWD: "/repo/work"},
	})

	// First poll only seeds the previous set — nothing recorded.
	d.trackClosedSessions(first, nil)
	if got := d.ClosedSessions().List(); len(got) != 0 {
		t.Fatalf("after first poll = %#v", got)
	}

	// "work" disappears; the peer session disappearing must NOT be recorded.
	second := snap(map[string]SessionStatus{"idle": {Session: "idle"}}, nil)
	d.trackClosedSessions(second, nil)
	got := d.ClosedSessions().List()
	if len(got) != 1 {
		t.Fatalf("after close = %#v", got)
	}
	if got[0].Session != "work" || got[0].CWD != "/repo/work" || got[0].Runtime != "claude" || !got[0].ClosedAt.Equal(now) {
		t.Fatalf("entry = %#v", got[0])
	}

	// A failed scan must not diff (its session set may be partial).
	d.trackClosedSessions(snap(nil, nil), errTestScan)
	if got := d.ClosedSessions().List(); len(got) != 1 {
		t.Fatalf("after failed scan = %#v", got)
	}

	// "work" coming back live prunes its history entry.
	third := snap(map[string]SessionStatus{
		"idle": {Session: "idle"},
		"work": {Session: "work"},
	}, nil)
	d.trackClosedSessions(third, nil)
	if got := d.ClosedSessions().List(); len(got) != 0 {
		t.Fatalf("after reopen = %#v", got)
	}
}
