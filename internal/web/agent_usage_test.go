package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/statusd"
)

func TestAgentUsageReturns404WhenDisabled(t *testing.T) {
	handler := (&Server{UsageEnabled: false}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent-usage", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestAgentUsageReturns503BeforeFirstFetch(t *testing.T) {
	handler := (&Server{UsageEnabled: true}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent-usage", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestAgentUsageServesCachedSnapshot(t *testing.T) {
	s := &Server{UsageEnabled: true}
	s.usage.store(agentusage.Snapshot{
		GeneratedAt: time.Now(),
		Providers: []agentusage.ProviderUsage{
			{Provider: "claude", Plan: "max", Windows: []agentusage.RateWindow{
				{Name: "session", UsedPercent: 7},
			}},
		},
	})
	handler := s.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent-usage", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got agentusage.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Providers) != 1 || got.Providers[0].Provider != "claude" {
		t.Fatalf("unexpected providers: %+v", got.Providers)
	}
}

func TestRunUsageRefreshPopulatesCacheOnce(t *testing.T) {
	calls := 0
	s := &Server{
		UsageEnabled:  true,
		UsageInterval: time.Hour, // long enough that only the immediate fetch runs
		FetchUsage: func(context.Context) agentusage.Snapshot {
			calls++
			return agentusage.Snapshot{Providers: []agentusage.ProviderUsage{{Provider: "codex"}}}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { s.runUsageRefresh(ctx); close(done) }()

	// Poll until the immediate fetch lands, then stop the goroutine.
	deadline := time.After(2 * time.Second)
	for {
		if _, ok := s.usage.load(); ok {
			break
		}
		select {
		case <-deadline:
			t.Fatal("cache never populated")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
	if calls < 1 {
		t.Fatalf("fetch calls = %d, want >= 1", calls)
	}
}

func TestCarryForwardStaleKeepsLastKnownWindows(t *testing.T) {
	now := time.Now()
	prev := agentusage.Snapshot{Providers: []agentusage.ProviderUsage{
		{Provider: "claude", Plan: "max", Windows: []agentusage.RateWindow{{Name: "session", UsedPercent: 30}}},
	}}
	// Fresh fetch fails for claude (expired token) with no windows.
	fresh := agentusage.Snapshot{Providers: []agentusage.ProviderUsage{
		{Provider: "claude", Error: "access token expired (run `claude` to re-authenticate)"},
	}}

	out := carryForwardStale(prev, true, fresh, now)
	got := out.Providers[0]
	if !got.Stale {
		t.Fatalf("provider should be marked stale: %+v", got)
	}
	if len(got.Windows) != 1 || got.Windows[0].UsedPercent != 30 {
		t.Fatalf("last-known windows not carried forward: %+v", got.Windows)
	}
	if got.Error == "" {
		t.Fatalf("stale provider should keep the failure reason for a tooltip")
	}
	if got.StaleSince == nil || !got.StaleSince.Equal(now) {
		t.Fatalf("StaleSince = %v, want %v", got.StaleSince, now)
	}
}

func TestCarryForwardStalePrefersFreshSuccess(t *testing.T) {
	prev := agentusage.Snapshot{Providers: []agentusage.ProviderUsage{
		{Provider: "claude", Stale: true, Windows: []agentusage.RateWindow{{Name: "session", UsedPercent: 30}}},
	}}
	// Fresh fetch succeeds — must replace the stale reading entirely.
	fresh := agentusage.Snapshot{Providers: []agentusage.ProviderUsage{
		{Provider: "claude", Windows: []agentusage.RateWindow{{Name: "session", UsedPercent: 80}}},
	}}

	out := carryForwardStale(prev, true, fresh, time.Now())
	got := out.Providers[0]
	if got.Stale || got.Windows[0].UsedPercent != 80 {
		t.Fatalf("fresh success should win, got %+v", got)
	}
}

func TestCarryForwardStaleNoPrevReturnsFresh(t *testing.T) {
	fresh := agentusage.Snapshot{Providers: []agentusage.ProviderUsage{
		{Provider: "claude", Error: "not logged in"},
	}}
	out := carryForwardStale(agentusage.Snapshot{}, false, fresh, time.Now())
	if out.Providers[0].Stale || out.Providers[0].Error != "not logged in" {
		t.Fatalf("with no prior data the fresh error must pass through: %+v", out.Providers[0])
	}
}

func TestCarryForwardStalePreservesOnsetAcrossChain(t *testing.T) {
	onset := time.Now().Add(-2 * time.Hour)
	prev := agentusage.Snapshot{Providers: []agentusage.ProviderUsage{
		{Provider: "claude", Stale: true, StaleSince: &onset, Error: "expired",
			Windows: []agentusage.RateWindow{{Name: "session", UsedPercent: 30}}},
	}}
	fresh := agentusage.Snapshot{Providers: []agentusage.ProviderUsage{
		{Provider: "claude", Error: "expired"},
	}}
	out := carryForwardStale(prev, true, fresh, time.Now())
	if got := out.Providers[0].StaleSince; got == nil || !got.Equal(onset) {
		t.Fatalf("StaleSince should keep the original onset across chained staleness: %v", got)
	}
}

// peerWithSpend starts a fake peer serving /api/agent-usage with the given
// per-provider week/month spend.
func peerWithSpend(t *testing.T, claudeWeek, claudeMonth float64) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent-usage" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(agentusage.Snapshot{
			Providers: []agentusage.ProviderUsage{
				{Provider: "claude", Spend: &agentusage.SpendWindow{WeekUSD: claudeWeek, MonthUSD: claudeMonth}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func localSpend(week, month float64) map[string]agentusage.SpendWindow {
	return map[string]agentusage.SpendWindow{
		"claude": {WeekUSD: week, MonthUSD: month},
	}
}

func TestAddPeerSpendSumsAcrossMachines(t *testing.T) {
	peer := peerWithSpend(t, 100, 500)
	s := &Server{Peers: []statusd.Peer{{Name: "peer-a", URL: peer.URL}}}
	out := localSpend(10, 50)
	s.addPeerSpend(context.Background(), out)

	got := out["claude"]
	if got.WeekUSD != 110 || got.MonthUSD != 550 {
		t.Fatalf("claude spend = %+v, want week 110 month 550", got)
	}
}

func TestAddPeerSpendIncludesCostPeers(t *testing.T) {
	panePeer := peerWithSpend(t, 100, 500)
	costPeer := peerWithSpend(t, 7, 9)
	s := &Server{
		Peers:     []statusd.Peer{{Name: "peer-a", URL: panePeer.URL}},
		CostPeers: []statusd.Peer{{Name: "z13", URL: costPeer.URL}},
	}
	out := localSpend(10, 50)
	s.addPeerSpend(context.Background(), out)

	got := out["claude"]
	if got.WeekUSD != 117 || got.MonthUSD != 559 {
		t.Fatalf("claude spend = %+v, want week 117 month 559", got)
	}
}

func TestAddPeerSpendDedupesCostPeerByName(t *testing.T) {
	peer := peerWithSpend(t, 100, 500)
	s := &Server{
		Peers:     []statusd.Peer{{Name: "peer-a", URL: peer.URL}},
		CostPeers: []statusd.Peer{{Name: "peer-a", URL: peer.URL}},
	}
	out := localSpend(10, 50)
	s.addPeerSpend(context.Background(), out)

	got := out["claude"]
	if got.WeekUSD != 110 || got.MonthUSD != 550 {
		t.Fatalf("claude spend = %+v, want single peer contribution", got)
	}
}

func TestAddPeerSpendFallsBackToCachedWhenPeerDown(t *testing.T) {
	peer := peerWithSpend(t, 100, 500)
	s := &Server{Peers: []statusd.Peer{{Name: "peer-a", URL: peer.URL}}}

	// First refresh primes the cache.
	out1 := localSpend(10, 50)
	s.addPeerSpend(context.Background(), out1)
	if out1["claude"].WeekUSD != 110 {
		t.Fatalf("priming failed: %+v", out1["claude"])
	}

	// Peer goes down; merge must reuse the last-known 100/500.
	peer.Close()
	out2 := localSpend(10, 50)
	s.addPeerSpend(context.Background(), out2)
	if out2["claude"].WeekUSD != 110 || out2["claude"].MonthUSD != 550 {
		t.Fatalf("fallback spend = %+v, want week 110 month 550", out2["claude"])
	}
}

// Cost-only mode: a peer runs spend with quota disabled. The endpoint must
// still serve (not 404) and synthesize providers from the spend map so a hub
// can aggregate them.
func TestServeCostOnlyWhenQuotaDisabled(t *testing.T) {
	s := &Server{UsageEnabled: false, SpendEnabled: true}
	s.spend.store(map[string]agentusage.SpendWindow{
		"claude": {WeekUSD: 12, MonthUSD: 34},
		"codex":  {WeekUSD: 56, MonthUSD: 78},
	})

	handler := s.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent-usage", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (cost-only must serve)", rec.Code)
	}
	var got agentusage.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Providers) != 2 {
		t.Fatalf("providers = %d, want 2 synthesized from spend", len(got.Providers))
	}
	// Sorted: claude before codex.
	if got.Providers[0].Provider != "claude" || got.Providers[0].Spend == nil || got.Providers[0].Spend.WeekUSD != 12 {
		t.Fatalf("provider[0] = %+v", got.Providers[0])
	}
	if got.Providers[0].Windows != nil {
		t.Fatalf("cost-only provider should have no quota windows: %+v", got.Providers[0].Windows)
	}
}

func TestServeReturns404WhenBothDisabled(t *testing.T) {
	s := &Server{UsageEnabled: false, SpendEnabled: false}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent-usage", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when both quota and cost are off", rec.Code)
	}
}

// Serve-time merge: quota snapshot + separately-cached spend, without mutating
// the cached quota snapshot across requests.
func TestServeMergesSpendCacheWithoutMutatingQuota(t *testing.T) {
	s := &Server{UsageEnabled: true}
	s.usage.store(agentusage.Snapshot{Providers: []agentusage.ProviderUsage{
		{Provider: "claude", Plan: "max"},
	}})
	s.spend.store(map[string]agentusage.SpendWindow{"claude": {WeekUSD: 42, MonthUSD: 99}})

	handler := s.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/agent-usage", nil))
	var got agentusage.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Providers[0].Spend == nil || got.Providers[0].Spend.WeekUSD != 42 {
		t.Fatalf("served spend = %+v, want week 42", got.Providers[0].Spend)
	}
	// The cached quota snapshot must remain spend-free (no cross-request mutation).
	cached, _ := s.usage.load()
	if cached.Providers[0].Spend != nil {
		t.Fatalf("cached quota snapshot was mutated: %+v", cached.Providers[0].Spend)
	}
}
