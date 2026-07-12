package agents

import (
	"testing"

	"github.com/leolin310148/tmact/internal/tmux"
)

func TestBuildPanelReportPlansSessionAndWindows(t *testing.T) {
	cfg := Config{
		Agents: []AgentConfig{
			{Name: "main", Session: "sample-team", Window: "main", Type: "codex"},
			{Name: "claude", Session: "sample-team", Window: "claude", Type: "claude"},
			{Name: "gemini", Session: "sample-team", Window: "gemini", Type: "gemini"},
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
	if got := report.Operations[2].Command; len(got) != 1 || got[0] != "gemini" {
		t.Fatalf("gemini command = %#v", got)
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

func TestBuildPanelReportPlansExactFolderTrust(t *testing.T) {
	cfg := Config{Agents: []AgentConfig{{Name: "worker", Session: "agents", Window: "worker", Repo: "/repo", Type: "codex", TrustFolder: true}}}
	report, err := buildPanelReport(cfg, PanelOptions{}, tmux.Layout{Sessions: map[string]bool{}, Windows: map[string]map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	op := report.Operations[0]
	if !op.TrustFolder || op.Launcher != "codex" || op.Repo != "/repo" {
		t.Fatalf("operation = %#v", op)
	}
}

func TestBuildPanelReportTrustOverrideRequiresRepo(t *testing.T) {
	cfg := Config{Agents: []AgentConfig{{Name: "worker", Session: "agents", Window: "worker", Type: "claude"}}}
	_, err := buildPanelReport(cfg, PanelOptions{TrustFolders: true}, tmux.Layout{Sessions: map[string]bool{}, Windows: map[string]map[string]bool{}})
	if err == nil {
		t.Fatal("expected exact repo error")
	}
}

func TestLaunchCommandRejectsUnsupportedLauncher(t *testing.T) {
	_, err := launchCommand(AgentConfig{Name: "sample", Launcher: "bash"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestConfigRejectsFolderTrustWithoutExactRepo(t *testing.T) {
	err := validateConfig(Config{Agents: []AgentConfig{{Name: "worker", Target: "work:0.0", Type: "codex", TrustFolder: true}}})
	if err == nil {
		t.Fatal("expected repo requirement")
	}
}

func TestConfigRejectsFolderTrustForNonClaudeCodex(t *testing.T) {
	err := validateConfig(Config{Agents: []AgentConfig{{Name: "worker", Target: "work:0.0", Repo: "/repo", Type: "gemini", TrustFolder: true}}})
	if err == nil {
		t.Fatal("expected launcher restriction")
	}
}
