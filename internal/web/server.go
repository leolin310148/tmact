// Package web serves a browser UI for the statusd snapshot: it lists tmux
// sessions, streams a selected pane's output over a WebSocket, and relays
// keyboard input back into that pane.
package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"tmact/internal/stt"
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

	maxAudioUploadBytes = 25 << 20
	maxImageUploadBytes = 25 << 20
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
	// Logf logs server-side diagnostics; defaults to writing to stderr, which
	// statusd routes to its log file.
	Logf func(format string, args ...any)
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
	mux.HandleFunc("/api/settings/stt", s.handleSTTSettings)
	mux.HandleFunc("/api/transcribe", s.handleTranscribe)
	mux.HandleFunc("/api/paste-image", s.handlePasteImage)
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

// sttSettings is the masked STT provider config exposed to the browser. The
// API key is write-only — never sent back, only whether one is configured.
type sttSettings struct {
	Model      string `json:"model"`
	Endpoint   string `json:"endpoint"`
	Configured bool   `json:"configured"`
}

// handleSTTSettings reads (GET) or updates (PUT) the server-side STT provider
// config. A PUT with a blank api_key keeps the stored key, so the model or
// endpoint can be changed without re-entering the secret.
func (s *Server) handleSTTSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		out := sttSettings{Model: stt.DefaultModel, Endpoint: stt.DefaultEndpoint}
		if cfg, err := s.sttProvider(); err == nil {
			out.Model = cfg.Model
			out.Endpoint = cfg.Endpoint
			out.Configured = cfg.APIKey != ""
		}
		writeJSON(w, http.StatusOK, out)

	case http.MethodPut:
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var body struct {
			Model    string `json:"model"`
			Endpoint string `json:"endpoint"`
			APIKey   string `json:"api_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
		cfg := stt.ProviderConfig{
			Provider: stt.DefaultProvider,
			Model:    strings.TrimSpace(body.Model),
			Endpoint: strings.TrimSpace(body.Endpoint),
			APIKey:   strings.TrimSpace(body.APIKey),
		}
		// A blank api_key means "keep the current key" — load it back in.
		if cfg.APIKey == "" {
			if existing, err := s.sttProvider(); err == nil {
				cfg.APIKey = existing.APIKey
			}
		}
		if err := s.saveSTT()(cfg); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		_ = cfg.NormalizeAndValidate() // fill defaults for the response
		writeJSON(w, http.StatusOK, sttSettings{
			Model:      cfg.Model,
			Endpoint:   cfg.Endpoint,
			Configured: cfg.APIKey != "",
		})

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	provider, err := s.sttProvider()
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "voice transcription is not configured: "+err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAudioUploadBytes)
	if err := r.ParseMultipartForm(maxAudioUploadBytes); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "request body too large") {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSONError(w, status, "invalid audio upload: "+err.Error())
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, `missing "audio" upload`)
		return
	}
	defer file.Close()

	// The browser's declared filename and content type are unreliable — iOS
	// Safari records MP4 but labels the blob audio/webm — and the API rejects a
	// file whose extension does not match its bytes. Sniff the container from
	// the leading bytes and name the upload ourselves.
	filename := header.Filename
	contentType := header.Header.Get("Content-Type")
	if ext := sniffAudioExtension(file); ext != "" {
		filename = "recording." + ext
		contentType = audioMIMEByExt[ext]
	} else if filename == "" {
		filename = "recording.webm"
	}

	transcript, err := s.transcribeAudio(r.Context(), provider, filename, contentType, file)
	if err != nil {
		s.logf("transcribe failed (file=%s): %v", filename, err)
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		writeJSONError(w, http.StatusBadGateway, "transcription API returned an empty transcript")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"text": transcript})
}

func (s *Server) transcribeAudio(ctx context.Context, provider stt.ProviderConfig, filename, contentType string, audio io.Reader) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("model", provider.Model); err != nil {
		return "", fmt.Errorf("build transcription request: %w", err)
	}
	if err := mw.WriteField("response_format", "json"); err != nil {
		return "", fmt.Errorf("build transcription request: %w", err)
	}

	part, err := createMultipartFile(mw, "file", filename, contentType)
	if err != nil {
		return "", fmt.Errorf("build transcription request: %w", err)
	}
	if _, err := io.Copy(part, audio); err != nil {
		return "", fmt.Errorf("read audio upload: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("build transcription request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.Endpoint, &body)
	if err != nil {
		return "", fmt.Errorf("build transcription request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := s.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("transcription API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return "", fmt.Errorf("transcription API returned HTTP %d: %s", resp.StatusCode, msg)
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode transcription response: %w", err)
	}
	return out.Text, nil
}

// audioMIMEByExt maps a sniffed container extension to the MIME type sent to
// the transcription API.
var audioMIMEByExt = map[string]string{
	"webm": "audio/webm",
	"ogg":  "audio/ogg",
	"m4a":  "audio/mp4",
	"mp3":  "audio/mpeg",
	"wav":  "audio/wav",
	"flac": "audio/flac",
}

// sniffAudioExtension reads the leading bytes of an upload and returns a
// canonical container extension (no dot), or "" if it recognises nothing. It
// rewinds the reader so the caller still sees the whole file.
func sniffAudioExtension(rs io.ReadSeeker) string {
	head := make([]byte, 16)
	n, _ := io.ReadFull(rs, head)
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	head = head[:n]
	switch {
	case len(head) >= 4 && head[0] == 0x1A && head[1] == 0x45 && head[2] == 0xDF && head[3] == 0xA3:
		return "webm" // EBML — WebM / Matroska
	case len(head) >= 4 && string(head[:4]) == "OggS":
		return "ogg"
	case len(head) >= 8 && string(head[4:8]) == "ftyp":
		return "m4a" // ISO base media — MP4 / M4A
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WAVE":
		return "wav"
	case len(head) >= 4 && string(head[:4]) == "fLaC":
		return "flac"
	case len(head) >= 3 && string(head[:3]) == "ID3":
		return "mp3"
	case len(head) >= 2 && head[0] == 0xFF && head[1]&0xE0 == 0xE0:
		return "mp3" // MPEG audio frame sync
	default:
		return ""
	}
}

func createMultipartFile(mw *multipart.Writer, field, filename, contentType string) (io.Writer, error) {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name":     field,
		"filename": filename,
	}))
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	return mw.CreatePart(h)
}

// handlePasteImage accepts a clipboard image upload, writes it to a server-side
// file, and returns that file's absolute path. A terminal pane is a keystroke
// stream with no channel for raw image bytes, so the browser relays the path as
// text instead — every supported agent reads an image when given its path.
func (s *Server) handlePasteImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImageUploadBytes)
	if err := r.ParseMultipartForm(maxImageUploadBytes); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "request body too large") {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSONError(w, status, "invalid image upload: "+err.Error())
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, `missing "image" upload`)
		return
	}
	defer file.Close()

	// Trust the bytes, not the browser's declared type: sniff the container so
	// the saved file always carries a correct, agent-recognisable extension.
	ext := sniffImageExtension(file)
	if ext == "" {
		writeJSONError(w, http.StatusUnsupportedMediaType,
			"unsupported image format (expected PNG, JPEG, GIF, WebP, or BMP)")
		return
	}

	dir := s.pasteImageDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.logf("paste-image: mkdir %s: %v", dir, err)
		writeJSONError(w, http.StatusInternalServerError, "could not create the image directory")
		return
	}
	out, err := os.CreateTemp(dir, "paste-"+time.Now().Format("20060102-150405")+"-*."+ext)
	if err != nil {
		s.logf("paste-image: create file in %s: %v", dir, err)
		writeJSONError(w, http.StatusInternalServerError, "could not save the image")
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		_ = os.Remove(out.Name())
		s.logf("paste-image: write %s: %v", out.Name(), err)
		writeJSONError(w, http.StatusInternalServerError, "could not save the image")
		return
	}

	path := out.Name()
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// sniffImageExtension reads an upload's leading bytes and returns a canonical
// image extension (no dot), or "" for anything it does not recognise as an
// image. It rewinds the reader so the caller still sees the whole file.
func sniffImageExtension(rs io.ReadSeeker) string {
	head := make([]byte, 16)
	n, _ := io.ReadFull(rs, head)
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	head = head[:n]
	switch {
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		return "png"
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return "jpg"
	case len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return "gif"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "webp"
	case len(head) >= 2 && head[0] == 'B' && head[1] == 'M':
		return "bmp"
	default:
		return ""
	}
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
