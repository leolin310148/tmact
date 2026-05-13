package workflow

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const CommentMarker = "TMAct-OpenSpec-Comment:"

type Comment struct {
	ID            string    `json:"id"`
	Timestamp     time.Time `json:"ts"`
	Role          string    `json:"role"`
	Kind          string    `json:"kind"`
	ChangeHash    string    `json:"change_hash"`
	OpenSpecValid bool      `json:"openspec_valid"`
	Blocking      bool      `json:"blocking"`
	ReplyTo       string    `json:"reply_to,omitempty"`
	Body          string    `json:"body,omitempty"`
	Raw           string    `json:"raw,omitempty"`
}

func CommentsPath(changeDir string) string {
	return filepath.Join(changeDir, "phase1-comments.jsonl")
}

func ParseCommentsFromText(text string, now time.Time) ([]Comment, error) {
	var comments []Comment
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		comment, ok, err := ParseCommentLine(scanner.Text(), now)
		if err != nil {
			return nil, err
		}
		if ok {
			comments = append(comments, comment)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return comments, nil
}

func ParseCommentLine(line string, now time.Time) (Comment, bool, error) {
	index := strings.Index(line, CommentMarker)
	if index < 0 {
		return Comment{}, false, nil
	}
	raw := strings.TrimSpace(line[index:])
	fields, err := parseMarkerFields(strings.TrimSpace(strings.TrimPrefix(raw, CommentMarker)))
	if err != nil {
		return Comment{}, false, err
	}
	comment := Comment{
		Timestamp:  now,
		Role:       fields["role"],
		Kind:       fields["kind"],
		ChangeHash: fields["change_hash"],
		ReplyTo:    fields["reply_to"],
		Body:       fields["body"],
		Raw:        raw,
	}
	if comment.Role == "" || comment.Kind == "" || comment.ChangeHash == "" {
		return Comment{}, false, fmt.Errorf("invalid comment marker: role, kind, and change_hash are required")
	}
	if value := fields["openspec_valid"]; value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return Comment{}, false, fmt.Errorf("invalid openspec_valid value %q", value)
		}
		comment.OpenSpecValid = parsed
	}
	if value := fields["blocking"]; value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return Comment{}, false, fmt.Errorf("invalid blocking value %q", value)
		}
		comment.Blocking = parsed
	}
	if isBlockingKind(comment.Kind) {
		comment.Blocking = true
	}
	comment.ID = commentFingerprint(comment)
	return comment, true, nil
}

func parseMarkerFields(text string) (map[string]string, error) {
	fields := map[string]string{}
	for len(text) > 0 {
		text = strings.TrimLeft(text, " \t")
		if text == "" {
			break
		}
		eq := strings.IndexByte(text, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("invalid marker field near %q", text)
		}
		key := text[:eq]
		rest := text[eq+1:]
		value := ""
		if strings.HasPrefix(rest, `"`) {
			parsed, tail, err := parseQuoted(rest)
			if err != nil {
				return nil, err
			}
			value = parsed
			text = tail
		} else {
			end := strings.IndexAny(rest, " \t")
			if end < 0 {
				value = rest
				text = ""
			} else {
				value = rest[:end]
				text = rest[end:]
			}
		}
		fields[key] = value
	}
	return fields, nil
}

func parseQuoted(text string) (string, string, error) {
	for i := 1; i < len(text); i++ {
		if text[i] == '"' && text[i-1] != '\\' {
			value, err := strconv.Unquote(text[:i+1])
			if err != nil {
				return "", "", err
			}
			return value, text[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("unterminated quoted marker value")
}

func LoadComments(path string) ([]Comment, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var comments []Comment
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var comment Comment
		if err := json.Unmarshal([]byte(line), &comment); err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}
	return comments, scanner.Err()
}

func AppendNewComments(path string, existing []Comment, observed []Comment) ([]Comment, error) {
	seen := map[string]bool{}
	for _, comment := range existing {
		seen[commentFingerprint(comment)] = true
	}
	var fresh []Comment
	for _, comment := range observed {
		fingerprint := commentFingerprint(comment)
		if seen[fingerprint] {
			continue
		}
		comment.ID = fingerprint
		fresh = append(fresh, comment)
		seen[fingerprint] = true
	}
	if len(fresh) == 0 {
		return existing, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	for _, comment := range fresh {
		data, err := json.Marshal(comment)
		if err != nil {
			return nil, err
		}
		if _, err := file.Write(append(data, '\n')); err != nil {
			return nil, err
		}
		existing = append(existing, comment)
	}
	return existing, nil
}

func commentFingerprint(comment Comment) string {
	input := strings.Join([]string{
		comment.Role,
		comment.Kind,
		comment.ChangeHash,
		strconv.FormatBool(comment.OpenSpecValid),
		strconv.FormatBool(comment.Blocking),
		comment.ReplyTo,
		comment.Body,
		comment.Raw,
	}, "\x00")
	sum := sha256.Sum256([]byte(input))
	return "c-" + hex.EncodeToString(sum[:])[:12]
}

func isBlockingKind(kind string) bool {
	switch kind {
	case "reject", "request_changes":
		return true
	default:
		return false
	}
}
