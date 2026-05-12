package runmeta

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

const DefaultDir = ".tmact/runs"

type TmuxInfo struct {
	PaneID      string `json:"pane_id,omitempty"`
	Session     string `json:"session,omitempty"`
	WindowIndex int    `json:"window_index,omitempty"`
	WindowName  string `json:"window_name,omitempty"`
	PaneIndex   int    `json:"pane_index,omitempty"`
}

type Run struct {
	ID         string    `json:"id"`
	Kind       string    `json:"kind"`
	ConfigPath string    `json:"config_path"`
	Target     string    `json:"target"`
	LogPath    string    `json:"log_path,omitempty"`
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"started_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	StoppedAt  time.Time `json:"stopped_at,omitempty"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	Tmux       TmuxInfo  `json:"tmux,omitempty"`
}

type EventSummary struct {
	Timestamp time.Time `json:"ts,omitempty"`
	Type      string    `json:"type,omitempty"`
	Target    string    `json:"target,omitempty"`
	Action    string    `json:"action,omitempty"`
	Stage     string    `json:"stage,omitempty"`
	Cycle     int       `json:"cycle,omitempty"`
	Status    string    `json:"status,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

type Status struct {
	Run            Run            `json:"run"`
	RuntimeStatus  string         `json:"runtime_status"`
	LastEvent      *EventSummary  `json:"last_event,omitempty"`
	RecentProblems []EventSummary `json:"recent_problems,omitempty"`
}

type RegisterOptions struct {
	Kind       string
	ConfigPath string
	Target     string
	LogPath    string
	Tmux       TmuxInfo
	Now        time.Time
}

func Register(dir string, opts RegisterOptions) (Run, error) {
	if dir == "" {
		dir = DefaultDir
	}
	if opts.Kind == "" {
		return Run{}, errors.New("kind is required")
	}
	if opts.ConfigPath == "" {
		return Run{}, errors.New("config path is required")
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	configPath := opts.ConfigPath
	if abs, err := filepath.Abs(configPath); err == nil {
		configPath = abs
	}
	id := buildID(opts.Kind, opts.ConfigPath, opts.Target, os.Getpid())
	run := Run{
		ID:         id,
		Kind:       opts.Kind,
		ConfigPath: configPath,
		Target:     opts.Target,
		LogPath:    opts.LogPath,
		PID:        os.Getpid(),
		StartedAt:  opts.Now,
		UpdatedAt:  opts.Now,
		Status:     "running",
		Tmux:       opts.Tmux,
	}
	return run, Write(dir, run)
}

func Finish(dir string, run Run, status string, reason string, now time.Time) error {
	if dir == "" {
		dir = DefaultDir
	}
	if now.IsZero() {
		now = time.Now()
	}
	run.Status = status
	run.Reason = reason
	run.UpdatedAt = now
	run.StoppedAt = now
	return Write(dir, run)
}

func Mark(dir string, run Run, status string, reason string, now time.Time) error {
	if dir == "" {
		dir = DefaultDir
	}
	if now.IsZero() {
		now = time.Now()
	}
	run.Status = status
	run.Reason = reason
	run.UpdatedAt = now
	return Write(dir, run)
}

func Write(dir string, run Run) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	path := Path(dir, run.ID)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Path(dir string, id string) string {
	return filepath.Join(dir, id+".json")
}

func Read(dir string, id string) (Run, error) {
	data, err := os.ReadFile(Path(dir, id))
	if err != nil {
		return Run{}, err
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func List(dir string, kind string, now time.Time) ([]Status, error) {
	if dir == "" {
		dir = DefaultDir
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var statuses []Status
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var run Run
		if err := json.Unmarshal(data, &run); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		if kind != "" && run.Kind != kind {
			continue
		}
		status, err := BuildStatus(run, now)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Run.StartedAt.After(statuses[j].Run.StartedAt)
	})
	return statuses, nil
}

func BuildStatus(run Run, now time.Time) (Status, error) {
	status := Status{Run: run, RuntimeStatus: RuntimeStatus(run)}
	if run.LogPath != "" {
		last, problems, err := ReadLogSummary(run.LogPath)
		if err != nil {
			problems = append(problems, EventSummary{Type: "error", Reason: err.Error()})
		}
		status.LastEvent = last
		status.RecentProblems = problems
	}
	return status, nil
}

func RuntimeStatus(run Run) string {
	if run.Status == "stopped" || run.Status == "error" {
		return run.Status
	}
	if run.PID <= 0 {
		return "unknown"
	}
	if ProcessAlive(run.PID) {
		return run.Status
	}
	return "dead"
}

func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func Select(statuses []Status, id string, configPath string) (Run, error) {
	status, err := SelectStatus(statuses, id, configPath)
	if err != nil {
		return Run{}, err
	}
	return status.Run, nil
}

func SelectStatus(statuses []Status, id string, configPath string) (Status, error) {
	if id == "" && configPath == "" {
		return Status{}, errors.New("--id or --config is required")
	}
	if configPath != "" {
		if abs, err := filepath.Abs(configPath); err == nil {
			configPath = abs
		}
	}
	var matches []Status
	var active []Status
	for _, status := range statuses {
		run := status.Run
		switch {
		case id != "" && run.ID == id:
			matches = append(matches, status)
		case configPath != "" && run.ConfigPath == configPath:
			matches = append(matches, status)
			if status.RuntimeStatus == "running" || status.RuntimeStatus == "stopping" {
				active = append(active, status)
			}
		}
	}
	if len(matches) == 0 {
		return Status{}, errors.New("run not found")
	}
	if configPath != "" && len(active) == 1 {
		return active[0], nil
	}
	if configPath != "" && len(active) > 1 {
		return Status{}, fmt.Errorf("multiple active runs matched; use --id")
	}
	if len(matches) > 1 {
		return Status{}, fmt.Errorf("multiple runs matched; use --id")
	}
	return matches[0], nil
}

func ReadLogSummary(path string) (*EventSummary, []EventSummary, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var last *EventSummary
	var problems []EventSummary
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		event, err := parseEvent(line)
		if err != nil {
			continue
		}
		last = &event
		if isProblem(event) {
			problems = append(problems, event)
			if len(problems) > 3 {
				problems = problems[len(problems)-3:]
			}
		}
	}
	return last, problems, nil
}

func parseEvent(line string) (EventSummary, error) {
	var raw struct {
		Timestamp string `json:"ts"`
		Type      string `json:"type"`
		Target    string `json:"target"`
		Action    string `json:"action"`
		Stage     string `json:"stage"`
		Cycle     int    `json:"cycle"`
		Status    string `json:"status"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return EventSummary{}, err
	}
	event := EventSummary{
		Type:   raw.Type,
		Target: raw.Target,
		Action: raw.Action,
		Stage:  raw.Stage,
		Cycle:  raw.Cycle,
		Status: raw.Status,
		Reason: raw.Reason,
	}
	if raw.Timestamp != "" {
		if ts, err := time.Parse(time.RFC3339, raw.Timestamp); err == nil {
			event.Timestamp = ts
		}
	}
	return event, nil
}

func isProblem(event EventSummary) bool {
	return event.Type == "error" || event.Type == "stop" || event.Type == "blocked" || event.Status == "failed"
}

var nonID = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func buildID(kind string, configPath string, target string, pid int) string {
	base := strings.TrimSuffix(filepath.Base(configPath), filepath.Ext(configPath))
	if base == "" || base == "." {
		base = target
	}
	slug := strings.Trim(nonID.ReplaceAllString(base, "-"), "-")
	if slug == "" {
		slug = "run"
	}
	return fmt.Sprintf("%s-%s-%d", kind, slug, pid)
}
