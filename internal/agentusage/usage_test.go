package agentusage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestApplyClaudeUsage(t *testing.T) {
	const body = `{
		"five_hour": {"utilization": 6.5, "resets_at": "2026-05-29T11:44:53Z"},
		"seven_day": {"utilization": 8, "resets_at": "2026-06-02T03:37:52.123Z"},
		"seven_day_opus": {"utilization": 0},
		"seven_day_fable": {"utilization": 74, "resets_at": "2026-06-02T03:37:52.123Z"},
		"extra_usage": {"is_enabled": true, "used_credits": 250, "monthly_limit": 1000, "currency": "USD"}
	}`
	var resp claudeUsageResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := ProviderUsage{Provider: "claude"}
	applyClaudeUsage(&out, &resp, time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC))

	if len(out.Windows) != 4 {
		t.Fatalf("want 4 windows, got %d: %+v", len(out.Windows), out.Windows)
	}
	if out.Windows[0].Name != "session" || out.Windows[0].UsedPercent != 6.5 {
		t.Errorf("session window wrong: %+v", out.Windows[0])
	}
	if out.Windows[0].ResetsAt == nil {
		t.Error("session window missing reset time")
	}
	if out.Windows[1].WindowMinutes != 10080 {
		t.Errorf("weekly window minutes = %d, want 10080", out.Windows[1].WindowMinutes)
	}
	if out.Windows[3].Name != "weekly_fable" || out.Windows[3].UsedPercent != 74 {
		t.Errorf("fable window wrong: %+v", out.Windows[3])
	}
	if out.Cost == nil || !out.Cost.Enabled {
		t.Fatalf("expected enabled cost window, got %+v", out.Cost)
	}
	if out.Cost.Used != 2.5 || out.Cost.Limit != 10 { // cents -> dollars
		t.Errorf("cost = %+v, want used 2.5 limit 10", out.Cost)
	}
}

func TestApplyClaudeUsageLimits(t *testing.T) {
	const body = `{
		"five_hour": {"utilization": 85, "resets_at": "2026-07-07T08:30:00Z"},
		"seven_day": {"utilization": 61, "resets_at": "2026-07-08T03:00:00Z"},
		"limits": [
			{"group": "session", "kind": "session", "percent": 85, "resets_at": "2026-07-07T08:30:00Z"},
			{"group": "weekly", "kind": "weekly_all", "percent": 61, "resets_at": "2026-07-08T03:00:00Z"},
			{"group": "weekly", "kind": "weekly_scoped", "percent": 75, "resets_at": "2026-07-08T03:00:00Z", "scope": {"model": {"id": null, "display_name": "Fable"}}}
		]
	}`
	var resp claudeUsageResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := ProviderUsage{Provider: "claude"}
	applyClaudeUsage(&out, &resp, time.Date(2026, 7, 7, 8, 0, 0, 0, time.UTC))

	if len(out.Windows) != 3 {
		t.Fatalf("want 3 windows, got %d: %+v", len(out.Windows), out.Windows)
	}
	if out.Windows[0].Name != "session" || out.Windows[0].UsedPercent != 85 {
		t.Errorf("session window wrong: %+v", out.Windows[0])
	}
	if out.Windows[1].Name != "weekly" || out.Windows[1].UsedPercent != 61 {
		t.Errorf("weekly window wrong: %+v", out.Windows[1])
	}
	if out.Windows[2].Name != "weekly_fable" || out.Windows[2].UsedPercent != 75 {
		t.Errorf("fable window wrong: %+v", out.Windows[2])
	}
}

func TestApplyClaudeUsageExtraDisabled(t *testing.T) {
	const body = `{"five_hour": {"utilization": 1}, "extra_usage": {"is_enabled": false, "used_credits": 5}}`
	var resp claudeUsageResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := ProviderUsage{Provider: "claude"}
	applyClaudeUsage(&out, &resp, time.Now())
	if out.Cost != nil {
		t.Errorf("disabled extra_usage should not produce a cost window, got %+v", out.Cost)
	}
}

func TestApplyCodexUsage(t *testing.T) {
	const body = `{
		"plan_type": "pro",
		"rate_limit": {
			"primary_window": {"used_percent": 17, "reset_at": 1748000000, "limit_window_seconds": 18000},
			"secondary_window": {"used_percent": 59, "reset_at": 1748100000, "limit_window_seconds": 604800}
		},
		"credits": {"has_credits": true, "unlimited": false, "balance": "12.50"}
	}`
	var resp codexUsageResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := ProviderUsage{Provider: "codex"}
	applyCodexUsage(&out, &resp, time.Now())

	if out.Plan != "pro" {
		t.Errorf("plan = %q, want pro", out.Plan)
	}
	if len(out.Windows) != 2 {
		t.Fatalf("want 2 windows, got %d", len(out.Windows))
	}
	if out.Windows[0].Name != "session" || out.Windows[0].WindowMinutes != 300 {
		t.Errorf("session window wrong: %+v", out.Windows[0])
	}
	if out.Windows[1].WindowMinutes != 10080 {
		t.Errorf("weekly window minutes = %d, want 10080", out.Windows[1].WindowMinutes)
	}
	if out.Windows[1].Name != "weekly" {
		t.Errorf("secondary window name = %q, want weekly", out.Windows[1].Name)
	}
	if out.Cost == nil || out.Cost.Used != 12.5 { // balance parsed from string
		t.Errorf("cost = %+v, want used 12.5", out.Cost)
	}
}

func TestApplyCodexUsageWeeklyPrimaryWithoutSession(t *testing.T) {
	const body = `{
		"plan_type": "prolite",
		"rate_limit": {
			"primary_window": {"used_percent": 2, "reset_at": 1784489304, "limit_window_seconds": 604800}
		}
	}`
	var resp codexUsageResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := ProviderUsage{Provider: "codex"}
	applyCodexUsage(&out, &resp, time.Now())

	if len(out.Windows) != 1 {
		t.Fatalf("want 1 window, got %d", len(out.Windows))
	}
	if got := out.Windows[0]; got.Name != "weekly" || got.WindowMinutes != 10080 {
		t.Errorf("weekly primary window wrong: %+v", got)
	}
}

func TestCodexBaseURL(t *testing.T) {
	// No config file in a temp dir -> default.
	if got := codexBaseURL(t.TempDir()); got != codexDefaultBaseURL {
		t.Errorf("default base = %q, want %q", got, codexDefaultBaseURL)
	}
}

func TestFlexFloat(t *testing.T) {
	cases := map[string]*float64{
		`12.5`:   ptr(12.5),
		`"12.5"`: ptr(12.5),
		`null`:   nil,
		`""`:     nil,
		`0`:      ptr(0),
	}
	for in, want := range cases {
		var f flexFloat
		if err := json.Unmarshal([]byte(in), &f); err != nil {
			t.Errorf("%s: unexpected error %v", in, err)
			continue
		}
		switch {
		case want == nil && f.Value != nil:
			t.Errorf("%s: want nil, got %v", in, *f.Value)
		case want != nil && (f.Value == nil || *f.Value != *want):
			t.Errorf("%s: want %v, got %v", in, *want, f.Value)
		}
	}
}

func ptr(f float64) *float64 { return &f }

func TestFreshestClaudeCredentials(t *testing.T) {
	blob := func(token string, expiresAt int64) string {
		b, _ := json.Marshal(claudeCredentialsFile{ClaudeAiOauth: claudeCredentials{
			AccessToken: token,
			ExpiresAt:   expiresAt,
		}})
		return string(b)
	}

	t.Run("prefers latest expiresAt across stores", func(t *testing.T) {
		// A stale file (expired) alongside a fresh keychain blob: the newer one
		// must win so the leftover file no longer masks the live token.
		stale := blob("old", 1000)
		fresh := blob("new", 5000)
		got, ok := freshestClaudeCredentials([]string{stale, fresh})
		if !ok || got.AccessToken != "new" {
			t.Fatalf("want fresh token, got ok=%v token=%q", ok, got.AccessToken)
		}
		// Order must not matter.
		got, ok = freshestClaudeCredentials([]string{fresh, stale})
		if !ok || got.AccessToken != "new" {
			t.Fatalf("order flip: want fresh token, got ok=%v token=%q", ok, got.AccessToken)
		}
	})

	t.Run("skips blobs without an access token", func(t *testing.T) {
		empty := blob("", 9000)
		valid := blob("real", 3000)
		got, ok := freshestClaudeCredentials([]string{empty, valid})
		if !ok || got.AccessToken != "real" {
			t.Fatalf("want real token, got ok=%v token=%q", ok, got.AccessToken)
		}
	})

	t.Run("reports none when no blob carries a token", func(t *testing.T) {
		if _, ok := freshestClaudeCredentials([]string{blob("", 1), "not json"}); ok {
			t.Fatal("want ok=false when no candidate has an access token")
		}
	})
}
