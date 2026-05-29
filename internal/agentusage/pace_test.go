package agentusage

import (
	"testing"
	"time"
)

func TestComputePace(t *testing.T) {
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	// A 7-day (10080-minute) window, half elapsed: reset is 3.5 days out.
	resetHalf := now.Add(time.Duration(10080/2) * time.Minute)

	t.Run("ahead", func(t *testing.T) {
		// 50% used at the halfway point but expected ~50 → on track; push to 70.
		p := computePace(70, 10080, &resetHalf, now)
		if p == nil {
			t.Fatal("expected pace")
		}
		if p.DeltaPercent <= 0 {
			t.Errorf("expected positive delta (ahead), got %v", p.DeltaPercent)
		}
		if p.Stage != paceFarAhead {
			t.Errorf("stage = %q, want far_ahead (delta ~20)", p.Stage)
		}
		// At 70% with half elapsed, rate exhausts before reset → ETA set.
		if p.LastsUntilReset || p.ETASeconds == nil {
			t.Errorf("expected ETA before reset, got lasts=%v eta=%v", p.LastsUntilReset, p.ETASeconds)
		}
	})

	t.Run("behind", func(t *testing.T) {
		p := computePace(10, 10080, &resetHalf, now)
		if p == nil {
			t.Fatal("expected pace")
		}
		if p.DeltaPercent >= 0 {
			t.Errorf("expected negative delta (reserve), got %v", p.DeltaPercent)
		}
		if p.Stage != paceFarBehind {
			t.Errorf("stage = %q, want far_behind", p.Stage)
		}
		if !p.LastsUntilReset {
			t.Errorf("low usage should last until reset, eta=%v", p.ETASeconds)
		}
	})

	t.Run("on track", func(t *testing.T) {
		p := computePace(50, 10080, &resetHalf, now)
		if p == nil {
			t.Fatal("expected pace")
		}
		if p.Stage != paceOnTrack {
			t.Errorf("stage = %q, want on_track (delta ~0)", p.Stage)
		}
	})

	t.Run("nil when no reset", func(t *testing.T) {
		if p := computePace(50, 10080, nil, now); p != nil {
			t.Errorf("expected nil without reset time, got %+v", p)
		}
	})

	t.Run("nil when window already reset", func(t *testing.T) {
		past := now.Add(-time.Hour)
		if p := computePace(50, 10080, &past, now); p != nil {
			t.Errorf("expected nil when reset is in the past, got %+v", p)
		}
	})

	t.Run("nil when reset farther than window", func(t *testing.T) {
		// Reset 8 days out for a 7-day window is impossible → not in window.
		far := now.Add(8 * 24 * time.Hour)
		if p := computePace(50, 10080, &far, now); p != nil {
			t.Errorf("expected nil when reset exceeds window length, got %+v", p)
		}
	})
}
