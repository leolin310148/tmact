package agentusage

import "time"

// Pace describes whether a rolling window is being consumed faster or slower
// than a linear "spread evenly until reset" pace. It is the Go port of
// CodexBar's UsagePace.weekly computation.
//
// DeltaPercent is actual − expected: positive means AHEAD of pace (burning too
// fast, "in deficit"); negative means BEHIND pace (conserving, "in reserve").
type Pace struct {
	Stage           string  `json:"stage"` // see paceStage values
	DeltaPercent    float64 `json:"delta_percent"`
	ExpectedPercent float64 `json:"expected_percent"`
	ActualPercent   float64 `json:"actual_percent"`
	// ETASeconds projects when usage would hit 100% at the current rate, set
	// only when the window is expected to run out before it resets.
	ETASeconds *float64 `json:"eta_seconds,omitempty"`
	// LastsUntilReset is true when the current rate would not exhaust the window
	// before it resets.
	LastsUntilReset bool `json:"lasts_until_reset"`
}

// Pace stage values. "ahead" variants mean over-pace (using too fast);
// "behind" variants mean under-pace (reserve to spare).
const (
	paceOnTrack        = "on_track"
	paceSlightlyAhead  = "slightly_ahead"
	paceAhead          = "ahead"
	paceFarAhead       = "far_ahead"
	paceSlightlyBehind = "slightly_behind"
	paceBehind         = "behind"
	paceFarBehind      = "far_behind"
)

// computePace returns the linear-pace assessment for a window, or nil when it
// cannot be computed meaningfully (unknown reset/duration, the window already
// reset, now is outside the window, or no time has elapsed yet).
func computePace(usedPercent float64, windowMinutes int, resetsAt *time.Time, now time.Time) *Pace {
	if resetsAt == nil || windowMinutes <= 0 {
		return nil
	}
	duration := float64(windowMinutes) * 60
	timeUntilReset := resetsAt.Sub(now).Seconds()
	if timeUntilReset <= 0 || timeUntilReset > duration {
		return nil
	}
	elapsed := clampFloat(duration-timeUntilReset, 0, duration)
	expected := clampFloat(elapsed/duration*100, 0, 100)
	actual := clampFloat(usedPercent, 0, 100)
	// No elapsed time but usage already recorded: pace is not meaningful.
	if elapsed == 0 && actual > 0 {
		return nil
	}
	delta := actual - expected

	pace := &Pace{
		Stage:           paceStage(delta),
		DeltaPercent:    delta,
		ExpectedPercent: expected,
		ActualPercent:   actual,
	}

	switch {
	case elapsed > 0 && actual > 0:
		rate := actual / elapsed // percent per second
		if rate > 0 {
			remaining := 100 - actual
			if remaining < 0 {
				remaining = 0
			}
			candidate := remaining / rate
			if candidate >= timeUntilReset {
				pace.LastsUntilReset = true
			} else {
				pace.ETASeconds = &candidate
			}
		}
	case elapsed > 0 && actual == 0:
		pace.LastsUntilReset = true
	}

	return pace
}

func paceStage(delta float64) string {
	abs := delta
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs <= 2:
		return paceOnTrack
	case abs <= 6:
		if delta >= 0 {
			return paceSlightlyAhead
		}
		return paceSlightlyBehind
	case abs <= 12:
		if delta >= 0 {
			return paceAhead
		}
		return paceBehind
	default:
		if delta >= 0 {
			return paceFarAhead
		}
		return paceFarBehind
	}
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
