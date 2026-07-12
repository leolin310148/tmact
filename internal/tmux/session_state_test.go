package tmux

import "testing"

func TestParseSessionState(t *testing.T) {
	raw := "work\t1\teditor\tb25f,140x40,0,0{70x40,0,0,1,69x40,71,0,2}\t140\t40\t1\t0\t/Users/puni/w/work\t1\n"
	panes, err := ParseSessionState(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 1 {
		t.Fatalf("len = %d, want 1", len(panes))
	}
	got := panes[0]
	if got.Session != "work" || got.WindowIndex != 1 || got.WindowName != "editor" {
		t.Fatalf("identity = %+v", got)
	}
	if got.CurrentPath != "/Users/puni/w/work" || !got.WindowActive || !got.Active {
		t.Fatalf("state = %+v", got)
	}
	if got.WindowWidth != 140 || got.WindowHeight != 40 {
		t.Fatalf("window size = %dx%d", got.WindowWidth, got.WindowHeight)
	}
}

func TestParseSessionStateRejectsMalformedRows(t *testing.T) {
	for _, raw := range []string{
		"too\tfew\tfields\n",
		"work\tbad\teditor\tlayout\t80\t24\t1\t0\t/tmp\t1\n",
		"work\t1\teditor\tlayout\tbad\t24\t1\t0\t/tmp\t1\n",
		"work\t1\teditor\tlayout\t80\tbad\t1\t0\t/tmp\t1\n",
		"work\t1\teditor\tlayout\t80\t24\t1\tbad\t/tmp\t1\n",
	} {
		if _, err := ParseSessionState(raw); err == nil {
			t.Fatalf("ParseSessionState(%q) succeeded", raw)
		}
	}
}

func TestRestoreTargetsUseExactSessionMatch(t *testing.T) {
	if got := exactSessionTarget("work"); got != "=work" {
		t.Fatalf("session target = %q", got)
	}
	if got := windowTarget("work", 2); got != "=work:2" {
		t.Fatalf("window target = %q", got)
	}
	if got := paneTarget("work", 2, 3); got != "=work:2.3" {
		t.Fatalf("pane target = %q", got)
	}
}
