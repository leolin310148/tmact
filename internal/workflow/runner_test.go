package workflow

import (
	"context"
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

func TestRunnerUsesStageTarget(t *testing.T) {
	runner := newTestRunner()
	runner.cfg.Stages[0].Target = "planner:0.0"
	var capturedTarget string
	var sentTarget string
	runner.capturePane = func(target string, _ int) (string, error) {
		capturedTarget = target
		return "enter your prompt", nil
	}
	runner.sendKeys = func(target string, keys []string) error {
		sentTarget = target
		return nil
	}

	state := runState{lastChanged: testNow.Add(-time.Minute), nextCycleRun: testNow}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if capturedTarget != "planner:0.0" {
		t.Fatalf("captured target = %q", capturedTarget)
	}
	if sentTarget != "planner:0.0" {
		t.Fatalf("sent target = %q", sentTarget)
	}
}

func TestRunnerStartsFromNamedStage(t *testing.T) {
	runner := newTestRunner()
	runner.options.StartStage = "review"
	runner.options.Once = true
	runner.options.AssumeIdleOnStart = true
	var sentText []string
	runner.pasteText = func(_ string, text string, enter bool) error {
		sentText = append(sentText, text)
		return nil
	}
	runner.capturePane = func(string, int) (string, error) {
		return "enter your prompt", nil
	}

	if err := runner.Run(testContext()); err != nil {
		t.Fatal(err)
	}
	if len(sentText) != 1 || sentText[0] != "review prompt" {
		t.Fatalf("sent text = %#v", sentText)
	}
}

func TestRunnerRejectsUnknownStartStage(t *testing.T) {
	runner := newTestRunner()
	runner.options.StartStage = "missing"
	runner.options.Once = true
	runner.capturePane = func(string, int) (string, error) {
		t.Fatal("capturePane should not run")
		return "", nil
	}

	if err := runner.Run(testContext()); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunnerRepeatsStageBeforeAdvancing(t *testing.T) {
	runner := newTestRunner()
	runner.cfg.Stages[0].Repeat = 3
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
	if state.stageIndex != 0 {
		t.Fatalf("stage index = %d", state.stageIndex)
	}
	if state.stageRepeatsDone != 1 {
		t.Fatalf("stage repeats done = %d", state.stageRepeatsDone)
	}
}

func TestRunnerWaitsStageEveryBetweenRepeats(t *testing.T) {
	runner := newTestRunner()
	runner.cfg.StageEvery = Duration{Duration: 20 * time.Minute}
	runner.cfg.Stages[0].Repeat = 2
	runner.capturePane = func(string, int) (string, error) {
		return "commit hash abc123\nenter your prompt", nil
	}

	state := runState{
		stageIndex:   0,
		stageStarted: true,
		lastChanged:  testNow.Add(-time.Minute),
		lastHash:     hashText("commit hash abc123\nenter your prompt"),
		nextStageRun: testNow.Add(20 * time.Minute),
		nextCycleRun: testNow,
	}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if !state.nextStageRun.Equal(testNow.Add(20 * time.Minute)) {
		t.Fatalf("next stage run = %s", state.nextStageRun)
	}

	runner.capturePane = func(string, int) (string, error) {
		return "enter your prompt", nil
	}
	runner.sendKeys = func(string, []string) error {
		t.Fatal("sendKeys should not run before stage_every elapses")
		return nil
	}
	if err := runner.runOnce(testNow.Add(10*time.Minute), &state); err != nil {
		t.Fatal(err)
	}
	if state.stageStarted {
		t.Fatal("stage should not restart before stage_every elapses")
	}

	var sentText []string
	runner.sendKeys = func(string, []string) error { return nil }
	runner.pasteText = func(_ string, text string, enter bool) error {
		sentText = append(sentText, text)
		return nil
	}
	if err := runner.runOnce(testNow.Add(20*time.Minute), &state); err != nil {
		t.Fatal(err)
	}
	if !state.stageStarted {
		t.Fatal("stage should restart after stage_every elapses")
	}
	if len(sentText) != 1 || sentText[0] != "implement prompt" {
		t.Fatalf("sent text = %#v", sentText)
	}
}

func TestRunnerWaitsStageEveryBetweenStages(t *testing.T) {
	runner := newTestRunner()
	runner.cfg.StageEvery = Duration{Duration: 20 * time.Minute}
	runner.capturePane = func(string, int) (string, error) {
		return "commit hash abc123\nenter your prompt", nil
	}

	state := runState{
		stageIndex:   0,
		stageStarted: true,
		lastChanged:  testNow.Add(-time.Minute),
		lastHash:     hashText("commit hash abc123\nenter your prompt"),
		nextStageRun: testNow.Add(20 * time.Minute),
		nextCycleRun: testNow,
	}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if state.stageIndex != 1 {
		t.Fatalf("stage index = %d", state.stageIndex)
	}
	if !state.nextStageRun.Equal(testNow.Add(20 * time.Minute)) {
		t.Fatalf("next stage run = %s", state.nextStageRun)
	}

	runner.capturePane = func(string, int) (string, error) {
		return "enter your prompt", nil
	}
	runner.sendKeys = func(string, []string) error {
		t.Fatal("sendKeys should not run before stage_every elapses")
		return nil
	}
	if err := runner.runOnce(testNow.Add(10*time.Minute), &state); err != nil {
		t.Fatal(err)
	}
	if state.stageStarted {
		t.Fatal("review stage should not start before stage_every elapses")
	}
}

func TestRunnerSchedulesStageEveryFromStageStart(t *testing.T) {
	runner := newTestRunner()
	runner.cfg.StageEvery = Duration{Duration: 20 * time.Minute}
	runner.capturePane = func(string, int) (string, error) {
		return "enter your prompt", nil
	}

	state := runState{
		lastChanged:  testNow.Add(-time.Minute),
		nextCycleRun: testNow,
	}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if !state.stageStarted {
		t.Fatal("stage should be started")
	}
	if !state.nextStageRun.Equal(testNow.Add(20 * time.Minute)) {
		t.Fatalf("next stage run = %s", state.nextStageRun)
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

func TestRunnerStopsAfterMaxCycles(t *testing.T) {
	runner := newTestRunner()
	runner.cfg.MaxCycles = 1
	runner.capturePane = func(string, int) (string, error) {
		return "enter your prompt", nil
	}
	runner.sendKeys = func(string, []string) error {
		t.Fatal("sendKeys should not run after max cycles")
		return nil
	}

	state := runState{
		cyclesDone:   1,
		lastChanged:  testNow.Add(-time.Minute),
		nextCycleRun: testNow,
	}
	if err := runner.runOnce(testNow, &state); err != nil {
		t.Fatal(err)
	}
	if !state.stopped {
		t.Fatal("runner should be stopped")
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
	if !state.stopped {
		t.Fatal("runner should be stopped")
	}
}

var testNow = time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

func newTestRunner() *Runner {
	cfg := Config{
		Target:       "sample:0.0",
		PollInterval: Duration{Duration: time.Second},
		IdleAfter:    Duration{Duration: time.Second},
		CycleEvery:   Duration{Duration: 20 * time.Minute},
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

func testContext() context.Context {
	return context.Background()
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
