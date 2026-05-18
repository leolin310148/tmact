// Package web serves a read-only browser UI for the statusd snapshot: it lists
// tmux sessions and lets the user inspect a pane's captured output. It performs
// no key sending and only ever reads pane content.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"tmact/internal/tmux"
)

//go:embed static
var staticFS embed.FS

// paneIDPattern restricts /api/pane to canonical tmux pane ids (%12). Capturing
// strictly by pane id means a request value can never be read as a tmux flag.
var paneIDPattern = regexp.MustCompile(`^%[0-9]+$`)

const (
	defaultCaptureLines = 200
	maxCaptureLines     = 5000
)

// Server is the daemon-side HTTP server for the read-only web UI.
type Server struct {
	// Addr is the listen address, e.g. "0.0.0.0:7890".
	Addr string
	// StatePath is the statusd snapshot file served verbatim at /api/snapshot.
	StatePath string
	// CapturePane captures pane output; defaults to tmux.CapturePane.
	CapturePane func(target string, lines int) (string, error)
}

func (s *Server) capture() func(string, int) (string, error) {
	if s.CapturePane != nil {
		return s.CapturePane
	}
	return tmux.CapturePane
}

// Handler builds the HTTP routes without binding a socket; useful for tests.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // embedded path is fixed at build time
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/snapshot", s.handleSnapshot)
	mux.HandleFunc("/api/pane", s.handlePane)
	return mux
}

// Serve runs the HTTP server until ctx is cancelled, then shuts down gracefully.
func (s *Server) Serve(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.StatePath)
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "snapshot not available: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *Server) handlePane(w http.ResponseWriter, r *http.Request) {
	pane := r.URL.Query().Get("pane")
	if !paneIDPattern.MatchString(pane) {
		writeJSONError(w, http.StatusBadRequest, `invalid "pane" parameter, expected a tmux pane id like %12`)
		return
	}

	lines := defaultCaptureLines
	if raw := r.URL.Query().Get("lines"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeJSONError(w, http.StatusBadRequest, `invalid "lines" parameter`)
			return
		}
		lines = n
	}
	if lines > maxCaptureLines {
		lines = maxCaptureLines
	}

	content, err := s.capture()(pane, lines)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"pane":    pane,
		"lines":   lines,
		"content": content,
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
