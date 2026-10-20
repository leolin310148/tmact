package web

import (
	"bufio"
	"encoding/json"
	"net/http"
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
	Machine       string                  `json:"machine"`
	Daily         []agentspend.DailySpend `json:"daily"`
	Usage         []usageSample           `json:"usage"`
	Error         string                  `json:"error,omitempty"`
	UpdatedAt     *time.Time              `json:"updated_at,omitempty"`
	CostUpdatedAt *time.Time              `json:"cost_updated_at,omitempty"`
	Pending       bool                    `json:"pending,omitempty"`
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
	start := agentspend.TrendDayStart(now).AddDate(0, 0, -29)
	// Physical JSONL filenames stay UTC for compatibility. A 30-day Taiwan
	// interval can overlap 31 UTC files; filter by absolute sample time below.
	for i := 30; i >= 0; i-- {
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
			if json.Unmarshal(scanner.Bytes(), &row) == nil && !row.At.Before(start) && !row.At.After(now) {
				// Pace is derived for the current quota panel, not used by history.
				// Keep the durable JSONL intact but omit it from published snapshots.
				for i := range row.Windows {
					row.Windows[i].Pace = nil
				}
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
	// Request handling never scans session files or waits for remote machines.
	rows := s.historySnapshot(r.URL.Query().Get("local") == "1")
	if r.URL.Query().Get("cost_only") == "1" {
		for i := range rows {
			rows[i].Usage = []usageSample{}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Machines []machineHistory `json:"machines"`
		Timezone string           `json:"timezone"`
	}{rows, agentspend.TrendTimezone})
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
