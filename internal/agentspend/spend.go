package agentspend

import "time"

// Spend is the dollar-equivalent token spend for one provider over the
// calendar week-to-date and month-to-date, in USD.
type Spend struct {
	WeekUSD  float64
	MonthUSD float64
}

// scanners is the set of providers we price. Keyed by provider name so callers
// can request one or merge into their own per-provider structures.
var scanners = []scanner{claudeScanner{}, codexScanner{}}

// sharedCache persists across Compute calls for the life of the process, so a
// long-running statusd only re-reads session files that changed since the last
// refresh.
var sharedCache = newFileCache()

// Compute prices every provider's local session logs and returns the
// week-to-date and month-to-date spend per provider name. Calendar boundaries
// are taken in now's location (week starts Monday 00:00, month on the 1st).
// Providers with no readable sessions simply report zero.
func Compute(now time.Time) map[string]Spend {
	return compute(now, sharedCache)
}

func compute(now time.Time, fc *fileCache) map[string]Spend {
	weekStart, monthStart := windowBounds(now)
	earliest := weekStart
	if monthStart.Before(earliest) {
		earliest = monthStart
	}

	out := make(map[string]Spend, len(scanners))
	for _, s := range scanners {
		rows := scanRows(s, earliest, fc)
		seen := make(map[string]struct{}, len(rows))
		var week, month float64
		for _, r := range rows {
			if _, dup := seen[r.dedup]; dup {
				continue
			}
			seen[r.dedup] = struct{}{}
			if !r.ts.Before(monthStart) {
				month += r.cost
			}
			if !r.ts.Before(weekStart) {
				week += r.cost
			}
		}
		out[s.provider()] = Spend{WeekUSD: week, MonthUSD: month}
	}
	return out
}

// windowBounds returns the start of the current calendar week (Monday 00:00)
// and month (day 1, 00:00), both in now's location.
func windowBounds(now time.Time) (weekStart, monthStart time.Time) {
	loc := now.Location()
	y, m, d := now.Date()
	monthStart = time.Date(y, m, 1, 0, 0, 0, 0, loc)

	dayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
	// Go's Weekday: Sunday=0..Saturday=6. Days since Monday:
	offset := (int(now.Weekday()) + 6) % 7
	weekStart = dayStart.AddDate(0, 0, -offset)
	return weekStart, monthStart
}
