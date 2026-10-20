package modelroute

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
)

func quotaSnapshot(now time.Time, claudeSession, claudeWeekly, codexWeekly float64) agentusage.Snapshot {
	reset := now.Add(48 * time.Hour)
	return agentusage.Snapshot{GeneratedAt: now, Providers: []agentusage.ProviderUsage{
		{Provider: "claude", Account: "private-account", Windows: []agentusage.RateWindow{{Name: "session", UsedPercent: claudeSession, ResetsAt: &reset}, {Name: "weekly", UsedPercent: claudeWeekly, ResetsAt: &reset}}},
		{Provider: "codex", Windows: []agentusage.RateWindow{{Name: "weekly", UsedPercent: codexWeekly, ResetsAt: &reset}}},
	}}
}

func TestQuotaEligibility(t *testing.T) {
	now := time.Now()
	for _, tt := range []struct {
		name                  string
		session, cw, xw       float64
		wantClaude, wantCodex bool
	}{
		{"both healthy", 5, 10, 20, true, true},
		{"Claude weekly generous but five-hour exhausted", 99, 5, 20, false, true},
		{"reserve boundary", 80, 5, 20, false, true},
		{"above reserve", 79, 5, 20, true, true},
		{"Claude weekly empty", 5, 100, 20, false, true},
		{"Codex weekly empty", 5, 10, 100, true, false},
		{"both empty", 100, 100, 100, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			in, err := Prepare("task", "", "", "", quotaSnapshot(now, tt.session, tt.cw, tt.xw), now)
			if tt.wantClaude || tt.wantCodex {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("expected unavailable error")
			}
			if in.Quotas[0].Eligible != tt.wantClaude || in.Quotas[1].Eligible != tt.wantCodex {
				t.Fatalf("quotas=%+v", in.Quotas)
			}
			if in.Quotas[1].SessionRemaining != nil {
				t.Fatal("Codex must not require five-hour data")
			}
			for _, c := range in.Candidates {
				if c.Agent == "claude" && !tt.wantClaude || c.Agent == "codex" && !tt.wantCodex {
					t.Fatal("ineligible candidate", c)
				}
			}
		})
	}
}

func TestQuotaFreshnessAndPrivacy(t *testing.T) {
	now := time.Now()
	snapshot := quotaSnapshot(now, 1, 2, 3)
	in, err := Prepare("task", "", "", "", snapshot, now)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(in)
	if strings.Contains(string(b), "private-account") {
		t.Fatal("account leaked")
	}
	if *in.Quotas[1].WeeklyRemainingPerDay != 48.5 {
		t.Fatal("incorrect relative weekly allowance")
	}
	for _, mode := range []string{"stale", "error", "old", "missing-weekly", "expired-window"} {
		t.Run(mode, func(t *testing.T) {
			s := quotaSnapshot(now, 1, 2, 3)
			switch mode {
			case "stale":
				s.Providers[1].Stale = true
			case "error":
				s.Providers[1].Error = "private provider body"
			case "old":
				s.GeneratedAt = now.Add(-11 * time.Minute)
			case "missing-weekly":
				s.Providers[1].Windows = nil
			case "expired-window":
				past := now.Add(-time.Minute)
				s.Providers[1].Windows[0].ResetsAt = &past
			}
			in, _ := Prepare("task", "", "", "", s, now)
			if in.Quotas[1].Eligible {
				t.Fatal("unavailable Codex was eligible")
			}
			b, _ := json.Marshal(in)
			if strings.Contains(string(b), "private provider body") {
				t.Fatal("provider body leaked")
			}
		})
	}
}

func TestExplicitConstraints(t *testing.T) {
	now := time.Now()
	s := quotaSnapshot(now, 1, 2, 3)
	in, err := Prepare("task", "codex", "gpt-6.1-sol", "high", s, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(in.Candidates) != 1 || in.Candidates[0].Effort != "high" {
		t.Fatal(in.Candidates)
	}
	in, err = Prepare("task", "", "", "ultra", s, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range in.Candidates {
		if c.Agent != "codex" {
			t.Fatal("Claude cannot use ultra")
		}
	}
}

func TestDecisionResponseValidation(t *testing.T) {
	profiles := map[string]Candidate{"profile_0": {Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium"}}
	for _, tt := range []struct {
		name, raw string
		ok        bool
	}{
		{"valid", `{"answers":{"route":{"choice":"profile_0"}},"confidences":{"route":0.9},"gate_passed":true}`, true},
		{"low confidence", `{"answers":{"route":{"choice":"profile_0"}},"confidences":{"route":0.2},"gate_passed":true}`, true},
		{"unknown profile", `{"answers":{"route":{"choice":"shell-injection"}},"confidences":{"route":1},"gate_passed":true}`, false},
		{"defer", `{"answers":{"route":{"choice":"defer"}},"confidences":{"route":1},"gate_passed":true}`, false},
		{"missing gate", `{"answers":{"route":{"choice":"profile_0"}},"confidences":{"route":1}}`, true},
		{"gate false", `{"answers":{"route":{"choice":"profile_0"}},"confidences":{"route":1},"gate_passed":false}`, true},
		{"zero confidence", `{"answers":{"route":{"choice":"profile_0"}},"confidences":{"route":0},"gate_passed":null}`, true},
		{"missing confidence", `{"answers":{"route":{"choice":"profile_0"}}}`, true},
		{"malformed", `not json`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSelection(Input{}, profiles, []byte(tt.raw))
			if (err == nil) != tt.ok {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestChooseExecutesDecideWithPrivateInput(t *testing.T) {
	t.Setenv("TMACT_ROUTING_LOG", "off")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.json")
	script := `#!/bin/sh
[ "$JEV_API_KEY" = "unit-test-key" ] || exit 9
[ "$DECIDE_USAGE_LOG" = "off" ] || exit 9
[ "$1" = "-config" ] || exit 9
[ "$2" = "none" ] || exit 9
for arg in "$@"; do
 [ "$arg" != "--min-confidence" ] || exit 9
done
cat > "$ROUTING_TEST_INPUT"
printf '%s\n' '{"model":"fake-decide","answers":{"route":{"choice":"profile_0"}},"confidences":{"route":0.9},"gate_passed":true}'
`
	if err := os.WriteFile(filepath.Join(dir, "decide"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("JEV_API_KEY", "unit-test-key")
	t.Setenv("DECIDE_USAGE_LOG", "on")
	t.Setenv("DECIDE_CLI_CONFIG", "none")
	t.Setenv("ROUTING_TEST_INPUT", inputPath)
	now := time.Now()
	in, err := Prepare("task", "codex", "", "", quotaSnapshot(now, 1, 2, 3), now)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := Choose(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Agent != "codex" || selection.DecisionModel != "fake-decide" {
		t.Fatal(selection)
	}
	b, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "unit-test-key") || strings.Contains(string(b), "private-account") {
		t.Fatal("secret/account in decision input")
	}
}

func TestTrustedConfigRejectsRepositoryRelativeConfig(t *testing.T) {
	t.Setenv("DECIDE_CLI_CONFIG", "./decide.yaml")
	if _, err := trustedConfig(); err == nil {
		t.Fatal("relative config must be rejected")
	}
}

func TestChooseAcceptsLowConfidenceDespiteExternalGate(t *testing.T) {
	t.Setenv("TMACT_ROUTING_LOG", "off")
	dir := t.TempDir()
	script := `#!/bin/sh
cat >/dev/null
printf '%s\n' '{"answers":{"route":{"choice":"profile_0"}},"confidences":{"route":0.32},"gate_passed":false}'
exit 3
`
	if err := os.WriteFile(filepath.Join(dir, "decide"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DECIDE_CLI_CONFIG", "none")
	selection, err := Choose(context.Background(), Input{Candidates: []Candidate{{Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium"}}})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Model != "gpt-6.1-sol" || selection.Confidence != 0.32 {
		t.Fatalf("selection=%+v", selection)
	}
}

func TestModelWeeklyCeilingsAndEffortCompatibility(t *testing.T) {
	now := time.Now()
	s := quotaSnapshot(now, 1, 2, 3)
	s.Providers[0].Windows = append(s.Providers[0].Windows, agentusage.RateWindow{Name: "weekly_opus", UsedPercent: 100})
	in, err := Prepare("task", "", "", "", s, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range in.Candidates {
		if c.Model == "claude-opus-5-5" {
			t.Fatal("exhausted model-specific allowance ignored")
		}
	}
	in, err = Prepare("task", "codex", "", "ultra", s, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range in.Candidates {
		if c.Model == "gpt-6-luna" {
			t.Fatal("Luna does not support ultra")
		}
	}
}
