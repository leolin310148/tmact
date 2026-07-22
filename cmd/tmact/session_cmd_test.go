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
	closed        []statusd.ClosedSession
	closeName     string
	closeExecute  bool
	reopenName    string
	reopenExecute bool
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

func TestSessionCommandsRequireExactOneName(t *testing.T) {
	fake := &fakeSessionLifecycle{}
	withFakeSessionLifecycle(t, fake)
	for _, args := range [][]string{
		{"session", "close"},
		{"session", "close", "one", "two"},
		{"session", "reopen"},
		{"session", "closed", "work"},
	} {
		if _, err := captureRun(t, args...); err == nil {
			t.Fatalf("%v unexpectedly succeeded", args)
		}
	}
}
