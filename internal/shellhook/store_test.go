package shellhook

import (
	"testing"
	"time"
)

func testClock(start time.Time) func() time.Time {
	current := start
	return func() time.Time {
		current = current.Add(time.Second)
		return current
	}
}

func intPtr(v int) *int { return &v }

func TestStorePreexecSetsActive(t *testing.T) {
	store := NewStore()
	store.SetNow(testClock(time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)))

	err := store.Record(Event{Type: TypePreexec, PaneID: "%5", CommandID: "c1", Command: "make test", CWD: "/repo"})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	state, ok := store.State("%5")
	if !ok {
		t.Fatal("State: pane not found")
	}
	if state.Active == nil || state.Active.Command != "make test" || state.Active.CommandID != "c1" {
		t.Fatalf("active = %+v", state.Active)
	}
	if state.Completed != nil {
		t.Fatalf("completed = %+v, want nil", state.Completed)
	}
}

func TestStoreMatchingPrecmdCompletesActive(t *testing.T) {
	store := NewStore()
	store.SetNow(testClock(time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)))

	mustRecord(t, store, Event{Type: TypePreexec, PaneID: "%5", CommandID: "c1", Command: "make test"})
	mustRecord(t, store, Event{Type: TypePrecmd, PaneID: "%5", CommandID: "c1", ExitCode: intPtr(0)})

	state, _ := store.State("%5")
	if state.Active != nil {
		t.Fatalf("active = %+v, want nil", state.Active)
	}
	completed := state.Completed
	if completed == nil {
		t.Fatal("completed is nil")
	}
	if !completed.Matched {
		t.Fatal("completed.Matched = false, want true")
	}
	if completed.Command != "make test" || completed.ExitCode == nil || *completed.ExitCode != 0 {
		t.Fatalf("completed = %+v", completed)
	}
	if completed.StartedAt.IsZero() || completed.EndedAt.Before(completed.StartedAt) {
		t.Fatalf("timestamps: started=%v ended=%v", completed.StartedAt, completed.EndedAt)
	}
}

func TestStoreMismatchedPrecmdStillCompletes(t *testing.T) {
	store := NewStore()
	mustRecord(t, store, Event{Type: TypePreexec, PaneID: "%5", CommandID: "c1", Command: "sleep 5"})
	mustRecord(t, store, Event{Type: TypePrecmd, PaneID: "%5", CommandID: "other", ExitCode: intPtr(130)})

	state, _ := store.State("%5")
	if state.Active != nil {
		t.Fatal("active should be cleared: the shell prompt is back")
	}
	if state.Completed == nil || state.Completed.Matched {
		t.Fatalf("completed = %+v, want unmatched completion", state.Completed)
	}
}

func TestStoreDelayedStalePrecmdIsIgnored(t *testing.T) {
	base := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	store := NewStore()
	// Command A finished, but its precmd emit got delayed; command B's
	// preexec lands first. The stale precmd (older timestamp, different id)
	// must not clear B's active state.
	mustRecord(t, store, Event{Type: TypePreexec, PaneID: "%5", CommandID: "b", Command: "sleep 60", Timestamp: base})
	mustRecord(t, store, Event{Type: TypePrecmd, PaneID: "%5", CommandID: "a", ExitCode: intPtr(0), Timestamp: base.Add(-time.Second)})

	state, _ := store.State("%5")
	if state.Active == nil || state.Active.CommandID != "b" {
		t.Fatalf("active = %+v, want command b still active", state.Active)
	}
	if state.Completed != nil {
		t.Fatalf("completed = %+v, want stale precmd dropped", state.Completed)
	}
}

func TestStoreBarePrecmdRecordsCompletion(t *testing.T) {
	store := NewStore()
	mustRecord(t, store, Event{Type: TypePrecmd, PaneID: "%9", ExitCode: intPtr(1)})

	state, ok := store.State("%9")
	if !ok || state.Completed == nil {
		t.Fatalf("state = %+v ok=%t", state, ok)
	}
	if state.Completed.Matched || state.Completed.Command != "" {
		t.Fatalf("completed = %+v", state.Completed)
	}
}

func TestStoreRecordRejectsInvalid(t *testing.T) {
	store := NewStore()
	if err := store.Record(Event{Type: "nope", PaneID: "%1"}); err == nil {
		t.Fatal("Record accepted invalid type")
	}
	if err := store.Record(Event{Type: TypePreexec, PaneID: "bad"}); err == nil {
		t.Fatal("Record accepted invalid pane id")
	}
}

func TestStorePrune(t *testing.T) {
	base := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	store := NewStore()
	mustRecord(t, store, Event{Type: TypePreexec, PaneID: "%1", Timestamp: base.Add(-2 * time.Minute)})
	mustRecord(t, store, Event{Type: TypePreexec, PaneID: "%2", Timestamp: base.Add(-2 * time.Minute)})
	mustRecord(t, store, Event{Type: TypePreexec, PaneID: "%3", Timestamp: base.Add(-time.Second)})

	// %1 is still seen; %2 is gone and old; %3 is unseen but fresh (event
	// racing the pane scan) and must survive.
	store.Prune(map[string]bool{"%1": true}, base.Add(-time.Minute))

	if _, ok := store.State("%1"); !ok {
		t.Fatal("%1 pruned despite being seen")
	}
	if _, ok := store.State("%2"); ok {
		t.Fatal("%2 not pruned")
	}
	if _, ok := store.State("%3"); !ok {
		t.Fatal("%3 pruned despite fresh update")
	}
}

func TestStoreStatesReturnsCopies(t *testing.T) {
	store := NewStore()
	mustRecord(t, store, Event{Type: TypePreexec, PaneID: "%5", Command: "vim"})

	states := store.States()
	states["%5"].Active.Command = "mutated"

	state, _ := store.State("%5")
	if state.Active.Command != "vim" {
		t.Fatalf("store mutated through States copy: %q", state.Active.Command)
	}
}

func mustRecord(t *testing.T, store *Store, e Event) {
	t.Helper()
	if err := store.Record(e); err != nil {
		t.Fatalf("Record(%+v): %v", e, err)
	}
}
