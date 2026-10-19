package dispatch_test

import (
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	queueBusyAbove = "Claude Code\n⏺ Writing the report\n\n✻ Working… (12s · esc to interrupt)\n\n"
	queueRule      = "────────────────────────────────────────\n"
	queueDim       = "\x1b[2m"
	queueBright    = "\x1b[38;2;205;214;244m"
	queueReset     = "\x1b[0m"
)

// queueScreen is a busy Claude pane whose input box reacts to paste and
// Enter: a paste fills the box, Enter moves the text above it as a queued
// message and leaves the dim queued-messages hint behind.
type queueScreen struct {
	input    string
	queued   string
	asking   bool
	dropNext int
	ignore   int
}

func (s *queueScreen) plain() string {
	if s.asking {
		return "Claude Code\nDo you want to proceed?\n❯ 1. Yes\n  2. No\n\nEsc to cancel\n"
	}
	out := queueBusyAbove
	if s.queued != "" {
		out += "❯ " + s.queued + "\n"
	}
	out += queueRule + "❯ " + s.inputText() + "\n" + queueRule
	return out
}

func (s *queueScreen) ansi() string {
	if s.asking {
		return s.plain()
	}
	out := queueBusyAbove
	if s.queued != "" {
		out += "❯ " + s.queued + "\n"
	}
	input := ""
	switch {
	case s.input != "":
		input = queueBright + s.input + queueReset
	case s.queued != "":
		input = queueDim + "Press up to edit queued messages" + queueReset
	}
	return out + queueRule + "❯ " + input + "\n" + queueRule
}

func (s *queueScreen) inputText() string {
	if s.input == "" && s.queued != "" {
		return "Press up to edit queued messages"
	}
	return s.input
}

func queueDeps(screen *queueScreen) (*recorder, dispatch.Deps) {
	rec, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{claudePane()}, nil }
	deps.CapturePane = func(string, int) (string, error) { return screen.plain(), nil }
	deps.CapturePaneANSI = func(string, int) (string, error) { return screen.ansi(), nil }
	deps.PasteText = func(target, text string, enter bool) error {
		rec.pastes = append(rec.pastes, paste{target, text, enter})
		if screen.dropNext > 0 {
			screen.dropNext--
			return nil
		}
		screen.input += text
		return nil
	}
	deps.SendKeys = func(target string, keys []string) error {
		rec.keys = append(rec.keys, keyPress{target, keys})
		if len(keys) == 1 && keys[0] == "Enter" && screen.input != "" {
			if screen.ignore > 0 {
				screen.ignore--
				return nil
			}
			screen.queued, screen.input = screen.input, ""
		}
		return nil
	}
	return rec, deps
}

func queueOpts() dispatch.Options {
	opts := baseOpts()
	opts.Execute = true
	opts.NoClear = true
	opts.Queue = true
	return opts
}

func TestQueueTypesIntoBusyAgentWithoutClear(t *testing.T) {
	screen := &queueScreen{}
	rec, deps := queueDeps(screen)

	report, err := dispatch.RunWithDeps(queueOpts(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if !report.AgentWasRunning || stepStatus(t, report, "queue-prompt") != dispatch.StatusOK {
		t.Fatalf("report = %+v", report)
	}
	if len(rec.pastes) != 1 || rec.pastes[0].text != "do the thing" || rec.pastes[0].enter {
		t.Fatalf("pastes = %+v, want one paste without Enter and no /clear", rec.pastes)
	}
	if enterCount(rec) != 1 || screen.queued != "do the thing" {
		t.Fatalf("enters = %d queued = %q", enterCount(rec), screen.queued)
	}
}

func TestQueueRepastesOnlyWhileInputIsEmpty(t *testing.T) {
	screen := &queueScreen{dropNext: 1}
	rec, deps := queueDeps(screen)

	if _, err := dispatch.RunWithDeps(queueOpts(), deps); err != nil {
		t.Fatal(err)
	}
	if len(rec.pastes) != 2 || screen.queued != "do the thing" {
		t.Fatalf("pastes = %+v queued = %q", rec.pastes, screen.queued)
	}
}

func TestQueueResendsSwallowedEnter(t *testing.T) {
	screen := &queueScreen{ignore: 1}
	rec, deps := queueDeps(screen)

	if _, err := dispatch.RunWithDeps(queueOpts(), deps); err != nil {
		t.Fatal(err)
	}
	if len(rec.pastes) != 1 || enterCount(rec) != 2 || screen.queued != "do the thing" {
		t.Fatalf("pastes = %+v enters = %d queued = %q", rec.pastes, enterCount(rec), screen.queued)
	}
}

func TestQueueAcceptsCollapsedPaste(t *testing.T) {
	screen := &queueScreen{}
	rec, deps := queueDeps(screen)
	deps.PasteText = func(target, text string, enter bool) error {
		rec.pastes = append(rec.pastes, paste{target, text, enter})
		screen.input = "[Pasted text #1 +12 lines]"
		return nil
	}
	opts := queueOpts()
	opts.Prompt = strings.Repeat("a long multi-line prompt\n", 12)

	if _, err := dispatch.RunWithDeps(opts, deps); err != nil {
		t.Fatal(err)
	}
	if len(rec.pastes) != 1 || enterCount(rec) != 1 {
		t.Fatalf("pastes = %d enters = %d, want a single paste and Enter", len(rec.pastes), enterCount(rec))
	}
}

func TestQueueIgnoresDimSuggestionEchoingPrompt(t *testing.T) {
	screen := &queueScreen{}
	rec, deps := queueDeps(screen)
	deps.CapturePane = func(string, int) (string, error) {
		if screen.queued != "" {
			return queueBusyAbove + queueRule + "❯ do the thing\n" + queueRule, nil
		}
		return screen.plain(), nil
	}
	deps.CapturePaneANSI = func(string, int) (string, error) {
		if screen.queued != "" {
			return queueBusyAbove + queueRule + "❯ " + queueDim + "do the thing" + queueReset + "\n" + queueRule, nil
		}
		return screen.ansi(), nil
	}

	if _, err := dispatch.RunWithDeps(queueOpts(), deps); err != nil {
		t.Fatal(err)
	}
	if enterCount(rec) != 1 {
		t.Fatalf("enters = %d, want 1; Enter on a dim suggestion would submit it", enterCount(rec))
	}
}

func TestQueueRefusesUnsentDraft(t *testing.T) {
	screen := &queueScreen{input: "half typed"}
	rec, deps := queueDeps(screen)

	_, err := dispatch.RunWithDeps(queueOpts(), deps)
	if err == nil || !strings.Contains(err.Error(), "unsent draft") {
		t.Fatalf("err = %v", err)
	}
	if len(rec.pastes) != 0 || len(rec.keys) != 0 {
		t.Fatalf("draft pane received input: pastes=%+v keys=%+v", rec.pastes, rec.keys)
	}
}

func TestQueueRefusesPromptBeforePaste(t *testing.T) {
	screen := &queueScreen{asking: true}
	rec, deps := queueDeps(screen)

	_, err := dispatch.RunWithDeps(queueOpts(), deps)
	if err == nil || !strings.Contains(err.Error(), "waiting on a prompt") {
		t.Fatalf("err = %v", err)
	}
	if len(rec.pastes) != 0 || len(rec.keys) != 0 {
		t.Fatalf("asking pane received input: pastes=%+v keys=%+v", rec.pastes, rec.keys)
	}
}

func TestQueueNeverPressesEnterWhenPromptAppearsAfterPaste(t *testing.T) {
	screen := &queueScreen{}
	rec, deps := queueDeps(screen)
	deps.PasteText = func(target, text string, enter bool) error {
		rec.pastes = append(rec.pastes, paste{target, text, enter})
		screen.asking = true
		return nil
	}

	_, err := dispatch.RunWithDeps(queueOpts(), deps)
	if err == nil || !strings.Contains(err.Error(), "before Enter") {
		t.Fatalf("err = %v", err)
	}
	if len(rec.keys) != 0 {
		t.Fatalf("keys = %+v, want no Enter while a prompt is showing", rec.keys)
	}
}

func TestQueueIdlePaneSkipsSettle(t *testing.T) {
	rec, deps := baseDeps()
	deps.ListLayout = func() (tmux.Layout, error) {
		return tmux.Layout{Sessions: map[string]bool{"work": true}}, nil
	}
	deps.ListSessionPanes = func(string) ([]tmux.Pane, error) { return []tmux.Pane{claudePane()}, nil }
	input := ""
	deps.CapturePane = func(string, int) (string, error) { return "Claude Code\n❯ " + input, nil }
	deps.CapturePaneANSI = func(string, int) (string, error) {
		return "Claude Code\n\x1b[0m❯ " + queueBright + input + queueReset, nil
	}
	deps.PasteText = func(target, text string, enter bool) error {
		rec.pastes = append(rec.pastes, paste{target, text, enter})
		input = text
		return nil
	}
	deps.SendKeys = func(target string, keys []string) error {
		rec.keys = append(rec.keys, keyPress{target, keys})
		input = ""
		return nil
	}
	opts := queueOpts()
	opts.ReadySettle = dispatch.DefaultReadySettleDelay

	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if stepStatus(t, report, "queue-prompt") != dispatch.StatusOK || len(rec.pastes) != 1 || enterCount(rec) != 1 {
		t.Fatalf("report = %+v pastes = %+v keys = %+v", report, rec.pastes, rec.keys)
	}
}

func TestQueueBusyRefusedWithoutQueue(t *testing.T) {
	screen := &queueScreen{}
	rec, deps := queueDeps(screen)
	opts := queueOpts()
	opts.Queue = false

	_, err := dispatch.RunWithDeps(opts, deps)
	if err == nil || !strings.Contains(err.Error(), "busy working") {
		t.Fatalf("err = %v", err)
	}
	if len(rec.pastes) != 0 {
		t.Fatalf("pastes = %+v", rec.pastes)
	}
}

func TestQueueOptionValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*dispatch.Options)
		wantErr string
	}{
		{"gemini", func(o *dispatch.Options) { o.Agent = "gemini" }, "queue only supports claude or codex"},
		{"clear", func(o *dispatch.Options) { o.NoClear = false }, "queue requires no-clear"},
		{"wait", func(o *dispatch.Options) { o.Wait = true }, "queue does not support wait"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, deps := queueDeps(&queueScreen{})
			opts := queueOpts()
			tt.mutate(&opts)
			_, err := dispatch.RunWithDeps(opts, deps)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			if len(rec.pastes) != 0 || len(rec.keys) != 0 {
				t.Fatalf("invalid options touched the pane: pastes=%+v keys=%+v", rec.pastes, rec.keys)
			}
		})
	}
}

func TestQueueDryRunPlansQueueStep(t *testing.T) {
	rec, deps := queueDeps(&queueScreen{})
	opts := queueOpts()
	opts.Execute = false

	report, err := dispatch.RunWithDeps(opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Steps) != 1 || stepStatus(t, report, "queue-prompt") != dispatch.StatusPlanned {
		t.Fatalf("steps = %+v", report.Steps)
	}
	if len(rec.pastes) != 0 || len(rec.keys) != 0 {
		t.Fatalf("dry run touched the pane")
	}
}
