package statusd

import (
	"sync"
	"time"
)

const (
	// DefaultIdleInterval is the slow scan cadence used when no human has
	// been seen (locally or via a peer) for IdleAfter. Kept at half the
	// default StaleAfter so an idle daemon's snapshot never reads as stale.
	DefaultIdleInterval = 5 * time.Second
	// DefaultIdleAfter is how long without any human signal before the
	// daemon drops to the idle cadence. Matches the web UI's
	// /api/human-active threshold.
	DefaultIdleAfter = 10 * time.Minute
)

// HumanIdleHeader carries the requesting side's local human idle time (in
// seconds) on peer snapshot fetches. Peers that never see a browser or an
// attached tmux client learn from it that a human is active somewhere in the
// federation and keep polling at the fast cadence.
const HumanIdleHeader = "X-Tmact-Human-Idle"

// humanActivity aggregates the signals behind the adaptive scan interval.
// Only locally observed activity is ever propagated to peers — forwarding
// the federated value back out would let two hubs keep each other awake
// forever on an echo.
type humanActivity struct {
	mu sync.Mutex
	// local is the most recent locally observed human action (web UI input /
	// pane switch, or an attached tmux client's last input), refreshed once
	// per scan tick.
	local time.Time
	// federated is the most recent human action reported by a peer, already
	// converted to the local clock from the relative idle duration on the
	// wire (clock skew between machines never enters the comparison).
	federated time.Time
}

func (a *humanActivity) recordLocal(t time.Time) {
	a.mu.Lock()
	if t.After(a.local) {
		a.local = t
	}
	a.mu.Unlock()
}

func (a *humanActivity) recordFederated(t time.Time) {
	a.mu.Lock()
	if t.After(a.federated) {
		a.federated = t
	}
	a.mu.Unlock()
}

// lastLocal returns the most recent locally observed human action.
func (a *humanActivity) lastLocal() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.local
}

// lastSeen returns the most recent human action from any source.
func (a *humanActivity) lastSeen() time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.federated.After(a.local) {
		return a.federated
	}
	return a.local
}

// SetWebActivity injects the web server's human-activity clock (last web UI
// action, ok=false when none has been seen). Wired after the web server is
// constructed; must be set before Start.
func (d *Daemon) SetWebActivity(fn func() (time.Time, bool)) {
	d.webActivity = fn
}

// refreshLocalActivity folds the current local signals — the web tracker and
// attached tmux clients' last input — into the activity clock. Runs once per
// scan tick so peer fetches and interval decisions read a cached value
// instead of shelling out to tmux.
func (d *Daemon) refreshLocalActivity(now time.Time) {
	if d.webActivity != nil {
		if t, ok := d.webActivity(); ok {
			d.activity.recordLocal(t)
		}
	}
	if d.cfg.ListClientActivity != nil {
		// A missing tmux server or transient error just means no attached
		// clients contribute this tick.
		if times, err := d.cfg.ListClientActivity(); err == nil {
			for _, t := range times {
				if !t.After(now) {
					d.activity.recordLocal(t)
				}
			}
		}
	}
}

// LocalHumanIdle reports how long ago the last locally observed human action
// was. ok is false when none has been seen since the daemon started; such a
// daemon sends no idle header and contributes nothing to the federation.
func (d *Daemon) LocalHumanIdle() (time.Duration, bool) {
	last := d.activity.lastLocal()
	if last.IsZero() {
		return 0, false
	}
	idle := d.cfg.Now().Sub(last)
	if idle < 0 {
		idle = 0
	}
	return idle, true
}

// RecordFederatedActivity ingests a peer-reported human idle duration
// (from the fetch header on inbound snapshot requests, or the
// human_idle_seconds field of a fetched peer snapshot) and wakes the scan
// loop when it flips the daemon from idle to active pacing.
func (d *Daemon) RecordFederatedActivity(idle time.Duration) {
	if idle < 0 {
		return
	}
	now := d.cfg.Now()
	wasActive := d.humanActive(now)
	d.activity.recordFederated(now.Add(-idle))
	if !wasActive && d.humanActive(now) {
		d.Wake()
	}
}

// NoteHumanActivity is the web server's low-latency nudge: called right after
// it records a local human action so an idle-paced daemon rescans immediately
// instead of waiting out a slow tick. Record it synchronously as well so other
// adaptive consumers (notably live pane streams) see the active state before
// that rescan completes.
func (d *Daemon) NoteHumanActivity() {
	now := d.cfg.Now()
	wasActive := d.humanActive(now)
	d.activity.recordLocal(now)
	if !wasActive {
		d.Wake()
	}
}

// HumanActive reports whether the daemon currently has a fresh local or
// federated human signal. It lets statusd-owned live capture paths share the
// same activity decision as the main scan loop.
func (d *Daemon) HumanActive() bool {
	return d.humanActive(d.cfg.Now())
}

// Wake nudges the Start loop to skip the remainder of the current sleep.
func (d *Daemon) Wake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// humanActive reports whether any human signal is within IdleAfter.
func (d *Daemon) humanActive(now time.Time) bool {
	last := d.activity.lastSeen()
	return !last.IsZero() && now.Sub(last) <= d.cfg.IdleAfter
}

// effectiveInterval picks the scan cadence for the current activity state.
// IdleInterval never speeds the loop up beyond the configured fast interval.
func (d *Daemon) effectiveInterval(now time.Time) time.Duration {
	if d.humanActive(now) || d.cfg.IdleInterval <= d.cfg.Interval {
		return d.cfg.Interval
	}
	return d.cfg.IdleInterval
}
