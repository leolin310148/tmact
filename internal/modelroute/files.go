package modelroute

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	MaxTaskFiles      = 8
	MaxTaskFileBytes  = 64 * 1024
	MaxTaskFilesBytes = 128 * 1024
)

type TaskFile struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Content   string `json:"content,omitempty"`
	SHA256    string `json:"sha256,omitempty"` // hash of the exact supplied content
	Truncated bool   `json:"truncated,omitempty"`
}

var quotedTaskPath = regexp.MustCompile("`([^`\\n]+)`|\"([^\"\\n]+)\"|'([^'\\n]+)'")
var taskToken = regexp.MustCompile("[^\\s\\x00\"'`<>()\\[\\]{},;!?，。；！？、]+")
var taskSuffix = regexp.MustCompile(`(?i)\.(md|txt)(?::[0-9]+(?::[0-9]+)?)?(?:#[^\s]*)?$`)

// PromptPaths recognizes explicit text-file references, including Markdown
// links and quoted paths with spaces. No shell expansion or URL fetch occurs.
func PromptPaths(prompt string) []string {
	paths := []string{}
	seen := map[string]bool{}
	add := func(token string) {
		token = strings.TrimRight(strings.TrimSpace(token), ".,;:!?，。；：！？、")
		if strings.Contains(token, "://") {
			u, err := url.Parse(token)
			if err != nil || u.Scheme != "file" || u.Host != "" && u.Host != "localhost" {
				return
			}
			token = u.Path
		}
		loc := taskSuffix.FindStringIndex(token)
		if loc == nil {
			return
		}
		ext := strings.IndexAny(token[loc[0]:], ":#")
		if ext >= 0 {
			token = token[:loc[0]+ext]
		}
		if token == "" || seen[token] {
			return
		}
		seen[token] = true
		paths = append(paths, token)
	}
	// Treat quoted paths as whole tokens and don't also match their fragments.
	cursor := 0
	for _, match := range quotedTaskPath.FindAllStringSubmatchIndex(prompt, -1) {
		for _, token := range taskToken.FindAllString(prompt[cursor:match[0]], -1) {
			add(token)
		}
		for i := 2; i < len(match); i += 2 {
			if match[i] >= 0 {
				add(prompt[match[i]:match[i+1]])
				break
			}
		}
		cursor = match[1]
	}
	for _, token := range taskToken.FindAllString(prompt[cursor:], -1) {
		add(token)
	}
	return paths
}

// AttachPromptFiles reads only explicit paths from the original prompt, never
// references found inside attached files. Missing/large files are metadata,
// not a new dispatch blocker; original task delivery remains unchanged.
func AttachPromptFiles(in *Input, dir string) {
	in.Files = CollectPromptFiles(in.Task, dir)
}

func CollectPromptFiles(prompt, dir string) []TaskFile {
	var files []TaskFile
	seen := map[string]bool{}
	total := 0
	for _, reference := range PromptPaths(prompt) {
		p := reference
		if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[2:])
			}
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		p, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		file := TaskFile{Path: p}
		if len(files) >= MaxTaskFiles {
			files = append(files, TaskFile{Path: p, Status: "file_count_limit"})
			break
		}
		if total >= MaxTaskFilesBytes {
			files = append(files, TaskFile{Path: p, Status: "total_size_limit"})
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			file.Status = "unreadable"
			files = append(files, file)
			continue
		}
		if !info.Mode().IsRegular() {
			file.Status = "not_regular"
			files = append(files, file)
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			file.Status = "unreadable"
			files = append(files, file)
			continue
		}
		opened, err := f.Stat()
		if err != nil || !opened.Mode().IsRegular() {
			f.Close()
			file.Status = "not_regular"
			files = append(files, file)
			continue
		}
		limit := min(MaxTaskFileBytes, MaxTaskFilesBytes-total)
		data, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
		f.Close()
		if err != nil {
			file.Status = "unreadable"
			files = append(files, file)
			continue
		}
		file.Truncated = len(data) > limit
		if file.Truncated {
			data = data[:limit]
			for trim := 0; trim < utf8.UTFMax-1 && len(data) > 0 && !utf8.Valid(data); trim++ {
				data = data[:len(data)-1]
			}
		}
		if !utf8.Valid(data) {
			file.Status = "invalid_utf8"
			files = append(files, file)
			continue
		}
		file.Status = "attached"
		file.Content = string(data)
		file.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
		total += len(data)
		files = append(files, file)
	}
	return files
}

func fileMetadata(files []TaskFile) []TaskFile {
	out := append([]TaskFile(nil), files...)
	for i := range out {
		out[i].Content = ""
	}
	return out
}

// FingerprintPromptFiles covers entire referenced files, including bytes beyond
// the bounded routing excerpt. It never retains their content in a plan.
func FingerprintPromptFiles(prompt, dir string) ([]TaskFile, error) {
	files := CollectPromptFiles(prompt, dir)
	for i := range files {
		f := &files[i]
		f.Content = ""
		if f.Status != "attached" {
			continue
		}
		r, err := os.Open(f.Path)
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		_, err = io.Copy(h, r)
		r.Close()
		if err != nil {
			return nil, err
		}
		f.SHA256 = fmt.Sprintf("%x", h.Sum(nil))
	}
	return files, nil
}
