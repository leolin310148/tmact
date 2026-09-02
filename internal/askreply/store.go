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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	Version = 2
	// DefaultTimeout bounds how long an asker waits for the next answer.
	DefaultTimeout = 30 * time.Minute
	// DefaultReplyWaitTimeout bounds how long an answerer using `reply --wait`
	// blocks for the asker's follow-up. It is deliberately short because the
	// answerer's wait occupies one agent tool call.
	DefaultReplyWaitTimeout = 10 * time.Minute
	DefaultPollInterval     = 100 * time.Millisecond
	MaxReplyBytes           = 1024 * 1024

	RoleAsker    = "asker"
	RoleAnswerer = "answerer"

	// KindPrompt is an asker message: the initial task or a follow-up.
	KindPrompt = "prompt"
	// KindAnswer is an answerer message that leaves the question open.
	KindAnswer = "answer"
	// KindQuestion is an answerer message whose author is blocked waiting
	// for the asker's follow-up.
	KindQuestion = "question"
	// KindFinal is an answerer message that closes the question.
	KindFinal = "final"

	// DeliveryMailbox means the text travelled through this store and is
	// persisted in the message file.
	DeliveryMailbox = "mailbox"
	// DeliveryPane means the text was typed into the answerer's tmux pane and
	// only routing metadata is persisted.
	DeliveryPane = "pane"

	requestFile   = "request.json"
	closedFile    = "closed.json"
	waiterFile    = "waiter.json"
	messagePrefix = "msg-"
	messageSuffix = ".json"
	ackPrefix     = "ack-"
)

var (
	ErrAlreadyFinalized = errors.New("question already closed")
	ErrClosed           = errors.New("question closed")
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

// Message is one turn in a question thread. Seq is assigned by the store and
// increases by one per message regardless of author.
type Message struct {
	QuestionID string    `json:"question_id"`
	Seq        int       `json:"seq"`
	From       string    `json:"from"`
	Kind       string    `json:"kind"`
	Text       string    `json:"text,omitempty"`
	Delivery   string    `json:"delivery"`
	Target     string    `json:"target,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Closed records who ended the thread and why.
type Closed struct {
	By        string    `json:"by"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// Thread is a consistent read of one question directory.
type Thread struct {
	Request  Request
	Messages []Message
	Closed   *Closed
	// ExpiresAt is the effective deadline after which answerer messages are
	// rejected: the request deadline extended by every asker wait.
	ExpiresAt time.Time
}

type messageRecord struct {
	Version int `json:"version"`
	Message
	WaitUntil time.Time `json:"wait_until,omitempty"`
}

type closedRecord struct {
	Version    int    `json:"version"`
	QuestionID string `json:"question_id"`
	Closed
}

type waiterRecord struct {
	PID   int       `json:"pid"`
	Until time.Time `json:"until"`
}

type ackRecord struct {
	Seq int       `json:"seq"`
	PID int       `json:"pid"`
	At  time.Time `json:"at"`
}

// Store is a local, filesystem-backed request/reply mailbox. A random question
// ID is the capability needed to take part in its thread.
type Store struct {
	Dir          string
	Now          func() time.Time
	PollInterval time.Duration
	Random       io.Reader
	// ProcessAlive reports whether pid still runs; tests override it.
	ProcessAlive func(pid int) bool
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
		ProcessAlive: processAlive,
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

// Load reads the request, every complete message, and the closed marker.
func (s *Store) Load(id string) (Thread, error) {
	if err := validateID(id); err != nil {
		return Thread{}, err
	}
	request, err := s.readRequest(id)
	if err != nil {
		return Thread{}, err
	}
	return s.loadThread(request)
}

func (s *Store) loadThread(request Request) (Thread, error) {
	thread := Thread{Request: request, ExpiresAt: request.ExpiresAt}
	records, err := s.readMessages(request.ID)
	if err != nil {
		return Thread{}, err
	}
	for _, record := range records {
		thread.Messages = append(thread.Messages, record.Message)
		if record.WaitUntil.After(thread.ExpiresAt) {
			thread.ExpiresAt = record.WaitUntil
		}
	}
	closed, err := s.readClosed(request.ID)
	if err != nil {
		return Thread{}, err
	}
	thread.Closed = closed
	return thread, nil
}

// LastSeq returns the highest sequence number authored by from, or 0.
func (t Thread) LastSeq(from string) int {
	last := 0
	for _, message := range t.Messages {
		if message.From == from && message.Seq > last {
			last = message.Seq
		}
	}
	return last
}

// LastTarget returns the pane the asker most recently dispatched into.
func (t Thread) LastTarget() string {
	target := ""
	for _, message := range t.Messages {
		if message.From == RoleAsker && message.Target != "" {
			target = message.Target
		}
	}
	return target
}

// Post appends one message. waitUntil, when non-zero, extends the deadline
// before which answerer messages are still accepted; only asker messages set
// it. Answerer messages are rejected once the thread is closed or expired.
func (s *Store) Post(id string, message Message, waitUntil time.Time) (Message, error) {
	if err := validateID(id); err != nil {
		return Message{}, err
	}
	switch message.From {
	case RoleAsker:
		if message.Kind != KindPrompt {
			return Message{}, fmt.Errorf("asker messages must be %q, got %q", KindPrompt, message.Kind)
		}
	case RoleAnswerer:
		switch message.Kind {
		case KindAnswer, KindQuestion, KindFinal:
		default:
			return Message{}, fmt.Errorf("invalid answerer message kind %q", message.Kind)
		}
		if waitUntil != (time.Time{}) {
			return Message{}, errors.New("only asker messages extend the deadline")
		}
	default:
		return Message{}, fmt.Errorf("invalid message author %q", message.From)
	}
	switch message.Delivery {
	case DeliveryMailbox:
		if strings.TrimSpace(message.Text) == "" {
			return Message{}, errors.New("message text is required")
		}
	case DeliveryPane:
		if message.From != RoleAsker {
			return Message{}, errors.New("only asker messages are delivered by pane")
		}
		message.Text = ""
	default:
		return Message{}, fmt.Errorf("invalid delivery %q", message.Delivery)
	}
	if len([]byte(message.Text)) > MaxReplyBytes {
		return Message{}, fmt.Errorf("message exceeds %d bytes", MaxReplyBytes)
	}
	request, err := s.readRequest(id)
	if err != nil {
		return Message{}, err
	}
	thread, err := s.loadThread(request)
	if err != nil {
		return Message{}, err
	}
	if thread.Closed != nil {
		return Message{}, fmt.Errorf("%w: %s", ErrAlreadyFinalized, thread.Closed.Reason)
	}
	now := s.now()
	if message.From == RoleAnswerer && !now.Before(thread.ExpiresAt) {
		_ = s.Close(id, RoleAsker, "expired")
		return Message{}, fmt.Errorf("%w: %s", ErrExpired, id)
	}
	message.QuestionID = id
	message.CreatedAt = now
	record := messageRecord{Version: Version, Message: message, WaitUntil: waitUntil}
	seq := 0
	for _, existing := range thread.Messages {
		if existing.Seq > seq {
			seq = existing.Seq
		}
	}
	for attempts := 0; attempts < 100; attempts++ {
		seq++
		record.Seq = seq
		err := writeExclusiveJSON(s.messagePath(id, seq), record)
		if err == nil {
			break
		}
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return Message{}, fmt.Errorf("write question message: %w", err)
	}
	if record.Seq != seq {
		return Message{}, errors.New("allocate message sequence")
	}
	// The asker may have given up between our open check and the write. Be
	// conservative and tell the answerer the delivery is not guaranteed.
	if message.From == RoleAnswerer {
		closed, err := s.readClosed(id)
		if err != nil {
			return Message{}, err
		}
		if closed != nil {
			return record.Message, fmt.Errorf("%w while replying: %s", ErrClosed, closed.Reason)
		}
	}
	return record.Message, nil
}

// Reply appends an ordinary answer that keeps the thread open.
func (s *Store) Reply(id, text string) (Message, error) {
	return s.Post(id, Message{From: RoleAnswerer, Kind: KindAnswer, Text: text, Delivery: DeliveryMailbox}, time.Time{})
}

// Close ends the thread. Later messages from either side are rejected.
func (s *Store) Close(id, by, reason string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		reason = "closed"
	}
	record := closedRecord{
		Version:    Version,
		QuestionID: id,
		Closed:     Closed{By: by, Reason: reason, CreatedAt: s.now()},
	}
	if err := writeExclusiveJSON(filepath.Join(s.questionDir(id), closedFile), record); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: %s", ErrAlreadyFinalized, id)
		}
		return fmt.Errorf("write question closed marker: %w", err)
	}
	return nil
}

// Cancel closes an unanswered question on the asker's behalf. It is used when
// dispatch itself fails before an answer can be awaited.
func (s *Store) Cancel(id, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "canceled"
	}
	return s.Close(id, RoleAsker, reason)
}

// AwaitAnswer blocks until the answerer posts a message with Seq greater than
// afterSeq, the thread closes, the deadline passes, or ctx is canceled. A
// deadline or cancellation closes the thread so a late answer cannot be
// mistaken for a live reply; a message that raced the close is still returned.
func (s *Store) AwaitAnswer(ctx context.Context, id string, afterSeq int) (Message, error) {
	if err := validateID(id); err != nil {
		return Message{}, err
	}
	request, err := s.readRequest(id)
	if err != nil {
		return Message{}, err
	}
	ticker := time.NewTicker(s.pollInterval())
	defer ticker.Stop()
	for {
		thread, err := s.loadThread(request)
		if err != nil {
			return Message{}, err
		}
		if message, ok := nextFrom(thread, RoleAnswerer, afterSeq, ""); ok {
			return message, nil
		}
		if thread.Closed != nil {
			return Message{}, fmt.Errorf("%w: %s", ErrClosed, thread.Closed.Reason)
		}
		if !s.now().Before(thread.ExpiresAt) {
			return s.closeOrReadAnswer(request, afterSeq, "expired", ErrExpired)
		}
		select {
		case <-ctx.Done():
			return s.closeOrReadAnswer(request, afterSeq, ctx.Err().Error(), ctx.Err())
		case <-ticker.C:
		}
	}
}

func (s *Store) closeOrReadAnswer(request Request, afterSeq int, reason string, cause error) (Message, error) {
	closeErr := s.Close(request.ID, RoleAsker, reason)
	if closeErr != nil && !errors.Is(closeErr, ErrAlreadyFinalized) {
		return Message{}, closeErr
	}
	// An answer written just before the close marker is still a live reply.
	thread, err := s.loadThread(request)
	if err != nil {
		return Message{}, err
	}
	if message, ok := nextFrom(thread, RoleAnswerer, afterSeq, ""); ok {
		return message, nil
	}
	if closeErr != nil && thread.Closed != nil && thread.Closed.By == RoleAnswerer {
		return Message{}, fmt.Errorf("wait for reply %s: %w: %s", request.ID, ErrClosed, thread.Closed.Reason)
	}
	return Message{}, fmt.Errorf("wait for reply %s: %w", request.ID, cause)
}

// AwaitPrompt blocks on the answerer's side until the asker posts a
// mailbox-delivered follow-up with Seq greater than afterSeq, the thread
// closes, or ctx is canceled. While waiting it advertises a live waiter so the
// asker delivers through the mailbox instead of the pane. A timeout does not
// close the thread: the asker can still continue through the pane.
func (s *Store) AwaitPrompt(ctx context.Context, id string, afterSeq int) (Message, error) {
	if err := validateID(id); err != nil {
		return Message{}, err
	}
	request, err := s.readRequest(id)
	if err != nil {
		return Message{}, err
	}
	until := s.now().Add(DefaultReplyWaitTimeout)
	if deadline, ok := ctx.Deadline(); ok {
		until = deadline
	}
	waiterPath := filepath.Join(s.questionDir(id), waiterFile)
	if err := writePrivateJSON(waiterPath, waiterRecord{PID: os.Getpid(), Until: until}); err != nil {
		return Message{}, fmt.Errorf("register waiter: %w", err)
	}
	defer os.Remove(waiterPath)

	ticker := time.NewTicker(s.pollInterval())
	defer ticker.Stop()
	for {
		thread, err := s.loadThread(request)
		if err != nil {
			return Message{}, err
		}
		if message, ok := nextFrom(thread, RoleAsker, afterSeq, DeliveryMailbox); ok {
			return s.acknowledge(id, message)
		}
		if thread.Closed != nil {
			return Message{}, fmt.Errorf("%w: %s", ErrClosed, thread.Closed.Reason)
		}
		select {
		case <-ctx.Done():
			// The asker may have posted between the last poll and now.
			thread, err := s.loadThread(request)
			if err != nil {
				return Message{}, err
			}
			if message, ok := nextFrom(thread, RoleAsker, afterSeq, DeliveryMailbox); ok {
				return s.acknowledge(id, message)
			}
			return Message{}, fmt.Errorf("wait for follow-up %s: %w", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

// acknowledge records that the waiter consumed message before the deferred
// waiter-marker removal runs, so an asker rechecking the waiter can tell
// "delivered" from "the waiter gave up".
func (s *Store) acknowledge(id string, message Message) (Message, error) {
	if err := writePrivateJSON(s.ackPath(id, message.Seq), ackRecord{Seq: message.Seq, PID: os.Getpid(), At: s.now()}); err != nil {
		return Message{}, fmt.Errorf("acknowledge follow-up: %w", err)
	}
	return message, nil
}

// Acknowledged reports whether an answerer waiter has consumed the
// mailbox-delivered message with the given seq.
func (s *Store) Acknowledged(id string, seq int) (bool, error) {
	if err := validateID(id); err != nil {
		return false, err
	}
	_, err := os.Stat(s.ackPath(id, seq))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read acknowledgement marker: %w", err)
	}
	return true, nil
}

func (s *Store) ackPath(id string, seq int) string {
	return filepath.Join(s.questionDir(id), fmt.Sprintf("%s%06d%s", ackPrefix, seq, messageSuffix))
}

// HasWaiter reports whether an answerer process is currently blocked in
// AwaitPrompt for id. A stale marker from a dead or expired waiter is removed.
func (s *Store) HasWaiter(id string) (bool, error) {
	if err := validateID(id); err != nil {
		return false, err
	}
	path := filepath.Join(s.questionDir(id), waiterFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read waiter marker: %w", err)
	}
	var record waiterRecord
	if err := json.Unmarshal(data, &record); err != nil {
		// The waiter is rewriting the marker; treat it as present.
		return true, nil
	}
	alive := s.ProcessAlive
	if alive == nil {
		alive = processAlive
	}
	if record.PID <= 0 || !s.now().Before(record.Until) || !alive(record.PID) {
		_ = os.Remove(path)
		return false, nil
	}
	return true, nil
}

func nextFrom(thread Thread, from string, afterSeq int, delivery string) (Message, bool) {
	for _, message := range thread.Messages {
		if message.From != from || message.Seq <= afterSeq {
			continue
		}
		if delivery != "" && message.Delivery != delivery {
			continue
		}
		return message, true
	}
	return Message{}, false
}

func (s *Store) readMessages(id string) ([]messageRecord, error) {
	entries, err := os.ReadDir(s.questionDir(id))
	if err != nil {
		return nil, fmt.Errorf("read question mailbox: %w", err)
	}
	seqs := make([]int, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, messagePrefix) || !strings.HasSuffix(name, messageSuffix) {
			continue
		}
		seq, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, messagePrefix), messageSuffix))
		if err != nil || seq <= 0 {
			continue
		}
		seqs = append(seqs, seq)
	}
	sort.Ints(seqs)
	records := make([]messageRecord, 0, len(seqs))
	for _, seq := range seqs {
		data, err := os.ReadFile(s.messagePath(id, seq))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read question message: %w", err)
		}
		var record messageRecord
		if err := json.Unmarshal(data, &record); err != nil {
			if isIncompleteJSON(err) {
				// The exclusive file is visible while its one writer finishes.
				// Stop here so sequence order is preserved for the next poll.
				break
			}
			return nil, fmt.Errorf("decode question message %d: %w", seq, err)
		}
		if record.Version != Version || record.QuestionID != id || record.Seq != seq {
			return nil, fmt.Errorf("invalid message %d for question %s", seq, id)
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *Store) readClosed(id string) (*Closed, error) {
	data, err := os.ReadFile(filepath.Join(s.questionDir(id), closedFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read question closed marker: %w", err)
	}
	var record closedRecord
	if err := json.Unmarshal(data, &record); err != nil {
		if isIncompleteJSON(err) {
			// Visible but still being written: report closed with a generic reason.
			return &Closed{Reason: "closing"}, nil
		}
		return nil, fmt.Errorf("decode question closed marker: %w", err)
	}
	if record.Version != Version || record.QuestionID != id {
		return nil, fmt.Errorf("invalid closed marker for question %s", id)
	}
	closed := record.Closed
	return &closed, nil
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
	if request.Version != Version {
		return Request{}, fmt.Errorf("question %s was created by a different tmact mailbox version (%d, want %d); start a new ask", id, request.Version, Version)
	}
	if request.ID != id || request.ExpiresAt.IsZero() {
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

func (s *Store) messagePath(id string, seq int) string {
	return filepath.Join(s.questionDir(id), fmt.Sprintf("%s%06d%s", messagePrefix, seq, messageSuffix))
}

func (s *Store) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

func (s *Store) pollInterval() time.Duration {
	if s.PollInterval <= 0 {
		return DefaultPollInterval
	}
	return s.PollInterval
}

func validateID(id string) error {
	if !questionIDPattern.MatchString(id) {
		return fmt.Errorf("invalid question id %q", id)
	}
	return nil
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func writeExclusiveJSON(path string, value any) error {
	return writeJSONFile(path, value, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
}

// writePrivateJSON overwrites a marker file whose content is advisory, so a
// truncate-and-write is acceptable.
func writePrivateJSON(path string, value any) error {
	return writeJSONFile(path, value, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
}

func writeJSONFile(path string, value any, flags int) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, flags, 0o600)
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
