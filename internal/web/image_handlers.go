package web

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.maybeProxyPeerHTTP(w, r, "/api/image") {
		return
	}

	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		writeJSONError(w, http.StatusBadRequest, `missing "path" parameter`)
		return
	}
	if scheme, ok := localImagePathScheme(path); ok {
		if !strings.EqualFold(scheme, "file") {
			writeJSONError(w, http.StatusBadRequest, "unsupported image path scheme")
			return
		}
		path = path[len(scheme)+len("://"):]
		if strings.HasPrefix(path, "localhost/") {
			path = strings.TrimPrefix(path, "localhost")
		}
	}
	if strings.HasPrefix(path, "~/") {
		writeJSONError(w, http.StatusBadRequest, "home-relative image paths are not supported")
		return
	}
	if !filepath.IsAbs(path) {
		cwd := strings.TrimSpace(r.URL.Query().Get("cwd"))
		if cwd == "" || !filepath.IsAbs(cwd) {
			writeJSONError(w, http.StatusBadRequest, `relative image paths require an absolute "cwd" parameter`)
			return
		}
		path = filepath.Join(cwd, path)
	}
	path = filepath.Clean(path)

	file, err := os.Open(path)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "image not readable")
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "image not readable")
		return
	}
	if info.IsDir() {
		writeJSONError(w, http.StatusBadRequest, "path is a directory")
		return
	}

	ext := sniffImageExtension(file)
	mimeType := imageMIMEByExt[ext]
	if mimeType == "" {
		writeJSONError(w, http.StatusUnsupportedMediaType,
			"unsupported image format (expected PNG, JPEG, GIF, WebP, BMP, or SVG)")
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+sanitizeDownloadFilename(filepath.Base(path))+`"`)
	}
	if ext == "svg" {
		// Belt-and-suspenders: an SVG rendered via <img> never executes scripts,
		// but a user opening the URL directly would view it as a document. A
		// strict CSP blocks scripts and external loads in that case too.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	}
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

func sanitizeDownloadFilename(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < 0x20 || r == 0x7f {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "download"
	}
	return name
}

func localImagePathScheme(path string) (string, bool) {
	i := strings.Index(path, "://")
	if i <= 0 {
		return "", false
	}
	scheme := path[:i]
	for j, r := range scheme {
		if j == 0 {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
				return "", false
			}
			continue
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') || r == '+' || r == '-' || r == '.' {
			continue
		}
		return "", false
	}
	return scheme, true
}

// sniffImageExtension reads an upload's leading bytes and returns a canonical
// image extension (no dot), or "" for anything it does not recognise as an
// image. It rewinds the reader so the caller still sees the whole file.
//
// SVG is text-based without a fixed binary signature, so the check is a
// case-insensitive search for "<svg" in the first 512 bytes — after any BOM,
// XML declaration, or DOCTYPE. False positives (e.g. HTML containing an
// inline <svg>) get treated as SVG, which is acceptable here: the renderer
// is <img> + a strict CSP, and the file the user pointed at is theirs.
func sniffImageExtension(rs io.ReadSeeker) string {
	head := make([]byte, 512)
	n, _ := io.ReadFull(rs, head)
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	head = head[:n]
	switch {
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		return "png"
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return "jpg"
	case len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return "gif"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "webp"
	case len(head) >= 2 && head[0] == 'B' && head[1] == 'M':
		return "bmp"
	case looksLikeSVG(head):
		return "svg"
	default:
		return ""
	}
}

func looksLikeSVG(b []byte) bool {
	// Drop UTF-8 BOM so the substring search lines up with the document body.
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	return bytes.Contains(bytes.ToLower(b), []byte("<svg"))
}

var imageMIMEByExt = map[string]string{
	"png":  "image/png",
	"jpg":  "image/jpeg",
	"gif":  "image/gif",
	"webp": "image/webp",
	"bmp":  "image/bmp",
	"svg":  "image/svg+xml",
}
