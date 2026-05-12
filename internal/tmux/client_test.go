package tmux

import "testing"

func TestParsePanes(t *testing.T) {
	raw := "z_sample-project|0|codex-aarch64-a|0|%14|70365|codex-aarch64-a|/Users/example/workspace|1|0|1\n"

	panes, err := ParsePanes(raw)
	if err != nil {
		t.Fatalf("ParsePanes returned error: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("panes len = %d", len(panes))
	}
	pane := panes[0]
	if pane.Session != "z_sample-project" {
		t.Fatalf("session = %q", pane.Session)
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
