package panestatus

import (
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/tmux"
)

func TestRuntimeCacheReusesUntilCommandChangesOrExpires(t *testing.T) {
	now := time.Unix(0, 0)
	cache := NewRuntimeCache(5 * time.Second)
	cache.now = func() time.Time { return now }

	calls := 0
	compute := func() RuntimeDetection {
		calls++
		return RuntimeDetection{Runtime: RuntimeClaude, Confidence: ConfidenceHigh, Signals: []string{"child_process"}}
	}

	// First lookup computes; second within TTL and same command reuses.
	cache.lookup(100, "node", compute)
	cache.lookup(100, "node", compute)
	if calls != 1 {
		t.Fatalf("calls after two same-command lookups = %d, want 1", calls)
	}

	// Command change invalidates immediately.
	cache.lookup(100, "python", compute)
	if calls != 2 {
		t.Fatalf("calls after command change = %d, want 2", calls)
	}

	// TTL expiry forces a recompute even when the command is unchanged.
	now = now.Add(6 * time.Second)
	cache.lookup(100, "python", compute)
	if calls != 3 {
		t.Fatalf("calls after TTL expiry = %d, want 3", calls)
	}
}

func TestRuntimeCacheDoesNotCacheNonPositivePID(t *testing.T) {
	cache := NewRuntimeCache(time.Minute)
	calls := 0
	compute := func() RuntimeDetection { calls++; return RuntimeDetection{Runtime: RuntimeShell} }
	cache.lookup(0, "zsh", compute)
	cache.lookup(0, "zsh", compute)
	if calls != 2 {
		t.Fatalf("pid<=0 should never cache; calls = %d, want 2", calls)
	}
}

func TestRuntimeCacheRetainEvictsDeadPIDs(t *testing.T) {
	cache := NewRuntimeCache(time.Hour)
	compute := func() RuntimeDetection { return RuntimeDetection{Runtime: RuntimeCodex} }
	cache.lookup(1, "node", compute)
	cache.lookup(2, "node", compute)
	cache.retain(map[int]struct{}{1: {}})
	if _, ok := cache.entries[2]; ok {
		t.Fatal("pid 2 should have been evicted by retain")
	}
	if _, ok := cache.entries[1]; !ok {
		t.Fatal("pid 1 should have been retained")
	}
}

func TestRuntimeCacheCloneIsolatesSignals(t *testing.T) {
	cache := NewRuntimeCache(time.Minute)
	compute := func() RuntimeDetection {
		return RuntimeDetection{Runtime: RuntimeClaude, Signals: []string{"child_process"}}
	}
	first := cache.lookup(7, "node", compute)
	first.Signals = append(first.Signals, "mutated")
	second := cache.lookup(7, "node", compute)
	if len(second.Signals) != 1 || second.Signals[0] != "child_process" {
		t.Fatalf("cache signals leaked mutation: %#v", second.Signals)
	}
}

// TestDetectRuntimeSkipsWalkForAgentCommand verifies the cheap path: when
// pane_current_command names an agent, the expensive process-tree walk is not
// invoked at all, while a wrapper command (node) still triggers it.
func TestDetectRuntimeSkipsWalkForAgentCommand(t *testing.T) {
	walks := 0
	processRuntime := func(int) RuntimeDetection {
		walks++
		return RuntimeDetection{Runtime: RuntimeClaude, Confidence: ConfidenceHigh}
	}

	agentPane := tmux.Pane{PaneID: "%1", PanePID: 10, CurrentCommand: "codex"}
	wrapperPane := tmux.Pane{PaneID: "%2", PanePID: 20, CurrentCommand: "node"}

	// detectRuntime runs twice per pane (pre- and post-capture). With the cache
	// wired in, the agent pane skips the walk entirely and the node pane walks
	// once (the second call hits the cache), so a single walk total proves both
	// the command shortcut and the memoization.
	report, err := inspectPanes(
		[]tmux.Pane{agentPane, wrapperPane},
		Options{RuntimeCache: NewRuntimeCache(time.Minute)},
		func(string, int) (string, error) { return "›\n", nil },
		func(time.Duration) {},
		processRuntime,
	)
	if err != nil {
		t.Fatalf("inspectPanes: %v", err)
	}
	if walks != 1 {
		t.Fatalf("process walk count = %d, want 1 (node pane once, agent pane skipped)", walks)
	}
	if report.Panes[0].Runtime != RuntimeCodex {
		t.Fatalf("agent pane runtime = %q, want codex", report.Panes[0].Runtime)
	}
}
