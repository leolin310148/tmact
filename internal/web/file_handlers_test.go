package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileEndpointDownloadsAbsoluteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, `report"final.txt`)
	body := "download me"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/file?path="+url.QueryEscape(path), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="report_final.txt"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/octet-stream") {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
	if rec.Body.String() != body {
		t.Fatalf("body = %q, want %q", rec.Body.String(), body)
	}
}

func TestFileEndpointResolvesRelativePathFromCWD(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "dist", "app.log")
	if err := os.WriteFile(path, []byte("log"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/file?path="+url.QueryEscape("dist/app.log")+"&cwd="+url.QueryEscape(dir), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "log" {
		t.Fatalf("body = %q, want log", rec.Body.String())
	}
}

func TestFileEndpointRejectsRemoteURL(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/file?path="+url.QueryEscape("https://example.test/report.txt"), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestFileEndpointRejectsDirectory(t *testing.T) {
	dir := t.TempDir()

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/file?path="+url.QueryEscape(dir), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
