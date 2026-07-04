package web

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// handlePasteImage accepts a clipboard image upload, writes it to a server-side
// file, and returns that file's absolute path. A terminal pane is a keystroke
// stream with no channel for raw image bytes, so the browser relays the path as
// text instead — every supported agent reads an image when given its path.
func (s *Server) handlePasteImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.maybeProxyPeerHTTP(w, r, "/api/paste-image") {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxImageUploadBytes)
	if err := r.ParseMultipartForm(maxImageUploadBytes); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "request body too large") {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSONError(w, status, "invalid image upload: "+err.Error())
		return
	}

	file, _, err := r.FormFile("image")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, `missing "image" upload`)
		return
	}
	defer file.Close()

	// Trust the bytes, not the browser's declared type: sniff the container so
	// the saved file always carries a correct, agent-recognisable extension.
	ext := sniffImageExtension(file)
	if ext == "" {
		writeJSONError(w, http.StatusUnsupportedMediaType,
			"unsupported image format (expected PNG, JPEG, GIF, WebP, BMP, or SVG)")
		return
	}

	dir := s.pasteImageDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.logf("paste-image: mkdir %s: %v", dir, err)
		writeJSONError(w, http.StatusInternalServerError, "could not create the image directory")
		return
	}
	out, err := os.CreateTemp(dir, "paste-"+time.Now().Format("20060102-150405")+"-*."+ext)
	if err != nil {
		s.logf("paste-image: create file in %s: %v", dir, err)
		writeJSONError(w, http.StatusInternalServerError, "could not save the image")
		return
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		_ = os.Remove(out.Name())
		s.logf("paste-image: write %s: %v", out.Name(), err)
		writeJSONError(w, http.StatusInternalServerError, "could not save the image")
		return
	}

	path := out.Name()
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

// handleUploadFile accepts explicit browser file uploads, writes them to
// server-side files, and returns their absolute paths. The browser then pastes
// the paths into the selected pane because tmux input is text-only.
func (s *Server) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.maybeProxyPeerHTTP(w, r, "/api/upload-file") {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxFileUploadBytes)
	if err := r.ParseMultipartForm(maxFileUploadBytes); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "request body too large") {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSONError(w, status, "invalid file upload: "+err.Error())
		return
	}

	if r.MultipartForm == nil || len(r.MultipartForm.File["file"]) == 0 {
		writeJSONError(w, http.StatusBadRequest, `missing "file" upload`)
		return
	}

	dir := s.uploadDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.logf("upload-file: mkdir %s: %v", dir, err)
		writeJSONError(w, http.StatusInternalServerError, "could not create the upload directory")
		return
	}

	var paths []string
	for _, header := range r.MultipartForm.File["file"] {
		path, err := saveUploadedFile(dir, header)
		if err != nil {
			for _, saved := range paths {
				_ = os.Remove(saved)
			}
			s.logf("upload-file: save file in %s: %v", dir, err)
			writeJSONError(w, http.StatusInternalServerError, "could not save the upload")
			return
		}
		paths = append(paths, path)
	}

	resp := map[string]any{"paths": paths}
	if len(paths) > 0 {
		resp["path"] = paths[0]
	}
	writeJSON(w, http.StatusOK, resp)
}

func saveUploadedFile(dir string, header *multipart.FileHeader) (string, error) {
	if header == nil {
		return "", errors.New("missing file header")
	}
	file, err := header.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	out, err := createUploadFile(dir, sanitizeUploadFilename(header.Filename))
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, file); err != nil {
		_ = os.Remove(out.Name())
		return "", err
	}

	path := out.Name()
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return path, nil
}

func createUploadFile(dir, name string) (*os.File, error) {
	if name == "" {
		name = "file"
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" {
		base = "file"
	}
	for i := 0; i < 1000; i++ {
		candidate := name
		if i > 0 {
			candidate = base + "-" + strconv.Itoa(i) + ext
		}
		path := filepath.Join(dir, candidate)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, nil
		}
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return nil, err
	}
	return nil, errors.New("could not allocate a unique upload filename")
}

func sanitizeUploadFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	if name == "." || name == string(filepath.Separator) {
		name = ""
	}

	// Keep Unicode letters/digits verbatim so non-ASCII names (Chinese,
	// accented Latin, …) survive; collapse everything else — path separators,
	// punctuation, whitespace, control runes — to a single dash.
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	clean := strings.Trim(b.String(), ".-_")
	clean = strings.ReplaceAll(clean, "-.", ".")
	if clean == "" {
		return "file"
	}
	if len(clean) <= 120 {
		return clean
	}
	ext := filepath.Ext(clean)
	if len(ext) > 20 {
		ext = ""
	}
	base := strings.TrimSuffix(clean, ext)
	maxBase := 120 - len(ext)
	if maxBase < 1 {
		maxBase = 120
		ext = ""
	}
	base = strings.TrimRight(truncateToBytes(base, maxBase), ".-_")
	if base == "" {
		return "file" + ext
	}
	return base + ext
}

// truncateToBytes shortens s to at most max bytes without splitting a multibyte
// rune, so a clipped Chinese/UTF-8 filename never becomes invalid UTF-8.
func truncateToBytes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for len(s) > 0 {
		if r, _ := utf8.DecodeLastRuneInString(s); r != utf8.RuneError {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}
