package agents

import (
	"testing"

	"tmact/internal/tmux"
)

func TestBuildPanelReportPlansSessionAndWindows(t *testing.T) {
	cfg := Config{
		Agents: []AgentConfig{
			{Name: "main", Session: "IDLL", Window: "main", Type: "codex"},
			{Name: "claude", Session: "IDLL", Window: "claude", Type: "claude"},
			{Name: "copilot", Session: "IDLL", Window: "copilot", Type: "copilot", AllowAllTools: true},
		},
	}

	report, err := buildPanelReport(cfg, PanelOptions{}, tmux.Layout{
		Sessions: map[string]bool{},
		Windows:  map[string]map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}

	wantActions := []string{"new_session", "new_window", "new_window"}
	for i, want := range wantActions {
		if report.Operations[i].Action != want {
			t.Fatalf("op %d action = %q", i, report.Operations[i].Action)
		}
	}
	if got := report.Operations[2].Command; len(got) != 2 || got[0] != "copilot" || got[1] != "--allow-all-tools" {
		t.Fatalf("copilot command = %#v", got)
	}
}

func TestBuildPanelReportUsesOverrideSession(t *testing.T) {
	cfg := Config{Agents: []AgentConfig{{Name: "worker", Target: "old:0.0", Window: "worker"}}}

	report, err := buildPanelReport(cfg, PanelOptions{Session: "agents"}, tmux.Layout{
		Sessions: map[string]bool{"agents": true},
		Windows:  map[string]map[string]bool{"agents": {}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Operations[0].Session != "agents" {
		t.Fatalf("session = %q", report.Operations[0].Session)
	}
	if report.Operations[0].Action != "new_window" {
		t.Fatalf("action = %q", report.Operations[0].Action)
	}
}

func TestLaunchCommandRejectsUnsupportedLauncher(t *testing.T) {
	_, err := launchCommand(AgentConfig{Name: "sample", Launcher: "bash"})
	if err == nil {
		t.Fatal("expected error")
	}
}
