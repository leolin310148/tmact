package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/leolin310148/tmact/internal/agentspend"
	"github.com/leolin310148/tmact/internal/agentusage"
)

type usageSample struct {
	At       time.Time               `json:"at"`
	Provider string                  `json:"provider"`
	Windows  []agentusage.RateWindow `json:"windows"`
}

type machineHistory struct {
	Machine string                  `json:"machine"`
	Daily   []agentspend.DailySpend `json:"daily"`
	Usage   []usageSample           `json:"usage"`
	Error   string                  `json:"error,omitempty"`
}

var usageHistoryMu sync.Mutex

func (s *Server) usageHistoryDir() (string, error) {
	if s.UsageHistoryDir != "" {
		return s.UsageHistoryDir, nil
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".tmact", "usage-history"), err
}

// Persist only successful, fresh quota windows: no account, credentials, or
// error payloads. Daily append-only files survive restarts and pricing changes.
func (s *Server) recordUsage(snap agentusage.Snapshot) error {
	if snap.GeneratedAt.IsZero() {
		snap.GeneratedAt = time.Now()
	}
	usageHistoryMu.Lock()
	defer usageHistoryMu.Unlock()
	dir, err := s.usageHistoryDir()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, snap.GeneratedAt.UTC().Format("2006-01-02")+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, p := range snap.Providers {
		if p.Error != "" || p.Stale || len(p.Windows) == 0 {
			continue
		}
		if err := json.NewEncoder(f).Encode(usageSample{At: snap.GeneratedAt, Provider: p.Provider, Windows: p.Windows}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) localHistory(now time.Time) machineHistory {
	out := machineHistory{Machine: "local", Daily: []agentspend.DailySpend{}, Usage: []usageSample{}}
	if s.SpendEnabled {
		out.Daily = s.dailyHistory(now)
	}
	if !s.UsageEnabled {
		return out
	}
	dir, err := s.usageHistoryDir()
	if err != nil {
		out.Error = "無法讀取 usage 歷史"
		return out
	}
	usageHistoryMu.Lock()
	defer usageHistoryMu.Unlock()
	for i := 29; i >= 0; i-- {
		path := filepath.Join(dir, now.UTC().AddDate(0, 0, -i).Format("2006-01-02")+".jsonl")
		f, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			out.Error = "部分 usage 歷史無法讀取"
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var row usageSample
			if json.Unmarshal(scanner.Bytes(), &row) == nil {
				out.Usage = append(out.Usage, row)
			}
		}
		if scanner.Err() != nil {
			out.Error = "部分 usage 歷史無法讀取"
		}
		f.Close()
	}
	if s.usageHistoryError != "" {
		out.Error = s.usageHistoryError
	}
	return out
}

func (s *Server) handleUsageHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSONError(w, 405, "method not allowed")
		return
	}
	if !s.UsageEnabled && !s.SpendEnabled {
		writeJSONError(w, 404, "agent usage disabled")
		return
	}
	rows := []machineHistory{s.localHistory(time.Now())}
	// local=1 is an explicit non-recursive peer boundary. Quotas belong to each
	// machine/account; they must never be summed across machines.
	if r.URL.Query().Get("local") != "1" {
		peers := s.spendPeers()
		results := make([]machineHistory, len(peers))
		var wg sync.WaitGroup
		for i, peer := range peers {
			wg.Add(1)
			go func(i int, name, base string) {
				defer wg.Done()
				results[i] = machineHistory{Machine: name, Daily: []agentspend.DailySpend{}, Usage: []usageSample{}, Error: "此機器的趨勢暫時無法取得"}
				endpoint, err := peerHTTPURL(base, "/api/agent-usage/history", url.Values{"local": {"1"}})
				if err != nil {
					return
				}
				ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
				if err != nil {
					return
				}
				resp, err := s.httpClient().Do(req)
				if err != nil {
					return
				}
				defer resp.Body.Close()
				var data struct {
					Machines []machineHistory `json:"machines"`
				}
				if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&data) != nil || len(data.Machines) != 1 {
					return
				}
				results[i] = data.Machines[0]
				results[i].Machine = name
			}(i, peer.Name, peer.URL)
		}
		wg.Wait()
		rows = append(rows, results...)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Machines []machineHistory `json:"machines"`
		Timezone string           `json:"timezone"`
	}{rows, "UTC"})
}

// History opens on demand; share its potentially expensive first scan across
// concurrent readers and reuse for the spend refresh interval, or UTC rollover.
func (s *Server) dailyHistory(now time.Time) []agentspend.DailySpend {
	s.historyCostMu.Lock()
	defer s.historyCostMu.Unlock()
	if s.historyCostAt.IsZero() || now.Sub(s.historyCostAt) >= s.spendInterval() || now.UTC().Format("2006-01-02") != s.historyCostAt.UTC().Format("2006-01-02") {
		s.historyCostRows = agentspend.Daily(now, 30)
		s.historyCostAt = now
	}
	return s.historyCostRows
}

func (s *Server) saveUsageHistory(snap agentusage.Snapshot) {
	err := s.recordUsage(snap)
	usageHistoryMu.Lock()
	defer usageHistoryMu.Unlock()
	s.usageHistoryError = ""
	if err != nil {
		s.usageHistoryError = "usage 歷史寫入失敗，請檢查儲存目錄權限或磁碟空間"
	}
}
