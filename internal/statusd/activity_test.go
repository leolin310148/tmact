package statusd

import (
	"testing"
	"time"
)

func activityTestDaemon(now *time.Time) *Daemon {
	return NewDaemon(Config{
		Now:                func() time.Time { return *now },
		ListClientActivity: func() ([]time.Time, error) { return nil, nil },
	})
}

func TestEffectiveIntervalIdleByDefault(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	d := activityTestDaemon(&now)
	if got := d.effectiveInterval(now); got != DefaultIdleInterval {
		t.Fatalf("interval with no activity = %v, want %v", got, DefaultIdleInterval)
	}
}

func TestEffectiveIntervalFastWhileHumanActive(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	d := activityTestDaemon(&now)
	d.activity.recordLocal(now)

	if got := d.effectiveInterval(now); got != DefaultInterval {
		t.Fatalf("interval right after activity = %v, want %v", got, DefaultInterval)
	}
	now = now.Add(DefaultIdleAfter - time.Second)
	if got := d.effectiveInterval(now); got != DefaultInterval {
		t.Fatalf("interval just inside idle-after = %v, want %v", got, DefaultInterval)
	}
	now = now.Add(2 * time.Second)
	if got := d.effectiveInterval(now); got != DefaultIdleInterval {
		t.Fatalf("interval past idle-after = %v, want %v", got, DefaultIdleInterval)
	}
}

func TestEffectiveIntervalFederatedActivityCounts(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	d := activityTestDaemon(&now)
	d.RecordFederatedActivity(1 * time.Minute)
	if got := d.effectiveInterval(now); got != DefaultInterval {
		t.Fatalf("interval with fresh federated activity = %v, want %v", got, DefaultInterval)
	}
}

func TestEffectiveIntervalIdlePacingDisabled(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	d := NewDaemon(Config{
		Now:                func() time.Time { return now },
		Interval:           time.Second,
		IdleInterval:       -1,
		ListClientActivity: func() ([]time.Time, error) { return nil, nil },
	})
	if got := d.effectiveInterval(now); got != time.Second {
		t.Fatalf("interval with idle pacing disabled = %v, want 1s", got)
	}
}

func TestRefreshLocalActivityAggregatesSignals(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	webSeen := now.Add(-5 * time.Minute)
	clientSeen := now.Add(-2 * time.Minute)
	d := NewDaemon(Config{
		Now: func() time.Time { return now },
		ListClientActivity: func() ([]time.Time, error) {
			// A tmux clock skewed into the future must not count.
			return []time.Time{clientSeen, now.Add(time.Hour)}, nil
		},
	})
	d.SetWebActivity(func() (time.Time, bool) { return webSeen, true })

	d.refreshLocalActivity(now)

	idle, ok := d.LocalHumanIdle()
	if !ok {
		t.Fatal("LocalHumanIdle ok = false after refresh")
	}
	if idle != 2*time.Minute {
		t.Fatalf("local idle = %v, want 2m (newest of web/client signals)", idle)
	}
}

func TestRecordFederatedActivityWakesIdleDaemon(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	d := activityTestDaemon(&now)

	d.RecordFederatedActivity(30 * time.Second)
	select {
	case <-d.wake:
	default:
		t.Fatal("expected wake nudge on idle→active transition")
	}

	// Already active: further reports must not re-nudge (a hub polls every
	// second; waking each time would defeat the pacing).
	d.RecordFederatedActivity(1 * time.Second)
	select {
	case <-d.wake:
		t.Fatal("unexpected wake nudge while already active")
	default:
	}

	// Stale reports past the idle threshold never activate.
	d2 := activityTestDaemon(&now)
	d2.RecordFederatedActivity(DefaultIdleAfter + time.Minute)
	if d2.humanActive(now) {
		t.Fatal("stale federated report marked daemon active")
	}
}

func TestNoteHumanActivityWakesOnlyWhenIdle(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	d := activityTestDaemon(&now)

	d.NoteHumanActivity()
	if !d.HumanActive() {
		t.Fatal("HumanActive = false immediately after NoteHumanActivity")
	}
	if idle, ok := d.LocalHumanIdle(); !ok || idle != 0 {
		t.Fatalf("LocalHumanIdle = %v ok=%t, want 0 true", idle, ok)
	}
	select {
	case <-d.wake:
	default:
		t.Fatal("expected wake nudge while idle")
	}

	d.NoteHumanActivity()
	select {
	case <-d.wake:
		t.Fatal("unexpected wake nudge while active")
	default:
	}

	now = now.Add(DefaultIdleAfter + time.Second)
	if d.HumanActive() {
		t.Fatal("HumanActive = true after idle threshold elapsed")
	}
}

func TestStampActivityScalesMetadataWhenIdle(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	d := NewDaemon(Config{
		Now:                func() time.Time { return now },
		Interval:           500 * time.Millisecond,
		IdleInterval:       30 * time.Second,
		StaleAfter:         10 * time.Second,
		ListClientActivity: func() ([]time.Time, error) { return nil, nil },
	})

	// buildSnapshot pre-stamps the configured values; stampActivity adjusts.
	snap := Snapshot{IntervalMS: 500, StaleAfterMS: 10_000}
	d.stampActivity(&snap)
	if snap.IntervalMS != 30_000 {
		t.Fatalf("idle IntervalMS = %d, want 30000", snap.IntervalMS)
	}
	if snap.StaleAfterMS != 60_000 {
		t.Fatalf("idle StaleAfterMS = %d, want 60000 (2× idle interval)", snap.StaleAfterMS)
	}
	if snap.HumanIdleSeconds != nil {
		t.Fatalf("HumanIdleSeconds = %v with no local activity, want nil", *snap.HumanIdleSeconds)
	}

	d.activity.recordLocal(now.Add(-90 * time.Second))
	d.activity.recordFederated(now) // must not leak into the local-only stamp
	snap = Snapshot{IntervalMS: 500, StaleAfterMS: 10_000}
	d.stampActivity(&snap)
	if snap.IntervalMS != 500 {
		t.Fatalf("active IntervalMS = %d, want 500", snap.IntervalMS)
	}
	if snap.StaleAfterMS != 10_000 {
		t.Fatalf("active StaleAfterMS = %d, want the configured 10000", snap.StaleAfterMS)
	}
	if snap.HumanIdleSeconds == nil || *snap.HumanIdleSeconds != 90 {
		t.Fatalf("HumanIdleSeconds = %v, want 90 (local activity only)", snap.HumanIdleSeconds)
	}
}
