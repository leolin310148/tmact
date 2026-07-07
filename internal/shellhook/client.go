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
