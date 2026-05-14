package panestatus

import (
	"testing"
	"time"

	"tmact/internal/agents"
	"tmact/internal/tmux"
)

func TestClassifyRuntimeDetectsCodexCommand(t *testing.T) {
	pane := tmux.Pane{CurrentCommand: "codex-aarch64-a"}

	detected := ClassifyRuntime(pane, "")
	if detected.Runtime != RuntimeCodex {
		t.Fatalf("runtime = %q", detected.Runtime)
	}
	if detected.Confidence != ConfidenceHigh {
		t.Fatalf("confidence = %q", detected.Confidence)
	}
}

func TestClassifyRuntimeDetectsClaudeFromPaneText(t *testing.T) {
	pane := tmux.Pane{CurrentCommand: "2.1.138", WindowName: "2.1.138"}

	detected := ClassifyRuntime(pane, "╭─── Claude Code v2.1.138 ─╮")
	if detected.Runtime != RuntimeClaude {
		t.Fatalf("runtime = %q", detected.Runtime)
	}
	if detected.Confidence != ConfidenceMedium {
		t.Fatalf("confidence = %q", detected.Confidence)
	}
}

func TestClassifyRuntimeDetectsShellCommand(t *testing.T) {
	pane := tmux.Pane{CurrentCommand: "zsh"}

	detected := ClassifyRuntime(pane, "project $")
	if detected.Runtime != RuntimeShell {
		t.Fatalf("runtime = %q", detected.Runtime)
	}
}

func TestClassifyRuntimeShellCommandBeatsStaleAgentScrollback(t *testing.T) {
	pane := tmux.Pane{CurrentCommand: "zsh", WindowName: "zsh"}
	raw := "Claude Code v2.1.139\nold answer\nGemini reviewer failed\n❯"

	detected := ClassifyRuntime(pane, raw)
	if detected.Runtime != RuntimeShell {
		t.Fatalf("runtime = %q", detected.Runtime)
	}
	if detected.Confidence != ConfidenceHigh {
		t.Fatalf("confidence = %q", detected.Confidence)
	}
}

func TestInspectPaneMarksChangedCaptureWorking(t *testing.T) {
	panes := []tmux.Pane{{
		Session:        "work",
		WindowIndex:    0,
		WindowName:     "codex",
		PaneIndex:      0,
		PaneID:         "%1",
		CurrentCommand: "codex",
	}}
	captures := []string{"thinking\n", "thinking harder\n"}
	report, err := InspectPanes(panes, Options{Samples: 2, Interval: time.Second}, func(string, int) (string, error) {
		next := captures[0]
		captures = captures[1:]
		return next, nil
	}, func(time.Duration) {})
	if err != nil {
		t.Fatalf("InspectPanes returned error: %v", err)
	}
	if len(report.Panes) != 1 {
		t.Fatalf("panes len = %d", len(report.Panes))
	}
	status := report.Panes[0]
	if status.State != agents.StateWorking {
		t.Fatalf("state = %q", status.State)
	}
	if status.Idle {
		t.Fatal("pane should not be idle")
	}
}

func TestInspectPaneMarksStableUnknownCaptureIdle(t *testing.T) {
	panes := []tmux.Pane{{
		Session:        "work",
		WindowIndex:    0,
		WindowName:     "worker",
		PaneIndex:      1,
		PaneID:         "%2",
		CurrentCommand: "long-job",
	}}
	report, err := InspectPanes(panes, Options{Samples: 2}, func(string, int) (string, error) {
		return "same output\n", nil
	}, func(time.Duration) {})
	if err != nil {
		t.Fatalf("InspectPanes returned error: %v", err)
	}
	status := report.Panes[0]
	if status.State != agents.StateIdle {
		t.Fatalf("state = %q", status.State)
	}
	if !status.Idle {
		t.Fatal("pane should be idle")
	}
}

func TestInspectPaneIgnoresDefaultAgentStatusLines(t *testing.T) {
	panes := []tmux.Pane{{
		Session:        "work",
		WindowIndex:    0,
		WindowName:     "codex",
		PaneIndex:      0,
		PaneID:         "%1",
		CurrentCommand: "codex",
	}}
	captures := []string{
		"›\n~/w/project · main · 5h 28% · Context 22% used · 258K window\n",
		"›\n~/w/project · main · 5h 28% · Context 23% used · 258K window\n",
	}
	report, err := InspectPanes(panes, Options{Samples: 2}, func(string, int) (string, error) {
		next := captures[0]
		captures = captures[1:]
		return next, nil
	}, func(time.Duration) {})
	if err != nil {
		t.Fatalf("InspectPanes returned error: %v", err)
	}
	status := report.Panes[0]
	if status.State != agents.StateWaitingInput {
		t.Fatalf("state = %q", status.State)
	}
	if !status.Idle {
		t.Fatal("pane should be idle")
	}
	if !status.InputReady {
		t.Fatal("pane should be input ready")
	}
}

func TestInspectPaneUsesChildProcessRuntime(t *testing.T) {
	panes := []tmux.Pane{{
		Session:        "work",
		WindowIndex:    0,
		WindowName:     "2.1.138",
		PaneIndex:      0,
		PaneID:         "%1",
		PanePID:        14018,
		CurrentCommand: "2.1.138",
	}}
	report, err := inspectPanes(panes, Options{}, func(string, int) (string, error) {
		return "1 monitor\n", nil
	}, func(time.Duration) {}, func(pid int) RuntimeDetection {
		if pid != 14018 {
			t.Fatalf("pid = %d", pid)
		}
		return RuntimeDetection{Runtime: RuntimeClaude, Confidence: ConfidenceHigh, Signals: []string{"child_process"}}
	})
	if err != nil {
		t.Fatalf("inspectPanes returned error: %v", err)
	}
	status := report.Panes[0]
	if status.Runtime != RuntimeClaude {
		t.Fatalf("runtime = %q", status.Runtime)
	}
	if status.Confidence != ConfidenceHigh {
		t.Fatalf("confidence = %q", status.Confidence)
	}
	if len(status.Signals) == 0 || status.Signals[0] != "child_process" {
		t.Fatalf("signals = %#v", status.Signals)
	}
}

func TestInspectPaneSkipsCaptureForUnmatchedRuntime(t *testing.T) {
	panes := []tmux.Pane{{
		Session:        "work",
		WindowIndex:    0,
		WindowName:     "shell",
		PaneIndex:      0,
		PaneID:         "%1",
		CurrentCommand: "zsh",
	}}
	captures := 0
	report, err := InspectPanes(panes, Options{CaptureRuntimes: []string{RuntimeCodex}}, func(string, int) (string, error) {
		captures++
		return "project $\n", nil
	}, func(time.Duration) {})
	if err != nil {
		t.Fatalf("InspectPanes returned error: %v", err)
	}
	if captures != 0 {
		t.Fatalf("captures = %d", captures)
	}
	status := report.Panes[0]
	if status.Runtime != RuntimeShell {
		t.Fatalf("runtime = %q", status.Runtime)
	}
	if status.State != agents.StateUnknown {
		t.Fatalf("state = %q", status.State)
	}
	if len(status.Signals) == 0 || status.Signals[len(status.Signals)-1] != "capture_skipped" {
		t.Fatalf("signals = %#v", status.Signals)
	}
}

func TestInspectPaneCapturesMatchedChildProcessRuntime(t *testing.T) {
	panes := []tmux.Pane{{
		Session:        "work",
		WindowIndex:    0,
		WindowName:     "shell",
		PaneIndex:      0,
		PaneID:         "%1",
		PanePID:        14018,
		CurrentCommand: "zsh",
	}}
	captures := 0
	report, err := inspectPanes(panes, Options{CaptureRuntimes: []string{RuntimeCodex}}, func(string, int) (string, error) {
		captures++
		return "›\n", nil
	}, func(time.Duration) {}, func(pid int) RuntimeDetection {
		if pid != 14018 {
			t.Fatalf("pid = %d", pid)
		}
		return RuntimeDetection{Runtime: RuntimeCodex, Confidence: ConfidenceHigh, Signals: []string{"child_process"}}
	})
	if err != nil {
		t.Fatalf("inspectPanes returned error: %v", err)
	}
	if captures != 1 {
		t.Fatalf("captures = %d", captures)
	}
	status := report.Panes[0]
	if status.Runtime != RuntimeCodex {
		t.Fatalf("runtime = %q", status.Runtime)
	}
	if status.State != agents.StateWaitingInput {
		t.Fatalf("state = %q", status.State)
	}
}

func TestInspectPaneDetectsTrustPrompt(t *testing.T) {
	panes := []tmux.Pane{{
		Session:        "work",
		WindowIndex:    0,
		WindowName:     "gemini",
		PaneIndex:      0,
		PaneID:         "%1",
		CurrentCommand: "node",
	}}
	report, err := InspectPanes(panes, Options{}, func(string, int) (string, error) {
		return "Do you trust the files in this folder?\n1. Trust folder\n3. Don't trust\n", nil
	}, func(time.Duration) {})
	if err != nil {
		t.Fatalf("InspectPanes returned error: %v", err)
	}
	status := report.Panes[0]
	if status.State != agents.StateWaitingPermission {
		t.Fatalf("state = %q", status.State)
	}
	if status.Idle {
		t.Fatal("pane should not be idle")
	}
}

func TestInspectPaneDetectsProceedQuestion(t *testing.T) {
	panes := []tmux.Pane{{
		Session:        "hc-api-sb3",
		WindowIndex:    0,
		WindowName:     "codex",
		PaneIndex:      0,
		PaneID:         "%1",
		CurrentCommand: "codex",
	}}
	report, err := InspectPanes(panes, Options{}, func(string, int) (string, error) {
		return "Do you want to Proceed?\n1. Yes\n2. No\n", nil
	}, func(time.Duration) {})
	if err != nil {
		t.Fatalf("InspectPanes returned error: %v", err)
	}
	status := report.Panes[0]
	if status.State != agents.StateWaitingPermission {
		t.Fatalf("state = %q", status.State)
	}
	if !status.Asking {
		t.Fatalf("pane should be asking: %#v", status)
	}
	if status.Idle {
		t.Fatal("pane should not be idle")
	}
}
