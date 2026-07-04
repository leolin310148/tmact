package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxMarkdownPreviewBytes = 5 << 20

type markdownPreviewResponse struct {
	Content  string `json:"content"`
	Path     string `json:"path"`
	BaseDir  string `json:"baseDir"`
	Filename string `json:"filename"`
}

func (s *Server) handleMarkdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.maybeProxyPeerHTTP(w, r, "/api/markdown") {
		return
	}

	path, errMsg, status := resolveLocalMarkdownPath(r)
	if errMsg != "" {
		writeJSONError(w, status, errMsg)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "markdown not readable")
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
	if info.Size() > maxMarkdownPreviewBytes {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "markdown file is too large")
		return
	}

	content, err := os.ReadFile(path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "markdown not readable")
		return
	}
	writeJSON(w, http.StatusOK, markdownPreviewResponse{
		Content:  string(content),
		Path:     path,
		BaseDir:  filepath.Dir(path),
		Filename: filepath.Base(path),
	})
}

func resolveLocalMarkdownPath(r *http.Request) (string, string, int) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		return "", `missing "path" parameter`, http.StatusBadRequest
	}
	if scheme, ok := localImagePathScheme(path); ok {
		if !strings.EqualFold(scheme, "file") {
			return "", "unsupported markdown path scheme", http.StatusBadRequest
		}
		var err error
		path, err = decodeLocalFileURLPath(path, scheme)
		if err != nil {
			return "", "invalid markdown path URL escape", http.StatusBadRequest
		}
	}
	if strings.HasPrefix(path, "~/") {
		return "", "home-relative markdown paths are not supported", http.StatusBadRequest
	}
	if !filepath.IsAbs(path) {
		cwd := strings.TrimSpace(r.URL.Query().Get("cwd"))
		if cwd == "" || !filepath.IsAbs(cwd) {
			return "", `relative markdown paths require an absolute "cwd" parameter`, http.StatusBadRequest
		}
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)
	if !isMarkdownPreviewExt(filepath.Ext(path)) {
		return "", "unsupported markdown format (expected .md or .markdown)", http.StatusUnsupportedMediaType
	}
	return path, "", 0
}

func isMarkdownPreviewExt(ext string) bool {
	return strings.EqualFold(ext, ".md") || strings.EqualFold(ext, ".markdown")
}
