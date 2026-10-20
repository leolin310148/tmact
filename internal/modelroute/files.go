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
	MaxTaskReferences = 32
	MaxSourceBytes    = 8 * 1024 * 1024
	MaxTaskFileBytes  = 64 * 1024
	MaxTaskFilesBytes = 128 * 1024
)

type TaskFile struct {
	Section       string `json:"section,omitempty"`
	SourceBytes   int64  `json:"source_bytes,omitempty"`
	SourceSHA256  string `json:"source_sha256,omitempty"`
	OriginalBytes int    `json:"original_bytes,omitempty"`
	IncludedBytes int    `json:"included_bytes,omitempty"`
	StartLine     int    `json:"start_line,omitempty"`
	EndLine       int    `json:"end_line,omitempty"`
	Omitted       bool   `json:"omitted,omitempty"`
	Reason        string `json:"reason,omitempty"`
	Path          string `json:"path"`
	Status        string `json:"status"`
	Content       string `json:"content,omitempty"`
	SHA256        string `json:"sha256,omitempty"` // hash of the exact supplied content
	Truncated     bool   `json:"truncated,omitempty"`
}

var quotedTaskPath = regexp.MustCompile("`([^`\\n]+)`|\"([^\"\\n]+)\"|'([^'\\n]+)'")
var taskToken = regexp.MustCompile("[^\\s\\x00\"'`<>()\\[\\]{},;!?，。；！？、]+")
var taskSuffix = regexp.MustCompile(`(?i)\.(md|txt)(?::[0-9]+(?::[0-9]+)?)?(?:#[^\r\n]*)?$`)

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
			if u.Fragment != "" {
				token += "#" + u.Fragment
			}
		}
		loc := taskSuffix.FindStringIndex(token)
		if loc == nil {
			return
		}
		section := ""
		if idx := strings.Index(token[loc[0]:], "#"); idx >= 0 {
			section = token[loc[0]+idx+1:]
		}
		end := loc[0] + 3
		if strings.HasPrefix(strings.ToLower(token[loc[0]:]), ".txt") {
			end++
		}
		token = token[:end]
		if section != "" {
			if decoded, err := url.PathUnescape(section); err == nil {
				section = decoded
			}
			token += "#" + section
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
	sources := map[string]bool{}
	total := 0
	refs := PromptPaths(prompt)
	for _, reference := range refs {
		p, section, _ := strings.Cut(reference, "#")
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
		key := p + "#" + section
		if seen[key] {
			continue
		}
		seen[key] = true
		file := TaskFile{Path: p, Section: section}
		if len(files) >= MaxTaskReferences {
			file.Status = "reference_count_limit"
			file.Omitted = true
			file.Reason = file.Status
			files = append(files, file)
			break
		}
		if !sources[p] && len(sources) >= MaxTaskFiles {
			file.Status = "file_count_limit"
			file.Omitted = true
			file.Reason = "file_count_limit"
			files = append(files, file)
			break
		}
		sources[p] = true
		info, err := os.Stat(p)
		if err != nil {
			file.Status = "unreadable"
			files = append(files, file)
			continue
		}
		file.SourceBytes = info.Size()
		if !info.Mode().IsRegular() {
			file.Status = "not_regular"
			files = append(files, file)
			continue
		}
		if info.Size() > MaxSourceBytes {
			file.Status = "omitted"
			file.Omitted = true
			file.Reason = "source_size_limit"
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
		data, err := io.ReadAll(io.LimitReader(f, MaxSourceBytes+1))
		f.Close()
		if err != nil {
			file.Status = "unreadable"
			files = append(files, file)
			continue
		}
		file.SourceBytes = int64(len(data))
		if len(data) > MaxSourceBytes {
			file.Status = "omitted"
			file.Omitted = true
			file.Reason = "source_size_limit"
			files = append(files, file)
			continue
		}
		file.SourceSHA256 = contentHash(string(data))
		if !utf8.Valid(data) {
			file.Status = "invalid_utf8"
			files = append(files, file)
			continue
		}
		text := string(data)
		file.Status = "attached"
		if section != "" {
			text, file.StartLine, file.EndLine, file.Status = sectionContent(text, section)
			if file.Status != "attached" {
				file.Omitted = true
				file.Reason = file.Status
				files = append(files, file)
				continue
			}
		}
		file.OriginalBytes = len(text)
		remaining := max(1, min(MaxTaskReferences, len(refs))-len(files))
		limit := min(MaxTaskFileBytes, max(0, MaxTaskFilesBytes-total)/remaining)
		file.Content = text
		file.IncludedBytes = len(text)
		file.SHA256 = contentHash(text)
		if len(text) > limit {
			file = clipFile(file, limit, "read_size_limit")
		}
		total += len(file.Content)
		files = append(files, file)
	}
	return files
}

func contentHash(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

func clipFile(f TaskFile, limit int, reason string) TaskFile {
	if len(f.Content) <= limit {
		return f
	}
	prefix := f.Content[:max(0, limit)]
	for len(prefix) > 0 && !utf8.ValidString(prefix) {
		prefix = prefix[:len(prefix)-1]
	}
	f.Content = prefix
	f.IncludedBytes = len(prefix)
	f.SHA256 = contentHash(prefix)
	f.Truncated = true
	f.Reason = reason
	if prefix == "" {
		f.Status = "omitted"
		f.Omitted = true
	} else {
		f.Status = "attached"
		f.Omitted = false
	}
	return f
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
		if f.SourceSHA256 == "" && f.Reason == "source_size_limit" {
			r, err := os.Open(f.Path)
			if err != nil {
				return nil, err
			}
			info, err := r.Stat()
			if err != nil || !info.Mode().IsRegular() {
				r.Close()
				return nil, fmt.Errorf("plan source is not a regular file")
			}
			h := sha256.New()
			_, err = io.Copy(h, r)
			r.Close()
			if err != nil {
				return nil, err
			}
			f.SourceSHA256 = fmt.Sprintf("%x", h.Sum(nil))
		}
		if f.SourceSHA256 != "" {
			f.SHA256 = f.SourceSHA256
		}
	}
	return files, nil
}

// SameFileEvidence compares source identity/selection, independently of the
// request's context clipping. Plans retain full-source fingerprints separately.
func SameFileEvidence(current, routed []TaskFile) bool {
	if len(current) != len(routed) {
		return false
	}
	for i, c := range current {
		r := routed[i]
		if c.Path != r.Path || c.Section != r.Section {
			return false
		}
		if c.SourceSHA256 != "" && r.SourceSHA256 != "" {
			if c.SourceSHA256 != r.SourceSHA256 || c.SourceBytes != r.SourceBytes || c.StartLine != r.StartLine || c.EndLine != r.EndLine {
				return false
			}
		} else if r.Omitted && r.Reason == "source_size_limit" {
			if c.SourceBytes != r.SourceBytes || c.Status != r.Status {
				return false
			}
		} else if c.Status != r.Status || c.SHA256 != r.SHA256 {
			return false
		}
	}
	return true
}
