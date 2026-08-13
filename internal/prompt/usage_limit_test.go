package prompt

import (
	"strings"
	"testing"
	"time"
)

const codexUsageLimitFixture = `
› Act as the Coordinator.

■ You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at Aug 18th, 2026 9:34
AM.

› Summarize recent commits

  ~/w/ndt/mxcp-flow · main · Context 0% used
`

func TestDetectCodexUsageLimitParsesExactCurrentScreenInLocalTimezone(t *testing.T) {
	location, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 8, 13, 11, 55, 0, 0, location)
	limit, ok := DetectUsageLimit(codexUsageLimitFixture, Detect(codexUsageLimitFixture), observedAt)
	want := time.Date(2026, 8, 18, 9, 34, 0, 0, location)
	if !ok || limit.Provider != UsageLimitProviderCodex || !limit.ResetAt.Equal(want) || limit.Timezone != "Asia/Taipei" || limit.WaitSelection {
		t.Fatalf("limit=%#v ok=%t want reset=%s", limit, ok, want)
	}
}

func TestDetectCodexUsageLimitParsesYearOrdinalAndTwelveHourEdges(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		stamp string
		want  time.Time
	}{
		{"Jan 1st, 2027 12:00 AM", time.Date(2027, 1, 1, 0, 0, 0, 0, location)},
		{"Feb 2nd, 2028 12:01 PM", time.Date(2028, 2, 2, 12, 1, 0, 0, location)},
		{"Mar 23rd, 2029 11:59 PM", time.Date(2029, 3, 23, 23, 59, 0, 0, location)},
	} {
		raw := "■ You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at " + tc.stamp + ".\n› Suggestion\n/project · Context 0% used\n"
		limit, ok := DetectUsageLimit(raw, Detect(raw), time.Date(2026, 1, 1, 0, 0, 0, 0, location))
		if !ok || !limit.ResetAt.Equal(tc.want) {
			t.Fatalf("stamp=%q limit=%#v ok=%t", tc.stamp, limit, ok)
		}
	}
}

func TestDetectCodexUsageLimitFailsClosedOnFormatDriftAndStaleScrollback(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	tests := []string{
		strings.Replace(codexUsageLimitFixture, "purchase more credits", "upgrade your plan", 1),
		strings.Replace(codexUsageLimitFixture, "You've hit your usage limit", "Usage limit reached", 1),
		strings.Replace(codexUsageLimitFixture, "https://chatgpt.com/codex/settings/usage", "https://example.com/usage", 1),
		strings.Replace(codexUsageLimitFixture, "Aug 18th, 2026", "Aug 18th", 1),
		strings.Replace(codexUsageLimitFixture, "Aug 18th", "Aug 18st", 1),
		strings.Replace(codexUsageLimitFixture, "9:34", "25:99", 1),
		codexUsageLimitFixture + "› A newer submitted turn\nDone.\n› Suggestion\n/project · Context 0% used\n",
	}
	for _, raw := range tests {
		if limit, ok := DetectUsageLimit(raw, Detect(raw), now); ok {
			t.Fatalf("drift was allowlisted: %#v", limit)
		}
	}
	if raw := strings.Replace(codexUsageLimitFixture, "Aug 18th, 2026", "Aug 18th", 1); !HasCurrentUsageLimitMarker(raw, Detect(raw)) {
		t.Fatal("current malformed limit marker was not retained for fail-closed handling")
	}
}

func TestDetectCodexUsageLimitFailsClosedOnDSTGapAndOverlap(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, stamp := range []string{"Mar 8th, 2026 2:30 AM", "Nov 1st, 2026 1:30 AM"} {
		raw := "■ You've hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at " + stamp + ".\n› Suggestion\n/project · Context 0% used\n"
		if limit, ok := DetectUsageLimit(raw, Detect(raw), time.Date(2026, 1, 1, 0, 0, 0, 0, location)); ok {
			t.Fatalf("ambiguous or nonexistent local time %q was accepted: %#v", stamp, limit)
		}
		if !HasCurrentUsageLimitMarker(raw, Detect(raw)) {
			t.Fatalf("unsafe DST timestamp %q did not remain fail-closed marker", stamp)
		}
	}
}
