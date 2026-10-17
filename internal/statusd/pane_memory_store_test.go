package statusd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/panestatus"
)

func TestPaneMemoryRoundTripKeepsOnlyChangeHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", DefaultPaneMemoryName)
	changed := time.Date(2026, 10, 18, 1, 0, 0, 0, time.UTC)
	mem := NewMemory()
	mem.panes["%1"] = paneMemory{Hash: "h1", LastChangedAt: changed}
	mem.panes["%2"] = paneMemory{Hash: "h2"} // never changed: nothing worth keeping

	if err := SavePaneMemory(path, mem); err != nil {
		t.Fatalf("save: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("stat = %v, %v; want 0600 file", info, err)
	}

	loaded := NewMemory()
	if err := LoadPaneMemory(path, loaded); err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.panes) != 1 {
		t.Fatalf("loaded %d panes, want 1: %+v", len(loaded.panes), loaded.panes)
	}
	got := loaded.panes["%1"]
	if got.Hash != "h1" || !got.LastChangedAt.Equal(changed) || !got.restored {
		t.Fatalf("loaded %+v", got)
	}
	if loaded.scanned {
		t.Fatal("restored memory must not count as a first scan")
	}
}

func TestLoadPaneMemoryMissingFileIsNotAnError(t *testing.T) {
	mem := NewMemory()
	if err := LoadPaneMemory(filepath.Join(t.TempDir(), "absent.json"), mem); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := LoadPaneMemory("", mem); err != nil {
		t.Fatalf("load empty path: %v", err)
	}
	if len(mem.panes) != 0 {
		t.Fatalf("panes = %+v", mem.panes)
	}
}

func TestLoadPaneMemoryRejectsUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultPaneMemoryName)
	if err := os.WriteFile(path, []byte(`{"version":99,"panes":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadPaneMemory(path, NewMemory()); err == nil {
		t.Fatal("want version error")
	}
}

func TestRunningStateRestoredSameScreenKeepsHistory(t *testing.T) {
	cfg := Config{}.withDefaults()
	now := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	changed := now.Add(-90 * time.Minute)
	mem := NewMemory()
	mem.panes["%1"] = paneMemory{Hash: "same", LastChangedAt: changed, restored: true}

	pane := panestatus.PaneStatus{PaneID: "%1", NormalizedHash: "same", State: panestate.StateIdle}
	running, at, hasHistory := runningState(pane, cfg, mem, now)
	if running || !at.Equal(changed) || !hasHistory {
		t.Fatalf("running=%v at=%v history=%v; want idle with saved time", running, at, hasHistory)
	}
	if mem.panes["%1"].restored {
		t.Fatal("entry should be confirmed after the first scan")
	}
}

func TestRunningStateRestoredChangedScreenStampsWithoutFlashingRunning(t *testing.T) {
	cfg := Config{}.withDefaults()
	now := time.Date(2026, 10, 18, 2, 0, 0, 0, time.UTC)
	mem := NewMemory()
	mem.panes["%1"] = paneMemory{Hash: "old", LastChangedAt: now.Add(-time.Hour), restored: true}

	idle := panestatus.PaneStatus{PaneID: "%1", NormalizedHash: "new", State: panestate.StateIdle}
	running, at, _ := runningState(idle, cfg, mem, now)
	if running {
		t.Fatal("a screen that changed while statusd was down must not read as running")
	}
	if age := now.Sub(at); age <= cfg.RunningDebounce || age > cfg.RunningDebounce+time.Second {
		t.Fatalf("changed %v ago; want just past the %v debounce", age, cfg.RunningDebounce)
	}

	// Next scan, screen unchanged: still idle (no delayed flash).
	if running, _, _ := runningState(idle, cfg, mem, now.Add(time.Second)); running {
		t.Fatal("an unchanged screen after restart must stay idle")
	}
	// A real change after that reads as running, as before.
	busy := idle
	busy.NormalizedHash = "newer"
	if running, _, _ := runningState(busy, cfg, mem, now.Add(2*time.Second)); !running {
		t.Fatal("a screen change after restart should read as running")
	}

	mem.panes["%2"] = paneMemory{Hash: "old", LastChangedAt: now.Add(-time.Hour), restored: true}
	working := panestatus.PaneStatus{PaneID: "%2", NormalizedHash: "new", State: panestate.StateWorking}
	if running, _, _ := runningState(working, cfg, mem, now); !running {
		t.Fatal("a pane classified working on the first scan still reads as running")
	}
}
