package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
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
