package web

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageEndpointServesAnyReadableImagePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.png")
	png := "\x89PNG\r\n\x1a\n" + "preview bytes"
	if err := os.WriteFile(path, []byte(png), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/image?path="+url.QueryEscape(path), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	if rec.Body.String() != png {
		t.Fatalf("body = %q, want image bytes", rec.Body.String())
	}
}

func TestImageEndpointServesFileURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.png")
	png := "\x89PNG\r\n\x1a\n" + "preview bytes"
	if err := os.WriteFile(path, []byte(png), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/image?path="+url.QueryEscape("file://"+path), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != png {
		t.Fatalf("body = %q, want image bytes", rec.Body.String())
	}
}

func TestImageEndpointResolvesRelativePathFromCWD(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "img", "sample.gif")
	gif := "GIF89a" + "preview bytes"
	if err := os.WriteFile(path, []byte(gif), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/image?path="+url.QueryEscape("img/sample.gif")+"&cwd="+url.QueryEscape(dir), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/gif") {
		t.Fatalf("Content-Type = %q, want image/gif", got)
	}
}

func TestImageEndpointRejectsRemoteURL(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "https:", "example.test"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "https:", "example.test", "sample.png")
	png := "\x89PNG\r\n\x1a\n" + "preview bytes"
	if err := os.WriteFile(path, []byte(png), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/image?path="+url.QueryEscape("https://example.test/sample.png")+"&cwd="+url.QueryEscape(dir), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestImageEndpointRejectsHomeRelativePath(t *testing.T) {
	dir := t.TempDir()

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/image?path="+url.QueryEscape("~/sample.png")+"&cwd="+url.QueryEscape(dir), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestImageEndpointRejectsNonImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-image.png")
	if err := os.WriteFile(path, []byte("plain text"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/image?path="+url.QueryEscape(path), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
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
