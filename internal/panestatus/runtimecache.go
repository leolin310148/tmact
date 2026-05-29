package panestatus

import (
	"sync"
	"time"
)

// RuntimeCache memoizes the result of the (expensive) child-process-tree
// runtime detection so a poller such as statusd does not re-walk every pane's
// process tree on every tick. Each walk shells out to pgrep/ps several times,
// so caching collapses a per-tick fork/exec storm into one walk per pane every
// ttl (and one immediately whenever a pane's foreground command changes).
//
// Entries are keyed by the pane's pid and invalidated as soon as the pane's
// foreground command changes — so an agent starting or exiting is picked up at
// once — or after ttl elapses, a safety net for the rare deeper-tree change
// that leaves pane_current_command unchanged.
//
// A nil *RuntimeCache is valid and disables caching (every lookup recomputes),
// keeping non-daemon callers and tests on the original code path.
type RuntimeCache struct {
	ttl time.Duration
	now func() time.Time

	mu      sync.Mutex
	entries map[int]runtimeCacheEntry
}

type runtimeCacheEntry struct {
	command string
	det     RuntimeDetection
	expires time.Time
}

// NewRuntimeCache returns a cache whose entries live for ttl while the pane's
// foreground command is unchanged.
func NewRuntimeCache(ttl time.Duration) *RuntimeCache {
	return &RuntimeCache{ttl: ttl, now: time.Now, entries: map[int]runtimeCacheEntry{}}
}

// lookup returns the cached detection for (pid, command) when it is still
// fresh; otherwise it calls compute, stores the result, and returns it. A
// pid <= 0 has no stable key and is never cached.
func (c *RuntimeCache) lookup(pid int, command string, compute func() RuntimeDetection) RuntimeDetection {
	if c == nil || pid <= 0 {
		return compute()
	}
	now := c.now()
	c.mu.Lock()
	if entry, ok := c.entries[pid]; ok && entry.command == command && now.Before(entry.expires) {
		det := cloneDetection(entry.det)
		c.mu.Unlock()
		return det
	}
	c.mu.Unlock()

	det := compute()

	c.mu.Lock()
	c.entries[pid] = runtimeCacheEntry{command: command, det: cloneDetection(det), expires: now.Add(c.ttl)}
	c.mu.Unlock()
	return cloneDetection(det)
}

// retain drops cache entries for pids that are no longer present, keeping the
// map from growing without bound as panes come and go. Called once per cycle.
func (c *RuntimeCache) retain(live map[int]struct{}) {
	if c == nil {
		return
	}
	c.mu.Lock()
	for pid := range c.entries {
		if _, ok := live[pid]; !ok {
			delete(c.entries, pid)
		}
	}
	c.mu.Unlock()
}

// cloneDetection copies the Signals slice so a cached entry can never be
// mutated by a caller that appends to the returned detection's signals (e.g.
// mergeRuntimeSignals).
func cloneDetection(d RuntimeDetection) RuntimeDetection {
	if d.Signals == nil {
		return d
	}
	signals := make([]string, len(d.Signals))
	copy(signals, d.Signals)
	d.Signals = signals
	return d
}
