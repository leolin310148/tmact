package shellhook

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSendPostsEvent(t *testing.T) {
	socketPath := shortSocketPath(t)
	received := make(chan Event, 1)
	serveUnix(t, socketPath, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/hook-event" || r.Method != http.MethodPost {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var e Event
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		received <- e
		w.WriteHeader(http.StatusOK)
	})

	event := Event{Version: 1, Type: TypePreexec, PaneID: "%3", Command: "ls", Timestamp: time.Now()}
	if err := Send(socketPath, event, time.Second); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case got := <-received:
		if got.PaneID != "%3" || got.Type != TypePreexec || got.Command != "ls" {
			t.Fatalf("received = %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("server never received the event")
	}
}

func TestSendMissingSocketIsQuietSentinel(t *testing.T) {
	err := Send(filepath.Join(t.TempDir(), "nope.sock"), Event{}, time.Second)
	if !errors.Is(err, ErrDaemonUnavailable) {
		t.Fatalf("err = %v, want ErrDaemonUnavailable", err)
	}
}

func TestSendSurfacesDaemonRejection(t *testing.T) {
	socketPath := shortSocketPath(t)
	serveUnix(t, socketPath, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad event", http.StatusBadRequest)
	})
	err := Send(socketPath, Event{}, time.Second)
	if err == nil || errors.Is(err, ErrDaemonUnavailable) {
		t.Fatalf("err = %v, want explicit rejection error", err)
	}
}

// shortSocketPath returns a socket path short enough for the unix sockaddr
// limit (t.TempDir can exceed it on macOS).
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s.sock")
}

func serveUnix(t *testing.T, socketPath string, handler http.HandlerFunc) {
	t.Helper()
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
}
