package workflow

import (
	"strings"
	"testing"
	"time"
)

func TestRunnerStartsStageWhenIdle(t *testing.T) {
	runner := newTestRunner()
	var sentKeys [][]string
	var sentText []string
	runner.sendKeys = func(_ string, keys []string) error {
		sentKeys = append(sentKeys, append([]string(nil), keys...))
		return nil
	}
	runner.pasteText = func(_ string, text string, enter bool) error {
		if !enter {
			t.Fatal("expected enter")
		}
		sentText = append(sentText, text)
		return nil
	}
	runner.capturePane = func(string, int) (string, error) {
		return "enter your prompt", nil
	}

	state := runState{lastChanged: testNow.Add(-time.Minute), nextCycleRun: testNow}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if !state.stageStarted {
		t.Fatal("stage should be started")
	}
	if len(sentKeys) != 1 || strings.Join(sentKeys[0], ",") != "C-u" {
		t.Fatalf("sent keys = %#v", sentKeys)
	}
	if len(sentText) != 1 || sentText[0] != "implement prompt" {
		t.Fatalf("sent text = %#v", sentText)
	}
}

func TestRunnerDryRunDoesNotSendStagePrompt(t *testing.T) {
	runner := newTestRunner()
	runner.options.DryRun = true
	runner.sendKeys = func(string, []string) error {
		t.Fatal("sendKeys should not run in dry-run")
		return nil
	}
	runner.pasteText = func(string, string, bool) error {
		t.Fatal("pasteText should not run in dry-run")
		return nil
	}
	runner.capturePane = func(string, int) (string, error) {
		return "enter your prompt", nil
	}

	state := runState{lastChanged: testNow.Add(-time.Minute), nextCycleRun: testNow}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if !state.stageStarted {
		t.Fatal("stage should be started")
	}
}

func TestRunnerAdvancesFromImplementToReview(t *testing.T) {
	runner := newTestRunner()
	runner.capturePane = func(string, int) (string, error) {
		return "commit hash abc123\nenter your prompt", nil
	}

	state := runState{
		stageIndex:   0,
		stageStarted: true,
		lastChanged:  testNow.Add(-time.Minute),
		lastHash:     hashText("commit hash abc123\nenter your prompt"),
		nextCycleRun: testNow,
	}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if state.stageStarted {
		t.Fatal("stage should no longer be active")
	}
	if state.stageIndex != 1 {
		t.Fatalf("stage index = %d", state.stageIndex)
	}
	if state.cyclesDone != 0 {
		t.Fatalf("cycles done = %d", state.cyclesDone)
	}
}

func TestRunnerCompletesCycleAfterReview(t *testing.T) {
	runner := newTestRunner()
	runner.capturePane = func(string, int) (string, error) {
		return "review complete\nenter your prompt", nil
	}

	state := runState{
		stageIndex:   1,
		stageStarted: true,
		lastChanged:  testNow.Add(-time.Minute),
		lastHash:     hashText("review complete\nenter your prompt"),
		nextCycleRun: testNow,
	}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if state.stageStarted {
		t.Fatal("stage should no longer be active")
	}
	if state.stageIndex != 0 {
		t.Fatalf("stage index = %d", state.stageIndex)
	}
	if state.cyclesDone != 1 {
		t.Fatalf("cycles done = %d", state.cyclesDone)
	}
	if !state.nextCycleRun.Equal(testNow.Add(20 * time.Minute)) {
		t.Fatalf("next cycle run = %s", state.nextCycleRun)
	}
}

func TestRunnerDoesNotStartWhenPaneWorking(t *testing.T) {
	runner := newTestRunner()
	runner.capturePane = func(string, int) (string, error) {
		return "Working\nEsc to interrupt", nil
	}
	runner.sendKeys = func(string, []string) error {
		t.Fatal("sendKeys should not run while pane is working")
		return nil
	}

	state := runState{lastChanged: testNow.Add(-time.Minute), nextCycleRun: testNow}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if state.stageStarted {
		t.Fatal("stage should not be started")
	}
}

func TestRunnerStopsOnPermissionPrompt(t *testing.T) {
	runner := newTestRunner()
	runner.cfg.StopOnPermissionPrompt = true
	runner.capturePane = func(string, int) (string, error) {
		return permissionPrompt(), nil
	}
	runner.sendKeys = func(string, []string) error {
		t.Fatal("sendKeys should not run on permission prompt")
		return nil
	}

	state := runState{lastChanged: testNow.Add(-time.Minute), nextCycleRun: testNow}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if state.stageStarted {
		t.Fatal("stage should not be started")
	}
}

var testNow = time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

func newTestRunner() *Runner {
	cfg := Config{
		Target:     "sample:0.0",
		IdleAfter:  Duration{Duration: time.Second},
		CycleEvery: Duration{Duration: 20 * time.Minute},
		Stages: []StageConfig{
			{
				Name:   "implement",
				Prompt: "implement prompt",
				CompleteWhen: CompleteWhenConfig{
					Idle:                true,
					RecentOutputMatches: []string{"(?i)commit hash|blocked|done"},
				},
			},
			{
				Name:   "review",
				Prompt: "review prompt",
				CompleteWhen: CompleteWhenConfig{
					Idle:                true,
					RecentOutputMatches: []string{"(?i)review complete|blocked|done"},
				},
			},
		},
	}
	runner := NewRunner(cfg, Options{})
	runner.now = func() time.Time { return testNow }
	runner.sleep = func(time.Duration) {}
	runner.sendKeys = func(string, []string) error { return nil }
	runner.pasteText = func(string, string, bool) error { return nil }
	return runner
}

func permissionPrompt() string {
	return `
╭────────────────────────────────────────────────────────╮
│ Allow directory access                                 │
│ This action may read or write paths outside the list.  │
│ /tmp/project                                           │
│ Do you want to allow this?                             │
│ ❯ 1. Yes, allow this session                           │
│   2. No                                                │
╰────────────────────────────────────────────────────────╯
`
}
