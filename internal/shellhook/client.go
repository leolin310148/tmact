package shellhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

// ErrDaemonUnavailable means the statusd unix socket doesn't exist or refused
// the connection — the daemon isn't running. Emitters treat this as a normal,
// quiet condition: the hooks are opt-in and statusd may simply be off.
var ErrDaemonUnavailable = errors.New("statusd daemon unavailable")

// DefaultSendTimeout bounds one emit round-trip. Kept tight because emits run
// on every shell prompt; a wedged daemon must not pile up hook processes.
const DefaultSendTimeout = 500 * time.Millisecond

// DefaultFetchTimeout bounds one hook-state read round-trip. Looser than the
// emit timeout because reads are interactive (diagnostics), not per-prompt.
const DefaultFetchTimeout = 2 * time.Second

// StatesResponse is the body of GET /api/hook-state: the daemon's recorded
// per-pane hook state, keyed by tmux pane id.
type StatesResponse struct {
	Panes map[string]PaneState `json:"panes"`
}

// Send posts one event to the statusd unix socket at socketPath
// (POST /api/hook-event). A zero timeout uses DefaultSendTimeout.
func Send(socketPath string, e Event, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultSendTimeout
	}
	if _, err := os.Stat(socketPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrDaemonUnavailable
		}
		return fmt.Errorf("stat %s: %w", socketPath, err)
	}
	body, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode shell hook event: %w", err)
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
	req, err := http.NewRequest(http.MethodPost, "http://daemon/api/hook-event", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		// The socket node exists but the daemon is gone or its listener is
		// dead; same quiet class as "not running".
		return ErrDaemonUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("daemon returned status %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}
	return nil
}

// FetchStates reads the daemon's recorded per-pane hook state over the unix
// socket at socketPath (GET /api/hook-state). A non-empty paneID narrows the
// result to that pane. A zero timeout uses DefaultFetchTimeout. Returns
// ErrDaemonUnavailable when the socket is missing or the daemon is gone, so
// callers can distinguish "not running" from a real request failure. Unlike
// Send, this never defaults socketPath — callers resolve it (shellhook must
// not import statusd for the default path).
func FetchStates(socketPath, paneID string, timeout time.Duration) (map[string]PaneState, error) {
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}
	if _, err := os.Stat(socketPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrDaemonUnavailable
		}
		return nil, fmt.Errorf("stat %s: %w", socketPath, err)
	}
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}
	endpoint := "http://daemon/api/hook-state"
	if paneID != "" {
		endpoint += "?pane-id=" + url.QueryEscape(paneID)
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		// Socket node exists but the daemon is gone or its listener is dead:
		// same quiet class as "not running".
		return nil, ErrDaemonUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, ErrDaemonUnavailable
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("daemon returned status %d: %s", resp.StatusCode, bytes.TrimSpace(msg))
	}
	var out StatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode hook state: %w", err)
	}
	if out.Panes == nil {
		out.Panes = map[string]PaneState{}
	}
	return out.Panes, nil
}
