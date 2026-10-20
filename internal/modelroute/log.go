package modelroute

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const PolicyVersion = "routing-v4-sections-budget-errors"

// Trace correlates a decision with dispatch and ask outcomes. It never holds
// ask capability IDs or provider credentials.
type Trace struct {
	Dir        string `json:"dir,omitempty"`
	DispatchID string `json:"dispatch_id,omitempty"`
	Command    string `json:"command,omitempty"`
	Session    string `json:"session,omitempty"`
	Peer       string `json:"peer,omitempty"`
	Execute    bool   `json:"execute"`
}

type Outcome struct {
	ReplyKind  string            `json:"reply_kind,omitempty"`
	Agent      string            `json:"agent,omitempty"`
	Model      string            `json:"model,omitempty"`
	Effort     string            `json:"effort,omitempty"`
	Target     string            `json:"target,omitempty"`
	Status     string            `json:"status"` // planned, dispatched, failed, answered, timeout, reused
	WaitReason string            `json:"wait_reason,omitempty"`
	Steps      map[string]string `json:"steps,omitempty"`
}

type DecisionAttempt struct {
	Number    int             `json:"number"`
	Input     *Input          `json:"input"`
	Budget    *RequestBudget  `json:"budget"`
	Response  json.RawMessage `json:"response,omitempty"`
	ErrorCode string          `json:"error_code,omitempty"`
	ElapsedMS int64           `json:"elapsed_ms"`
}

type LogEntry struct {
	Attempts      []DecisionAttempt `json:"attempts,omitempty"`
	Budget        *RequestBudget    `json:"budget,omitempty"`
	ErrorCode     string            `json:"error_code,omitempty"`
	Time          time.Time         `json:"time"`
	Event         string            `json:"event"`
	Trace         Trace             `json:"trace"`
	PolicyVersion string            `json:"policy_version,omitempty"`
	PolicySHA256  string            `json:"policy_sha256,omitempty"`
	Input         *Input            `json:"input,omitempty"`
	Questions     json.RawMessage   `json:"questions,omitempty"`
	Response      json.RawMessage   `json:"response,omitempty"`
	Selection     *Selection        `json:"selection,omitempty"`
	Outcome       *Outcome          `json:"outcome,omitempty"`
	Error         string            `json:"error,omitempty"`
	ElapsedMS     int64             `json:"elapsed_ms,omitempty"`
}

var fallbackTrace atomic.Uint64
var appendLogMu sync.Mutex

func NewDispatchID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "d_" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("d_%x_%x_%x", time.Now().UnixNano(), os.Getpid(), fallbackTrace.Add(1))
}

func LogPath() (string, error) {
	if p := os.Getenv("TMACT_ROUTING_LOG"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tmact", "routing.jsonl"), nil
}

// Log is best-effort: storage problems must never become a dispatch gate.
func Log(entry LogEntry) {
	if err := AppendLog(entry); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not write routing log:", err)
	}
}

func AppendLog(entry LogEntry) error {
	path, err := LogPath()
	if err != nil {
		return err
	}
	if path == "off" {
		return nil
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	appendLogMu.Lock()
	defer appendLogMu.Unlock()
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("routing log must be a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	link, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || link.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, link) {
		return fmt.Errorf("routing log must be a regular file, not a symlink")
	}
	if err = f.Chmod(0600); err != nil {
		return err
	}
	// A single append write keeps each JSONL record together across processes.
	n, err := f.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

// ReadLog returns newest matching entries. Full task/file content and provider
// responses are private unless explicitly requested with --show-content.
func ReadLog(dispatchID string, limit int, showContent bool) ([]LogEntry, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("routing log limit must be positive")
	}
	entries := []LogEntry{}
	path, err := LogPath()
	if err != nil {
		return nil, err
	}
	if path == "off" {
		return entries, nil
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return entries, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		var e LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if dispatchID != "" && e.Trace.DispatchID != dispatchID {
			continue
		}
		if !showContent {
			for i := range e.Attempts {
				a := &e.Attempts[i]
				a.Response = nil
				if a.Input != nil {
					copy := *a.Input
					copy.Task = ""
					copy.Files = fileMetadata(copy.Files)
					a.Input = &copy
				}
			}

			e.Response = nil
			e.Questions = nil
			if e.Selection != nil {
				selection := *e.Selection
				selection.Files = fileMetadata(selection.Files)
				e.Selection = &selection
			}
			if e.Input != nil {
				copy := *e.Input
				copy.Task = ""
				copy.Files = fileMetadata(copy.Files)
				e.Input = &copy
			}
		}
		entries = append(entries, e)
		if len(entries) > limit {
			copy(entries, entries[len(entries)-limit:])
			entries = entries[:limit]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}
