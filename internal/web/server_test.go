package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/leolin310148/tmact/internal/stt"
)

/* ---- HTTP endpoints ---- */

func TestSnapshotServesFileVerbatim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "status.json")
	body := `{"version":1,"summary":{"sessions":2}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{StatePath: path}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != body {
		t.Fatalf("body = %q, want %q", got, body)
	}
}

func TestSnapshotMissingFileReturns503(t *testing.T) {
	handler := (&Server{StatePath: filepath.Join(t.TempDir(), "absent.json")}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestIndexPageServed(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>tmact") {
		t.Fatal("index page body missing expected title")
	}
}

func TestIndexIncludesPWAInstallHooks(t *testing.T) {
	handler := (&Server{}).Handler()
	body := servedBody(t, handler, "/")
	app := servedBody(t, handler, "/app.js")

	for _, want := range []string{
		`<meta name="theme-color" content="#0e1116" />`,
		`<link rel="manifest" href="/manifest.json" />`,
		`<link rel="apple-touch-icon" href="/icons/icon-180.png" />`,
		`<link rel="stylesheet" href="/app.css" />`,
		`<script src="/app.js"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index page missing %q", want)
		}
	}
	if !strings.Contains(app, `navigator.serviceWorker.register("/sw.js")`) {
		t.Fatal("app script missing service worker registration")
	}
}

func TestIndexIncludesVoiceTranscribeControls(t *testing.T) {
	handler := (&Server{}).Handler()
	body := servedBody(t, handler, "/")
	app := servedBody(t, handler, "/app.js")

	for _, want := range []string{
		`id="record-btn"`,
		`id="rec-send"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index page missing %q", want)
		}
	}
	for _, want := range []string{
		`MediaRecorder`,
		`navigator.mediaDevices.getUserMedia`,
		`fetch("/api/transcribe"`,
		`insertTranscript`,
		`finishRecordingConfirm`,
		`suppressRecordTextInput`,
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script missing %q", want)
		}
	}
}

func TestIndexIncludesMobileUploadControls(t *testing.T) {
	handler := (&Server{}).Handler()
	body := servedBody(t, handler, "/")
	style := servedBody(t, handler, "/app.css")
	app := servedBody(t, handler, "/app.js")

	for _, want := range []string{
		`id="upload-btn"`,
		`id="selection-btn"`,
		`id="file-upload"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index page missing %q", want)
		}
	}
	for _, want := range []string{
		`/api/upload-file`,
		`openFileUploadPicker`,
		`upload-btn").addEventListener("click"`,
		`selection-btn").addEventListener("click", toggleSelectionMode)`,
		`if (state.selectionMode)`,
		`!state.selectionMode && document.activeElement === $("direct-input")`,
		`tone: "upload"`,
		`tone: "selection"`,
		`tone: "settings"`,
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script missing %q", want)
		}
	}
	for _, want := range []string{
		`.selection-btn`,
		`.selection-btn.active`,
		`.content-wrap.selection-mode::after`,
		`.help-ring.tone-upload`,
		`.help-tip.tone-settings`,
		`bottom: 60px;`,
	} {
		if !strings.Contains(style, want) {
			t.Fatalf("style sheet missing %q", want)
		}
	}
}

func TestIndexIncludesSettingsControls(t *testing.T) {
	handler := (&Server{}).Handler()
	body := servedBody(t, handler, "/")
	style := servedBody(t, handler, "/app.css")
	app := servedBody(t, handler, "/app.js")

	for _, want := range []string{
		`id="gear-btn"`,
		`id="settings-overlay"`,
		`id="font-range"`,
		`id="running-effect"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("index page missing %q", want)
		}
	}
	if !strings.Contains(style, `--pane-font`) {
		t.Fatal("app stylesheet missing pane font variable")
	}
	if !strings.Contains(app, `fetch("/api/settings/stt"`) {
		t.Fatal("app script missing settings API call")
	}
	if !strings.Contains(app, `applyRunningEffect`) {
		t.Fatal("app script missing running effect setting")
	}
}

func TestAppIncludesAgentChipIconsAndAsciiRules(t *testing.T) {
	handler := (&Server{}).Handler()
	app := servedBody(t, handler, "/app.js")
	style := servedBody(t, handler, "/app.css")

	for _, want := range []string{
		`const RUNTIME_ICON = { claude: "cc", codex: "cx", copilot: "cp", gemini: "g" }`,
		`function paneIndicator(p)`,
		`class: cls.join(" ")`,
		`if (!p.asking) return null;`,
		`/^[─-]+$/`,
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("app script missing %q", want)
		}
	}
	for _, want := range []string{
		`.agent-icon.runtime-claude`,
		`.agent-icon.runtime-codex`,
		`.agent-icon.runtime-copilot`,
		`.agent-icon.runtime-gemini`,
		`@keyframes agent-shine`,
		`@keyframes agent-rainbow`,
		`display: block;`,
	} {
		if !strings.Contains(style, want) {
			t.Fatalf("app style missing %q", want)
		}
	}
}

func TestPWAAssetsContentTypes(t *testing.T) {
	handler := (&Server{}).Handler()
	tests := []struct {
		path        string
		contentType string
	}{
		{"/manifest.json", "application/json"},
		{"/app.css", "text/css"},
		{"/app.js", "text/javascript"},
		{"/sw.js", "text/javascript"},
		{"/icons/icon-180.png", "image/png"},
		{"/icons/icon-192.png", "image/png"},
		{"/icons/icon-512.png", "image/png"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tt.contentType) {
				t.Fatalf("Content-Type = %q, want prefix %q", got, tt.contentType)
			}
		})
	}
}

func servedBody(t *testing.T, handler http.Handler, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200", path, rec.Code)
	}
	return rec.Body.String()
}

func TestServiceWorkerBypassesLiveEndpoints(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sw.js", nil))

	body := rec.Body.String()
	for _, want := range []string{
		`url.pathname.startsWith("/api/")`,
		`url.pathname.startsWith("/ws/")`,
		`APP_SHELL_PATHS.has(url.pathname)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("service worker missing %q", want)
		}
	}
}

func audioUploadRequest(t *testing.T, path string, bodyText string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("audio", "recording.webm")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, bodyText); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestTranscribeMissingProviderConfigReturns503(t *testing.T) {
	handler := (&Server{STTProviderPath: filepath.Join(t.TempDir(), "missing.json")}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, audioUploadRequest(t, "/api/transcribe", "audio bytes"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "tmact stt-set --provider openai --api-key") {
		t.Fatalf("body = %q, want stt-set guidance", rec.Body.String())
	}
}

func TestTranscribeForwardsAudioToAPI(t *testing.T) {
	var sawRequest bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if got := r.FormValue("model"); got != "whisper-1" {
			t.Errorf("model = %q, want whisper-1", got)
		}
		if got := r.FormValue("response_format"); got != "json" {
			t.Errorf("response_format = %q, want json", got)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer file.Close()
		if header.Filename != "recording.webm" {
			t.Errorf("filename = %q, want recording.webm", header.Filename)
		}
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "audio bytes" {
			t.Errorf("file body = %q, want audio bytes", string(data))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "hello from voice"})
	}))
	defer api.Close()

	handler := (&Server{
		LoadSTTProvider: func() (stt.ProviderConfig, error) {
			return stt.ProviderConfig{
				Provider: "openai",
				APIKey:   "test-key",
				Model:    "whisper-1",
				Endpoint: api.URL,
			}, nil
		},
	}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, audioUploadRequest(t, "/api/transcribe", "audio bytes"))

	if !sawRequest {
		t.Fatal("mock API was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["text"] != "hello from voice" {
		t.Fatalf("text = %q, want hello from voice", got["text"])
	}
}

func TestSniffAudioExtension(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		want string
	}{
		{"webm", []byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0}, "webm"},
		{"ogg", []byte("OggS\x00\x00\x00\x00"), "ogg"},
		{"mp4", []byte("\x00\x00\x00\x20ftypM4A "), "m4a"},
		{"wav", []byte("RIFF\x00\x00\x00\x00WAVEfmt "), "wav"},
		{"mp3-id3", []byte("ID3\x04\x00\x00\x00\x00"), "mp3"},
		{"unknown", []byte("audio bytes here"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := bytes.NewReader(tc.head)
			if got := sniffAudioExtension(rs); got != tc.want {
				t.Fatalf("sniffAudioExtension = %q, want %q", got, tc.want)
			}
			if pos, _ := rs.Seek(0, io.SeekCurrent); pos != 0 {
				t.Fatalf("reader left at offset %d, want rewound to 0", pos)
			}
		})
	}
}

// iOS Safari records MP4 but labels the upload .webm; the server must rename
// it to match the sniffed container so the transcription API accepts it.
func TestTranscribeRenamesUploadToSniffedContainer(t *testing.T) {
	var gotFilename string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		_, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		gotFilename = header.Filename
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "ok"})
	}))
	defer api.Close()

	handler := (&Server{
		LoadSTTProvider: func() (stt.ProviderConfig, error) {
			return stt.ProviderConfig{Provider: "openai", APIKey: "test-key", Endpoint: api.URL}, nil
		},
	}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, audioUploadRequest(t, "/api/transcribe", "\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if gotFilename != "recording.m4a" {
		t.Fatalf("forwarded filename = %q, want recording.m4a (sniffed from MP4 bytes)", gotFilename)
	}
}

func TestTranscribeAPIFailureReturns502(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed", http.StatusBadGateway)
	}))
	defer api.Close()

	handler := (&Server{
		LoadSTTProvider: func() (stt.ProviderConfig, error) {
			return stt.ProviderConfig{Provider: "openai", APIKey: "test-key", Endpoint: api.URL}, nil
		},
	}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, audioUploadRequest(t, "/api/transcribe", "audio bytes"))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "transcription API returned HTTP 502") {
		t.Fatalf("body = %q, want upstream error", rec.Body.String())
	}
}

func TestTranscribeEmptyTranscriptReturns502(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "   "})
	}))
	defer api.Close()

	handler := (&Server{
		LoadSTTProvider: func() (stt.ProviderConfig, error) {
			return stt.ProviderConfig{Provider: "openai", APIKey: "test-key", Endpoint: api.URL}, nil
		},
	}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, audioUploadRequest(t, "/api/transcribe", "audio bytes"))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "empty transcript") {
		t.Fatalf("body = %q, want empty transcript error", rec.Body.String())
	}
}

func TestTranscribeMissingAudioReturns400(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/transcribe", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	handler := (&Server{
		LoadSTTProvider: func() (stt.ProviderConfig, error) {
			return stt.ProviderConfig{Provider: "openai", APIKey: "test-key"}, nil
		},
	}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

/* ---- image paste ---- */

func imageUploadRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("image", "clipboard.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, body); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/paste-image", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestPasteImageSavesFileAndReturnsPath(t *testing.T) {
	dir := t.TempDir()
	handler := (&Server{PasteImageDir: dir}).Handler()
	rec := httptest.NewRecorder()
	png := "\x89PNG\r\n\x1a\n" + "pretend image body"
	handler.ServeHTTP(rec, imageUploadRequest(t, png))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	path := got["path"]
	if !strings.HasPrefix(path, dir) {
		t.Fatalf("path = %q, want it saved under %q", path, dir)
	}
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("path = %q, want a .png extension sniffed from the bytes", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("saved image not readable: %v", err)
	}
	if string(data) != png {
		t.Fatalf("saved bytes = %q, want the uploaded image verbatim", string(data))
	}
}

func TestPasteImageRejectsNonImage(t *testing.T) {
	handler := (&Server{PasteImageDir: t.TempDir()}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, imageUploadRequest(t, "this is plain text, not an image"))

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
}

func TestPasteImageRejectsNonPOST(t *testing.T) {
	handler := (&Server{PasteImageDir: t.TempDir()}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/paste-image", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestSniffImageExtension(t *testing.T) {
	cases := []struct {
		name string
		head []byte
		want string
	}{
		{"png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x00"), "png"},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0}, "jpg"},
		{"gif87", []byte("GIF87a\x00\x00\x00\x00\x00\x00"), "gif"},
		{"gif89", []byte("GIF89a\x00\x00\x00\x00\x00\x00"), "gif"},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "webp"},
		{"bmp", []byte("BM\x00\x00\x00\x00\x00\x00"), "bmp"},
		{"unknown", []byte("plain text bytes"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rs := bytes.NewReader(tc.head)
			if got := sniffImageExtension(rs); got != tc.want {
				t.Fatalf("sniffImageExtension = %q, want %q", got, tc.want)
			}
			if pos, _ := rs.Seek(0, io.SeekCurrent); pos != 0 {
				t.Fatalf("reader left at offset %d, want rewound to 0", pos)
			}
		})
	}
}

/* ---- file upload ---- */

func fileUploadRequest(t *testing.T, field, filename, bodyText string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, bodyText); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/upload-file", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestUploadFileSavesFileAndReturnsPath(t *testing.T) {
	dir := t.TempDir()
	handler := (&Server{UploadDir: dir}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fileUploadRequest(t, "file", "../notes?.txt", "hello upload"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	path := got["path"]
	if !strings.HasPrefix(path, dir) {
		t.Fatalf("path = %q, want it saved under %q", path, dir)
	}
	if !strings.HasSuffix(path, "notes.txt") {
		t.Fatalf("path = %q, want sanitized filename suffix notes.txt", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("saved upload not readable: %v", err)
	}
	if string(data) != "hello upload" {
		t.Fatalf("saved bytes = %q, want uploaded file verbatim", string(data))
	}
}

func TestUploadFileRejectsMissingFile(t *testing.T) {
	handler := (&Server{UploadDir: t.TempDir()}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, fileUploadRequest(t, "other", "notes.txt", "hello"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestUploadFileRejectsNonPOST(t *testing.T) {
	handler := (&Server{UploadDir: t.TempDir()}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/upload-file", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestSanitizeUploadFilename(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"../notes?.txt", "notes.txt"},
		{"  .hidden  ", "hidden"},
		{"", "file"},
		{"résumé 2026.pdf", "r-sum-2026.pdf"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := sanitizeUploadFilename(tc.in); got != tc.want {
				t.Fatalf("sanitizeUploadFilename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

/* ---- STT settings ---- */

func TestSTTSettingsGetUnconfigured(t *testing.T) {
	handler := (&Server{STTProviderPath: filepath.Join(t.TempDir(), "missing.json")}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings/stt", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got sttSettings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Configured {
		t.Fatal("configured = true, want false for a missing config")
	}
	if got.Model != stt.DefaultModel || got.Endpoint != stt.DefaultEndpoint {
		t.Fatalf("got %+v, want defaults", got)
	}
}

func TestSTTSettingsGetMasksAPIKey(t *testing.T) {
	handler := (&Server{
		LoadSTTProvider: func() (stt.ProviderConfig, error) {
			return stt.ProviderConfig{
				Provider: "openai", APIKey: "sk-secret",
				Model: "whisper-1", Endpoint: "https://api.example/v1",
			}, nil
		},
	}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings/stt", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "sk-secret") {
		t.Fatalf("GET leaked the API key: %q", rec.Body.String())
	}
	var got sttSettings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Configured || got.Model != "whisper-1" {
		t.Fatalf("got %+v, want configured whisper-1", got)
	}
}

// A PUT with a real key writes it to disk, but it must never come back out:
// neither the PUT response nor a follow-up GET may echo the secret.
func TestSTTSettingsPutPersistsAndKeepsKeySecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stt.json")
	handler := (&Server{STTProviderPath: path}).Handler()

	put := `{"model":"gpt-4o-transcribe","endpoint":"https://api.example/v1","api_key":"sk-secret"}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/settings/stt", strings.NewReader(put)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sk-secret") {
		t.Fatalf("PUT response leaked the API key: %q", rec.Body.String())
	}

	saved, err := stt.LoadProvider(path)
	if err != nil {
		t.Fatalf("LoadProvider: %v", err)
	}
	if saved.APIKey != "sk-secret" {
		t.Fatalf("saved api key = %q, want sk-secret", saved.APIKey)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings/stt", nil))
	if strings.Contains(rec.Body.String(), "sk-secret") {
		t.Fatalf("GET response leaked the API key: %q", rec.Body.String())
	}
	var got sttSettings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Configured || got.Model != "gpt-4o-transcribe" || got.Endpoint != "https://api.example/v1" {
		t.Fatalf("GET got %+v", got)
	}
}

// A PUT with a blank api_key changes the model/endpoint while keeping the
// stored key, so the secret never has to be re-typed.
func TestSTTSettingsPutBlankKeyKeepsExistingKey(t *testing.T) {
	var saved stt.ProviderConfig
	handler := (&Server{
		LoadSTTProvider: func() (stt.ProviderConfig, error) {
			return stt.ProviderConfig{
				Provider: "openai", APIKey: "old-key",
				Model: "old-model", Endpoint: "https://old.example",
			}, nil
		},
		SaveSTTProvider: func(cfg stt.ProviderConfig) error {
			saved = cfg
			return nil
		},
	}).Handler()

	put := `{"model":"new-model","endpoint":"https://new.example","api_key":""}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/settings/stt", strings.NewReader(put)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if saved.APIKey != "old-key" {
		t.Fatalf("saved api key = %q, want the existing old-key kept", saved.APIKey)
	}
	if saved.Model != "new-model" || saved.Endpoint != "https://new.example" {
		t.Fatalf("saved %+v, want the new model/endpoint", saved)
	}
}

func TestSTTSettingsPutMissingKeyRejected(t *testing.T) {
	handler := (&Server{STTProviderPath: filepath.Join(t.TempDir(), "stt.json")}).Handler()
	put := `{"model":"whisper-1","endpoint":"https://api.example","api_key":""}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/settings/stt", strings.NewReader(put)))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "api_key") {
		t.Fatalf("body = %q, want a missing-key error", rec.Body.String())
	}
}

func TestSTTSettingsRejectsPost(t *testing.T) {
	handler := (&Server{STTProviderPath: filepath.Join(t.TempDir(), "stt.json")}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/settings/stt", strings.NewReader("{}")))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

/* ---- input validation ---- */

func TestKeyAllowed(t *testing.T) {
	allowed := []string{"Enter", "BSpace", "BTab", "Escape", "Up", "PageDown", "C-c", "C-z", "Space"}
	denied := []string{"", "rm", "C-C", "C-1", "M-x", "Enter; rm", "-X", "ArrowUp"}
	for _, k := range allowed {
		if !keyAllowed(k) {
			t.Errorf("keyAllowed(%q) = false, want true", k)
		}
	}
	for _, k := range denied {
		if keyAllowed(k) {
			t.Errorf("keyAllowed(%q) = true, want false", k)
		}
	}
}

func TestApplyInputText(t *testing.T) {
	var gotTarget, gotText string
	var gotEnter bool
	s := &Server{SendText: func(target, text string, enter bool) error {
		gotTarget, gotText, gotEnter = target, text, enter
		return nil
	}}
	if err := s.applyInput("%7", inputMsg{T: "text", S: "hi"}); err != nil {
		t.Fatal(err)
	}
	if gotTarget != "%7" || gotText != "hi" || gotEnter {
		t.Fatalf("SendText got (%q, %q, %v)", gotTarget, gotText, gotEnter)
	}
}

func TestApplyInputSendUsesEnter(t *testing.T) {
	var gotEnter bool
	s := &Server{SendText: func(_, _ string, enter bool) error {
		gotEnter = enter
		return nil
	}}
	if err := s.applyInput("%7", inputMsg{T: "send", S: "a prompt"}); err != nil {
		t.Fatal(err)
	}
	if !gotEnter {
		t.Fatal(`"send" message must paste with Enter`)
	}
}

func TestApplyInputKeyAllowed(t *testing.T) {
	var gotKey string
	s := &Server{SendKey: func(_, key string) error {
		gotKey = key
		return nil
	}}
	if err := s.applyInput("%7", inputMsg{T: "key", K: "C-c"}); err != nil {
		t.Fatal(err)
	}
	if gotKey != "C-c" {
		t.Fatalf("SendKey got %q, want C-c", gotKey)
	}
}

func TestApplyInputKeyRejected(t *testing.T) {
	s := &Server{SendKey: func(_, _ string) error {
		t.Fatal("SendKey must not run for a disallowed key")
		return nil
	}}
	if err := s.applyInput("%7", inputMsg{T: "key", K: "rm -rf"}); err == nil {
		t.Fatal("expected error for disallowed key")
	}
}

func TestApplyInputUnknownType(t *testing.T) {
	if err := (&Server{}).applyInput("%7", inputMsg{T: "bogus"}); err == nil {
		t.Fatal("expected error for unknown message type")
	}
}

/* ---- /ws/pane integration ---- */

func wsURL(httpURL string) string {
	return "ws" + strings.TrimPrefix(httpURL, "http")
}

func dialPane(t *testing.T, srv *httptest.Server, pane string) (*websocket.Conn, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	t.Cleanup(cancel)
	c, _, err := websocket.Dial(ctx, wsURL(srv.URL)+"/ws/pane?pane="+pane, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c, ctx
}

func TestPaneWSStreamsContent(t *testing.T) {
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return "pane body", nil },
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	var m outMsg
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatal(err)
	}
	if m.T != "content" || m.S != "pane body" {
		t.Fatalf("got %+v, want content/pane body", m)
	}
}

func TestPaneWSContentCarriesDetectedQuestion(t *testing.T) {
	menu := "Which approach?\n❯ 1. Use a library\n  2. Hand-roll it\n"
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return menu, nil },
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	var m outMsg
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatal(err)
	}
	if m.T != "content" || m.Q == nil {
		t.Fatalf("got %+v, want a content message with a question", m)
	}
	if len(m.Q.Choices) != 2 {
		t.Fatalf("question choices = %d, want 2", len(m.Q.Choices))
	}
}

func TestPaneWSContentOmitsQuestionWhenNone(t *testing.T) {
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return "plain output, no menu", nil },
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	var m outMsg
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatal(err)
	}
	if m.T != "content" || m.Q != nil {
		t.Fatalf("got %+v, want a content message with no question", m)
	}
}

func TestPaneWSAppliesTextInput(t *testing.T) {
	type call struct {
		target, text string
		enter        bool
	}
	calls := make(chan call, 4)
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return "", nil },
		SendText: func(target, text string, enter bool) error {
			calls <- call{target, text, enter}
			return nil
		},
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	if err := wsjson.Write(ctx, c, inputMsg{T: "text", S: "hello"}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-calls:
		if got.target != "%11" || got.text != "hello" || got.enter {
			t.Fatalf("SendText got %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("SendText was not called")
	}
}

func TestPaneWSRejectsDisallowedKey(t *testing.T) {
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return "", nil },
		SendKey: func(string, string) error {
			t.Error("SendKey must not run for a disallowed key")
			return nil
		},
	}).Handler())
	defer srv.Close()

	c, ctx := dialPane(t, srv, "%2511")
	if err := wsjson.Write(ctx, c, inputMsg{T: "key", K: "Dangerous"}); err != nil {
		t.Fatal(err)
	}
	var m outMsg
	if err := wsjson.Read(ctx, c, &m); err != nil {
		t.Fatal(err)
	}
	if m.T != "error" {
		t.Fatalf("got %+v, want an error message", m)
	}
}

func TestPaneWSRejectsBadPaneID(t *testing.T) {
	srv := httptest.NewServer((&Server{}).Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, wsURL(srv.URL)+"/ws/pane?pane=not-a-pane", nil)
	if err == nil {
		c.CloseNow()
		t.Fatal("expected dial to fail for an invalid pane id")
	}
}
