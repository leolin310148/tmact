package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/leolin310148/tmact/internal/agentspend"
	"github.com/leolin310148/tmact/internal/statusd"
)

// Published slices are immutable. Expensive work and disk/network I/O always
// happen outside mu, so a cold or failed refresh never blocks a trend request.
type trendCache struct {
	mu           sync.RWMutex
	state        historyState
	persistError string
}

type historyState struct {
	Version      int                       `json:"version"`
	Local        machineHistory            `json:"local"`
	Peers        map[string]machineHistory `json:"peers"`
	HistoricalAt time.Time                 `json:"historical_at"`
}

func pendingHistory(name string) machineHistory {
	return machineHistory{Machine: name, Daily: []agentspend.DailySpend{}, Usage: []usageSample{}, Pending: true}
}

func (s *Server) historySnapshot(localOnly bool) []machineHistory {
	s.history.mu.RLock()
	state, persistenceError := s.history.state, s.history.persistError
	s.history.mu.RUnlock()
	local := state.Local
	if local.Machine == "" {
		local = pendingHistory("local")
	}
	if !s.SpendEnabled {
		local.Daily = nil
		local.CostUpdatedAt = nil
	}
	if !s.UsageEnabled {
		local.Usage = nil
	}
	if persistenceError != "" {
		local.Error = joinHistoryErrors(local.Error, persistenceError)
	}
	if local.Daily == nil {
		local.Daily = []agentspend.DailySpend{}
	}
	if local.Usage == nil {
		local.Usage = []usageSample{}
	}
	rows := []machineHistory{local}
	if !localOnly {
		for _, peer := range s.spendPeers() {
			row, ok := state.Peers[peer.Name]
			if !ok {
				row = pendingHistory(peer.Name)
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func joinHistoryErrors(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "；" + b
}

func (s *Server) historyCachePath() (string, error) {
	dir, err := s.usageHistoryDir()
	return filepath.Join(dir, "trend-cache.json"), err
}

func (s *Server) loadHistoryCache() {
	path, err := s.historyCachePath()
	if err != nil {
		return
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return
	}
	var state historyState
	if err == nil {
		defer f.Close()
		err = json.NewDecoder(io.LimitReader(f, 128<<20)).Decode(&state)
	}
	s.history.mu.Lock()
	defer s.history.mu.Unlock()
	if err != nil || state.Version != 1 {
		s.history.persistError = "趨勢快取無法讀取，背景重新建立中"
		return
	}
	s.history.state = state
}

func (s *Server) persistHistoryCache() error {
	path, err := s.historyCachePath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	s.history.mu.RLock()
	state := s.history.state
	s.history.mu.RUnlock()
	f, err := os.CreateTemp(filepath.Dir(path), ".trend-cache-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(state); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (s *Server) refreshLocalHistory(now time.Time) {
	s.history.mu.RLock()
	state := s.history.state
	s.history.mu.RUnlock()
	previous := state.Local
	fresh := s.localHistory(now) // only quota history; never prices session logs
	if fresh.Error != "" && previous.Usage != nil {
		fresh.Usage = previous.Usage
		fresh.UpdatedAt = previous.UpdatedAt
	} else {
		fresh.UpdatedAt = &now
	}
	fresh.Daily = previous.Daily
	fresh.CostUpdatedAt = previous.CostUpdatedAt
	if s.SpendEnabled {
		full := state.HistoricalAt.IsZero() || state.HistoricalAt.UTC().Format("2006-01-02") != now.UTC().Format("2006-01-02")
		due := full || previous.CostUpdatedAt == nil || now.Sub(*previous.CostUpdatedAt) >= s.spendInterval()
		if due {
			days := 1
			if full {
				days = 30
			}
			compute := s.computeHistoryDaily
			if compute == nil {
				compute = agentspend.DailyChecked
			}
			rows, err := compute(now, days)
			if err != nil {
				fresh.Error = joinHistoryErrors(fresh.Error, "Cost 背景更新失敗，保留上次結果")
				s.logf("history cost refresh failed: %v", err)
			} else {
				if full {
					fresh.Daily = rows
					state.HistoricalAt = now
				} else {
					today := now.UTC().Format("2006-01-02")
					fresh.Daily = make([]agentspend.DailySpend, 0, len(previous.Daily))
					for _, row := range previous.Daily {
						if row.Date != today {
							fresh.Daily = append(fresh.Daily, row)
						}
					}
					fresh.Daily = append(fresh.Daily, rows...)
				}
				fresh.CostUpdatedAt = &now
			}
		}
	}
	fresh.Pending = s.SpendEnabled && fresh.CostUpdatedAt == nil
	if fresh.Daily == nil {
		fresh.Daily = []agentspend.DailySpend{}
	}
	state.Version = 1
	state.Local = fresh
	s.history.mu.Lock()
	s.history.state = state
	s.history.mu.Unlock()
}

func (s *Server) fetchPeerHistory(ctx context.Context, peer statusd.Peer) (machineHistory, error) {
	endpoint, err := peerHTTPURL(peer.URL, "/api/agent-usage/history", url.Values{"local": {"1"}})
	if err != nil {
		return machineHistory{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return machineHistory{}, err
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return machineHistory{}, err
	}
	defer resp.Body.Close()
	var data struct {
		Machines []machineHistory `json:"machines"`
	}
	if resp.StatusCode != 200 {
		return machineHistory{}, fmt.Errorf("history HTTP %d", resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&data); err != nil {
		return machineHistory{}, err
	}
	if len(data.Machines) != 1 {
		return machineHistory{}, fmt.Errorf("expected one local history")
	}
	row := data.Machines[0]
	row.Machine = peer.Name
	if row.Daily == nil {
		row.Daily = []agentspend.DailySpend{}
	}
	if row.Usage == nil {
		row.Usage = []usageSample{}
	}
	return row, nil
}

func (s *Server) refreshPeerHistory(ctx context.Context, now time.Time) {
	s.history.mu.RLock()
	old := s.history.state.Peers
	s.history.mu.RUnlock()
	peers := s.spendPeers()
	results := make([]machineHistory, len(peers))
	var wg sync.WaitGroup
	for i, peer := range peers {
		wg.Add(1)
		go func(i int, peer statusd.Peer) {
			defer wg.Done()
			row, err := s.fetchPeerHistory(ctx, peer)
			previous, ok := old[peer.Name]
			unavailable := row.Pending || (row.Error != "" && len(row.Daily) == 0 && len(row.Usage) == 0)
			if err != nil || (unavailable && ok && !previous.Pending) {
				if !ok {
					previous = pendingHistory(peer.Name)
				}
				previous.Error = "遠端背景更新失敗或尚未就緒，保留上次結果"
				row = previous
			} else if row.UpdatedAt == nil && !row.Pending {
				row.UpdatedAt = &now
			}
			results[i] = row
		}(i, peer)
	}
	wg.Wait()
	next := make(map[string]machineHistory, len(results))
	for _, row := range results {
		next[row.Machine] = row
	}
	s.history.mu.Lock()
	s.history.state.Peers = next
	s.history.mu.Unlock()
}

func (s *Server) runHistoryRefresh(ctx context.Context) {
	s.loadHistoryCache()
	for {
		if ctx.Err() != nil {
			return
		}
		now := time.Now()
		s.refreshLocalHistory(now)
		if ctx.Err() != nil {
			return
		}
		s.refreshPeerHistory(ctx, time.Now())
		if ctx.Err() != nil {
			return
		}
		err := s.persistHistoryCache()
		s.history.mu.Lock()
		s.history.persistError = ""
		if err != nil {
			s.history.persistError = "趨勢快取寫入失敗，目前僅保留記憶體結果"
			s.logf("history cache persist failed: %v", err)
		}
		s.history.mu.Unlock()
		// Re-read quota history and peers on the quota cadence. Cost has its own
		// hourly gate, while full historical pricing runs at UTC day rollover.
		delay := s.usageInterval()
		for _, row := range s.historySnapshot(false) {
			if row.Pending && row.Error == "" && delay > 5*time.Second {
				delay = 5 * time.Second
			}
		}
		if s.spendInterval() < delay {
			delay = s.spendInterval()
		}
		current := time.Now().UTC()
		midnight := time.Date(current.Year(), current.Month(), current.Day()+1, 0, 0, 0, 0, time.UTC)
		if until := time.Until(midnight); until < delay {
			delay = until
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
