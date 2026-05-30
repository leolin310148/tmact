package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/leolin310148/tmact/internal/agentspend"
	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/statusd"
)

// Quota and token-spend are refreshed on independent cadences. Quota hits
// rate-limited provider endpoints, so it refreshes slowly; spend is a cheap
// local disk scan (cached per file), so it refreshes near the client poll rate
// to feel live. They are merged only at serve time.
const (
	// defaultUsageInterval is how often agent quota / rate-limit usage is
	// refreshed. Provider endpoints are slow-moving and rate-limited, so a few
	// minutes is plenty — reset countdowns are recomputed client-side from
	// resets_at between refreshes.
	defaultUsageInterval = 5 * time.Minute
	// defaultSpendInterval is how often token spend (local + peers) is
	// recomputed. Cost accrues slowly over a week/month window, so an hourly
	// scan is plenty — and it keeps the per-file disk walk rare. Tunable via
	// spend_interval; disable spend entirely with agent_cost=false.
	defaultSpendInterval = 1 * time.Hour
)

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

func (s *Server) spendInterval() time.Duration {
	if s.SpendInterval > 0 {
		return s.SpendInterval
	}
	return defaultSpendInterval
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

// runUsageRefresh fetches quota usage into the cache once immediately, then on
// every usageInterval tick until ctx is done. Started from Serve only when
// UsageEnabled. Token spend is refreshed separately by runSpendRefresh.
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

// spendCache holds the most recent merged (local + peers) per-provider token
// spend. Single writer (the spend refresher), many readers (HTTP handlers).
type spendCache struct {
	mu      sync.RWMutex
	m       map[string]agentusage.SpendWindow
	hasData bool
}

func (c *spendCache) store(m map[string]agentusage.SpendWindow) {
	c.mu.Lock()
	c.m = m
	c.hasData = true
	c.mu.Unlock()
}

func (c *spendCache) load() (map[string]agentusage.SpendWindow, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.m, c.hasData
}

// runSpendRefresh recomputes token spend (local logs + peers) into the cache
// once immediately, then on every spendInterval tick until ctx is done.
// Independent of the slower quota refresher.
func (s *Server) runSpendRefresh(ctx context.Context) {
	refresh := func() {
		fctx, cancel := context.WithTimeout(ctx, usageFetchTimeout)
		defer cancel()
		s.spend.store(s.computeSpend(fctx))
	}
	refresh()
	ticker := time.NewTicker(s.spendInterval())
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

// computeSpend prices the local session logs and folds in every peer's spend,
// returning the combined per-provider week/month figure.
func (s *Server) computeSpend(ctx context.Context) map[string]agentusage.SpendWindow {
	out := map[string]agentusage.SpendWindow{}
	for prov, sp := range agentspend.Compute(time.Now()) {
		out[prov] = agentusage.SpendWindow{WeekUSD: sp.WeekUSD, MonthUSD: sp.MonthUSD}
	}
	s.addPeerSpend(ctx, out)
	return out
}

// peerSpendCache holds each peer's last-known per-provider spend (keyed by
// peer name → provider → spend). Reused when a peer is momentarily
// unreachable so the merged total stays steady across a flap.
type peerSpendCache struct {
	mu sync.Mutex
	m  map[string]map[string]agentusage.SpendWindow
}

func (c *peerSpendCache) store(peer string, byProvider map[string]agentusage.SpendWindow) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]map[string]agentusage.SpendWindow{}
	}
	c.m[peer] = byProvider
}

func (c *peerSpendCache) load(peer string) (map[string]agentusage.SpendWindow, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[peer]
	return v, ok
}

// addPeerSpend folds each peer's per-provider token spend into out, so the
// panel shows the combined cost across every machine (local + peers). Quota
// windows are account-level and are NOT merged — only spend, which is disjoint
// per machine, is summed. A peer that fails to respond contributes its
// last-known spend (or nothing if never seen). Best-effort: peer errors never
// fail the computation.
func (s *Server) addPeerSpend(ctx context.Context, out map[string]agentusage.SpendWindow) {
	if len(s.Peers) == 0 {
		return
	}

	var wg sync.WaitGroup
	results := make([]map[string]agentusage.SpendWindow, len(s.Peers))
	for i, p := range s.Peers {
		wg.Add(1)
		go func(i int, p statusd.Peer) {
			defer wg.Done()
			byProvider, ok := s.fetchPeerSpend(ctx, p)
			if ok {
				s.peerSpend.store(p.Name, byProvider)
				results[i] = byProvider
				return
			}
			// Unreachable: fall back to last-known so the total doesn't dip.
			if cached, ok := s.peerSpend.load(p.Name); ok {
				results[i] = cached
			}
		}(i, p)
	}
	wg.Wait()

	for _, byProvider := range results {
		for prov, sp := range byProvider {
			cur := out[prov]
			cur.WeekUSD += sp.WeekUSD
			cur.MonthUSD += sp.MonthUSD
			out[prov] = cur
		}
	}
}

// fetchPeerSpend GETs a peer's /api/agent-usage and returns its per-provider
// spend. ok is false on any transport/HTTP/decode failure so the caller can
// fall back to the cached value.
func (s *Server) fetchPeerSpend(ctx context.Context, p statusd.Peer) (map[string]agentusage.SpendWindow, bool) {
	upstream, err := peerHTTPURL(p.URL, "/api/agent-usage", url.Values{})
	if err != nil {
		return nil, false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream, nil)
	if err != nil {
		return nil, false
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	var peerSnap agentusage.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&peerSnap); err != nil {
		return nil, false
	}
	out := make(map[string]agentusage.SpendWindow, len(peerSnap.Providers))
	for _, pu := range peerSnap.Providers {
		if pu.Spend != nil {
			out[pu.Provider] = *pu.Spend
		}
	}
	return out, true
}

func (s *Server) handleAgentUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	// The panel is available when EITHER quota (usage) or cost (spend) is on.
	// A peer can run cost-only — it serves spend here for a hub to aggregate
	// while never touching the rate-limited quota endpoints.
	if !s.UsageEnabled && !s.SpendEnabled {
		writeJSONError(w, http.StatusNotFound, "agent usage disabled")
		return
	}
	snap, haveQuota := s.usage.load()
	spend, haveSpend := s.spend.load()
	if !haveQuota && !haveSpend {
		writeJSONError(w, http.StatusServiceUnavailable, "agent usage not yet available")
		return
	}
	if !haveQuota {
		// Cost-only mode (quota disabled): there is no quota snapshot to base
		// on, so the providers come entirely from the spend map.
		snap = agentusage.Snapshot{GeneratedAt: time.Now()}
	}
	if haveSpend {
		snap = withSpend(snap, spend)
	}
	writeJSON(w, http.StatusOK, snap)
}

// withSpend returns a copy of snap with each provider's Spend taken from the
// spend map. Providers present only in the spend map (e.g. cost-only mode,
// where there is no quota snapshot) are appended in stable name order. The
// Providers slice is copied so the shared cached quota snapshot is never
// mutated under concurrent requests.
func withSpend(snap agentusage.Snapshot, spend map[string]agentusage.SpendWindow) agentusage.Snapshot {
	out := snap
	out.Providers = make([]agentusage.ProviderUsage, len(snap.Providers))
	copy(out.Providers, snap.Providers)

	seen := make(map[string]bool, len(out.Providers))
	for i := range out.Providers {
		seen[out.Providers[i].Provider] = true
		if sp, ok := spend[out.Providers[i].Provider]; ok {
			spCopy := sp
			out.Providers[i].Spend = &spCopy
		}
	}

	extra := make([]string, 0, len(spend))
	for prov := range spend {
		if !seen[prov] {
			extra = append(extra, prov)
		}
	}
	sort.Strings(extra)
	for _, prov := range extra {
		spCopy := spend[prov]
		out.Providers = append(out.Providers, agentusage.ProviderUsage{Provider: prov, Spend: &spCopy})
	}
	return out
}
