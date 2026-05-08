package agents

import "testing"

func TestSelectedAgentsReturnsNamedAgent(t *testing.T) {
	cfg := Config{
		Agents: []AgentConfig{
			{Name: "one", Target: "one:0.0"},
			{Name: "two", Target: "two:0.0"},
		},
	}

	agents, err := selectedAgents(cfg, "two")
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Name != "two" {
		t.Fatalf("agents = %#v", agents)
	}
}

func TestSelectedAgentsRejectsUnknownAgent(t *testing.T) {
	cfg := Config{Agents: []AgentConfig{{Name: "one", Target: "one:0.0"}}}

	_, err := selectedAgents(cfg, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLastMeaningfulLinesReturnsTail(t *testing.T) {
	got := LastMeaningfulLines("\none\n\ntwo\nthree\n", 2)
	want := []string{"two", "three"}
	if len(got) != len(want) {
		t.Fatalf("lines = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q", i, got[i])
		}
	}
}

func TestChangedFilesParsesGitShortStatus(t *testing.T) {
	got := changedFiles("## main...origin/main\n M a.go\n?? docs/new.md\nR  old.go -> new.go")
	want := []string{"a.go", "docs/new.md", "old.go -> new.go"}
	if len(got) != len(want) {
		t.Fatalf("files = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("file %d = %q", i, got[i])
		}
	}
}

func TestRecommendNextActionForDirtyIdleAgent(t *testing.T) {
	action := RecommendNextAction(AgentSummary{
		State: StateIdle,
		Git:   &GitSummary{Dirty: true},
	})
	if action != "review dirty worktree and ask agent to test or commit" {
		t.Fatalf("action = %q", action)
	}
}
