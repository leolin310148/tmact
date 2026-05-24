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
	"path/filepath"
	"time"

	"github.com/leolin310148/tmact/internal/stt"
	"github.com/leolin310148/tmact/internal/tmux"
)

//go:embed static
var staticFS embed.FS

const (
	wsCaptureInterval = 200 * time.Millisecond
	wsCaptureLines    = 400
	wsReadLimit       = 1 << 20

	maxAudioUploadBytes = 25 << 20
	maxImageUploadBytes = 25 << 20
	maxFileUploadBytes  = 100 << 20
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
	// ClearPane clears the visible pane and its tmux scrollback history.
	ClearPane func(target string) error
	// STTProviderPath is the local provider config path; defaults to
	// ~/.tmact/stt_provider.json.
	STTProviderPath string
	// LoadSTTProvider loads speech-to-text provider config; defaults to reading
	// STTProviderPath.
	LoadSTTProvider func() (stt.ProviderConfig, error)
	// SaveSTTProvider persists speech-to-text provider config; defaults to
	// writing STTProviderPath.
	SaveSTTProvider func(stt.ProviderConfig) error
	// HTTPClient is used for transcription API calls; defaults to a 60s client.
	HTTPClient *http.Client
	// PasteImageDir is where pasted clipboard images are written; defaults to
	// <os.TempDir()>/tmact-paste.
	PasteImageDir string
	// UploadDir is where explicit browser file uploads are written; defaults to
	// <os.TempDir()>/tmact-upload.
	UploadDir string
	// Logf logs server-side diagnostics; defaults to writing to stderr, which
	// statusd routes to its log file.
	Logf func(format string, args ...any)
	// BuildTime is the VCS timestamp shown in the settings panel.
	BuildTime string
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

func (s *Server) clearPane() func(string) error {
	if s.ClearPane != nil {
		return s.ClearPane
	}
	return tmux.ClearPane
}

func (s *Server) sttProvider() (stt.ProviderConfig, error) {
	if s.LoadSTTProvider != nil {
		cfg, err := s.LoadSTTProvider()
		if err != nil {
			return stt.ProviderConfig{}, err
		}
		if err := cfg.NormalizeAndValidate(); err != nil {
			return stt.ProviderConfig{}, err
		}
		return cfg, nil
	}
	return stt.LoadProvider(s.STTProviderPath)
}

func (s *Server) saveSTT() func(stt.ProviderConfig) error {
	if s.SaveSTTProvider != nil {
		return s.SaveSTTProvider
	}
	return func(cfg stt.ProviderConfig) error {
		return stt.SaveProvider(s.STTProviderPath, cfg)
	}
}

func (s *Server) pasteImageDir() string {
	if s.PasteImageDir != "" {
		return s.PasteImageDir
	}
	return filepath.Join(os.TempDir(), "tmact-paste")
}

func (s *Server) uploadDir() string {
	if s.UploadDir != "" {
		return s.UploadDir
	}
	return filepath.Join(os.TempDir(), "tmact-upload")
}

func (s *Server) httpClient() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (s *Server) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
		return
	}
	fmt.Fprintf(os.Stderr, "statusd web: "+format+"\n", args...)
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
	mux.HandleFunc("/api/version", s.handleVersion)
	mux.HandleFunc("/api/settings/stt", s.handleSTTSettings)
	mux.HandleFunc("/api/transcribe", s.handleTranscribe)
	mux.HandleFunc("/api/paste-image", s.handlePasteImage)
	mux.HandleFunc("/api/upload-file", s.handleUploadFile)
	mux.HandleFunc("/api/image", s.handleImage)
	mux.HandleFunc("/ws/pane", s.handlePaneWS)
	return mux
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		BuildTime string `json:"build_time"`
	}{BuildTime: s.BuildTime})
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
