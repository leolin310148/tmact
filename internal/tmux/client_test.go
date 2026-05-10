package tmux

import "testing"

func TestParsePanes(t *testing.T) {
	raw := "z_sample-project\t0\tcodex-aarch64-a\t0\t%14\t70365\tcodex-aarch64-a\t/Users/example/workspace\t1\t0\n"

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
