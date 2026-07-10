package tmux

import "testing"

func TestShellJoinQuotesEveryArgument(t *testing.T) {
	got := shellJoin([]string{"/tmp/tmact test", "loop", "it's.yaml", ""})
	want := "'/tmp/tmact test' 'loop' 'it'\\''s.yaml' ''"
	if got != want {
		t.Fatalf("shellJoin = %q want %q", got, want)
	}
}

func TestParsePanes(t *testing.T) {
	raw := "sample|$1|0|codex-aarch64-a|0|%14|70365|codex-aarch64-a|/tmp/tmact-sample/project|1|0|1\n"

	panes, err := ParsePanes(raw)
	if err != nil {
		t.Fatalf("ParsePanes returned error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d", len(panes))
	}
	pane := panes[0]
	if pane.Session != "sample" {
		t.Fatalf("session = %q", pane.Session)
	}
	if pane.SessionID != "$1" {
		t.Fatalf("session id = %q", pane.SessionID)
	}
	if pane.WindowIndex != 0 || pane.PaneIndex != 0 {
		t.Fatalf("target indexes = %d.%d", pane.WindowIndex, pane.PaneIndex)
	}
	if pane.PaneID != "%14" {
		t.Fatalf("pane id = %q", pane.PaneID)
	}
	if pane.PanePID != 70365 {
		t.Fatalf("pane pid = %d", pane.PanePID)
	}
	if !pane.WindowActive {
		t.Fatal("window should be active")
	}
	if !pane.Active {
		t.Fatal("pane should be active")
	}
	if pane.InMode {
		t.Fatal("pane should not be in mode")
	}
}

func TestParsePanesAcceptsLegacyFormat(t *testing.T) {
	raw := "sample|0|codex-aarch64-a|0|%14|70365|codex-aarch64-a|/tmp/tmact-sample/project|1|0|1\n"

	panes, err := ParsePanes(raw)
	if err != nil {
		t.Fatalf("ParsePanes returned error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d", len(panes))
	}
	if panes[0].SessionID != "" {
		t.Fatalf("legacy session id = %q", panes[0].SessionID)
	}
	if panes[0].CurrentPath != "/tmp/tmact-sample/project" {
		t.Fatalf("legacy current path = %q", panes[0].CurrentPath)
	}
	if !panes[0].Active {
		t.Fatal("legacy pane should be active")
	}
	if !panes[0].WindowActive {
		t.Fatal("legacy window should be active")
	}
}

func TestParsePanesAcceptsSessionIDWithoutWindowActive(t *testing.T) {
	raw := "sample|$1|0|codex-aarch64-a|0|%14|70365|codex-aarch64-a|/tmp/tmact-sample/project|1|0\n"

	panes, err := ParsePanes(raw)
	if err != nil {
		t.Fatalf("ParsePanes returned error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d", len(panes))
	}
	if panes[0].SessionID != "$1" {
		t.Fatalf("session id = %q", panes[0].SessionID)
	}
	if !panes[0].WindowActive {
		t.Fatal("window active should default to true when window_active is absent")
	}
}

func TestParsePanesAcceptsTabDelimitedFormat(t *testing.T) {
	raw := "sample\t$1\t0\tcodex-aarch64-a\t0\t%14\t70365\tcodex-aarch64-a\t/tmp/tmact-sample/project\t1\t0\t1\n"

	panes, err := ParsePanes(raw)
	if err != nil {
		t.Fatalf("ParsePanes returned error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d", len(panes))
	}
	pane := panes[0]
	if pane.Session != "sample" {
		t.Fatalf("session = %q", pane.Session)
	}
	if pane.SessionID != "$1" {
		t.Fatalf("session id = %q", pane.SessionID)
	}
	if pane.WindowName != "codex-aarch64-a" {
		t.Fatalf("window name = %q", pane.WindowName)
	}
	if pane.CurrentPath != "/tmp/tmact-sample/project" {
		t.Fatalf("current path = %q", pane.CurrentPath)
	}
	if !pane.Active || pane.InMode || !pane.WindowActive {
		t.Fatalf("pane flags active=%v inMode=%v windowActive=%v", pane.Active, pane.InMode, pane.WindowActive)
	}
}

func TestParsePanesAllowsPipeInCurrentPath(t *testing.T) {
	raw := "sample|$1|0|codex-aarch64-a|0|%14|70365|codex-aarch64-a|/tmp/tmact-sample/with|pipe|1|0|1\n"

	panes, err := ParsePanes(raw)
	if err != nil {
		t.Fatalf("ParsePanes returned error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d", len(panes))
	}
	if panes[0].CurrentPath != "/tmp/tmact-sample/with|pipe" {
		t.Fatalf("current path = %q", panes[0].CurrentPath)
	}
	if !panes[0].WindowActive {
		t.Fatal("window should be active")
	}
}

func TestParsePanesAllowsPipeInCurrentCommand(t *testing.T) {
	raw := "sample|$1|0|codex-aarch64-a|0|%14|70365|agent|worker|/tmp/tmact-sample/project|1|0|1\n"

	panes, err := ParsePanes(raw)
	if err != nil {
		t.Fatalf("ParsePanes returned error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d", len(panes))
	}
	if panes[0].CurrentCommand != "agent|worker" {
		t.Fatalf("current command = %q", panes[0].CurrentCommand)
	}
	if panes[0].CurrentPath != "/tmp/tmact-sample/project" {
		t.Fatalf("current path = %q", panes[0].CurrentPath)
	}
}

func TestParsePanesAllowsPipeInWindowName(t *testing.T) {
	raw := "sample|$1|0|codex|aarch64|0|%14|70365|codex-aarch64-a|/tmp/tmact-sample/project|1|0|1\n"

	panes, err := ParsePanes(raw)
	if err != nil {
		t.Fatalf("ParsePanes returned error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d", len(panes))
	}
	if panes[0].WindowName != "codex|aarch64" {
		t.Fatalf("window name = %q", panes[0].WindowName)
	}
	if panes[0].CurrentPath != "/tmp/tmact-sample/project" {
		t.Fatalf("current path = %q", panes[0].CurrentPath)
	}
}

func TestParsePanesAllowsPipeInSessionName(t *testing.T) {
	raw := "sample|team|$1|0|codex-aarch64-a|0|%14|70365|codex-aarch64-a|/tmp/tmact-sample/project|1|0|1\n"

	panes, err := ParsePanes(raw)
	if err != nil {
		t.Fatalf("ParsePanes returned error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d", len(panes))
	}
	if panes[0].Session != "sample|team" {
		t.Fatalf("session = %q", panes[0].Session)
	}
	if panes[0].SessionID != "$1" {
		t.Fatalf("session id = %q", panes[0].SessionID)
	}
	if panes[0].WindowName != "codex-aarch64-a" {
		t.Fatalf("window name = %q", panes[0].WindowName)
	}
	if panes[0].CurrentPath != "/tmp/tmact-sample/project" {
		t.Fatalf("current path = %q", panes[0].CurrentPath)
	}
}

func TestParsePanesRejectsMalformedRow(t *testing.T) {
	_, err := ParsePanes("too\tfew\n")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetSessionOptionValidatesInput(t *testing.T) {
	if err := SetSessionOption("", "@ai-tag", "cx"); err == nil {
		t.Fatal("expected empty session error")
	}
	if err := SetSessionOption("work", "", "cx"); err == nil {
		t.Fatal("expected empty key error")
	}
}

func TestPasteBufferArgsUseBracketedPaste(t *testing.T) {
	got := pasteBufferArgs("%7", "tmact-paste-test")
	want := []string{"paste-buffer", "-p", "-t", "%7", "-b", "tmact-paste-test"}
	if len(got) != len(want) {
		t.Fatalf("args len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d] = %q, want %q; all args: %v", i, got[i], want[i], got)
		}
	}
}

func TestCanSendLiteralRejectsStandaloneSemicolon(t *testing.T) {
	if canSendLiteral(";") {
		t.Fatal("standalone semicolon must use paste-buffer, not send-keys -l")
	}
	if !canSendLiteral("a;b") {
		t.Fatal("embedded semicolon should remain eligible for literal send")
	}
}

func TestTmuxArgsForcesUTF8(t *testing.T) {
	args := []string{"list-panes", "-a"}

	got := tmuxArgs(args)
	want := []string{"-u", "list-panes", "-a"}
	if len(got) != len(want) {
		t.Fatalf("args len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d] = %q, want %q; all args: %v", i, got[i], want[i], got)
		}
	}
	if len(args) != 2 || args[0] != "list-panes" || args[1] != "-a" {
		t.Fatalf("tmuxArgs mutated input args: %v", args)
	}
}
