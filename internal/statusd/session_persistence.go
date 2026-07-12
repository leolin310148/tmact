package statusd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	SessionSnapshotVersion          = 1
	DefaultSessionSaveInterval      = 5 * time.Minute
	DefaultSessionSnapshotRetention = 10
	DefaultSessionSnapshotDirName   = "tmux-sessions"
	maxRestoreSessions              = 256
	maxRestoreWindows               = 4096
	maxRestorePanes                 = 16384
	maxRestoreIndex                 = 1_000_000
	maxRestoreLayoutLength          = 64 * 1024
	lastSessionSnapshotName         = "last"
)

var (
	ErrNoSessions      = errors.New("tmux session snapshot has no sessions")
	ErrNoValidSnapshot = errors.New("no valid tmux session snapshot")
)

type SessionSnapshot struct {
	Version  int            `json:"version"`
	SavedAt  time.Time      `json:"saved_at"`
	Sessions []SavedSession `json:"sessions"`
}

type SavedSession struct {
	Name              string        `json:"name"`
	ActiveWindowIndex int           `json:"active_window_index"`
	Windows           []SavedWindow `json:"windows"`
}

type SavedWindow struct {
	Index           int         `json:"index"`
	Name            string      `json:"name"`
	Layout          string      `json:"layout"`
	Width           int         `json:"width"`
	Height          int         `json:"height"`
	ActivePaneIndex int         `json:"active_pane_index"`
	Panes           []SavedPane `json:"panes"`
}

type SavedPane struct {
	Index int    `json:"index"`
	CWD   string `json:"cwd"`
}

// BuildSessionSnapshot turns one successful tmux capture into a stable,
// sorted on-disk representation. Empty or incomplete captures are rejected so
// they can never replace the last known-good snapshot.
func BuildSessionSnapshot(panes []tmux.SessionStatePane, savedAt time.Time) (SessionSnapshot, error) {
	snapshot := SessionSnapshot{Version: SessionSnapshotVersion, SavedAt: savedAt.UTC()}
	if len(panes) == 0 {
		return snapshot, ErrNoSessions
	}

	type windowBuilder struct {
		window     SavedWindow
		active     bool
		activePane bool
	}
	type sessionBuilder struct {
		windows map[int]*windowBuilder
	}
	builders := map[string]*sessionBuilder{}
	for _, pane := range panes {
		session := builders[pane.Session]
		if session == nil {
			session = &sessionBuilder{windows: map[int]*windowBuilder{}}
			builders[pane.Session] = session
		}
		window := session.windows[pane.WindowIndex]
		if window == nil {
			window = &windowBuilder{window: SavedWindow{
				Index:  pane.WindowIndex,
				Name:   pane.WindowName,
				Layout: pane.WindowLayout,
				Width:  pane.WindowWidth,
				Height: pane.WindowHeight,
			}}
			session.windows[pane.WindowIndex] = window
		} else if window.window.Name != pane.WindowName || window.window.Layout != pane.WindowLayout ||
			window.window.Width != pane.WindowWidth || window.window.Height != pane.WindowHeight {
			return snapshot, fmt.Errorf("inconsistent window metadata for %q:%d", pane.Session, pane.WindowIndex)
		}
		window.window.Panes = append(window.window.Panes, SavedPane{Index: pane.PaneIndex, CWD: pane.CurrentPath})
		if pane.Active {
			if window.activePane {
				return snapshot, fmt.Errorf("tmux window %q:%d has multiple active panes", pane.Session, pane.WindowIndex)
			}
			window.window.ActivePaneIndex = pane.PaneIndex
			window.activePane = true
		}
		if pane.WindowActive {
			window.active = true
		}
	}

	sessionNames := make([]string, 0, len(builders))
	for name := range builders {
		sessionNames = append(sessionNames, name)
	}
	sort.Strings(sessionNames)
	for _, name := range sessionNames {
		builder := builders[name]
		session := SavedSession{Name: name}
		activeWindow := false
		indexes := make([]int, 0, len(builder.windows))
		for index := range builder.windows {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		for _, index := range indexes {
			window := builder.windows[index]
			if !window.activePane {
				return snapshot, fmt.Errorf("tmux window %q:%d has no active pane", name, index)
			}
			sort.Slice(window.window.Panes, func(i, j int) bool {
				return window.window.Panes[i].Index < window.window.Panes[j].Index
			})
			if window.active {
				if activeWindow {
					return snapshot, fmt.Errorf("tmux session %q has multiple active windows", name)
				}
				session.ActiveWindowIndex = index
				activeWindow = true
			}
			session.Windows = append(session.Windows, window.window)
		}
		if !activeWindow {
			return snapshot, fmt.Errorf("tmux session %q has no active window", name)
		}
		snapshot.Sessions = append(snapshot.Sessions, session)
	}
	if err := ValidateSessionSnapshot(snapshot); err != nil {
		return SessionSnapshot{}, err
	}
	return snapshot, nil
}

func ValidateSessionSnapshot(snapshot SessionSnapshot) error {
	if snapshot.Version != SessionSnapshotVersion {
		return fmt.Errorf("unsupported tmux session snapshot version %d", snapshot.Version)
	}
	if snapshot.SavedAt.IsZero() {
		return errors.New("tmux session snapshot has no saved_at")
	}
	if len(snapshot.Sessions) == 0 {
		return ErrNoSessions
	}
	if len(snapshot.Sessions) > maxRestoreSessions {
		return fmt.Errorf("tmux session snapshot has too many sessions: %d", len(snapshot.Sessions))
	}
	seenSessions := map[string]bool{}
	windowCount, paneCount := 0, 0
	for _, session := range snapshot.Sessions {
		if strings.TrimSpace(session.Name) == "" || strings.ContainsAny(session.Name, ":.") {
			return fmt.Errorf("invalid tmux session name %q", session.Name)
		}
		if seenSessions[session.Name] {
			return fmt.Errorf("duplicate tmux session %q", session.Name)
		}
		seenSessions[session.Name] = true
		if len(session.Windows) == 0 {
			return fmt.Errorf("tmux session %q has no windows", session.Name)
		}
		seenWindows := map[int]bool{}
		activeWindowFound := false
		for _, window := range session.Windows {
			windowCount++
			if windowCount > maxRestoreWindows {
				return fmt.Errorf("tmux session snapshot has too many windows")
			}
			if window.Index < 0 || window.Index > maxRestoreIndex || seenWindows[window.Index] {
				return fmt.Errorf("invalid or duplicate window index %d in session %q", window.Index, session.Name)
			}
			seenWindows[window.Index] = true
			if window.Index == session.ActiveWindowIndex {
				activeWindowFound = true
			}
			if len(window.Layout) > maxRestoreLayoutLength || !validWindowLayout(window.Layout) {
				return fmt.Errorf("invalid layout for %q:%d", session.Name, window.Index)
			}
			if window.Width <= 0 || window.Width > maxRestoreIndex || window.Height <= 0 || window.Height > maxRestoreIndex {
				return fmt.Errorf("invalid size %dx%d for %q:%d", window.Width, window.Height, session.Name, window.Index)
			}
			if len(window.Panes) == 0 {
				return fmt.Errorf("tmux window %q:%d has no panes", session.Name, window.Index)
			}
			seenPanes := map[int]bool{}
			activePaneFound := false
			for _, pane := range window.Panes {
				paneCount++
				if paneCount > maxRestorePanes {
					return fmt.Errorf("tmux session snapshot has too many panes")
				}
				if pane.Index < 0 || pane.Index > maxRestoreIndex || seenPanes[pane.Index] {
					return fmt.Errorf("invalid or duplicate pane index %d in %q:%d", pane.Index, session.Name, window.Index)
				}
				seenPanes[pane.Index] = true
				if pane.Index == window.ActivePaneIndex {
					activePaneFound = true
				}
				if !filepath.IsAbs(pane.CWD) {
					return fmt.Errorf("pane cwd is not absolute in %q:%d.%d", session.Name, window.Index, pane.Index)
				}
			}
			if !activePaneFound {
				return fmt.Errorf("active pane %d missing from %q:%d", window.ActivePaneIndex, session.Name, window.Index)
			}
		}
		if !activeWindowFound {
			return fmt.Errorf("active window %d missing from session %q", session.ActiveWindowIndex, session.Name)
		}
	}
	return nil
}

type SessionRestorePlan struct {
	Sessions     []SavedSession
	CWDFallbacks int
}

func BuildSessionRestorePlan(snapshot SessionSnapshot, home string, dirExists func(string) bool) (SessionRestorePlan, error) {
	if err := ValidateSessionSnapshot(snapshot); err != nil {
		return SessionRestorePlan{}, err
	}
	if dirExists == nil {
		dirExists = directoryExists
	}
	if !filepath.IsAbs(home) || !dirExists(home) {
		return SessionRestorePlan{}, fmt.Errorf("restore fallback home is not an existing absolute directory: %q", home)
	}
	data, err := json.Marshal(snapshot.Sessions)
	if err != nil {
		return SessionRestorePlan{}, err
	}
	var sessions []SavedSession
	if err := json.Unmarshal(data, &sessions); err != nil {
		return SessionRestorePlan{}, err
	}
	plan := SessionRestorePlan{Sessions: sessions}
	for si := range plan.Sessions {
		for wi := range plan.Sessions[si].Windows {
			for pi := range plan.Sessions[si].Windows[wi].Panes {
				pane := &plan.Sessions[si].Windows[wi].Panes[pi]
				if !dirExists(pane.CWD) {
					pane.CWD = home
					plan.CWDFallbacks++
				}
			}
		}
	}
	return plan, nil
}

func ExecuteSessionRestore(plan SessionRestorePlan, client tmux.RestoreClient) error {
	if client == nil {
		return errors.New("tmux restore client is nil")
	}
	if err := ValidateSessionSnapshot(SessionSnapshot{
		Version:  SessionSnapshotVersion,
		SavedAt:  time.Now(),
		Sessions: plan.Sessions,
	}); err != nil {
		return fmt.Errorf("invalid tmux restore plan: %w", err)
	}
	created := make([]string, 0, len(plan.Sessions))
	rollback := func() {
		for i := len(created) - 1; i >= 0; i-- {
			_ = client.KillSession(created[i])
		}
	}
	fail := func(err error) error {
		rollback()
		return err
	}

	for _, session := range plan.Sessions {
		firstWindow := session.Windows[0]
		firstPane := firstWindow.Panes[0]
		if err := client.NewSession(session.Name, firstWindow.Name, firstPane.CWD, firstWindow.Index); err != nil {
			return fail(fmt.Errorf("create tmux session %q: %w", session.Name, err))
		}
		created = append(created, session.Name)
		if err := restoreWindow(client, session.Name, firstWindow, true); err != nil {
			return fail(err)
		}
		for _, window := range session.Windows[1:] {
			if err := client.NewWindow(session.Name, window.Name, window.Panes[0].CWD, window.Index); err != nil {
				return fail(fmt.Errorf("create tmux window %q:%d: %w", session.Name, window.Index, err))
			}
			if err := restoreWindow(client, session.Name, window, true); err != nil {
				return fail(err)
			}
		}
		if err := client.SelectWindow(session.Name, session.ActiveWindowIndex); err != nil {
			return fail(fmt.Errorf("select tmux window %q:%d: %w", session.Name, session.ActiveWindowIndex, err))
		}
	}
	return nil
}

func restoreWindow(client tmux.RestoreClient, session string, window SavedWindow, firstPaneExists bool) error {
	if err := client.ResizeWindow(session, window.Index, window.Width, window.Height); err != nil {
		return fmt.Errorf("resize tmux window %q:%d: %w", session, window.Index, err)
	}
	start := 0
	if firstPaneExists {
		start = 1
	}
	for _, pane := range window.Panes[start:] {
		if err := client.SplitWindow(session, window.Index, pane.CWD); err != nil {
			return fmt.Errorf("split tmux pane %q:%d.%d: %w", session, window.Index, pane.Index, err)
		}
	}
	if err := client.SelectLayout(session, window.Index, window.Layout); err != nil {
		return fmt.Errorf("restore tmux layout %q:%d: %w", session, window.Index, err)
	}
	if err := client.SelectPane(session, window.Index, window.ActivePaneIndex); err != nil {
		return fmt.Errorf("select tmux pane %q:%d.%d: %w", session, window.Index, window.ActivePaneIndex, err)
	}
	return nil
}

type SessionSnapshotStore struct {
	Dir       string
	Retention int
}

func (s SessionSnapshotStore) Save(snapshot SessionSnapshot) (string, error) {
	if err := ValidateSessionSnapshot(snapshot); err != nil {
		return "", err
	}
	if s.Dir == "" {
		return "", errors.New("tmux session snapshot directory is empty")
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	name := "session-" + snapshot.SavedAt.UTC().Format("20060102T150405.000000000Z") + ".json"
	path := filepath.Join(s.Dir, name)
	if err := writeAtomicFile(path, data, 0o600); err != nil {
		return "", err
	}
	if err := writeAtomicFile(filepath.Join(s.Dir, lastSessionSnapshotName), []byte(name+"\n"), 0o600); err != nil {
		return "", err
	}
	_ = syncDirectory(s.Dir)
	if err := s.prune(); err != nil {
		return path, err
	}
	return path, nil
}

func (s SessionSnapshotStore) LoadLatest() (SessionSnapshot, string, error) {
	if s.Dir == "" {
		return SessionSnapshot{}, "", ErrNoValidSnapshot
	}
	lastData, err := os.ReadFile(filepath.Join(s.Dir, lastSessionSnapshotName))
	if err == nil {
		name := strings.TrimSpace(string(lastData))
		if validSnapshotFilename(name) {
			if snapshot, readErr := readSessionSnapshot(filepath.Join(s.Dir, name)); readErr == nil {
				return snapshot, filepath.Join(s.Dir, name), nil
			}
		}
	}

	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SessionSnapshot{}, "", ErrNoValidSnapshot
		}
		return SessionSnapshot{}, "", err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && validSnapshotFilename(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names {
		path := filepath.Join(s.Dir, name)
		snapshot, readErr := readSessionSnapshot(path)
		if readErr != nil {
			continue
		}
		_ = writeAtomicFile(filepath.Join(s.Dir, lastSessionSnapshotName), []byte(name+"\n"), 0o600)
		return snapshot, path, nil
	}
	return SessionSnapshot{}, "", ErrNoValidSnapshot
}

func (s SessionSnapshotStore) prune() error {
	retention := s.Retention
	if retention <= 0 {
		retention = DefaultSessionSnapshotRetention
	}
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !validSnapshotFilename(entry.Name()) {
			continue
		}
		if _, err := readSessionSnapshot(filepath.Join(s.Dir, entry.Name())); err == nil {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names[minimum(retention, len(names)):] {
		if err := os.Remove(filepath.Join(s.Dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

type SessionRestoreOutcome string

const (
	SessionRestoreSkippedExisting   SessionRestoreOutcome = "existing_sessions"
	SessionRestoreSkippedNoSnapshot SessionRestoreOutcome = "no_snapshot"
	SessionRestoreCompleted         SessionRestoreOutcome = "restored"
)

type SessionPersistence struct {
	Store     SessionSnapshotStore
	Capture   func() ([]tmux.SessionStatePane, error)
	Client    tmux.RestoreClient
	Now       func() time.Time
	HomeDir   func() (string, error)
	DirExists func(string) bool
}

func (p SessionPersistence) Save() (string, error) {
	panes, err := p.capture()()
	if err != nil {
		return "", fmt.Errorf("capture tmux sessions: %w", err)
	}
	snapshot, err := BuildSessionSnapshot(panes, p.now()())
	if err != nil {
		return "", err
	}
	return p.Store.Save(snapshot)
}

func (p SessionPersistence) RestoreIfEmpty() (SessionRestoreOutcome, string, int, error) {
	panes, err := p.capture()()
	if err != nil {
		return "", "", 0, fmt.Errorf("check tmux sessions before restore: %w", err)
	}
	if len(panes) > 0 {
		return SessionRestoreSkippedExisting, "", 0, nil
	}
	snapshot, path, err := p.Store.LoadLatest()
	if err != nil {
		if errors.Is(err, ErrNoValidSnapshot) {
			return SessionRestoreSkippedNoSnapshot, "", 0, nil
		}
		return "", "", 0, err
	}
	home, err := p.homeDir()()
	if err != nil {
		return "", path, 0, err
	}
	plan, err := BuildSessionRestorePlan(snapshot, home, p.DirExists)
	if err != nil {
		return "", path, 0, err
	}
	client := p.Client
	if client == nil {
		client = tmux.LiveRestoreClient{}
	}
	if err := ExecuteSessionRestore(plan, client); err != nil {
		return "", path, plan.CWDFallbacks, err
	}
	return SessionRestoreCompleted, path, plan.CWDFallbacks, nil
}

func (p SessionPersistence) capture() func() ([]tmux.SessionStatePane, error) {
	if p.Capture != nil {
		return p.Capture
	}
	return tmux.ListSessionState
}

func (p SessionPersistence) now() func() time.Time {
	if p.Now != nil {
		return p.Now
	}
	return time.Now
}

func (p SessionPersistence) homeDir() func() (string, error) {
	if p.HomeDir != nil {
		return p.HomeDir
	}
	return os.UserHomeDir
}

func DefaultSessionSnapshotDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, DefaultFileConfigDir, DefaultSessionSnapshotDirName)
}

func readSessionSnapshot(path string) (SessionSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionSnapshot{}, err
	}
	var snapshot SessionSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return SessionSnapshot{}, err
	}
	if err := ValidateSessionSnapshot(snapshot); err != nil {
		return SessionSnapshot{}, err
	}
	return snapshot, nil
}

func validSnapshotFilename(name string) bool {
	return filepath.Base(name) == name && strings.HasPrefix(name, "session-") && strings.HasSuffix(name, ".json")
}

func validWindowLayout(layout string) bool {
	if layout == "" || strings.HasPrefix(layout, "-") {
		return false
	}
	for _, r := range layout {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') ||
			r == 'x' || r == 'X' || r == ',' || r == '{' || r == '}' || r == '[' || r == ']' {
			continue
		}
		return false
	}
	return true
}

func writeAtomicFile(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if err = tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func minimum(a, b int) int {
	if a < b {
		return a
	}
	return b
}
