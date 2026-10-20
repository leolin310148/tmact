package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agentspend"
	"github.com/leolin310148/tmact/internal/statusd"
)

func TestHistoryCadencesPersistAndRestart(t *testing.T) {
	now := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	var calls []int
	s := &Server{SpendEnabled: true, UsageHistoryDir: t.TempDir()}
	s.computeHistoryDaily = func(at time.Time, days int) ([]agentspend.DailySpend, error) {
		calls = append(calls, days)
		rows := []agentspend.DailySpend{{Date: agentspend.TrendDay(at), Provider: "codex", USD: float64(len(calls))}}
		if days == 30 {
			rows = append(rows, agentspend.DailySpend{Date: at.AddDate(0, 0, -1).Format("2006-01-02"), Provider: "codex", USD: 5})
		}
		return rows, nil
	}
	s.refreshLocalHistory(now)
	s.refreshLocalHistory(now.Add(10 * time.Minute))
	s.refreshLocalHistory(now.Add(time.Hour))
	row := s.historySnapshot(true)[0]
	if !reflect.DeepEqual(calls, []int{30, 1}) || len(row.Daily) != 2 || row.Daily[0].USD != 5 || row.Daily[1].USD != 2 {
		t.Fatalf("calls=%v daily=%v", calls, row.Daily)
	}
	if err := s.persistHistoryCache(); err != nil {
		t.Fatal(err)
	}
	path, _ := s.historyCachePath()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("cache permissions: %v %v", info, err)
	}
	restarted := &Server{SpendEnabled: true, UsageHistoryDir: s.UsageHistoryDir, computeHistoryDaily: s.computeHistoryDaily}
	restarted.loadHistoryCache()
	if got := restarted.historySnapshot(true)[0]; got.Pending || got.CostUpdatedAt == nil || got.Daily[1].USD != 2 {
		t.Fatalf("restart lost cache: %+v", got)
	}
	restarted.refreshLocalHistory(now.Add(70 * time.Minute))
	if len(calls) != 2 {
		t.Fatalf("restart needlessly recomputed: %v", calls)
	}
	restarted.refreshLocalHistory(now.AddDate(0, 0, 1))
	restarted.refreshLocalHistory(now.AddDate(0, 0, 1).Add(10 * time.Minute))
	if !reflect.DeepEqual(calls, []int{30, 1, 30}) {
		t.Fatalf("Taiwan daily rollover: %v", calls)
	}
	// A failed new-hour refresh keeps yesterday's successful cost values/time.
	previous := restarted.historySnapshot(true)[0]
	restarted.computeHistoryDaily = func(time.Time, int) ([]agentspend.DailySpend, error) { return nil, errors.New("read failed") }
	restarted.refreshLocalHistory(now.AddDate(0, 0, 1).Add(time.Hour))
	failed := restarted.historySnapshot(true)[0]
	if failed.Error == "" || !reflect.DeepEqual(failed.Daily, previous.Daily) || !failed.CostUpdatedAt.Equal(*previous.CostUpdatedAt) {
		t.Fatalf("failed refresh lost good values: %+v", failed)
	}
}

func TestHistoryHTTPDoesNoWorkDuringBlockedBackgroundScan(t *testing.T) {
	var networkCalls atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { networkCalls.Add(1); w.WriteHeader(500) }))
	defer peer.Close()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	s := &Server{SpendEnabled: true, UsageHistoryDir: t.TempDir(), CostPeers: []statusd.Peer{{Name: "mini", URL: peer.URL}}, computeHistoryDaily: func(time.Time, int) ([]agentspend.DailySpend, error) {
		close(started)
		<-release
		return []agentspend.DailySpend{}, nil
	}}
	go func() { defer close(finished); s.refreshLocalHistory(time.Now()) }()
	<-started
	done := make(chan struct{})
	go func() {
		defer close(done)
		rec := httptest.NewRecorder()
		s.handleUsageHistory(rec, httptest.NewRequest("GET", "/api/agent-usage/history", nil))
		if rec.Code != 200 {
			t.Errorf("status=%d", rec.Code)
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		close(release)
		<-finished
		t.Fatal("HTTP waited on background work")
	}
	if networkCalls.Load() != 0 {
		t.Error("request fetched peers")
	}
	if !s.historySnapshot(true)[0].Pending {
		t.Error("cold data was rendered as a ready zero")
	}
	close(release)
	<-finished
}

func TestPeerHistoryRetainsLastGoodResultsAndRestoresThem(t *testing.T) {
	now := time.Now()
	var failed atomic.Bool
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failed.Load() {
			w.WriteHeader(503)
			return
		}
		if r.URL.Query().Get("local") != "1" {
			t.Error("recursive peer request")
		}
		_, _ = w.Write([]byte(`{"timezone":"Asia/Taipei","machines":[{"machine":"local","daily":[{"date":"2026-10-20","provider":"codex","usd":7}],"usage":[]}]}`))
	}))
	defer peer.Close()
	s := &Server{SpendEnabled: true, UsageHistoryDir: t.TempDir(), CostPeers: []statusd.Peer{{Name: "mini", URL: peer.URL}}}
	s.refreshPeerHistory(context.Background(), now)
	failed.Store(true)
	s.refreshPeerHistory(context.Background(), now.Add(time.Minute))
	got := s.historySnapshot(false)[1]
	if got.Error == "" || got.Pending || len(got.Daily) != 1 || got.Daily[0].USD != 7 || !got.UpdatedAt.Equal(now) {
		t.Fatalf("peer cache fallback failed: %+v", got)
	}
	s.history.state.Version = trendCacheVersion
	s.history.state.Timezone = agentspend.TrendTimezone
	if err := s.persistHistoryCache(); err != nil {
		t.Fatal(err)
	}
	restarted := &Server{SpendEnabled: true, UsageHistoryDir: s.UsageHistoryDir, CostPeers: s.CostPeers}
	restarted.loadHistoryCache()
	if row := restarted.historySnapshot(false)[1]; row.Daily[0].USD != 7 || row.Error == "" {
		t.Fatalf("peer restart: %+v", row)
	}
}

func TestCorruptHistoryCacheRebuildsWithoutDeletingSourceLogs(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "2026-10-20.jsonl")
	if err := os.WriteFile(source, []byte("original logs"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "trend-cache.json"), []byte("{partial"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Server{SpendEnabled: true, UsageHistoryDir: dir}
	s.loadHistoryCache()
	if row := s.historySnapshot(true)[0]; !row.Pending || row.Error == "" {
		t.Fatalf("invalid cache silently accepted: %+v", row)
	}
	if raw, err := os.ReadFile(source); err != nil || string(raw) != "original logs" {
		t.Fatal("source history altered")
	}
}

func TestHistoryPersistenceFailureKeepsExistingTargetAndCleansTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "trend-cache.json")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(target, "preserved")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Server{UsageHistoryDir: dir}
	if err := s.persistHistoryCache(); err == nil {
		t.Fatal("expected atomic rename failure")
	}
	if raw, err := os.ReadFile(sentinel); err != nil || string(raw) != "keep" {
		t.Fatal("existing target was modified")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary cache leaked: %v %v", entries, err)
	}
}
