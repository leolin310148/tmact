package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/statusd"
)

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

/* ---- file upload ---- */

func fileUploadRequest(t *testing.T, field, filename, bodyText string) *http.Request {
	t.Helper()
	return filesUploadRequest(t, field, []uploadPart{{filename: filename, bodyText: bodyText}})
}

type uploadPart struct {
	filename string
	bodyText string
}

func filesUploadRequest(t *testing.T, field string, parts []uploadPart) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, upload := range parts {
		part, err := mw.CreateFormFile(field, upload.filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, upload.bodyText); err != nil {
			t.Fatal(err)
		}
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
	var got struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	path := got.Path
	if !strings.HasPrefix(path, dir) {
		t.Fatalf("path = %q, want it saved under %q", path, dir)
	}
	if len(got.Paths) != 1 || got.Paths[0] != path {
		t.Fatalf("paths = %#v, want single path %q", got.Paths, path)
	}
	if path != filepath.Join(dir, "notes.txt") {
		t.Fatalf("path = %q, want sanitized original filename under upload dir", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("saved upload not readable: %v", err)
	}
	if string(data) != "hello upload" {
		t.Fatalf("saved bytes = %q, want uploaded file verbatim", string(data))
	}
}

func TestUploadFileSavesMultipleFilesAndReturnsPaths(t *testing.T) {
	dir := t.TempDir()
	handler := (&Server{UploadDir: dir}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, filesUploadRequest(t, "file", []uploadPart{
		{filename: "one.txt", bodyText: "first"},
		{filename: "../two?.md", bodyText: "second"},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	var got struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Paths) != 2 {
		t.Fatalf("paths = %#v, want two saved files", got.Paths)
	}
	if got.Path != got.Paths[0] {
		t.Fatalf("path = %q, want first path %q", got.Path, got.Paths[0])
	}

	for i, wantBody := range []string{"first", "second"} {
		path := got.Paths[i]
		if !strings.HasPrefix(path, dir) {
			t.Fatalf("path %d = %q, want it saved under %q", i, path, dir)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("saved upload %d not readable: %v", i, err)
		}
		if string(data) != wantBody {
			t.Fatalf("saved bytes %d = %q, want %q", i, string(data), wantBody)
		}
	}
	if got.Paths[0] != filepath.Join(dir, "one.txt") {
		t.Fatalf("first path = %q, want original filename one.txt", got.Paths[0])
	}
	if got.Paths[1] != filepath.Join(dir, "two.md") {
		t.Fatalf("second path = %q, want sanitized original filename two.md", got.Paths[1])
	}
}

func TestUploadFileAvoidsClobberingSameFilename(t *testing.T) {
	dir := t.TempDir()
	handler := (&Server{UploadDir: dir}).Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, filesUploadRequest(t, "file", []uploadPart{
		{filename: "notes.txt", bodyText: "first"},
		{filename: "notes.txt", bodyText: "second"},
	}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	var got struct {
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{
		filepath.Join(dir, "notes.txt"),
		filepath.Join(dir, "notes-1.txt"),
	}
	if len(got.Paths) != len(wantPaths) {
		t.Fatalf("paths = %#v, want %#v", got.Paths, wantPaths)
	}
	for i, want := range wantPaths {
		if got.Paths[i] != want {
			t.Fatalf("path %d = %q, want %q", i, got.Paths[i], want)
		}
	}
	for i, wantBody := range []string{"first", "second"} {
		data, err := os.ReadFile(wantPaths[i])
		if err != nil {
			t.Fatalf("saved upload %d not readable: %v", i, err)
		}
		if string(data) != wantBody {
			t.Fatalf("saved bytes %d = %q, want %q", i, string(data), wantBody)
		}
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

func TestUploadFileProxiesToPeer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("peer") != "" {
			t.Fatalf("upstream query still has peer: %s", r.URL.RawQuery)
		}
		if r.URL.Path != "/api/upload-file" {
			t.Fatalf("path = %q, want /api/upload-file", r.URL.Path)
		}
		if err := r.ParseMultipartForm(maxFileUploadBytes); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if string(body) != "hello remote" {
			t.Fatalf("body = %q, want hello remote", string(body))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"path":  "/remote/upload/notes.txt",
			"paths": []string{"/remote/upload/notes.txt"},
		})
	}))
	defer upstream.Close()

	handler := (&Server{
		Peers: []statusd.Peer{{Name: "peer-a", URL: upstream.URL}},
	}).Handler()
	rec := httptest.NewRecorder()
	req := fileUploadRequest(t, "file", "notes.txt", "hello remote")
	req.URL.RawQuery = "peer=peer-a"
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	var got struct {
		Path  string   `json:"path"`
		Paths []string `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Path != "/remote/upload/notes.txt" || len(got.Paths) != 1 {
		t.Fatalf("got %#v, want remote upload response", got)
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
		{"résumé 2026.pdf", "résumé-2026.pdf"},
		{"報告 final.pdf", "報告-final.pdf"},
		{"(新機種) 建立資料-Build Board.eml", "新機種-建立資料-Build-Board.eml"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := sanitizeUploadFilename(tc.in); got != tc.want {
				t.Fatalf("sanitizeUploadFilename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
