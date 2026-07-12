package statusd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/tmux"
)

func TestBuildSessionSnapshotSortsAndOmitsCommands(t *testing.T) {
	now := time.Date(2026, 7, 12, 1, 2, 3, 0, time.FixedZone("TST", 8*60*60))
	panes := []tmux.SessionStatePane{
		{Session: "z", WindowIndex: 2, WindowName: "two", WindowLayout: "abcd,80x24,0,0,0", WindowWidth: 80, WindowHeight: 24, WindowActive: true, PaneIndex: 1, CurrentPath: "/z/b", Active: true},
		{Session: "a", WindowIndex: 1, WindowName: "one", WindowLayout: "bcde,80x24,0,0,0", WindowWidth: 80, WindowHeight: 24, WindowActive: true, PaneIndex: 1, CurrentPath: "/a/b"},
		{Session: "a", WindowIndex: 1, WindowName: "one", WindowLayout: "bcde,80x24,0,0,0", WindowWidth: 80, WindowHeight: 24, WindowActive: true, PaneIndex: 0, CurrentPath: "/a/a", Active: true},
	}
	snapshot, err := BuildSessionSnapshot(panes, now)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SavedAt.Location() != time.UTC {
		t.Fatalf("SavedAt location = %v, want UTC", snapshot.SavedAt.Location())
	}
	if got := []string{snapshot.Sessions[0].Name, snapshot.Sessions[1].Name}; got[0] != "a" || got[1] != "z" {
		t.Fatalf("session order = %v", got)
	}
	if got := snapshot.Sessions[0].Windows[0].Panes; got[0].Index != 0 || got[1].Index != 1 {
		t.Fatalf("pane order = %+v", got)
	}
	data, err := os.ReadFile(writeSnapshotForTest(t, SessionSnapshotStore{Dir: t.TempDir(), Retention: 2}, snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(data)), "command") {
		t.Fatalf("snapshot unexpectedly contains a command field: %s", data)
	}
}

func TestBuildSessionSnapshotRejectsEmptyAndIncompleteCapture(t *testing.T) {
	if _, err := BuildSessionSnapshot(nil, time.Now()); !errors.Is(err, ErrNoSessions) {
		t.Fatalf("empty capture error = %v", err)
	}
	_, err := BuildSessionSnapshot([]tmux.SessionStatePane{{
		Session: "work", WindowIndex: 0, WindowName: "shell", WindowLayout: "abcd,80x24,0,0,0", WindowWidth: 80, WindowHeight: 24, PaneIndex: 0, CurrentPath: "/tmp",
	}}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("incomplete capture error = %v", err)
	}
}

func TestSessionPersistenceDoesNotReplaceLastOnEmptyOrCaptureFailure(t *testing.T) {
	dir := t.TempDir()
	store := SessionSnapshotStore{Dir: dir, Retention: 3}
	first := validSessionSnapshot(time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC))
	writeSnapshotForTest(t, store, first)
	before, err := os.ReadFile(filepath.Join(dir, lastSessionSnapshotName))
	if err != nil {
		t.Fatal(err)
	}

	p := SessionPersistence{Store: store, Capture: func() ([]tmux.SessionStatePane, error) { return nil, nil }}
	if _, err := p.Save(); !errors.Is(err, ErrNoSessions) {
		t.Fatalf("empty save error = %v", err)
	}
	assertFileContent(t, filepath.Join(dir, lastSessionSnapshotName), before)

	p.Capture = func() ([]tmux.SessionStatePane, error) { return nil, errors.New("tmux unavailable") }
	if _, err := p.Save(); err == nil || !strings.Contains(err.Error(), "tmux unavailable") {
		t.Fatalf("failed capture error = %v", err)
	}
	assertFileContent(t, filepath.Join(dir, lastSessionSnapshotName), before)
}

func TestSessionSnapshotStoreFallsBackFromCorruptLast(t *testing.T) {
	dir := t.TempDir()
	store := SessionSnapshotStore{Dir: dir, Retention: 3}
	oldPath := writeSnapshotForTest(t, store, validSessionSnapshot(time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC)))
	newPath := writeSnapshotForTest(t, store, validSessionSnapshot(time.Date(2026, 7, 12, 2, 0, 0, 0, time.UTC)))
	if err := os.WriteFile(newPath, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, lastSessionSnapshotName), []byte(filepath.Base(newPath)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, gotPath, err := store.LoadLatest()
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != oldPath {
		t.Fatalf("loaded %q, want %q", gotPath, oldPath)
	}
	assertFileContent(t, filepath.Join(dir, lastSessionSnapshotName), []byte(filepath.Base(oldPath)+"\n"))
}

func TestSessionSnapshotStoreRetainsNewestVersions(t *testing.T) {
	dir := t.TempDir()
	store := SessionSnapshotStore{Dir: dir, Retention: 2}
	for hour := 1; hour <= 3; hour++ {
		writeSnapshotForTest(t, store, validSessionSnapshot(time.Date(2026, 7, 12, hour, 0, 0, 0, time.UTC)))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if validSnapshotFilename(entry.Name()) {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("snapshot count = %d, want 2", count)
	}
}

func TestBuildSessionRestorePlanFallsBackMissingCWD(t *testing.T) {
	snapshot := validSessionSnapshot(time.Now())
	snapshot.Sessions[0].Windows[0].Panes = append(snapshot.Sessions[0].Windows[0].Panes, SavedPane{Index: 1, CWD: "/missing"})
	exists := func(path string) bool { return path == "/home/test" || path == "/work" }
	plan, err := BuildSessionRestorePlan(snapshot, "/home/test", exists)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CWDFallbacks != 1 {
		t.Fatalf("fallbacks = %d, want 1", plan.CWDFallbacks)
	}
	if got := plan.Sessions[0].Windows[0].Panes[1].CWD; got != "/home/test" {
		t.Fatalf("fallback cwd = %q", got)
	}
}

func TestExecuteSessionRestoreOrdersSideEffects(t *testing.T) {
	snapshot := validSessionSnapshot(time.Now())
	snapshot.Sessions[0].Windows[0].Panes = append(snapshot.Sessions[0].Windows[0].Panes, SavedPane{Index: 1, CWD: "/work/sub"})
	plan, err := BuildSessionRestorePlan(snapshot, "/home/test", func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	client := &recordingRestoreClient{}
	if err := ExecuteSessionRestore(plan, client); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"new-session work:1 shell /work",
		"resize work:1 80x24",
		"split work:1 /work/sub",
		"layout work:1 abcd,80x24,0,0,0",
		"pane work:1.0",
		"window work:1",
	}
	if strings.Join(client.calls, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(client.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestExecuteSessionRestoreRollsBackCreatedSessions(t *testing.T) {
	plan, err := BuildSessionRestorePlan(validSessionSnapshot(time.Now()), "/home/test", func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	client := &recordingRestoreClient{failAt: "layout"}
	if err := ExecuteSessionRestore(plan, client); err == nil {
		t.Fatal("restore succeeded, want failure")
	}
	if got := client.calls[len(client.calls)-1]; got != "kill work" {
		t.Fatalf("last call = %q, want rollback", got)
	}
}

func TestRestoreIfEmptyDecisionGate(t *testing.T) {
	dir := t.TempDir()
	store := SessionSnapshotStore{Dir: dir, Retention: 2}
	path := writeSnapshotForTest(t, store, validSessionSnapshot(time.Now()))
	client := &recordingRestoreClient{}
	p := SessionPersistence{
		Store:     store,
		Client:    client,
		HomeDir:   func() (string, error) { return "/home/test", nil },
		DirExists: func(string) bool { return true },
	}

	p.Capture = func() ([]tmux.SessionStatePane, error) {
		return []tmux.SessionStatePane{{Session: "existing"}}, nil
	}
	outcome, _, _, err := p.RestoreIfEmpty()
	if err != nil || outcome != SessionRestoreSkippedExisting || len(client.calls) != 0 {
		t.Fatalf("existing outcome=%q err=%v calls=%v", outcome, err, client.calls)
	}

	p.Capture = func() ([]tmux.SessionStatePane, error) { return nil, errors.New("capture failed") }
	if _, _, _, err := p.RestoreIfEmpty(); err == nil {
		t.Fatal("capture failure was treated as empty")
	}
	if len(client.calls) != 0 {
		t.Fatalf("capture failure restored: %v", client.calls)
	}

	p.Capture = func() ([]tmux.SessionStatePane, error) { return []tmux.SessionStatePane{}, nil }
	outcome, gotPath, _, err := p.RestoreIfEmpty()
	if err != nil || outcome != SessionRestoreCompleted || gotPath != path {
		t.Fatalf("empty outcome=%q path=%q err=%v", outcome, gotPath, err)
	}
	if len(client.calls) == 0 {
		t.Fatal("empty tmux did not restore")
	}
}

func TestDaemonSessionSaveInterval(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 12, 1, 0, 0, 0, time.UTC)
	d := NewDaemon(Config{
		SessionSave:              true,
		SessionSaveInterval:      5 * time.Minute,
		SessionSnapshotRetention: 10,
		SessionSnapshotDir:       dir,
		Now:                      func() time.Time { return now },
		ListSessionState: func() ([]tmux.SessionStatePane, error) {
			return validCapturedSessionState(), nil
		},
	})
	d.maybeSaveSessions()
	if got := countSnapshotFiles(t, dir); got != 1 {
		t.Fatalf("initial snapshot count = %d, want 1", got)
	}
	now = now.Add(time.Minute)
	d.maybeSaveSessions()
	if got := countSnapshotFiles(t, dir); got != 1 {
		t.Fatalf("early snapshot count = %d, want 1", got)
	}
	now = now.Add(4 * time.Minute)
	d.maybeSaveSessions()
	if got := countSnapshotFiles(t, dir); got != 2 {
		t.Fatalf("due snapshot count = %d, want 2", got)
	}
}

func validSessionSnapshot(savedAt time.Time) SessionSnapshot {
	return SessionSnapshot{
		Version: SessionSnapshotVersion,
		SavedAt: savedAt.UTC(),
		Sessions: []SavedSession{{
			Name:              "work",
			ActiveWindowIndex: 1,
			Windows: []SavedWindow{{
				Index:           1,
				Name:            "shell",
				Layout:          "abcd,80x24,0,0,0",
				Width:           80,
				Height:          24,
				ActivePaneIndex: 0,
				Panes:           []SavedPane{{Index: 0, CWD: "/work"}},
			}},
		}},
	}
}

func validCapturedSessionState() []tmux.SessionStatePane {
	return []tmux.SessionStatePane{{
		Session:      "work",
		WindowIndex:  1,
		WindowName:   "shell",
		WindowLayout: "abcd,80x24,0,0,0",
		WindowWidth:  80,
		WindowHeight: 24,
		WindowActive: true,
		PaneIndex:    0,
		CurrentPath:  "/work",
		Active:       true,
	}}
}

func countSnapshotFiles(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if validSnapshotFilename(entry.Name()) {
			count++
		}
	}
	return count
}

func writeSnapshotForTest(t *testing.T, store SessionSnapshotStore, snapshot SessionSnapshot) string {
	t.Helper()
	path, err := store.Save(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func assertFileContent(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

type recordingRestoreClient struct {
	calls  []string
	failAt string
}

func (c *recordingRestoreClient) record(kind, call string) error {
	c.calls = append(c.calls, call)
	if c.failAt == kind {
		return errors.New("injected failure")
	}
	return nil
}

func (c *recordingRestoreClient) NewSession(session, window, cwd string, index int) error {
	return c.record("new-session", fmt.Sprintf("new-session %s:%d %s %s", session, index, window, cwd))
}
func (c *recordingRestoreClient) NewWindow(session, window, cwd string, index int) error {
	return c.record("new-window", fmt.Sprintf("new-window %s:%d %s %s", session, index, window, cwd))
}
func (c *recordingRestoreClient) SplitWindow(session string, index int, cwd string) error {
	return c.record("split", fmt.Sprintf("split %s:%d %s", session, index, cwd))
}
func (c *recordingRestoreClient) ResizeWindow(session string, index, width, height int) error {
	return c.record("resize", fmt.Sprintf("resize %s:%d %dx%d", session, index, width, height))
}
func (c *recordingRestoreClient) SelectLayout(session string, index int, layout string) error {
	return c.record("layout", fmt.Sprintf("layout %s:%d %s", session, index, layout))
}
func (c *recordingRestoreClient) SelectPane(session string, window, pane int) error {
	return c.record("pane", fmt.Sprintf("pane %s:%d.%d", session, window, pane))
}
func (c *recordingRestoreClient) SelectWindow(session string, index int) error {
	return c.record("window", fmt.Sprintf("window %s:%d", session, index))
}
func (c *recordingRestoreClient) KillSession(session string) error {
	return c.record("kill", "kill "+session)
}
