package statusd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchSnapshotReturnsLatest(t *testing.T) {
	sock := tempUnixSocket(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()

	store := NewStore()
	store.Publish(Snapshot{Version: 1, Summary: Summary{Panes: 4}})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		snap, ok := store.Latest()
		if !ok {
			http.Error(w, "no snapshot", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())

	got, err := FetchSnapshot(sock)
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}
	if got.Summary.Panes != 4 {
		t.Fatalf("Panes = %d, want 4", got.Summary.Panes)
	}
}

func TestFetchSnapshotMissingSocketReturnsUnavailable(t *testing.T) {
	dir := t.TempDir()
	_, err := FetchSnapshot(filepath.Join(dir, "absent.sock"))
	if !errors.Is(err, ErrDaemonUnavailable) {
		t.Fatalf("err = %v, want ErrDaemonUnavailable", err)
	}
}

func TestFetchSnapshotStaleSocketReturnsUnavailable(t *testing.T) {
	sock := tempUnixSocket(t)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	ln.Close() // socket file lingers but nothing accepts.

	_, err = FetchSnapshot(sock)
	if !errors.Is(err, ErrDaemonUnavailable) {
		t.Fatalf("err = %v, want ErrDaemonUnavailable", err)
	}
}

// tempUnixSocket returns a unix-socket path short enough for the kernel's
// sun_path limit (~104 bytes on macOS). t.TempDir under /var/folders blows
// past that on macOS, so we anchor under /tmp.
func tempUnixSocket(t *testing.T) string {
	t.Helper()
	sock := filepath.Join("/tmp", fmt.Sprintf("tmact-test-%s.sock", randomToken(t)))
	t.Cleanup(func() { _ = os.Remove(sock) })
	return sock
}

func randomToken(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return hex.EncodeToString(b)
}
