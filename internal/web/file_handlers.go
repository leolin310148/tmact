package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.maybeProxyPeerHTTP(w, r, "/api/file") {
		return
	}

	path, errMsg, status := resolveLocalDownloadPath(r)
	if errMsg != "" {
		writeJSONError(w, status, errMsg)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "file not readable")
		return
	}
	if info.IsDir() {
		writeJSONError(w, http.StatusBadRequest, "path is a directory")
		return
	}
	if !info.Mode().IsRegular() {
		writeJSONError(w, http.StatusBadRequest, "path is not a regular file")
		return
	}

	file, err := os.Open(path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "file not readable")
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeDownloadFilename(filepath.Base(path))+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

func resolveLocalDownloadPath(r *http.Request) (string, string, int) {
	return resolveDownloadPath(r.URL.Query().Get("path"), r.URL.Query().Get("cwd"))
}

func resolveDownloadPath(rawPath, rawCwd string) (string, string, int) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", `missing "path" parameter`, http.StatusBadRequest
	}
	if scheme, ok := localImagePathScheme(path); ok {
		if !strings.EqualFold(scheme, "file") {
			return "", "unsupported file path scheme", http.StatusBadRequest
		}
		var err error
		path, err = decodeLocalFileURLPath(path, scheme)
		if err != nil {
			return "", "invalid file path URL escape", http.StatusBadRequest
		}
	}
	if strings.HasPrefix(path, "~/") {
		return "", "home-relative file paths are not supported", http.StatusBadRequest
	}
	if !filepath.IsAbs(path) {
		cwd := strings.TrimSpace(rawCwd)
		if cwd == "" || !filepath.IsAbs(cwd) {
			return "", `relative file paths require an absolute "cwd" parameter`, http.StatusBadRequest
		}
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path), "", 0
}

// filesCheckMaxPaths bounds one scan request; extra candidates are ignored so a
// huge pane buffer can't turn into an unbounded stat storm.
const filesCheckMaxPaths = 500

type filesCheckRequest struct {
	Cwd   string   `json:"cwd"`
	Paths []string `json:"paths"`
}

type downloadableFile struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Dir  string `json:"dir"`
	Size int64  `json:"size"`
}

// handleFilesCheck backs the selection-mode download list: the client scans the
// pane text for path-looking tokens and posts them here; only candidates that
// resolve to readable regular files come back. Download itself stays on
// /api/file, so this endpoint never opens file contents.
func (s *Server) handleFilesCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.maybeProxyPeerHTTP(w, r, "/api/files/check") {
		return
	}

	var req filesCheckRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	paths := req.Paths
	if len(paths) > filesCheckMaxPaths {
		paths = paths[:filesCheckMaxPaths]
	}

	seen := make(map[string]bool, len(paths))
	files := []downloadableFile{}
	for _, p := range paths {
		resolved, errMsg, _ := resolveDownloadPath(p, req.Cwd)
		if errMsg != "" || seen[resolved] {
			continue
		}
		seen[resolved] = true
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, downloadableFile{
			Path: p,
			Name: filepath.Base(resolved),
			Dir:  filepath.Dir(resolved),
			Size: info.Size(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}
