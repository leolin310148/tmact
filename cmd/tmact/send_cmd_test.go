package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
)

func TestListPrintsAndCachesNumberedTargets(t *testing.T) {
	t.Chdir(t.TempDir())
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	listAllTmuxPanes = func() ([]tmux.Pane, error) {
		return []tmux.Pane{{
			Session:        "sample-team",
			WindowIndex:    1,
			WindowName:     "main-codex",
			PaneIndex:      0,
			PaneID:         "%42",
			CurrentCommand: "codex",
			CurrentPath:    "/repo",
			Active:         true,
		}}, nil
	}
	tmactNow = func() time.Time { return time.Date(2026, 5, 11, 9, 30, 0, 0, time.UTC) }

	out, err := captureRun(t, "ls")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"#", "target", "0", "%42", "sample-team", "1:main-codex", "codex", "/repo"} {
		if !strings.Contains(out, want) {
			t.Fatalf("ls output missing %q: %s", want, out)
		}
	}

	data, err := os.ReadFile(filepath.Join(".cache", "tmact-targets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"target": "%42"`) {
		t.Fatalf("cache = %s", data)
	}
}

func TestSendDryRunResolvesNumberedTarget(t *testing.T) {
	t.Chdir(t.TempDir())
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	tmactNow = func() time.Time { return time.Date(2026, 5, 11, 9, 30, 0, 0, time.UTC) }
	if err := writeTargetCache(targetCache{
		GeneratedAt: tmactNow(),
		Panes: []listPaneRow{{
			Index:  0,
			Target: "%42",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	listTargetTmuxPanes = func(target string) ([]tmux.Pane, error) {
		if target != "%42" {
			t.Fatalf("target = %q", target)
		}
		return []tmux.Pane{{PaneID: "%42"}}, nil
	}
	pasteTmuxText = func(string, string, bool) error {
		t.Fatal("dry-run should not paste")
		return nil
	}

	out, err := captureRun(t, "-t", "0", "send", "--command", "go test ./...")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dry-run: would send command to %42: go test ./...") {
		t.Fatalf("output = %q", out)
	}
}

func TestSendRejectsExpiredNumberedTargetCache(t *testing.T) {
	t.Chdir(t.TempDir())
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	now := time.Date(2026, 5, 11, 10, 1, 0, 0, time.UTC)
	tmactNow = func() time.Time { return now }
	if err := writeTargetCache(targetCache{
		GeneratedAt: now.Add(-targetCacheMaxAge - time.Second),
		Panes: []listPaneRow{{
			Index:  0,
			Target: "%42",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	listTargetTmuxPanes = func(string) ([]tmux.Pane, error) {
		t.Fatal("expired cache should be rejected before tmux lookup")
		return nil, nil
	}

	_, err := captureRun(t, "-t", "0", "send", "--text", "status?")
	if err == nil || !strings.Contains(err.Error(), "target cache is older than 30m0s") {
		t.Fatalf("err = %v, want expired cache error", err)
	}
}

func TestSendExecuteCommandCanClearLine(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	var sentKeys [][]string
	var pastedTarget, pastedText string
	var pastedEnter bool
	sendTmuxKeys = func(_ string, keys []string) error {
		sentKeys = append(sentKeys, append([]string(nil), keys...))
		return nil
	}
	pasteTmuxText = func(target string, text string, enter bool) error {
		pastedTarget = target
		pastedText = text
		pastedEnter = enter
		return nil
	}

	out, err := captureRun(t, "-t", "%42", "send", "--clear-line", "--command", "go test ./...", "--execute")
	if err != nil {
		t.Fatal(err)
	}
	if len(sentKeys) != 1 || strings.Join(sentKeys[0], ",") != "C-u" {
		t.Fatalf("sent keys = %#v", sentKeys)
	}
	if pastedTarget != "%42" || pastedText != "go test ./..." || !pastedEnter {
		t.Fatalf("pasted target=%q text=%q enter=%t", pastedTarget, pastedText, pastedEnter)
	}
	if !strings.Contains(out, "clear line and send command to %42: go test ./...") {
		t.Fatalf("output = %q", out)
	}
}

func TestSendExecuteCombinesRepeatedAndCSVKeys(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	var sentTarget string
	var sentKeys []string
	sendTmuxKeys = func(target string, keys []string) error {
		sentTarget = target
		sentKeys = append([]string(nil), keys...)
		return nil
	}
	pasteTmuxText = func(string, string, bool) error {
		t.Fatal("key mode should not paste text")
		return nil
	}

	out, err := captureRun(t, "-t", "%42", "send", "--key", "C-a", "--keys", "Left,Enter", "--execute")
	if err != nil {
		t.Fatal(err)
	}
	if sentTarget != "%42" {
		t.Fatalf("sent target = %q", sentTarget)
	}
	if strings.Join(sentKeys, ",") != "C-a,Left,Enter" {
		t.Fatalf("sent keys = %#v", sentKeys)
	}
	if !strings.Contains(out, "send keys to %42: C-a,Left,Enter") {
		t.Fatalf("output = %q", out)
	}
}

func TestSendExecutePeerTarget(t *testing.T) {
	resetCLIHooks := stubCLIHooks(t)
	defer resetCLIHooks()

	configPath := filepath.Join(t.TempDir(), "statusd.json")
	writeText(t, configPath, `{"peers":[{"name":"peer-a","url":"http://peer-a.example:7890"}]}`)

	var gotPeer statusd.Peer
	var gotTarget string
	var gotReport sendReport
	sendPeerPaneInput = func(_ context.Context, peer statusd.Peer, target string, report sendReport) error {
		gotPeer = peer
		gotTarget = target
		gotReport = report
		return nil
	}
	sendTmuxKeys = func(string, []string) error {
		t.Fatal("peer send must not use local tmux")
		return nil
	}
	pasteTmuxText = func(string, string, bool) error {
		t.Fatal("peer send must not use local tmux")
		return nil
	}

	out, err := captureRun(t, "-t", "peer-a@%42", "send", "--config", configPath, "--command", "go test ./...", "--execute")
	if err != nil {
		t.Fatal(err)
	}
	if gotPeer.Name != "peer-a" || gotPeer.URL != "http://peer-a.example:7890" {
		t.Fatalf("peer = %#v", gotPeer)
	}
	if gotTarget != "%42" {
		t.Fatalf("target = %q", gotTarget)
	}
	if gotReport.Target != "peer-a@%42" || gotReport.RemoteTarget != "%42" || gotReport.Text != "go test ./..." || !gotReport.Enter {
		t.Fatalf("report = %#v", gotReport)
	}
	if !strings.Contains(out, "send command to peer-a@%42: go test ./...") {
		t.Fatalf("output = %q", out)
	}
}

func TestSendValidation(t *testing.T) {
	tests := [][]string{
		{"send", "--command", "go test ./..."},
		{"-t", "%42", "send"},
		{"-t", "%42", "send", "--text", "hi", "--command", "go test ./..."},
		{"-t", "%42", "send", "--key", "Enter", "--enter"},
		{"-t", "%42", "send", "--keys", "C-u,"},
	}
	for _, args := range tests {
		if _, err := captureRun(t, args...); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

func TestGlobalTargetRejectedForNonSendCommands(t *testing.T) {
	if _, err := captureRun(t, "-t", "%42", "ls"); err == nil {
		t.Fatal("expected global target to be rejected for ls")
	}
}
