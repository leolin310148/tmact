package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agentspend"
	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/statusd"
)

func TestSharedUsageSnapshotAndCostOnlyRequest(t *testing.T) {
	s := &Server{UsageEnabled: true, SpendEnabled: true, CostPeers: []statusd.Peer{{Name: "mini"}}}
	sample := usageSample{At: time.Now(), Provider: "codex", Windows: []agentusage.RateWindow{{Name: "weekly", UsedPercent: 30}}}
	s.history.state = historyState{Version: 1, Local: machineHistory{Machine: "local", Usage: []usageSample{sample}}, Peers: map[string]machineHistory{
		"mini": {Machine: "mini", Usage: []usageSample{sample}, Daily: []agentspend.DailySpend{{Date: "2026-10-20", Provider: "codex", USD: 7}}},
	}}
	rows := s.historySnapshot(false)
	if len(rows[0].Usage) != 1 || len(rows[1].Usage) != 0 || rows[1].Daily[0].USD != 7 {
		t.Fatalf("quota duplication or cost loss: %+v", rows)
	}
	rec := httptest.NewRecorder()
	s.handleUsageHistory(rec, httptest.NewRequest("GET", "/api/agent-usage/history?local=1&cost_only=1", nil))
	var got struct {
		Machines []machineHistory `json:"machines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Machines) != 1 || len(got.Machines[0].Usage) != 0 {
		t.Fatalf("cost-only leaked quota: %s", rec.Body.String())
	}
	if len(s.historySnapshot(true)[0].Usage) != 1 {
		t.Fatal("cost-only request mutated account quota cache")
	}
}

func TestPeerHistoryRequestsOnlyCostAndDiscardsLegacyUsage(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cost_only") != "1" || r.URL.Query().Get("local") != "1" {
			t.Errorf("wrong peer query: %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"timezone":"Asia/Taipei","machines":[{"machine":"local","daily":[{"date":"2026-10-20","provider":"codex","usd":7}],"usage":[{"provider":"codex","windows":[{"name":"weekly","used_percent":30}]}]}]}`))
	}))
	defer peer.Close()
	s := &Server{}
	row, err := s.fetchPeerHistory(context.Background(), statusd.Peer{Name: "mini", URL: peer.URL})
	if err != nil || len(row.Usage) != 0 || len(row.Daily) != 1 || row.Daily[0].USD != 7 {
		t.Fatalf("peer dedup failed: %+v %v", row, err)
	}
}
