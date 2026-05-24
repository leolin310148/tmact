package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/leolin310148/tmact/internal/stt"
)

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
