package web

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
)

// defaultUsageInterval is how often agent quota / rate-limit usage is
// refreshed. The provider endpoints are slow-moving and rate-limited, so a
// few minutes is plenty — the per-window reset countdowns are recomputed
// client-side from resets_at between refreshes.
const defaultUsageInterval = 5 * time.Minute

// usageFetchTimeout bounds a single refresh so a hung provider request can't
// wedge the refresher goroutine.
const usageFetchTimeout = 60 * time.Second

// usageCache holds the most recent agent-usage snapshot. The single writer is
// the refresher goroutine; readers are HTTP handlers. mu serializes both.
type usageCache struct {
	mu      sync.RWMutex
	snap    agentusage.Snapshot
	hasData bool
}

func (c *usageCache) store(s agentusage.Snapshot) {
	c.mu.Lock()
	c.snap = s
	c.hasData = true
	c.mu.Unlock()
}

func (c *usageCache) load() (agentusage.Snapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snap, c.hasData
}

func (s *Server) usageInterval() time.Duration {
	if s.UsageInterval > 0 {
		return s.UsageInterval
	}
	return defaultUsageInterval
}

// fetchUsage reads each agent's local OAuth credentials and queries the
// provider usage endpoint. It is strictly read-only: tmact never refreshes or
// rewrites the agents' credentials. Defaults to agentusage.Fetch over all
// providers; overridable in tests.
func (s *Server) fetchUsage(ctx context.Context) agentusage.Snapshot {
	if s.FetchUsage != nil {
		return s.FetchUsage(ctx)
	}
	return agentusage.Fetch(ctx)
}

// runUsageRefresh fetches agent usage into the cache once immediately, then on
// every tick until ctx is done. Started from Serve only when UsageEnabled.
func (s *Server) runUsageRefresh(ctx context.Context) {
	refresh := func() {
		fctx, cancel := context.WithTimeout(ctx, usageFetchTimeout)
		defer cancel()
		s.usage.store(s.fetchUsage(fctx))
	}
	refresh()
	ticker := time.NewTicker(s.usageInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		}
	}
}

func (s *Server) handleAgentUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.UsageEnabled {
		writeJSONError(w, http.StatusNotFound, "agent usage disabled")
		return
	}
	snap, ok := s.usage.load()
	if !ok {
		writeJSONError(w, http.StatusServiceUnavailable, "agent usage not yet available")
		return
	}
	writeJSON(w, http.StatusOK, snap)
}
