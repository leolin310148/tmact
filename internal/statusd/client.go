package statusd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// ErrDaemonUnavailable means the unix socket either doesn't exist or refused
// the connection — the daemon isn't running. CLI callers can fall back to a
// one-shot live capture.
var ErrDaemonUnavailable = errors.New("statusd daemon unavailable")

const fetchSnapshotTimeout = 2 * time.Second

// FetchSnapshot asks the running daemon for its latest snapshot over the unix
// socket at path. Returns ErrDaemonUnavailable if the daemon isn't running;
// other errors mean the daemon is reachable but the request failed.
func FetchSnapshot(path string) (Snapshot, error) {
	if path == "" {
		path = DefaultSocketPath
	}
	// Stat first so a missing-daemon case returns a clean sentinel before we
	// pay the HTTP client setup cost.
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, ErrDaemonUnavailable
		}
		return Snapshot{}, fmt.Errorf("stat %s: %w", path, err)
	}

	client := &http.Client{
		Timeout: fetchSnapshotTimeout,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", path)
			},
		},
	}
	req, err := http.NewRequest(http.MethodGet, "http://daemon/api/snapshot", nil)
	if err != nil {
		return Snapshot{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		// We already confirmed the socket node exists, so any transport error
		// here means the daemon is gone or its listener is dead. Treat the
		// whole class as "daemon not available" so the CLI fallback kicks in.
		return Snapshot{}, ErrDaemonUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		return Snapshot{}, ErrDaemonUnavailable
	}
	if resp.StatusCode != http.StatusOK {
		return Snapshot{}, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}
	var snap Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return Snapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	return snap, nil
}
