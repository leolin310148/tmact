package loop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
)

// quotaSnapshot builds a one-provider snapshot with the given session/weekly
// used-percent values.
func quotaSnapshot(provider string, sessionUsed, weeklyUsed float64) agentusage.Snapshot {
	return agentusage.Snapshot{
		Providers: []agentusage.ProviderUsage{{
			Provider: provider,
			Windows: []agentusage.RateWindow{
				{Name: "session", UsedPercent: sessionUsed, WindowMinutes: 300},
				{Name: "weekly", UsedPercent: weeklyUsed, WindowMinutes: 10080},
			},
		}},
	}
}

// newQuotaRunner builds a runner with quota enabled (defaults filled) and an
// injected fetcher that returns snap without any network.
func newQuotaRunner(t *testing.T, q QuotaConfig, snap agentusage.Snapshot) *Runner {
	t.Helper()
	if q.WeeklySkipAtPercent == 0 {
		q.WeeklySkipAtPercent = defaultWeeklySkipAtPercent
	}
	if q.SessionMinRemainingPercent == 0 {
		q.SessionMinRemainingPercent = defaultSessionMinRemainingPercent
	}
	if q.RefreshInterval.Duration == 0 {
		q.RefreshInterval.Duration = defaultQuotaRefreshInterval
	}
	q.Enabled = true
	r := NewRunner(Config{
		Target:  "demo:0.0",
		Actions: []ActionConfig{{Name: "nudge", Type: "send_text", Text: "go"}},
		Quota:   &q,
	}, Options{DryRun: true})
	r.fetchUsage = func(context.Context, ...string) agentusage.Snapshot { return snap }
	return r
}

func TestQuotaWeeklyReachedSkips(t *testing.T) {
	r := newQuotaRunner(t, QuotaConfig{Provider: "codex"}, quotaSnapshot("codex", 10, 100))
	skip, reason, err := r.evaluateQuota(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !skip || reason != "quota_weekly" {
		t.Fatalf("skip=%v reason=%q, want skip=true reason=quota_weekly", skip, reason)
	}
}

func TestQuotaSessionLowSkips(t *testing.T) {
	// 85% used => 15% remaining < 20% threshold.
	r := newQuotaRunner(t, QuotaConfig{Provider: "codex"}, quotaSnapshot("codex", 85, 30))
	skip, reason, err := r.evaluateQuota(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !skip || reason != "quota_session_low" {
		t.Fatalf("skip=%v reason=%q, want skip=true reason=quota_session_low", skip, reason)
	}
}

func TestQuotaBothOkRuns(t *testing.T) {
	r := newQuotaRunner(t, QuotaConfig{Provider: "codex"}, quotaSnapshot("codex", 50, 50))
	skip, reason, err := r.evaluateQuota(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatalf("skip=%v reason=%q, want skip=false", skip, reason)
	}
}

func TestQuotaWeeklyWinsOverSession(t *testing.T) {
	// Both conditions hold; weekly is the more severe reason and must win.
	r := newQuotaRunner(t, QuotaConfig{Provider: "codex"}, quotaSnapshot("codex", 95, 100))
	skip, reason, err := r.evaluateQuota(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !skip || reason != "quota_weekly" {
		t.Fatalf("skip=%v reason=%q, want skip=true reason=quota_weekly", skip, reason)
	}
}

func TestQuotaStaleFailsOpen(t *testing.T) {
	snap := quotaSnapshot("codex", 99, 100)
	snap.Providers[0].Stale = true
	snap.Providers[0].Error = "token expired"
	r := newQuotaRunner(t, QuotaConfig{Provider: "codex"}, snap)
	skip, _, err := r.evaluateQuota(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatalf("skip=%v, want fail-open (skip=false) on stale reading", skip)
	}
}

func TestQuotaErrorFailsOpen(t *testing.T) {
	snap := agentusage.Snapshot{Providers: []agentusage.ProviderUsage{{Provider: "codex", Error: "no credentials"}}}
	r := newQuotaRunner(t, QuotaConfig{Provider: "codex"}, snap)
	skip, _, err := r.evaluateQuota(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Fatalf("skip=%v, want fail-open (skip=false) on provider error", skip)
	}
}

func TestQuotaUnavailableFailsClosedWhenConfigured(t *testing.T) {
	snap := agentusage.Snapshot{Providers: []agentusage.ProviderUsage{{Provider: "codex", Error: "no credentials"}}}
	r := newQuotaRunner(t, QuotaConfig{Provider: "codex", FailClosed: true}, snap)
	skip, reason, err := r.evaluateQuota(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !skip || reason != "quota_unavailable" {
		t.Fatalf("skip=%v reason=%q, want skip=true reason=quota_unavailable", skip, reason)
	}
}

func TestQuotaCachesBetweenRefreshes(t *testing.T) {
	calls := 0
	r := newQuotaRunner(t, QuotaConfig{Provider: "codex", RefreshInterval: Duration{Duration: 5 * time.Minute}}, agentusage.Snapshot{})
	r.fetchUsage = func(context.Context, ...string) agentusage.Snapshot {
		calls++
		return quotaSnapshot("codex", 50, 50)
	}

	base := time.Now()
	if _, _, err := r.evaluateQuota(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	// Within the refresh interval: reuse cache, no new fetch.
	if _, _, err := r.evaluateQuota(context.Background(), base.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.evaluateQuota(context.Background(), base.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("fetchUsage called %d times within the interval, want 1", calls)
	}
	// Past the refresh interval: fetch again.
	if _, _, err := r.evaluateQuota(context.Background(), base.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("fetchUsage called %d times after the interval, want 2", calls)
	}
}

func TestQuotaSkipEmitsEventThroughRun(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "loop.jsonl")
	r := NewRunner(Config{
		Target:            "demo:0.0",
		CaptureLines:      80,
		IdleAfter:         Duration{Duration: time.Second},
		PollInterval:      Duration{Duration: time.Second},
		LogPath:           logPath,
		LogSkippedActions: true,
		Actions:           []ActionConfig{{Name: "nudge", Type: "send_text", Text: "go"}},
		Quota: &QuotaConfig{
			Enabled:                    true,
			Provider:                   "codex",
			WeeklySkipAtPercent:        defaultWeeklySkipAtPercent,
			SessionMinRemainingPercent: defaultSessionMinRemainingPercent,
			RefreshInterval:            Duration{Duration: 5 * time.Minute},
		},
	}, Options{DryRun: true, Once: true})
	r.capturePane = func(string, int) (string, error) { return "idle pane\n", nil }
	r.fetchUsage = func(context.Context, ...string) agentusage.Snapshot {
		return quotaSnapshot("codex", 10, 100) // weekly reached
	}

	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	var sawSkip, sawAction bool
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad log line %q: %v", line, err)
		}
		if e.Type == "skip" && e.Status == "quota_weekly" && e.Action == "nudge" {
			sawSkip = true
		}
		if e.Type == "action" {
			sawAction = true
		}
	}
	if !sawSkip {
		t.Fatalf("expected a quota_weekly skip event, log:\n%s", data)
	}
	if sawAction {
		t.Fatalf("action must not run while weekly quota is reached, log:\n%s", data)
	}
}
