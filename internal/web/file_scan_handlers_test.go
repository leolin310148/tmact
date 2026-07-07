package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func postFilesCheck(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/files/check", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeFilesCheck(t *testing.T, rec *httptest.ResponseRecorder) []downloadableFile {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
	var resp struct {
		Files []downloadableFile `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.Files
}

func TestFilesCheckReturnsOnlyExistingRegularFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(abs, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(dir, "dist", "app.log")
	if err := os.WriteFile(rel, []byte("log line"), 0o644); err != nil {
		t.Fatal(err)
	}

	req, err := json.Marshal(filesCheckRequest{
		Cwd: dir,
		Paths: []string{
			abs,                               // absolute, exists
			"dist/app.log",                    // relative to cwd, exists
			filepath.Join(dir, "missing.txt"), // does not exist
			dir,                               // directory — excluded
			"~/secret.txt",                    // home-relative — rejected by resolver
			"https://example.test/report.txt", // remote URL — rejected by resolver
			abs,                               // duplicate — deduped
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	files := decodeFilesCheck(t, postFilesCheck(t, string(req)))
	if len(files) != 2 {
		t.Fatalf("files = %+v, want 2 entries", files)
	}
	if files[0].Path != abs || files[0].Name != "report.txt" || files[0].Size != 5 {
		t.Fatalf("files[0] = %+v", files[0])
	}
	if files[1].Path != "dist/app.log" || files[1].Name != "app.log" || files[1].Dir != filepath.Join(dir, "dist") {
		t.Fatalf("files[1] = %+v", files[1])
	}
}

func TestFilesCheckRelativePathsWithoutCwdAreSkipped(t *testing.T) {
	req := `{"cwd":"","paths":["dist/app.log"]}`
	files := decodeFilesCheck(t, postFilesCheck(t, req))
	if len(files) != 0 {
		t.Fatalf("files = %+v, want none", files)
	}
}

func TestFilesCheckRejectsInvalidJSON(t *testing.T) {
	rec := postFilesCheck(t, "{not json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestFilesCheckMethodNotAllowedSetsAllowHeader(t *testing.T) {
	handler := (&Server{}).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/files/check", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", got, http.MethodPost)
	}
}

func TestFilesCheckCapsCandidateList(t *testing.T) {
	dir := t.TempDir()
	over := filepath.Join(dir, "over-cap.txt")
	if err := os.WriteFile(over, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths := make([]string, 0, filesCheckMaxPaths+1)
	for i := 0; i < filesCheckMaxPaths; i++ {
		paths = append(paths, filepath.Join(dir, "missing", "file.txt"))
	}
	paths = append(paths, over) // past the cap — must be ignored

	req, err := json.Marshal(filesCheckRequest{Cwd: dir, Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	files := decodeFilesCheck(t, postFilesCheck(t, string(req)))
	if len(files) != 0 {
		t.Fatalf("files = %+v, want none (entry past cap must be dropped)", files)
	}
}
