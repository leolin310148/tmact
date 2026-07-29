package askreply

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const (
	Version             = 1
	DefaultTimeout      = 30 * time.Minute
	DefaultPollInterval = 100 * time.Millisecond
	MaxReplyBytes       = 1024 * 1024

	requestFile = "request.json"
	outcomeFile = "outcome.json"
)

var (
	ErrAlreadyFinalized = errors.New("question already finalized")
	ErrCanceled         = errors.New("question canceled")
	ErrExpired          = errors.New("question expired")

	questionIDPattern = regexp.MustCompile(`^q_[a-z2-7]{26}$`)
)

// Request is the durable, metadata-only side of an ask. The original prompt is
// deliberately not persisted in the mailbox.
type Request struct {
	Version       int       `json:"version"`
	ID            string    `json:"id"`
	Session       string    `json:"session"`
	Dir           string    `json:"dir"`
	Agent         string    `json:"agent"`
	RequesterPane string    `json:"requester_pane,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// Reply is the one-shot answer returned to the waiting asker.
type Reply struct {
	QuestionID string    `json:"question_id"`
	Text       string    `json:"text"`
	RepliedAt  time.Time `json:"replied_at"`
}

type outcome struct {
	Version    int       `json:"version"`
	Kind       string    `json:"kind"`
	QuestionID string    `json:"question_id"`
	Text       string    `json:"text,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Store is a local, filesystem-backed request/reply mailbox. A random question
// ID is the capability needed to submit the single allowed reply.
type Store struct {
	Dir          string
	Now          func() time.Time
	PollInterval time.Duration
	Random       io.Reader
	defaultDir   bool
}

// DefaultDir returns a per-user mailbox below the temporary runtime directory.
// Agent sandboxes commonly allow this location without granting home-directory
// writes; Create still verifies private ownership and modes before using it.
func DefaultDir() (string, error) {
	tempDir := os.TempDir()
	if tempDir == "" {
		return "", errors.New("resolve temporary directory: empty path")
	}
	return filepath.Join(tempDir, fmt.Sprintf("tmact-%d", os.Getuid()), "asks"), nil
}

// New returns a store with an absolute path so a dispatched session with a
// different cwd can use the same --store-dir value.
func New(dir string) (*Store, error) {
	var err error
	defaultDir := strings.TrimSpace(dir) == ""
	if defaultDir {
		dir, err = DefaultDir()
		if err != nil {
			return nil, err
		}
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve ask store: %w", err)
	}
	return &Store{
		Dir:          dir,
		Now:          time.Now,
		PollInterval: DefaultPollInterval,
		Random:       rand.Reader,
		defaultDir:   defaultDir,
	}, nil
}

// Create allocates and persists a new question.
func (s *Store) Create(session, dir, agent, requesterPane string, timeout time.Duration) (Request, error) {
	if timeout <= 0 {
		return Request{}, errors.New("ask timeout must be positive")
	}
	var err error
	if s.defaultDir {
		err = ensurePrivateRuntimeDir(s.Dir)
	} else {
		err = os.MkdirAll(s.Dir, 0o700)
	}
	if err != nil {
		return Request{}, fmt.Errorf("create ask store: %w", err)
	}
	for attempts := 0; attempts < 10; attempts++ {
		id, err := s.newID()
		if err != nil {
			return Request{}, err
		}
		questionDir := s.questionDir(id)
		if err := os.Mkdir(questionDir, 0o700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return Request{}, fmt.Errorf("create question mailbox: %w", err)
		}
		now := s.now()
		request := Request{
			Version:       Version,
			ID:            id,
			Session:       session,
			Dir:           dir,
			Agent:         agent,
			RequesterPane: requesterPane,
			CreatedAt:     now,
			ExpiresAt:     now.Add(timeout),
		}
		if err := writeExclusiveJSON(filepath.Join(questionDir, requestFile), request); err != nil {
			return Request{}, fmt.Errorf("write question request: %w", err)
		}
		return request, nil
	}
	return Request{}, errors.New("allocate unique question id")
}

// Reply atomically publishes the only answer allowed for id.
func (s *Store) Reply(id, text string) (Reply, error) {
	if err := validateID(id); err != nil {
		return Reply{}, err
	}
	if strings.TrimSpace(text) == "" {
		return Reply{}, errors.New("reply text is required")
	}
	if len([]byte(text)) > MaxReplyBytes {
		return Reply{}, fmt.Errorf("reply exceeds %d bytes", MaxReplyBytes)
	}
	request, err := s.readRequest(id)
	if err != nil {
		return Reply{}, err
	}
	now := s.now()
	if !now.Before(request.ExpiresAt) {
		_ = s.finalize(outcome{
			Version: Version, Kind: "canceled", QuestionID: id,
			Reason: "expired", CreatedAt: now,
		})
		return Reply{}, fmt.Errorf("%w: %s", ErrExpired, id)
	}
	result := outcome{
		Version: Version, Kind: "reply", QuestionID: id,
		Text: text, CreatedAt: now,
	}
	if err := s.finalize(result); err != nil {
		return Reply{}, err
	}
	return Reply{QuestionID: id, Text: text, RepliedAt: now}, nil
}

// Cancel closes an unanswered question. It is used when dispatch itself fails
// before an answer can be awaited.
func (s *Store) Cancel(id, reason string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		reason = "canceled"
	}
	err := s.finalize(outcome{
		Version: Version, Kind: "canceled", QuestionID: id,
		Reason: reason, CreatedAt: s.now(),
	})
	if errors.Is(err, ErrAlreadyFinalized) {
		return err
	}
	return err
}

// Wait blocks until id receives a reply, expires, or ctx is canceled. Timeout
// and cancellation also finalize the mailbox so a late answer cannot be
// mistaken for a live reply.
func (s *Store) Wait(ctx context.Context, id string) (Reply, error) {
	if err := validateID(id); err != nil {
		return Reply{}, err
	}
	request, err := s.readRequest(id)
	if err != nil {
		return Reply{}, err
	}
	interval := s.PollInterval
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		reply, done, err := s.readFinal(id)
		if done {
			return reply, err
		}
		if err != nil && !isIncompleteJSON(err) {
			return Reply{}, err
		}
		now := s.now()
		if !now.Before(request.ExpiresAt) {
			return s.cancelOrReadReply(id, "expired", ErrExpired)
		}
		select {
		case <-ctx.Done():
			return s.cancelOrReadReply(id, ctx.Err().Error(), ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s *Store) cancelOrReadReply(id, reason string, cause error) (Reply, error) {
	err := s.finalize(outcome{
		Version: Version, Kind: "canceled", QuestionID: id,
		Reason: reason, CreatedAt: s.now(),
	})
	if err == nil {
		return Reply{}, fmt.Errorf("wait for reply %s: %w", id, cause)
	}
	if !errors.Is(err, ErrAlreadyFinalized) {
		return Reply{}, err
	}

	// A concurrent reply reserves outcome.json before its single write
	// completes. Give that small write a bounded chance to become readable.
	for attempts := 0; attempts < 100; attempts++ {
		reply, done, readErr := s.readFinal(id)
		if done || (readErr != nil && !isIncompleteJSON(readErr)) {
			return reply, readErr
		}
		time.Sleep(10 * time.Millisecond)
	}
	return Reply{}, fmt.Errorf("read finalized question %s", id)
}

func (s *Store) readFinal(id string) (Reply, bool, error) {
	data, err := os.ReadFile(filepath.Join(s.questionDir(id), outcomeFile))
	if errors.Is(err, os.ErrNotExist) {
		return Reply{}, false, nil
	}
	if err != nil {
		return Reply{}, false, fmt.Errorf("read question outcome: %w", err)
	}
	var result outcome
	if err := json.Unmarshal(data, &result); err != nil {
		// The exclusive outcome file may be visible while its one writer is
		// finishing. The polling caller will retry this transient state.
		return Reply{}, false, err
	}
	if result.Version != Version || result.QuestionID != id {
		return Reply{}, false, fmt.Errorf("invalid outcome for question %s", id)
	}
	switch result.Kind {
	case "reply":
		return Reply{QuestionID: id, Text: result.Text, RepliedAt: result.CreatedAt}, true, nil
	case "canceled":
		return Reply{}, true, fmt.Errorf("%w: %s", ErrCanceled, result.Reason)
	default:
		return Reply{}, false, fmt.Errorf("invalid outcome kind %q", result.Kind)
	}
}

func (s *Store) finalize(result outcome) error {
	path := filepath.Join(s.questionDir(result.QuestionID), outcomeFile)
	if err := writeExclusiveJSON(path, result); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s", ErrAlreadyFinalized, result.QuestionID)
		}
		return fmt.Errorf("write question outcome: %w", err)
	}
	return nil
}

func (s *Store) readRequest(id string) (Request, error) {
	data, err := os.ReadFile(filepath.Join(s.questionDir(id), requestFile))
	if errors.Is(err, os.ErrNotExist) {
		return Request{}, fmt.Errorf("unknown question id %q", id)
	}
	if err != nil {
		return Request{}, fmt.Errorf("read question request: %w", err)
	}
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		return Request{}, fmt.Errorf("decode question request: %w", err)
	}
	if request.Version != Version || request.ID != id || request.ExpiresAt.IsZero() {
		return Request{}, fmt.Errorf("invalid question request %q", id)
	}
	return request, nil
}

func (s *Store) newID() (string, error) {
	raw := make([]byte, 16)
	if _, err := io.ReadFull(s.Random, raw); err != nil {
		return "", fmt.Errorf("generate question id: %w", err)
	}
	return "q_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)), nil
}

func (s *Store) questionDir(id string) string {
	return filepath.Join(s.Dir, id)
}

func (s *Store) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

func validateID(id string) error {
	if !questionIDPattern.MatchString(id) {
		return fmt.Errorf("invalid question id %q", id)
	}
	return nil
}

func writeExclusiveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	writeErr := writeAll(file, append(data, '\n'))
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		data = data[n:]
	}
	return nil
}

func isIncompleteJSON(err error) bool {
	var syntaxError *json.SyntaxError
	return errors.As(err, &syntaxError) || errors.Is(err, io.ErrUnexpectedEOF)
}

func ensurePrivateRuntimeDir(askDir string) error {
	root := filepath.Dir(askDir)
	if err := ensureOwnedPrivateDir(root); err != nil {
		return err
	}
	return ensureOwnedPrivateDir(askDir)
}

func ensureOwnedPrivateDir(path string) error {
	err := os.Mkdir(path, 0o700)
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is not a real directory", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("%s is not owned by the current user", path)
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(path, 0o700); err != nil {
			return fmt.Errorf("secure %s: %w", path, err)
		}
	}
	return nil
}
