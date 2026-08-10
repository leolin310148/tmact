package dispatch

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/tmux"
)

func detectRuntime(deps Deps, pane tmux.Pane, raw string) string {
	if deps.ProcessRuntime != nil {
		detected := deps.ProcessRuntime(pane.PanePID)
		if detected.Runtime != panestatus.RuntimeUnknown && detected.Runtime != "" {
			return detected.Runtime
		}
	}
	return panestatus.ClassifyRuntime(pane, raw).Runtime
}

func classifyPane(deps Deps, target, raw string) (panestate.Result, error) {
	classified := panestate.Classify(raw)
	if deps.CapturePaneANSI == nil {
		return classified, nil
	}
	ansi, err := deps.CapturePaneANSI(target, captureLines)
	if err != nil {
		return classified, fmt.Errorf("capture styled pane: %w", err)
	}
	return panestate.ClassifyANSI(raw, ansi), nil
}

func resolveSessionTarget(deps Deps, session string) (string, error) {
	panes, err := deps.ListSessionPanes(session)
	if err != nil {
		return "", err
	}
	if len(panes) == 0 {
		return "", fmt.Errorf("session %s has no panes", session)
	}
	return paneTarget(activePane(panes)), nil
}

func activePane(panes []tmux.Pane) tmux.Pane {
	for _, pane := range panes {
		if pane.Active && pane.WindowActive {
			return pane
		}
	}
	for _, pane := range panes {
		if pane.Active {
			return pane
		}
	}
	return panes[0]
}

// findTargetPane resolves an explicit --target — a pane id ("%3"), a window
// index ("1"), or a window.pane pair ("1.0"), each optionally prefixed with
// "session:" — to a pane of the session, so dispatch acts on the requested
// window instead of whichever one tmux currently has selected. A bare window
// index picks that window's active pane. Only panes already listed for the
// session can match, so a target in another session never resolves.
func findTargetPane(panes []tmux.Pane, session, target string) (tmux.Pane, bool) {
	if strings.HasPrefix(target, "%") {
		for _, pane := range panes {
			if pane.PaneID == target {
				return pane, true
			}
		}
		return tmux.Pane{}, false
	}
	spec := strings.TrimPrefix(target, session+":")
	windowSpec := spec
	paneIndex := -1
	if dot := strings.IndexByte(spec, '.'); dot >= 0 {
		windowSpec = spec[:dot]
		idx, err := strconv.Atoi(spec[dot+1:])
		if err != nil {
			return tmux.Pane{}, false
		}
		paneIndex = idx
	}
	windowIndex, err := strconv.Atoi(windowSpec)
	if err != nil {
		return tmux.Pane{}, false
	}
	var fallback tmux.Pane
	found := false
	for _, pane := range panes {
		if pane.WindowIndex != windowIndex {
			continue
		}
		if paneIndex >= 0 {
			if pane.PaneIndex == paneIndex {
				return pane, true
			}
			continue
		}
		if pane.Active {
			return pane, true
		}
		if !found {
			fallback = pane
			found = true
		}
	}
	return fallback, found
}

func findPane(panes []tmux.Pane, target string) (tmux.Pane, bool) {
	for _, pane := range panes {
		if paneTarget(pane) == target {
			return pane, true
		}
	}
	return tmux.Pane{}, false
}

func paneTarget(pane tmux.Pane) string {
	if pane.PaneID != "" {
		return pane.PaneID
	}
	return fmt.Sprintf("%s:%d.%d", pane.Session, pane.WindowIndex, pane.PaneIndex)
}

func isAgentRuntime(runtime string) bool {
	switch runtime {
	case panestatus.RuntimeClaude, panestatus.RuntimeCodex, panestatus.RuntimeGemini:
		return true
	default:
		return false
	}
}

func promptKind(classified panestate.Result) string {
	if classified.InteractivePrompt != nil && classified.InteractivePrompt.Type != "" {
		return classified.InteractivePrompt.Type
	}
	return "interactive prompt"
}
