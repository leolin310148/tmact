package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/statusd"
)

func TestUsageHistoryPersistsFreshWindowsOnly(t *testing.T) {
	dir := t.TempDir()
	s := &Server{UsageEnabled: true, UsageHistoryDir: dir}
	now := time.Now()
	snap := agentusage.Snapshot{GeneratedAt: now, Providers: []agentusage.ProviderUsage{
		{Provider: "codex", Account: "private@example.com", Windows: []agentusage.RateWindow{{Name: "weekly", UsedPercent: 42}}},
		{Provider: "claude", Stale: true, Windows: []agentusage.RateWindow{{Name: "session", UsedPercent: 30}}},
		{Provider: "failed", Error: "sensitive error"},
	}}
	if err := s.recordUsage(snap); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, now.UTC().Format("2006-01-02")+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "sensitive") {
		t.Fatal("private data persisted")
	}
	// A restarted server reads the durable history, and ignores an incomplete tail.
	f, _ := os.OpenFile(filepath.Join(dir, now.UTC().Format("2006-01-02")+".jsonl"), os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString("{broken\n")
	f.Close()
	restarted := &Server{UsageEnabled: true, UsageHistoryDir: dir}
	rows := restarted.localHistory(now).Usage
	if len(rows) != 1 || rows[0].Windows[0].UsedPercent != 42 {
		t.Fatalf("unexpected history: %+v", rows)
	}
}

func TestUsageHistoryPeerBoundaryAndFailures(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("local") != "1" {
			t.Error("recursive peer request")
		}
		_, _ = w.Write([]byte(`{"machines":[{"machine":"local","daily":[],"usage":[]}]}`))
	}))
	defer peer.Close()
	s := &Server{UsageEnabled: true, UsageHistoryDir: t.TempDir(), CostPeers: []statusd.Peer{{Name: "peer", URL: peer.URL}, {Name: "offline", URL: "http://127.0.0.1:1"}}}
	rec := httptest.NewRecorder()
	s.handleUsageHistory(rec, httptest.NewRequest("GET", "/api/agent-usage/history", nil))
	var got struct {
		Machines []machineHistory `json:"machines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Machines) != 3 || got.Machines[1].Machine != "peer" || got.Machines[2].Error == "" {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.handleUsageHistory(rec, httptest.NewRequest("GET", "/api/agent-usage/history?local=1", nil))
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Machines) != 1 {
		t.Fatal("local boundary fetched peers")
	}
}

func TestUsageHistoryWriteFailureAndHTTPGates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Server{UsageEnabled: true, UsageHistoryDir: path}
	s.saveUsageHistory(agentusage.Snapshot{GeneratedAt: time.Now()})
	if s.localHistory(time.Now()).Error == "" {
		t.Fatal("write failure was hidden")
	}
	for _, tc := range []struct {
		method  string
		enabled bool
		status  int
	}{{"POST", true, 405}, {"GET", false, 404}} {
		s.UsageEnabled = tc.enabled
		rec := httptest.NewRecorder()
		s.handleUsageHistory(rec, httptest.NewRequest(tc.method, "/api/agent-usage/history", nil))
		if rec.Code != tc.status {
			t.Fatalf("status %d, want %d", rec.Code, tc.status)
		}
	}
}
