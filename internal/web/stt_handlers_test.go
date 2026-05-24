package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/stt"
)

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
