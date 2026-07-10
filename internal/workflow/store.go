package workflow

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const DefaultStoreDir = ".tmact/workflows"

var ErrStateConflict = errors.New("workflow state changed concurrently")

const (
	StagePending       = "pending"
	StageRunnable      = "runnable"
	StageRunning       = "running"
	StageWaitingReport = "waiting_report"
	StageWaitingHuman  = "waiting_human"
	StageSucceeded     = "succeeded"
	StageFailed        = "failed"
	StageBlocked       = "blocked"
	StageStale         = "stale"
	StageSkipped       = "skipped"
)

type State struct {
	RunID       string                `json:"run_id"`
	Status      string                `json:"status"`
	Desired     string                `json:"desired"`
	ConfigPath  string                `json:"config_path"`
	ConfigHash  string                `json:"config_hash"`
	Workspace   string                `json:"workspace"`
	Variables   map[string]any        `json:"variables"`
	Revisions   map[string]string     `json:"revisions"`
	Stages      map[string]StageState `json:"stages"`
	StartedAt   time.Time             `json:"started_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
	HeartbeatAt time.Time             `json:"heartbeat_at"`
	FinishedAt  time.Time             `json:"finished_at,omitempty"`
	Reason      string                `json:"reason,omitempty"`
	PID         int                   `json:"pid,omitempty"`
	Sequence    uint64                `json:"sequence"`
}

type StageState struct {
	ID             string            `json:"id"`
	Status         string            `json:"status"`
	Outcome        string            `json:"outcome,omitempty"`
	Disposition    string            `json:"disposition,omitempty"`
	Attempt        int               `json:"attempt"`
	Generation     int               `json:"generation,omitempty"`
	DispatchID     string            `json:"dispatch_id,omitempty"`
	BoundRevisions map[string]string `json:"bound_revisions,omitempty"`
	Evidence       *Evidence         `json:"evidence,omitempty"`
	Input          map[string]any    `json:"input,omitempty"`
	StartedAt      time.Time         `json:"started_at,omitempty"`
	FinishedAt     time.Time         `json:"finished_at,omitempty"`
	NextAttemptAt  time.Time         `json:"next_attempt_at,omitempty"`
	Error          string            `json:"error,omitempty"`
}

type Evidence struct {
	Result     string    `json:"result"`
	Summary    string    `json:"summary,omitempty"`
	Argv       []string  `json:"argv,omitempty"`
	Cwd        string    `json:"cwd,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	ExitCode   *int      `json:"exit_code,omitempty"`
	Stdout     string    `json:"stdout,omitempty"`
	Stderr     string    `json:"stderr,omitempty"`
	Body       string    `json:"body,omitempty"`
}

type Event struct {
	Timestamp time.Time `json:"ts"`
	Type      string    `json:"type"`
	RunID     string    `json:"run_id"`
	Stage     string    `json:"stage,omitempty"`
	Attempt   int       `json:"attempt,omitempty"`
	Status    string    `json:"status,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	Details   any       `json:"details,omitempty"`
}
type Dispatch struct {
	Timestamp time.Time         `json:"ts"`
	ID        string            `json:"id"`
	RunID     string            `json:"run_id"`
	Stage     string            `json:"stage"`
	Attempt   int               `json:"attempt"`
	Actor     string            `json:"actor"`
	Target    string            `json:"target,omitempty"`
	Runtime   string            `json:"runtime,omitempty"`
	Status    string            `json:"status"`
	Revisions map[string]string `json:"revisions"`
}
type Report struct {
	Timestamp  time.Time         `json:"ts"`
	ID         string            `json:"id"`
	DispatchID string            `json:"dispatch_id"`
	RunID      string            `json:"run_id"`
	Stage      string            `json:"stage"`
	Attempt    int               `json:"attempt"`
	Outcome    string            `json:"outcome"`
	Body       string            `json:"body,omitempty"`
	Revisions  map[string]string `json:"revisions"`
}

type Store struct{ Root, RunID, Dir string }

func NewStore(root, runID string) Store {
	if root == "" {
		root = DefaultStoreDir
	}
	return Store{Root: root, RunID: runID, Dir: filepath.Join(root, runID)}
}
func (s Store) StatePath() string      { return filepath.Join(s.Dir, "state.json") }
func (s Store) EventsPath() string     { return filepath.Join(s.Dir, "events.jsonl") }
func (s Store) DispatchesPath() string { return filepath.Join(s.Dir, "dispatches.jsonl") }
func (s Store) ReportsPath() string    { return filepath.Join(s.Dir, "reports.jsonl") }

func (s Store) Init(loaded Loaded, state State) error {
	if err := os.MkdirAll(filepath.Join(s.Dir, "evidence"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.Dir, "config.yaml"), loaded.Raw, 0o644); err != nil {
		return err
	}
	vars, _ := json.MarshalIndent(loaded.Variables, "", "  ")
	if err := os.WriteFile(filepath.Join(s.Dir, "variables.json"), append(vars, '\n'), 0o644); err != nil {
		return err
	}
	return s.Write(state)
}
func (s Store) Read() (State, error) {
	raw, err := os.ReadFile(s.StatePath())
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, err
	}
	return state, nil
}
func (s Store) Write(state State) error {
	return s.withLock(func() error {
		if current, err := s.Read(); err == nil {
			if current.Sequence != state.Sequence {
				return ErrStateConflict
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return s.writeUnlocked(state)
	})
}
func (s Store) writeUnlocked(state State) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	state.Sequence++
	state.UpdatedAt = time.Now()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.StatePath() + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.StatePath())
}
func (s Store) Update(fn func(*State) error) error {
	return s.withLock(func() error {
		state, err := s.Read()
		if err != nil {
			return err
		}
		if err := fn(&state); err != nil {
			return err
		}
		return s.writeUnlocked(state)
	})
}
func (s Store) withLock(fn func() error) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

func (s Store) AcquireRunnerLock() (func(), error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(s.Dir, "runner.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("workflow %s already has an active runner", s.RunID)
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}
func (s Store) Append(path string, value any) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}
func (s Store) Event(event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.RunID = s.RunID
	return s.Append(s.EventsPath(), event)
}
func (s Store) Dispatch(d Dispatch) error { return s.Append(s.DispatchesPath(), d) }
func (s Store) Report(r Report) error     { return s.Append(s.ReportsPath(), r) }

func RunID(hash string) string {
	value := strings.TrimPrefix(hash, "sha256:")
	if len(value) > 16 {
		value = value[:16]
	}
	return "wf-" + value
}

func Find(root, id, configPath string) (Store, State, error) {
	if id != "" {
		s := NewStore(root, id)
		state, err := s.Read()
		return s, state, err
	}
	if configPath == "" {
		return Store{}, State{}, errors.New("one of --id or --config is required")
	}
	abs, err := filepath.Abs(configPath)
	if err != nil {
		return Store{}, State{}, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return Store{}, State{}, err
	}
	var matches []State
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		s := NewStore(root, entry.Name())
		state, e := s.Read()
		if e == nil && state.ConfigPath == abs {
			matches = append(matches, state)
		}
	}
	if len(matches) == 0 {
		return Store{}, State{}, fmt.Errorf("workflow run not found for config %s", abs)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].StartedAt.After(matches[j].StartedAt) })
	state := matches[0]
	return NewStore(root, state.RunID), state, nil
}

func FindDispatch(root, id string) (Store, Dispatch, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return Store{}, Dispatch{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		store := NewStore(root, entry.Name())
		records, err := readJSONLines[Dispatch](store.DispatchesPath())
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return Store{}, Dispatch{}, err
		}
		for i := len(records) - 1; i >= 0; i-- {
			if records[i].ID == id {
				return store, records[i], nil
			}
		}
	}
	return Store{}, Dispatch{}, fmt.Errorf("dispatch %q not found", id)
}
func readJSONLines[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []T
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scan.Scan() {
		var v T
		if err := json.Unmarshal(scan.Bytes(), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, scan.Err()
}
func LastDispatch(s Store, id string) (Dispatch, bool, error) {
	records, err := readJSONLines[Dispatch](s.DispatchesPath())
	if errors.Is(err, os.ErrNotExist) {
		return Dispatch{}, false, nil
	}
	if err != nil {
		return Dispatch{}, false, err
	}
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].ID == id {
			return records[i], true, nil
		}
	}
	return Dispatch{}, false, nil
}
func HasReport(s Store, dispatchID string) (Report, bool, error) {
	records, err := readJSONLines[Report](s.ReportsPath())
	if errors.Is(err, os.ErrNotExist) {
		return Report{}, false, nil
	}
	if err != nil {
		return Report{}, false, err
	}
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].DispatchID == dispatchID {
			return records[i], true, nil
		}
	}
	return Report{}, false, nil
}
