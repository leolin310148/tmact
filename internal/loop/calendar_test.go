package loop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func taipeiWorkdays() *CalendarConfig {
	return &CalendarConfig{
		Timezone: "Asia/Taipei",
		Weekdays: []string{"mon", "tue", "wed", "thu", "fri"},
		Windows:  []CalendarWindow{{Start: "09:30", End: "18:00"}},
	}
}

func mustTaipei(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestCalendarWindowBoundaries(t *testing.T) {
	cal, err := CompileCalendar(taipeiWorkdays())
	if err != nil {
		t.Fatal(err)
	}
	tpe := mustTaipei(t)
	// 2026-10-06 is a Tuesday; 2026-10-10/11 are Saturday/Sunday.
	for _, tt := range []struct {
		name string
		at   time.Time
		want bool
	}{
		{"tue 09:29 closed", time.Date(2026, 10, 6, 9, 29, 0, 0, tpe), false},
		{"tue 09:29:59 closed", time.Date(2026, 10, 6, 9, 29, 59, 0, tpe), false},
		{"tue 09:30 open (inclusive)", time.Date(2026, 10, 6, 9, 30, 0, 0, tpe), true},
		{"tue 17:59 open", time.Date(2026, 10, 6, 17, 59, 0, 0, tpe), true},
		{"tue 17:59:59 open", time.Date(2026, 10, 6, 17, 59, 59, 0, tpe), true},
		{"tue 18:00 closed (exclusive)", time.Date(2026, 10, 6, 18, 0, 0, 0, tpe), false},
		{"fri 12:00 open", time.Date(2026, 10, 9, 12, 0, 0, 0, tpe), true},
		{"sat 12:00 closed", time.Date(2026, 10, 10, 12, 0, 0, 0, tpe), false},
		{"sun 12:00 closed", time.Date(2026, 10, 11, 12, 0, 0, 0, tpe), false},
		// UTC instants are judged in Asia/Taipei (UTC+8), never the host zone.
		{"utc 01:29 tue = 09:29 tpe", time.Date(2026, 10, 6, 1, 29, 0, 0, time.UTC), false},
		{"utc 01:30 tue = 09:30 tpe", time.Date(2026, 10, 6, 1, 30, 0, 0, time.UTC), true},
		{"utc 09:59 tue = 17:59 tpe", time.Date(2026, 10, 6, 9, 59, 0, 0, time.UTC), true},
		{"utc 10:00 tue = 18:00 tpe", time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC), false},
		{"utc mon 23:00 = tue 07:00 tpe", time.Date(2026, 10, 5, 23, 0, 0, 0, time.UTC), false},
		{"utc fri 03:00 = fri 11:00 tpe", time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC), true},
		{"utc fri 16:30 = sat 00:30 tpe", time.Date(2026, 10, 9, 16, 30, 0, 0, time.UTC), false},
		{"utc sun 02:00 = sun 10:00 tpe", time.Date(2026, 10, 11, 2, 0, 0, 0, time.UTC), false},
	} {
		if got := cal.Open(tt.at); got != tt.want {
			t.Errorf("%s: Open(%s) = %v, want %v", tt.name, tt.at.Format(time.RFC3339), got, tt.want)
		}
	}
}

func TestCalendarNextOpen(t *testing.T) {
	cal, err := CompileCalendar(taipeiWorkdays())
	if err != nil {
		t.Fatal(err)
	}
	tpe := mustTaipei(t)
	for _, tt := range []struct {
		name string
		at   time.Time
		want time.Time
	}{
		{"before open same day", time.Date(2026, 10, 6, 8, 0, 0, 0, tpe), time.Date(2026, 10, 6, 9, 30, 0, 0, tpe)},
		{"already open", time.Date(2026, 10, 6, 10, 0, 0, 0, tpe), time.Date(2026, 10, 6, 10, 0, 0, 0, tpe)},
		{"tue close rolls to wed", time.Date(2026, 10, 6, 18, 0, 0, 0, tpe), time.Date(2026, 10, 7, 9, 30, 0, 0, tpe)},
		{"fri close rolls to mon", time.Date(2026, 10, 9, 18, 0, 0, 0, tpe), time.Date(2026, 10, 12, 9, 30, 0, 0, tpe)},
		{"saturday rolls to mon", time.Date(2026, 10, 10, 11, 0, 0, 0, tpe), time.Date(2026, 10, 12, 9, 30, 0, 0, tpe)},
		{"utc input", time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC), time.Date(2026, 10, 7, 9, 30, 0, 0, tpe)},
	} {
		if got := cal.NextOpen(tt.at); !got.Equal(tt.want) {
			t.Errorf("%s: NextOpen(%s) = %s, want %s", tt.name, tt.at.Format(time.RFC3339), got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
		}
	}
	until, ok := cal.OpenUntil(time.Date(2026, 10, 6, 10, 0, 0, 0, tpe))
	if !ok || !until.Equal(time.Date(2026, 10, 6, 18, 0, 0, 0, tpe)) {
		t.Fatalf("OpenUntil = %s, %v", until, ok)
	}
}

func TestCalendarMultipleWindowsAndMidnightEnd(t *testing.T) {
	cal, err := CompileCalendar(&CalendarConfig{
		Timezone: "UTC",
		Weekdays: []string{"Monday", "SAT"},
		Windows:  []CalendarWindow{{Start: "13:00", End: "24:00"}, {Start: "08:00", End: "12:00"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		at   time.Time
		want bool
	}{
		{mon.Add(8 * time.Hour), true},
		{mon.Add(12 * time.Hour), false},
		{mon.Add(13 * time.Hour), true},
		{mon.Add(24*time.Hour - time.Second), true},
		{mon.Add(24 * time.Hour), false},
	} {
		if got := cal.Open(tt.at); got != tt.want {
			t.Errorf("Open(%s) = %v, want %v", tt.at.Format(time.RFC3339), got, tt.want)
		}
	}
	if got := cal.NextOpen(mon.Add(12 * time.Hour)); !got.Equal(mon.Add(13 * time.Hour)) {
		t.Errorf("NextOpen between windows = %s", got)
	}
	if got := cal.NextOpen(mon.Add(24 * time.Hour)); !got.Equal(mon.Add(5*24*time.Hour + 8*time.Hour)) {
		t.Errorf("NextOpen after monday = %s", got)
	}
}

func writeLoopConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "loop.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const calendarLoopBase = `target: demo:0.0
actions:
  - name: nudge
    type: send_text
    text: go
    every: 15m
`

func TestLoadConfigParsesCalendar(t *testing.T) {
	cfg, err := LoadConfig(writeLoopConfig(t, calendarLoopBase+`calendar:
  timezone: Asia/Taipei
  weekdays: [mon, tue, wed, thu, fri]
  windows:
    - start: "09:30"
      end: "18:00"
`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Calendar == nil || cfg.Calendar.Timezone != "Asia/Taipei" || len(cfg.Calendar.Weekdays) != 5 || cfg.Calendar.Windows[0] != (CalendarWindow{Start: "09:30", End: "18:00"}) {
		t.Fatalf("calendar = %#v", cfg.Calendar)
	}
}

func TestLoadConfigWithoutCalendarIsUnrestricted(t *testing.T) {
	cfg, err := LoadConfig(writeLoopConfig(t, calendarLoopBase))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Calendar != nil {
		t.Fatalf("calendar = %#v", cfg.Calendar)
	}
}

func TestLoadConfigRejectsInvalidCalendar(t *testing.T) {
	for _, tt := range []struct {
		name, calendar, want string
	}{
		{"unknown zone", "calendar:\n  timezone: Asia/Taipe\n  weekdays: [mon]\n  windows: [{start: \"09:30\", end: \"18:00\"}]\n", "invalid timezone"},
		{"missing zone", "calendar:\n  weekdays: [mon]\n  windows: [{start: \"09:30\", end: \"18:00\"}]\n", "timezone is required"},
		{"local zone", "calendar:\n  timezone: Local\n  weekdays: [mon]\n  windows: [{start: \"09:30\", end: \"18:00\"}]\n", "not Local"},
		{"bad weekday", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon, tues]\n  windows: [{start: \"09:30\", end: \"18:00\"}]\n", "invalid weekday"},
		{"duplicate weekday", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon, monday]\n  windows: [{start: \"09:30\", end: \"18:00\"}]\n", "duplicate weekday"},
		{"missing weekdays", "calendar:\n  timezone: Asia/Taipei\n  windows: [{start: \"09:30\", end: \"18:00\"}]\n", "weekdays is required"},
		{"missing windows", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon]\n", "at least one window"},
		{"single digit hour", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon]\n  windows: [{start: \"9:30\", end: \"18:00\"}]\n", "want HH:MM"},
		{"hour out of range", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon]\n  windows: [{start: \"09:30\", end: \"25:00\"}]\n", "want HH:MM"},
		{"minute out of range", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon]\n  windows: [{start: \"09:60\", end: \"18:00\"}]\n", "want HH:MM"},
		{"24:00 start", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon]\n  windows: [{start: \"24:00\", end: \"24:00\"}]\n", "only valid as a window end"},
		{"zero length", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon]\n  windows: [{start: \"09:30\", end: \"09:30\"}]\n", "must end after it starts"},
		{"reversed", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon]\n  windows: [{start: \"18:00\", end: \"09:30\"}]\n", "must end after it starts"},
		{"overlap", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon]\n  windows: [{start: \"09:00\", end: \"12:00\"}, {start: \"11:00\", end: \"13:00\"}]\n", "must not overlap"},
		{"misspelled calendar key", "calendar:\n  timezone: Asia/Taipei\n  weekday: [mon]\n  windows: [{start: \"09:30\", end: \"18:00\"}]\n", "unknown key \"weekday\""},
		{"misspelled window key", "calendar:\n  timezone: Asia/Taipei\n  weekdays: [mon]\n  windows: [{from: \"09:30\", end: \"18:00\"}]\n", "unknown key \"from\""},
		{"misspelled block", "calender:\n  timezone: Asia/Taipei\n", "under calendar:"},
		{"top-level timezone", "timezone: Asia/Taipei\n", "under calendar:"},
		{"empty block", "calendar: {}\n", "timezone is required"},
		{"scalar block", "calendar: workdays\n", "must be a mapping"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfig(writeLoopConfig(t, calendarLoopBase+tt.calendar))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadConfigRejectsCalendarInsideActionOrFlow(t *testing.T) {
	for _, body := range []string{
		"target: demo:0.0\nactions:\n  - name: a\n    type: send_text\n    text: go\n    calendar:\n      timezone: Asia/Taipei\n",
		"target: demo:0.0\nflows:\n  - name: f\n    weekdays: [mon]\n    steps:\n      - type: send_text\n        text: go\n",
		"target: demo:0.0\nflows:\n  - name: f\n    steps:\n      - type: send_text\n        text: go\n        calendar: {}\n",
	} {
		if _, err := LoadConfig(writeLoopConfig(t, body)); err == nil || !strings.Contains(err.Error(), "loop-wide top-level block") {
			t.Errorf("err = %v for:\n%s", err, body)
		}
	}
}

func TestRunnerRefusesInvalidCalendarInsteadOfRunningUnrestricted(t *testing.T) {
	runner := NewRunner(Config{
		Target:   "demo:0.0",
		Calendar: &CalendarConfig{Timezone: "Mars/Olympus", Weekdays: []string{"mon"}, Windows: []CalendarWindow{{Start: "09:30", End: "18:00"}}},
		Actions:  []ActionConfig{{Name: "nudge", Type: "send_text", Text: "go"}},
	}, Options{Once: true})
	runner.capturePane = func(string, int) (string, error) { t.Fatal("captured pane"); return "", nil }
	runner.sendText = func(string, string, bool) error { t.Fatal("sent input"); return nil }
	if err := runner.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid timezone") {
		t.Fatalf("err = %v", err)
	}
}

type sentInput struct {
	at   time.Time
	text string
}

// runCalendarClock drives the real Run loop with a fake clock that advances
// step after every pane capture (once per cycle), stopping once the clock
// passes end. Sends are recorded at the cycle time that triggered them.
func runCalendarClock(t *testing.T, cfg Config, start, end time.Time, step time.Duration) ([]sentInput, []event) {
	t.Helper()
	cfg.LogPath = filepath.Join(t.TempDir(), "loop.jsonl")
	cfg.PollInterval = Duration{time.Millisecond}
	cfg.LogSkippedActions = true
	cur := start
	inCycle := false
	runner := NewRunner(cfg, Options{
		Control: func() (string, error) {
			// Only stop between cycles so the last cycle at end completes.
			if !inCycle && cur.After(end) {
				return "stopped", nil
			}
			return "running", nil
		},
		Heartbeat: func(phase string) error {
			if phase == "sleeping" || strings.HasPrefix(phase, "waiting_") {
				inCycle = false
			}
			return nil
		},
	})
	runner.now = func() time.Time { return cur }
	runner.capturePane = func(string, int) (string, error) {
		inCycle = true
		defer func() { cur = cur.Add(step) }()
		return "❯ idle\n", nil
	}
	var sent []sentInput
	runner.sendText = func(_ string, text string, _ bool) error {
		// The capture earlier in this cycle already advanced the clock.
		sent = append(sent, sentInput{at: cur.Add(-step), text: text})
		return nil
	}
	runner.sendKeys = func(string, []string) error { t.Fatal("unexpected keys"); return nil }
	// Runs end by the fake clock (stop requested) or by a terminal loop stop
	// such as max_actions, which returns nil.
	if err := runner.Run(context.Background()); err != nil && !errors.Is(err, ErrStopRequested) {
		t.Fatalf("Run err = %v", err)
	}
	data, err := os.ReadFile(cfg.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	var events []event
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var e event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad log line %q: %v", line, err)
		}
		events = append(events, e)
	}
	return sent, events
}

func TestLoopCalendarGatesSendsAcrossDaysWithoutCatchup(t *testing.T) {
	tpe := mustTaipei(t)
	start := time.Date(2026, 10, 6, 17, 0, 0, 0, tpe) // Tuesday
	end := time.Date(2026, 10, 7, 10, 0, 0, 0, tpe)   // Wednesday
	sent, events := runCalendarClock(t, Config{
		Target:   "demo:0.0",
		Calendar: taipeiWorkdays(),
		Actions: []ActionConfig{{
			Name: "scan", Type: "send_text", Text: "scan", Every: Duration{15 * time.Minute},
			InitialDelay: Duration{15 * time.Minute}, OnlyWhenIdle: true, MaxRuns: 2016,
		}},
	}, start, end, time.Minute)

	want := []time.Time{
		time.Date(2026, 10, 6, 17, 15, 0, 0, tpe),
		time.Date(2026, 10, 6, 17, 30, 0, 0, tpe),
		time.Date(2026, 10, 6, 17, 45, 0, 0, tpe),
		// 18:00 Tue through 09:29 Wed: closed, nothing sent and nothing counted.
		time.Date(2026, 10, 7, 9, 30, 0, 0, tpe), // one resume run, no backlog burst
		time.Date(2026, 10, 7, 9, 45, 0, 0, tpe),
		time.Date(2026, 10, 7, 10, 0, 0, 0, tpe),
	}
	if len(sent) != len(want) {
		t.Fatalf("sent %d inputs, want %d: %#v", len(sent), len(want), sent)
	}
	for i := range want {
		if !sent[i].at.Equal(want[i]) {
			t.Errorf("send %d at %s, want %s", i, sent[i].at.In(tpe).Format(time.RFC3339), want[i].Format(time.RFC3339))
		}
	}

	var transitions []string
	for _, e := range events {
		switch e.Type {
		case "calendar":
			transitions = append(transitions, e.Status+" "+e.Reason)
		case "skip":
			t.Errorf("outside-window cycles must not log skip events: %#v", e)
		}
	}
	wantTransitions := []string{
		"open open_until=2026-10-06T18:00:00+08:00",
		"closed next_eligible=2026-10-07T09:30:00+08:00",
		"open open_until=2026-10-07T18:00:00+08:00",
	}
	if strings.Join(transitions, "|") != strings.Join(wantTransitions, "|") {
		t.Fatalf("calendar transitions = %q, want %q", transitions, wantTransitions)
	}
}

func TestLoopCalendarSkipsWeekendAndResumesMonday(t *testing.T) {
	tpe := mustTaipei(t)
	start := time.Date(2026, 10, 9, 17, 50, 0, 0, tpe) // Friday
	end := time.Date(2026, 10, 12, 9, 40, 0, 0, tpe)   // Monday
	sent, _ := runCalendarClock(t, Config{
		Target:   "demo:0.0",
		Calendar: taipeiWorkdays(),
		Actions: []ActionConfig{{
			Name: "scan", Type: "send_text", Text: "scan", Every: Duration{15 * time.Minute},
			InitialDelay: Duration{15 * time.Minute},
		}},
	}, start, end, 5*time.Minute)
	if len(sent) != 1 || !sent[0].at.Equal(time.Date(2026, 10, 12, 9, 30, 0, 0, tpe)) {
		t.Fatalf("sent = %v, want exactly one Monday 09:30 send", sent)
	}
}

func TestLoopCalendarOutsideWindowDoesNotConsumeMaxActions(t *testing.T) {
	tpe := mustTaipei(t)
	start := time.Date(2026, 10, 6, 17, 55, 0, 0, tpe)
	end := time.Date(2026, 10, 7, 9, 31, 0, 0, tpe)
	sent, events := runCalendarClock(t, Config{
		Target:     "demo:0.0",
		MaxActions: 2,
		Calendar:   taipeiWorkdays(),
		Actions:    []ActionConfig{{Name: "scan", Type: "send_text", Text: "scan", Every: Duration{time.Minute}}},
	}, start, end, time.Minute)
	// Inside the window the budget is spent normally: 17:55 and 17:56.
	if len(sent) != 2 || !sent[1].at.Equal(time.Date(2026, 10, 6, 17, 56, 0, 0, tpe)) {
		t.Fatalf("sent = %#v", sent)
	}
	last := events[len(events)-1]
	if last.Type != "stop" || last.Reason != "max_actions" {
		t.Fatalf("last event = %#v", last)
	}

	// Starting outside the window, hours of closed polling must leave the full
	// max_actions budget for the next window.
	sent, events = runCalendarClock(t, Config{
		Target:     "demo:0.0",
		MaxActions: 2,
		Calendar:   taipeiWorkdays(),
		Actions:    []ActionConfig{{Name: "scan", Type: "send_text", Text: "scan", Every: Duration{time.Minute}}},
	}, time.Date(2026, 10, 6, 18, 0, 0, 0, tpe), end.Add(5*time.Minute), time.Minute)
	if len(sent) != 2 || !sent[0].at.Equal(time.Date(2026, 10, 7, 9, 30, 0, 0, tpe)) || !sent[1].at.Equal(time.Date(2026, 10, 7, 9, 31, 0, 0, tpe)) {
		t.Fatalf("sent = %#v", sent)
	}
	if last := events[len(events)-1]; last.Type != "stop" || last.Reason != "max_actions" {
		t.Fatalf("last event = %#v", last)
	}
}

func TestLoopCalendarFlowStartedInWindowRunsAllSteps(t *testing.T) {
	tpe := mustTaipei(t)
	runner := NewRunner(Config{
		Target:       "demo:0.0",
		PollInterval: Duration{time.Second},
		Calendar:     taipeiWorkdays(),
		Flows: []FlowConfig{{Name: "flow", Steps: []ActionConfig{
			{Name: "one", Type: "send_text", Text: "one"},
			{Name: "two", Type: "send_text", Text: "two"},
		}}},
	}, Options{Once: true})
	runner.now = func() time.Time { return time.Date(2026, 10, 6, 17, 59, 59, 0, tpe) }
	runner.capturePane = func(string, int) (string, error) { return "❯ idle\n", nil }
	var sent []string
	runner.sendText = func(_ string, text string, _ bool) error { sent = append(sent, text); return nil }
	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(sent, ",") != "one,two" {
		t.Fatalf("sent = %v", sent)
	}
}

func TestLoopCalendarClosedOnceSendsNothing(t *testing.T) {
	tpe := mustTaipei(t)
	logPath := filepath.Join(t.TempDir(), "loop.jsonl")
	runner := NewRunner(Config{
		Target:       "demo:0.0",
		LogPath:      logPath,
		PollInterval: Duration{time.Second},
		Calendar:     taipeiWorkdays(),
		Actions:      []ActionConfig{{Name: "scan", Type: "send_text", Text: "scan"}},
	}, Options{Once: true})
	runner.now = func() time.Time { return time.Date(2026, 10, 10, 11, 0, 0, 0, tpe) } // Saturday
	runner.capturePane = func(string, int) (string, error) { return "❯ idle\n", nil }
	runner.sendText = func(string, string, bool) error { t.Fatal("sent input on Saturday"); return nil }
	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"status":"closed","reason":"next_eligible=2026-10-12T09:30:00+08:00"`) {
		t.Fatalf("log missing closed calendar event:\n%s", data)
	}
}

func TestLoopWithoutCalendarKeepsEveryBehavior(t *testing.T) {
	tpe := mustTaipei(t)
	start := time.Date(2026, 10, 11, 3, 0, 0, 0, tpe) // Sunday night
	sent, events := runCalendarClock(t, Config{
		Target:  "demo:0.0",
		Actions: []ActionConfig{{Name: "scan", Type: "send_text", Text: "scan", Every: Duration{15 * time.Minute}, InitialDelay: Duration{15 * time.Minute}}},
	}, start, start.Add(45*time.Minute), time.Minute)
	if len(sent) != 3 || !sent[0].at.Equal(start.Add(15*time.Minute)) || !sent[2].at.Equal(start.Add(45*time.Minute)) {
		t.Fatalf("sent = %#v", sent)
	}
	for _, e := range events {
		if e.Type == "calendar" {
			t.Fatalf("calendar event without calendar config: %#v", e)
		}
	}
}

func TestDryRunOncePreviewShiftsEligibleAtIntoCalendar(t *testing.T) {
	tpe := mustTaipei(t)
	logPath := filepath.Join(t.TempDir(), "preview.jsonl")
	runner := NewRunner(Config{
		Target:       "demo:0.0",
		LogPath:      logPath,
		PollInterval: Duration{time.Second},
		Calendar:     taipeiWorkdays(),
		Actions:      []ActionConfig{{Name: "scan", Type: "send_text", Text: "scan", InitialDelay: Duration{15 * time.Minute}, Every: Duration{15 * time.Minute}}},
	}, Options{DryRun: true, Once: true})
	runner.now = func() time.Time { return time.Date(2026, 10, 9, 17, 50, 0, 0, tpe) } // Friday
	runner.capturePane = func(string, int) (string, error) { return "❯ idle\n", nil }
	runner.sendText = func(string, string, bool) error { t.Fatal("dry-run sent input"); return nil }
	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"eligible_at":"2026-10-12T09:30:00+08:00"`, `"timezone":"Asia/Taipei"`, `"windows":["09:30-18:00"]`, `"type":"calendar"`, `"status":"open"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %s in %s", want, data)
		}
	}
}

func (s sentInput) String() string { return s.at.Format(time.RFC3339) + " " + s.text }
