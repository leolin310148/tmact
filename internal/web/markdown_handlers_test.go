package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/leolin310148/tmact/internal/statusd"
)

func TestMarkdownEndpointReadsAbsoluteMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	body := "# Title\n\nhello"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/markdown?path="+url.QueryEscape(path), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	var got markdownPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Content != body || got.Path != path || got.BaseDir != dir || got.Filename != "README.md" {
		t.Fatalf("response = %+v", got)
	}
}

func TestMarkdownEndpointReadsEscapedFileURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "release notes.md")
	body := "# Notes\n\nhello"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/markdown?path="+url.QueryEscape(escapedFileURL(path)), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	var got markdownPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Content != body || got.Path != path || got.BaseDir != dir || got.Filename != "release notes.md" {
		t.Fatalf("response = %+v", got)
	}
}

func TestMarkdownEndpointResolvesRelativePathFromCWD(t *testing.T) {
	dir := t.TempDir()
	docs := filepath.Join(dir, "docs")
	if err := os.Mkdir(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(docs, "guide.markdown")
	if err := os.WriteFile(path, []byte("guide"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/markdown?path="+url.QueryEscape("docs/guide.markdown")+"&cwd="+url.QueryEscape(dir), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	var got markdownPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != path || got.BaseDir != docs {
		t.Fatalf("path/baseDir = %q/%q, want %q/%q", got.Path, got.BaseDir, path, docs)
	}
}

func TestMarkdownEndpointRejectsInvalidRequests(t *testing.T) {
	dir := t.TempDir()
	textPath := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(textPath, []byte("not markdown"), 0o644); err != nil {
		t.Fatal(err)
	}
	largePath := filepath.Join(dir, "large.md")
	if err := os.WriteFile(largePath, []byte(strings.Repeat("x", maxMarkdownPreviewBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"missing path", "/api/markdown", http.StatusBadRequest},
		{"remote URL", "/api/markdown?path=" + url.QueryEscape("https://example.test/readme.md"), http.StatusBadRequest},
		{"home relative", "/api/markdown?path=" + url.QueryEscape("~/readme.md"), http.StatusBadRequest},
		{"relative without cwd", "/api/markdown?path=" + url.QueryEscape("readme.md"), http.StatusBadRequest},
		{"directory", "/api/markdown?path=" + url.QueryEscape(filepath.Join(dir, "subdir.md")), http.StatusBadRequest},
		{"non markdown extension", "/api/markdown?path=" + url.QueryEscape(textPath), http.StatusUnsupportedMediaType},
		{"too large", "/api/markdown?path=" + url.QueryEscape(largePath), http.StatusRequestEntityTooLarge},
	}

	handler := (&Server{}).Handler()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.raw, nil)
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %q", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestMarkdownEndpointRejectsNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipe.md")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}

	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/markdown?path="+url.QueryEscape(path), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestMarkdownEndpointProxiesPeer(t *testing.T) {
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/markdown" {
			t.Fatalf("path = %q, want /api/markdown", r.URL.Path)
		}
		if got := r.URL.Query().Get("peer"); got != "" {
			t.Fatalf("peer query leaked upstream: %q", got)
		}
		if got := r.URL.Query().Get("path"); got != "/tmp/readme.md" {
			t.Fatalf("path query = %q", got)
		}
		writeJSON(w, http.StatusOK, markdownPreviewResponse{
			Content:  "remote",
			Path:     "/tmp/readme.md",
			BaseDir:  "/tmp",
			Filename: "readme.md",
		})
	}))
	defer peer.Close()

	handler := (&Server{Peers: []statusd.Peer{{Name: "peer-a", URL: peer.URL}}}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/markdown?peer=peer-a&path="+url.QueryEscape("/tmp/readme.md"), nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "remote") {
		t.Fatalf("body = %q, want proxied JSON", rec.Body.String())
	}
}
