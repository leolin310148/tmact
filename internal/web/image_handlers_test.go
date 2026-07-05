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
	path := filepath.Join(dir, "sample image.png")
	png := "\x89PNG\r\n\x1a\n" + "preview bytes"
	if err := os.WriteFile(path, []byte(png), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/image?path="+url.QueryEscape(escapedFileURL(path)), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != png {
		t.Fatalf("body = %q, want image bytes", rec.Body.String())
	}
}

func TestImageEndpointRejectsFileURLWithRemoteHost(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "example.test"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "example.test", "sample.png")
	png := "\x89PNG\r\n\x1a\n" + "preview bytes"
	if err := os.WriteFile(path, []byte(png), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/image?path="+url.QueryEscape("file://example.test/sample.png")+"&cwd="+url.QueryEscape(dir), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestImageEndpointCanDownloadImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, `sample"quote.png`)
	png := "\x89PNG\r\n\x1a\n" + "preview bytes"
	if err := os.WriteFile(path, []byte(png), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/image?download=1&path="+url.QueryEscape(path), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="sample_quote.png"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if rec.Body.String() != png {
		t.Fatalf("body = %q, want image bytes", rec.Body.String())
	}
}

func TestImageEndpointPreviewDoesNotForceDownload(t *testing.T) {
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
	if got := rec.Header().Get("Content-Disposition"); got != "" {
		t.Fatalf("Content-Disposition = %q, want empty", got)
	}
}

func TestImageEndpointMethodNotAllowedSetsAllowHeader(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/image", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want %q", got, http.MethodGet)
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

func TestImageEndpointServesSVG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logo.svg")
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`
	if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/image?path="+url.QueryEscape(path), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/svg+xml") {
		t.Fatalf("Content-Type = %q, want image/svg+xml", got)
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("CSP = %q, want a strict policy", csp)
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
		{"svg-bare", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`), "svg"},
		{"svg-xml-decl", []byte(`<?xml version="1.0"?>` + "\n" + `<SVG xmlns="x"></SVG>`), "svg"},
		{"svg-bom", append([]byte{0xEF, 0xBB, 0xBF}, []byte(`<svg/>`)...), "svg"},
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
