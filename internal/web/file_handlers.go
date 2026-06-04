package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
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

	file, err := os.Open(path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "file not readable")
		return
	}
	defer file.Close()

	info, err := file.Stat()
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

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeDownloadFilename(filepath.Base(path))+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

func resolveLocalDownloadPath(r *http.Request) (string, string, int) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		return "", `missing "path" parameter`, http.StatusBadRequest
	}
	if scheme, ok := localImagePathScheme(path); ok {
		if !strings.EqualFold(scheme, "file") {
			return "", "unsupported file path scheme", http.StatusBadRequest
		}
		path = path[len(scheme)+len("://"):]
		if strings.HasPrefix(path, "localhost/") {
			path = strings.TrimPrefix(path, "localhost")
		}
	}
	if strings.HasPrefix(path, "~/") {
		return "", "home-relative file paths are not supported", http.StatusBadRequest
	}
	if !filepath.IsAbs(path) {
		cwd := strings.TrimSpace(r.URL.Query().Get("cwd"))
		if cwd == "" || !filepath.IsAbs(cwd) {
			return "", `relative file paths require an absolute "cwd" parameter`, http.StatusBadRequest
		}
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path), "", 0
}
