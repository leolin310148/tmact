// Package web serves a browser UI for the statusd snapshot: it lists tmux
// sessions, streams a selected pane's output over a WebSocket, and relays
// keyboard input back into that pane.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"tmact/internal/tmux"
)

//go:embed static
var staticFS embed.FS

// paneIDPattern restricts pane targets to canonical tmux pane ids (%12). Acting
// strictly on a pane id means a request value can never be read as a tmux flag.
var paneIDPattern = regexp.MustCompile(`^%[0-9]+$`)

const (
	wsCaptureInterval = 200 * time.Millisecond
	wsCaptureLines    = 400
	wsReadLimit       = 1 << 20
)

// Server is the daemon-side HTTP server for the web UI.
type Server struct {
	// Addr is the listen address, e.g. "0.0.0.0:7890".
	Addr string
	// StatePath is the statusd snapshot file served verbatim at /api/snapshot.
	StatePath string
	// CapturePane captures pane output; defaults to tmux.CapturePane.
	CapturePane func(target string, lines int) (string, error)
	// SendText inserts literal text into a pane; defaults to tmux.PasteText.
	SendText func(target, text string, enter bool) error
	// SendKey sends one tmux key to a pane; defaults to tmux.SendKeys.
	SendKey func(target, key string) error
}

func (s *Server) capture() func(string, int) (string, error) {
	if s.CapturePane != nil {
		return s.CapturePane
	}
	return tmux.CapturePane
}

func (s *Server) sendText() func(string, string, bool) error {
	if s.SendText != nil {
		return s.SendText
	}
	return tmux.PasteText
}

func (s *Server) sendKey() func(string, string) error {
	if s.SendKey != nil {
		return s.SendKey
	}
	return func(target, key string) error { return tmux.SendKeys(target, []string{key}) }
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
	mux.HandleFunc("/ws/pane", s.handlePaneWS)
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

// inputMsg is a client-to-server WebSocket message.
type inputMsg struct {
	T string `json:"t"`           // "text", "send", or "key"
	S string `json:"s,omitempty"` // literal text for "text"/"send"
	K string `json:"k,omitempty"` // tmux key name for "key"
}

// outMsg is a server-to-client WebSocket message.
type outMsg struct {
	T string `json:"t"` // "content" or "error"
	S string `json:"s"`
}

func (s *Server) handlePaneWS(w http.ResponseWriter, r *http.Request) {
	pane := r.URL.Query().Get("pane")
	if !paneIDPattern.MatchString(pane) {
		writeJSONError(w, http.StatusBadRequest, `invalid "pane" parameter, expected a tmux pane id like %12`)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(wsReadLimit)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var writeMu sync.Mutex
	write := func(m outMsg) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return wsjson.Write(ctx, conn, m)
	}

	// Reader goroutine: relay browser input into the pane.
	go func() {
		defer cancel()
		for {
			var m inputMsg
			if err := wsjson.Read(ctx, conn, &m); err != nil {
				return
			}
			if err := s.applyInput(pane, m); err != nil {
				_ = write(outMsg{T: "error", S: err.Error()})
			}
		}
	}()

	// Main loop: stream pane output to the browser.
	ticker := time.NewTicker(wsCaptureInterval)
	defer ticker.Stop()

	last := ""
	push := func() bool {
		content, err := s.capture()(pane, wsCaptureLines)
		if err != nil {
			_ = write(outMsg{T: "error", S: err.Error()})
			return false
		}
		if content == last {
			return true
		}
		last = content
		return write(outMsg{T: "content", S: content}) == nil
	}

	if !push() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !push() {
				return
			}
		}
	}
}

// applyInput validates and relays one browser input message into the pane.
func (s *Server) applyInput(target string, m inputMsg) error {
	switch m.T {
	case "text":
		if m.S == "" {
			return nil
		}
		return s.sendText()(target, m.S, false)
	case "send":
		if m.S == "" {
			return nil
		}
		return s.sendText()(target, m.S, true)
	case "key":
		if !keyAllowed(m.K) {
			return fmt.Errorf("key not allowed: %q", m.K)
		}
		return s.sendKey()(target, m.K)
	default:
		return fmt.Errorf("unknown message type: %q", m.T)
	}
}

var allowedNamedKeys = map[string]bool{
	"Enter": true, "BSpace": true, "Tab": true, "BTab": true, "Escape": true,
	"Up": true, "Down": true, "Left": true, "Right": true,
	"Home": true, "End": true, "PageUp": true, "PageDown": true,
	"Delete": true, "Space": true,
}

// keyAllowed gates the "key" message against a fixed set of tmux key names plus
// Ctrl+<letter> combos, so an arbitrary string can never reach send-keys.
func keyAllowed(k string) bool {
	if allowedNamedKeys[k] {
		return true
	}
	if len(k) == 3 && k[0] == 'C' && k[1] == '-' && k[2] >= 'a' && k[2] <= 'z' {
		return true
	}
	return false
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
