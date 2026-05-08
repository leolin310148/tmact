package agents

import "testing"

func TestValidateBroadcastOptionsRequiresText(t *testing.T) {
	err := validateBroadcastOptions(BroadcastOptions{Agent: "sample"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateBroadcastOptionsRequiresSelector(t *testing.T) {
	err := validateBroadcastOptions(BroadcastOptions{Text: "status"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateBroadcastOptionsRejectsMultipleSelectors(t *testing.T) {
	err := validateBroadcastOptions(BroadcastOptions{
		Agent: "sample",
		All:   true,
		Text:  "status",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSelectBroadcastAgentsByRole(t *testing.T) {
	cfg := Config{
		Agents: []AgentConfig{
			{Name: "one", Target: "one:0.0", Role: "frontend"},
			{Name: "two", Target: "two:0.0", Role: "backend"},
			{Name: "three", Target: "three:0.0", Role: "frontend"},
		},
	}

	selected, err := selectBroadcastAgents(cfg, BroadcastOptions{Role: "frontend", Text: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 {
		t.Fatalf("selected = %d", len(selected))
	}
	if selected[0].Name != "one" || selected[1].Name != "three" {
		t.Fatalf("selected = %#v", selected)
	}
}

func TestBroadcastDryRunDoesNotRequireTmux(t *testing.T) {
	cfg := Config{
		Agents: []AgentConfig{{Name: "sample", Target: "missing:0.0"}},
	}

	report, err := Broadcast(cfg, BroadcastOptions{
		Agent: "sample",
		Text:  "status",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.DryRun {
		t.Fatal("expected dry run")
	}
	if len(report.Results) != 1 || report.Results[0].Status != "dry_run" {
		t.Fatalf("results = %#v", report.Results)
	}
}
