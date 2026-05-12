package panestatus

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tmact/internal/panestate"
	"tmact/internal/prompt"
	"tmact/internal/tmux"
)

const (
	RuntimeClaude  = "claude"
	RuntimeCodex   = "codex"
	RuntimeCopilot = "copilot"
	RuntimeGemini  = "gemini"
	RuntimeShell   = "shell"
	RuntimeTmact   = "tmact"
	RuntimeUnknown = "unknown"

	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

var DefaultIdleIgnorePatterns = []string{
	`(?i)\bcontext [0-9]+% used\b`,
	`(?i)\bcost:\s*\$`,
	`(?i)\btoken usage:\b`,
	`(?i)\b[0-9]+k/[0-9]+[kmg]\b`,
}

type Options struct {
	Target             string
	Session            string
	Window             string
	All                bool
	Lines              int
	Samples            int
	Interval           time.Duration
	IdleIgnorePatterns []string
}

type Report struct {
	Timestamp string       `json:"ts"`
	Panes     []PaneStatus `json:"panes"`
}

type PaneStatus struct {
	Target         string                  `json:"target"`
	PaneID         string                  `json:"pane_id"`
	Session        string                  `json:"session"`
	WindowIndex    int                     `json:"window_index"`
	Window         string                  `json:"window"`
	WindowActive   bool                    `json:"-"`
	PaneIndex      int                     `json:"pane_index"`
	Active         bool                    `json:"-"`
	CWD            string                  `json:"cwd,omitempty"`
	CurrentCommand string                  `json:"current_command,omitempty"`
	Runtime        string                  `json:"runtime"`
	State          string                  `json:"state"`
	Idle           bool                    `json:"idle"`
	Asking         bool                    `json:"-"`
	Confidence     string                  `json:"confidence"`
	LastLine       string                  `json:"last_line,omitempty"`
	Signals        []string                `json:"signals,omitempty"`
	Prompt         *prompt.DirectoryAccess `json:"prompt,omitempty"`
	Error          string                  `json:"error,omitempty"`
	NormalizedHash string                  `json:"-"`
}

type RuntimeDetection struct {
	Runtime    string
	Confidence string
	Signals    []string
}

type captureFunc func(string, int) (string, error)
type sleepFunc func(time.Duration)
type processRuntimeFunc func(int) RuntimeDetection

type inspector struct {
	options        Options
	capturePane    captureFunc
	sleep          sleepFunc
	processRuntime processRuntimeFunc
	ignore         []*regexp.Regexp
}

func Inspect(options Options) (Report, error) {
	panes, err := listPanes(options)
	if err != nil {
		return Report{}, err
	}
	return InspectPanes(panes, options, tmux.CapturePane, time.Sleep)
}

func InspectPanes(panes []tmux.Pane, options Options, capturePane captureFunc, sleep sleepFunc) (Report, error) {
	return inspectPanes(panes, options, capturePane, sleep, DetectChildProcessRuntime)
}

func inspectPanes(panes []tmux.Pane, options Options, capturePane captureFunc, sleep sleepFunc, processRuntime processRuntimeFunc) (Report, error) {
	if options.Lines <= 0 {
		options.Lines = 120
	}
	if options.Samples <= 0 {
		options.Samples = 1
	}
	patterns := append([]string{}, DefaultIdleIgnorePatterns...)
	patterns = append(patterns, options.IdleIgnorePatterns...)
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return Report{}, err
		}
		compiled = append(compiled, re)
	}

	inspector := inspector{
		options:        options,
		capturePane:    capturePane,
		sleep:          sleep,
		processRuntime: processRuntime,
		ignore:         compiled,
	}
	report := Report{
		Timestamp: time.Now().Format(time.RFC3339),
		Panes:     make([]PaneStatus, 0, len(panes)),
	}
	for _, pane := range panes {
		report.Panes = append(report.Panes, inspector.inspectPane(pane))
	}
	return report, nil
}

func listPanes(options Options) ([]tmux.Pane, error) {
	switch {
	case options.All:
		return tmux.ListAllPanes()
	case options.Session != "" && options.Window != "":
		return tmux.ListPanes(options.Session + ":" + options.Window)
	case options.Session != "":
		return tmux.ListSessionPanes(options.Session)
	case options.Window != "":
		return tmux.ListPanes(options.Window)
	case options.Target != "":
		return tmux.ListPanes(options.Target)
	default:
		return tmux.ListAllPanes()
	}
}

func (i inspector) inspectPane(pane tmux.Pane) PaneStatus {
	status := PaneStatus{
		Target:         targetName(pane),
		PaneID:         pane.PaneID,
		Session:        pane.Session,
		WindowIndex:    pane.WindowIndex,
		Window:         pane.WindowName,
		WindowActive:   pane.WindowActive,
		PaneIndex:      pane.PaneIndex,
		Active:         pane.Active,
		CWD:            pane.CurrentPath,
		CurrentCommand: pane.CurrentCommand,
		Runtime:        RuntimeUnknown,
		State:          panestate.StateUnknown,
		Confidence:     ConfidenceLow,
	}

	raw, changed, normalizedHash, err := i.captureSamples(pane.PaneID)
	if err != nil {
		status.State = panestate.StateBlocked
		status.Error = err.Error()
		return status
	}
	status.NormalizedHash = normalizedHash

	runtime := i.detectRuntime(pane, raw)
	status.Runtime = runtime.Runtime
	status.Confidence = runtime.Confidence
	status.Signals = append(status.Signals, runtime.Signals...)
	classified := panestate.Classify(raw)
	status.LastLine = classified.LastLine

	textState := classified.State
	status.State = classified.State
	status.Asking = classified.Asking
	status.Prompt = classified.Prompt
	for _, signal := range classified.Signals {
		status.Signals = appendSignal(status.Signals, signal)
	}
	if status.Asking {
		status.Idle = false
		return status
	}

	switch {
	case changed:
		status.State = panestate.StateWorking
		status.Idle = false
		status.Signals = appendSignal(status.Signals, "changed_capture")
	case i.options.Samples > 1:
		status.Signals = appendSignal(status.Signals, "stable_capture")
		if status.State == panestate.StateUnknown {
			status.State = panestate.StateIdle
		}
		status.Idle = status.State == panestate.StateIdle
	default:
		status.Idle = status.State == panestate.StateIdle
	}
	if textState == panestate.StateWorking {
		status.Signals = appendSignal(status.Signals, "working_text")
	}
	if textState == panestate.StateIdle {
		status.Signals = appendSignal(status.Signals, "idle_text")
	}
	return status
}

func (i inspector) detectRuntime(pane tmux.Pane, raw string) RuntimeDetection {
	processRuntime := RuntimeDetection{Runtime: RuntimeUnknown, Confidence: ConfidenceLow}
	if i.processRuntime != nil {
		processRuntime = i.processRuntime(pane.PanePID)
	}
	if processRuntime.Runtime != RuntimeUnknown {
		return mergeRuntimeSignals(processRuntime, ClassifyRuntime(pane, raw))
	}
	return ClassifyRuntime(pane, raw)
}

func (i inspector) captureSamples(target string) (string, bool, string, error) {
	var raw string
	var previous string
	changed := false
	for sample := 0; sample < i.options.Samples; sample++ {
		if sample > 0 && i.options.Interval > 0 {
			i.sleep(i.options.Interval)
		}
		captured, err := i.capturePane(target, i.options.Lines)
		if err != nil {
			return "", false, "", err
		}
		raw = captured
		hash := hashText(i.idleText(captured))
		if previous != "" && hash != previous {
			changed = true
		}
		previous = hash
	}
	return raw, changed, previous, nil
}

func (i inspector) idleText(raw string) string {
	if len(i.ignore) == 0 {
		return raw
	}
	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		ignored := false
		for _, pattern := range i.ignore {
			if pattern.MatchString(line) {
				ignored = true
				break
			}
		}
		if !ignored {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func ClassifyRuntime(pane tmux.Pane, raw string) RuntimeDetection {
	cmd := strings.ToLower(pane.CurrentCommand)
	window := strings.ToLower(pane.WindowName)
	text := strings.ToLower(raw)

	if containsAny(cmd, "codex") {
		return RuntimeDetection{Runtime: RuntimeCodex, Confidence: ConfidenceHigh, Signals: []string{"pane_current_command"}}
	}
	if containsAny(window, "codex") {
		return RuntimeDetection{Runtime: RuntimeCodex, Confidence: ConfidenceHigh, Signals: []string{"window_name"}}
	}
	if containsAny(cmd, "claude") {
		return RuntimeDetection{Runtime: RuntimeClaude, Confidence: ConfidenceHigh, Signals: []string{"pane_current_command"}}
	}
	if containsAny(window, "claude") {
		return RuntimeDetection{Runtime: RuntimeClaude, Confidence: ConfidenceHigh, Signals: []string{"window_name"}}
	}
	if containsAny(cmd, "gemini") {
		return RuntimeDetection{Runtime: RuntimeGemini, Confidence: ConfidenceHigh, Signals: []string{"pane_current_command"}}
	}
	if containsAny(window, "gemini") {
		return RuntimeDetection{Runtime: RuntimeGemini, Confidence: ConfidenceHigh, Signals: []string{"window_name"}}
	}
	if containsAny(cmd, "copilot") {
		return RuntimeDetection{Runtime: RuntimeCopilot, Confidence: ConfidenceHigh, Signals: []string{"pane_current_command"}}
	}
	if containsAny(window, "copilot") {
		return RuntimeDetection{Runtime: RuntimeCopilot, Confidence: ConfidenceHigh, Signals: []string{"window_name"}}
	}
	if cmd == "tmact" {
		return RuntimeDetection{Runtime: RuntimeTmact, Confidence: ConfidenceHigh, Signals: []string{"pane_current_command"}}
	}
	if containsAny(window, "tmact") {
		return RuntimeDetection{Runtime: RuntimeTmact, Confidence: ConfidenceHigh, Signals: []string{"window_name"}}
	}

	if looksLikeVersion(cmd) && containsAny(text, "claude code") {
		return RuntimeDetection{Runtime: RuntimeClaude, Confidence: ConfidenceMedium, Signals: []string{"pane_text", "version_command"}}
	}

	switch {
	case containsAny(text, "openai codex", "codex app"):
		return RuntimeDetection{Runtime: RuntimeCodex, Confidence: ConfidenceMedium, Signals: []string{"pane_text"}}
	case containsAny(text, "claude code"):
		return RuntimeDetection{Runtime: RuntimeClaude, Confidence: ConfidenceMedium, Signals: []string{"pane_text"}}
	case containsAny(text, "gemini"):
		return RuntimeDetection{Runtime: RuntimeGemini, Confidence: ConfidenceMedium, Signals: []string{"pane_text"}}
	case containsAny(text, "github copilot", "copilot"):
		return RuntimeDetection{Runtime: RuntimeCopilot, Confidence: ConfidenceMedium, Signals: []string{"pane_text"}}
	}

	if isShellCommand(cmd) {
		return RuntimeDetection{Runtime: RuntimeShell, Confidence: ConfidenceHigh, Signals: []string{"pane_current_command"}}
	}
	if looksLikeShellPrompt(panestate.LastMeaningfulLine(raw)) {
		return RuntimeDetection{Runtime: RuntimeShell, Confidence: ConfidenceLow, Signals: []string{"shell_prompt"}}
	}
	return RuntimeDetection{Runtime: RuntimeUnknown, Confidence: ConfidenceLow}
}

func targetName(pane tmux.Pane) string {
	return pane.Session + ":" + intString(pane.WindowIndex) + "." + intString(pane.PaneIndex)
}

func intString(value int) string {
	return strconv.Itoa(value)
}

func isShellCommand(command string) bool {
	switch command {
	case "bash", "fish", "ksh", "sh", "tcsh", "zsh":
		return true
	default:
		return false
	}
}

func looksLikeShellPrompt(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasSuffix(text, "$") || strings.HasSuffix(text, "%") || strings.HasSuffix(text, ">")
}

func looksLikeVersion(text string) bool {
	matched, _ := regexp.MatchString(`^\d+\.\d+(\.\d+)?$`, text)
	return matched
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func mergeRuntimeSignals(primary RuntimeDetection, secondary RuntimeDetection) RuntimeDetection {
	if secondary.Runtime != primary.Runtime {
		return primary
	}
	for _, signal := range secondary.Signals {
		primary.Signals = appendSignal(primary.Signals, signal)
	}
	return primary
}

func appendSignal(signals []string, signal string) []string {
	for _, existing := range signals {
		if existing == signal {
			return signals
		}
	}
	return append(signals, signal)
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
