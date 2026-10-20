package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agentspend"
	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/statusd"
)

func TestHistoryRefreshUsesTaiwanDayRollover(t *testing.T) {
	s := &Server{SpendEnabled: true, UsageHistoryDir: t.TempDir()}
	var calls []int
	s.computeHistoryDaily = func(at time.Time, days int) ([]agentspend.DailySpend, error) {
		calls = append(calls, days)
		return []agentspend.DailySpend{{Date: agentspend.TrendDay(at), Provider: "codex", USD: 1}}, nil
	}
	before := time.Date(2026, 10, 20, 15, 59, 0, 0, time.UTC)
	s.refreshLocalHistory(before)
	s.refreshLocalHistory(before.Add(2 * time.Minute)) // Taiwan midnight, same UTC date.
	if !reflect.DeepEqual(calls, []int{30, 30}) {
		t.Fatalf("Taiwan midnight did not reprice history: %v", calls)
	}
	s.refreshLocalHistory(time.Date(2026, 10, 21, 0, 1, 0, 0, time.UTC)) // UTC midnight, same Taiwan date.
	if !reflect.DeepEqual(calls, []int{30, 30, 1}) {
		t.Fatalf("UTC midnight incorrectly triggered a full reprice: %v", calls)
	}
}

func TestUsageHistoryIncludesFirstEightHoursOfEarliestTaiwanDay(t *testing.T) {
	now := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	start := agentspend.TrendDayStart(now).AddDate(0, 0, -29)
	s := &Server{UsageEnabled: true, UsageHistoryDir: t.TempDir()}
	for _, at := range []time.Time{start.Add(-time.Second), start, now.Add(time.Second)} {
		if err := s.recordUsage(agentusage.Snapshot{GeneratedAt: at, Providers: []agentusage.ProviderUsage{{Provider: "codex", Windows: []agentusage.RateWindow{{Name: "weekly", UsedPercent: 30}}}}}); err != nil {
			t.Fatal(err)
		}
	}
	rows := s.localHistory(now).Usage
	if len(rows) != 1 || !rows[0].At.Equal(start) {
		t.Fatalf("Taiwan 30-day boundary rows: %+v", rows)
	}
}

func TestLegacyUTCCacheRebuildsCostWithoutLosingQuotaSamples(t *testing.T) {
	now := time.Now()
	s := &Server{UsageEnabled: true, SpendEnabled: true, UsageHistoryDir: t.TempDir(), CostPeers: []statusd.Peer{{Name: "mini"}}}
	old := historyState{Version: 1, HistoricalAt: now, Local: machineHistory{Machine: "local", Daily: []agentspend.DailySpend{{Date: "2026-10-20", Provider: "codex", USD: 42}}, CostUpdatedAt: &now, Usage: []usageSample{{At: now, Provider: "codex"}}}, Peers: map[string]machineHistory{"mini": {Machine: "mini", Daily: []agentspend.DailySpend{{Date: "2026-10-20", USD: 42}}}}}
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := s.historyCachePath()
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	s.loadHistoryCache()
	rows := s.historySnapshot(false)
	if !rows[0].Pending || len(rows[0].Daily) != 0 || rows[0].CostUpdatedAt != nil || len(rows[0].Usage) != 1 || !s.history.state.HistoricalAt.IsZero() {
		t.Fatalf("UTC cache was relabeled or quota lost: %+v", rows)
	}
	if !rows[1].Pending || len(rows[1].Daily) != 0 {
		t.Fatalf("legacy peer totals leaked: %+v", rows[1])
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(raw) {
		t.Fatal("migration modified source cache before a successful refresh")
	}
}

func TestPeerHistoryRejectsUTCBuckets(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"timezone":"UTC","machines":[{"machine":"local","daily":[{"date":"2026-10-20","provider":"codex","usd":42}],"usage":[]}]}`))
	}))
	defer peer.Close()
	s := &Server{}
	if _, err := s.fetchPeerHistory(context.Background(), statusd.Peer{Name: "mini", URL: peer.URL}); err == nil {
		t.Fatal("UTC and Taiwan daily buckets must not be merged")
	}
}
