package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
)

// DefaultHumanActiveThreshold is how long the web UI may sit without a human
// action (pane switch or input) before /api/human-active reports inactive.
const DefaultHumanActiveThreshold = 10 * time.Minute

const fetchHumanActiveTimeout = 2 * time.Second

// humanActivityTracker remembers the last moment a human acted through the
// web UI. Zero value means no activity has been observed since the daemon
// started, which reports as inactive.
type humanActivityTracker struct {
	mu   sync.Mutex
	last time.Time
}

func (t *humanActivityTracker) record(now time.Time) {
	t.mu.Lock()
	t.last = now
	t.mu.Unlock()
}

func (t *humanActivityTracker) lastSeen() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.last
}

// recordHumanActivity marks "a human just did something in the web UI".
// Called from every browser input path (WebSocket input frames, the
// /api/pane/input HTTP fallback, peer-bound input relayed for a local
// browser) and from the explicit /api/human-activity report the frontend
// sends on pane switches.
func (s *Server) recordHumanActivity() {
	s.humanActivity.record(s.humanNowFn()())
}

func (s *Server) humanNowFn() func() time.Time {
	if s.humanNow != nil {
		return s.humanNow
	}
	return time.Now
}

// HumanActiveStatus is the /api/human-active response body, shared with the
// CLI client so both sides agree on the wire shape.
type HumanActiveStatus struct {
	Active bool `json:"active"`
	// LastActivity is the wall-clock time of the most recent human action;
	// omitted when no activity has been seen since the daemon started.
	LastActivity *time.Time `json:"last_activity,omitempty"`
	// IdleSeconds is how long ago that action was; omitted with LastActivity.
	IdleSeconds *float64 `json:"idle_seconds,omitempty"`
	// ThresholdSeconds is the inactivity cutoff used to compute Active.
	ThresholdSeconds float64 `json:"threshold_seconds"`
}

// humanActiveStatus computes the current status against threshold.
func (s *Server) humanActiveStatus(threshold time.Duration) HumanActiveStatus {
	status := HumanActiveStatus{ThresholdSeconds: threshold.Seconds()}
	last := s.humanActivity.lastSeen()
	if last.IsZero() {
		return status
	}
	idle := s.humanNowFn()().Sub(last)
	idleSeconds := idle.Seconds()
	lastCopy := last
	status.LastActivity = &lastCopy
	status.IdleSeconds = &idleSeconds
	status.Active = idle <= threshold
	return status
}

// handleHumanActive serves GET /api/human-active: whether a human has acted
// in the web UI (pane switch or input) within the threshold, 10 minutes by
// default. An optional ?threshold=15m query overrides the cutoff per request.
func (s *Server) handleHumanActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	threshold := DefaultHumanActiveThreshold
	if raw := r.URL.Query().Get("threshold"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			writeJSONError(w, http.StatusBadRequest, `invalid "threshold" parameter, expected a positive Go duration like 10m`)
			return
		}
		threshold = d
	}
	writeJSON(w, http.StatusOK, s.humanActiveStatus(threshold))
}

// handleHumanActivity ingests POST /api/human-activity, the frontend's
// explicit "the human just switched panes" report. Input already reaches the
// server (WS frames, /api/pane/input) and is recorded there; pane switches
// are otherwise invisible server-side because a WebSocket reconnect is not
// reliably a human action.
func (s *Server) handleHumanActivity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.recordHumanActivity()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// FetchHumanActive asks the running statusd daemon for its human-activity
// status over the unix socket at socketPath. A zero threshold uses the
// server default. Returns statusd.ErrDaemonUnavailable when the daemon isn't
// running.
func FetchHumanActive(socketPath string, threshold time.Duration) (HumanActiveStatus, error) {
	if socketPath == "" {
		socketPath = statusd.DefaultSocketPath
	}
	if _, err := os.Stat(socketPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return HumanActiveStatus{}, statusd.ErrDaemonUnavailable
		}
		return HumanActiveStatus{}, fmt.Errorf("stat %s: %w", socketPath, err)
	}
	client := &http.Client{
		Timeout: fetchHumanActiveTimeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
	reqURL := "http://daemon/api/human-active"
	if threshold > 0 {
		reqURL += "?threshold=" + url.QueryEscape(threshold.String())
	}
	resp, err := client.Get(reqURL)
	if err != nil {
		return HumanActiveStatus{}, statusd.ErrDaemonUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return HumanActiveStatus{}, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	var status HumanActiveStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return HumanActiveStatus{}, fmt.Errorf("decode human-active status: %w", err)
	}
	return status, nil
}
