package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	sessionlife "github.com/leolin310148/tmact/internal/session"
	"github.com/leolin310148/tmact/internal/statusd"
)

type fakeSessionLifecycle struct {
	createName    string
	createDir     string
	createExecute bool
	closed        []statusd.ClosedSession
	closeName     string
	closeExecute  bool
	reopenName    string
	reopenExecute bool
	resumeName    string
	resumeDir     string
	resumeAgent   string
	resumeID      string
	resumeExecute bool
}

func (f *fakeSessionLifecycle) Create(name, dir string, execute bool) (sessionlife.Result, error) {
	f.createName, f.createDir, f.createExecute = name, dir, execute
	return sessionlife.Result{Action: "create", Status: sessionlife.StatusPlanned, Session: name, CWD: dir, Runtime: "shell", Executed: execute}, nil
}

func (f *fakeSessionLifecycle) Close(name string, execute bool) (sessionlife.Result, error) {
	f.closeName, f.closeExecute = name, execute
	return sessionlife.Result{Action: "close", Status: sessionlife.StatusPlanned, Session: name, CWD: "/repo", Runtime: "codex", Executed: execute}, nil
}

func (f *fakeSessionLifecycle) Closed() []statusd.ClosedSession { return f.closed }

func (f *fakeSessionLifecycle) Reopen(name string, execute bool) (sessionlife.Result, error) {
	f.reopenName, f.reopenExecute = name, execute
	return sessionlife.Result{Action: "reopen", Status: sessionlife.StatusPlanned, Session: name, CWD: "/repo", Runtime: "codex", RuntimeRestored: true, Executed: execute}, nil
}

func (f *fakeSessionLifecycle) Resume(name, dir, agent, sessionID string, execute bool) (sessionlife.Result, error) {
	f.resumeName, f.resumeDir, f.resumeAgent, f.resumeID, f.resumeExecute = name, dir, agent, sessionID, execute
	return sessionlife.Result{Action: "resume", Status: sessionlife.StatusPlanned, Session: name, CWD: dir, Runtime: agent, SessionID: sessionID, Executed: execute}, nil
}

func withFakeSessionLifecycle(t *testing.T, fake sessionLifecycle) {
	t.Helper()
	old := newSessionLifecycle
	newSessionLifecycle = func() (sessionLifecycle, error) { return fake, nil }
	t.Cleanup(func() { newSessionLifecycle = old })
}

func TestSessionCloseIsDryRunUnlessExecute(t *testing.T) {
	fake := &fakeSessionLifecycle{}
	withFakeSessionLifecycle(t, fake)
	out, err := captureRun(t, "session", "close", "work")
	if err != nil {
		t.Fatal(err)
	}
	if fake.closeName != "work" || fake.closeExecute || !strings.Contains(out, "dry-run session close work") {
		t.Fatalf("call=(%q,%t) output=%q", fake.closeName, fake.closeExecute, out)
	}

	out, err = captureRun(t, "session", "close", "work", "--execute", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if !fake.closeExecute {
		t.Fatal("--execute was not passed to service")
	}
	var result sessionlife.Result
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Action != "close" || result.Session != "work" || !result.Executed {
		t.Fatalf("result = %#v", result)
	}
}

func TestSessionCreateIsDryRunUnlessExecute(t *testing.T) {
	fake := &fakeSessionLifecycle{}
	withFakeSessionLifecycle(t, fake)
	out, err := captureRun(t, "session", "create", "work", "--dir", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if fake.createName != "work" || fake.createDir != "/repo" || fake.createExecute || !strings.Contains(out, "dry-run session create work") {
		t.Fatalf("call=(%q,%q,%t) output=%q", fake.createName, fake.createDir, fake.createExecute, out)
	}
	if _, err := captureRun(t, "session", "create", "work", "--dir", "/repo", "--execute"); err != nil {
		t.Fatal(err)
	}
	if !fake.createExecute {
		t.Fatal("--execute was not passed to create service")
	}
}

func TestSessionClosedJSONUsesSharedHistoryShape(t *testing.T) {
	closedAt := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	fake := &fakeSessionLifecycle{closed: []statusd.ClosedSession{{Session: "old", CWD: "/repo", Runtime: "claude", ClosedAt: closedAt}}}
	withFakeSessionLifecycle(t, fake)
	out, err := captureRun(t, "session", "closed", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Sessions []statusd.ClosedSession `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sessions) != 1 || body.Sessions[0].Session != "old" || body.Sessions[0].Runtime != "claude" {
		t.Fatalf("body = %#v", body)
	}
}

func TestSessionReopenIsPreviewable(t *testing.T) {
	fake := &fakeSessionLifecycle{}
	withFakeSessionLifecycle(t, fake)
	out, err := captureRun(t, "session", "reopen", "work")
	if err != nil {
		t.Fatal(err)
	}
	if fake.reopenName != "work" || fake.reopenExecute || !strings.Contains(out, "dry-run session reopen work") {
		t.Fatalf("call=(%q,%t) output=%q", fake.reopenName, fake.reopenExecute, out)
	}
	if _, err := captureRun(t, "session", "reopen", "work", "--execute"); err != nil {
		t.Fatal(err)
	}
	if !fake.reopenExecute {
		t.Fatal("--execute was not passed to reopen service")
	}
}

func TestSessionResumeRequiresExplicitProviderIDAndIsPreviewable(t *testing.T) {
	fake := &fakeSessionLifecycle{}
	withFakeSessionLifecycle(t, fake)
	args := []string{"session", "resume", "work", "--dir", "/repo", "--agent", "codex", "--session-id", "019c-example"}
	out, err := captureRun(t, args...)
	if err != nil {
		t.Fatal(err)
	}
	if fake.resumeName != "work" || fake.resumeDir != "/repo" || fake.resumeAgent != "codex" || fake.resumeID != "019c-example" || fake.resumeExecute {
		t.Fatalf("fake = %#v", fake)
	}
	if !strings.Contains(out, "dry-run session resume work") || !strings.Contains(out, "session-id=019c-example") {
		t.Fatalf("output = %q", out)
	}
	if _, err := captureRun(t, append(args, "--execute")...); err != nil {
		t.Fatal(err)
	}
	if !fake.resumeExecute {
		t.Fatal("--execute was not passed to resume service")
	}
	if _, err := captureRun(t, "session", "resume", "work", "--dir", "/repo", "--agent", "codex"); err == nil || !strings.Contains(err.Error(), "never infers") {
		t.Fatalf("missing id error = %v", err)
	}
}

func TestSessionCommandsRequireExactOneName(t *testing.T) {
	fake := &fakeSessionLifecycle{}
	withFakeSessionLifecycle(t, fake)
	for _, args := range [][]string{
		{"session", "create"},
		{"session", "create", "work"},
		{"session", "close"},
		{"session", "close", "one", "two"},
		{"session", "reopen"},
		{"session", "closed", "work"},
		{"session", "resume", "work", "--dir", "/repo", "--agent", "codex"},
	} {
		if _, err := captureRun(t, args...); err == nil {
			t.Fatalf("%v unexpectedly succeeded", args)
		}
	}
}
