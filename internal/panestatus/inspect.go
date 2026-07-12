package panestatus

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	RuntimeClaude  = "claude"
	RuntimeCodex   = "codex"
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
	CaptureRuntimes    []string
	// ForceCapturePaneIDs captures selected panes even when their detected
	// runtime is outside CaptureRuntimes. statusd uses this for panes with an
	// unfinished shell hook so a visible prompt can retire a lost preexec.
	ForceCapturePaneIDs map[string]bool
	// MaxConcurrency caps the number of panes inspected in parallel. Each
	// inspectPane fires a `tmux capture-pane` (and possibly pgrep/ps for
	// runtime detection) so the wall-time of a full cycle is dominated by
	// subprocess fork+exec. Defaults to defaultMaxConcurrency.
	MaxConcurrency int
	// RuntimeCache, when non-nil, memoizes the child-process-tree runtime
	// detection across inspect cycles so a repeated poller (statusd) need not
	// re-walk every pane's process tree on every tick. nil disables caching.
	RuntimeCache *RuntimeCache
}

// defaultMaxConcurrency is the worker-pool size when Options.MaxConcurrency is
// unset. The tmux server serializes commands internally, so values past ~8
// stop helping and start adding scheduler noise.
const defaultMaxConcurrency = 8

type Report struct {
	Timestamp string       `json:"ts"`
	Panes     []PaneStatus `json:"panes"`
}

type PaneStatus struct {
	Target            string                  `json:"target"`
	PaneID            string                  `json:"pane_id"`
	Session           string                  `json:"session"`
	SessionID         string                  `json:"session_id,omitempty"`
	WindowIndex       int                     `json:"window_index"`
	Window            string                  `json:"window"`
	WindowActive      bool                    `json:"-"`
	PaneIndex         int                     `json:"pane_index"`
	Active            bool                    `json:"-"`
	CWD               string                  `json:"cwd,omitempty"`
	CurrentCommand    string                  `json:"current_command,omitempty"`
	Runtime           string                  `json:"runtime"`
	State             string                  `json:"state"`
	Idle              bool                    `json:"idle"`
	InputReady        bool                    `json:"input_ready"`
	Asking            bool                    `json:"-"`
	Confidence        string                  `json:"confidence"`
	LastLine          string                  `json:"last_line,omitempty"`
	Signals           []string                `json:"signals,omitempty"`
	Prompt            *prompt.DirectoryAccess `json:"prompt,omitempty"`
	InteractivePrompt *prompt.Prompt          `json:"interactive_prompt,omitempty"`
	Error             string                  `json:"error,omitempty"`
	NormalizedHash    string                  `json:"-"`
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
	captureANSI    captureFunc
	sleep          sleepFunc
	processRuntime processRuntimeFunc
	ignore         []*regexp.Regexp
	captureRuntime map[string]bool
	runtimeCache   *RuntimeCache
}

func Inspect(options Options) (Report, error) {
	panes, err := listPanes(options)
	if err != nil {
		return Report{}, err
	}
	return inspectPanesStyled(panes, options, tmux.CapturePane, tmux.CapturePaneANSI, time.Sleep, DetectChildProcessRuntime)
}

func InspectPanes(panes []tmux.Pane, options Options, capturePane captureFunc, sleep sleepFunc) (Report, error) {
	return inspectPanes(panes, options, capturePane, sleep, DetectChildProcessRuntime)
}

// InspectPanesStyled is InspectPanes with an additional ANSI-preserving
// capture used to distinguish generated suggestions from operator drafts.
func InspectPanesStyled(panes []tmux.Pane, options Options, capturePane, captureANSI captureFunc, sleep sleepFunc) (Report, error) {
	return inspectPanesStyled(panes, options, capturePane, captureANSI, sleep, DetectChildProcessRuntime)
}

func inspectPanes(panes []tmux.Pane, options Options, capturePane captureFunc, sleep sleepFunc, processRuntime processRuntimeFunc) (Report, error) {
	return inspectPanesStyled(panes, options, capturePane, nil, sleep, processRuntime)
}

func inspectPanesStyled(panes []tmux.Pane, options Options, capturePane, captureANSI captureFunc, sleep sleepFunc, processRuntime processRuntimeFunc) (Report, error) {
	if options.Lines <= 0 {
		options.Lines = 120
	}
	if options.Samples <= 0 {
		options.Samples = 1
	}
	compiled, err := compileIdlePatterns(options.IdleIgnorePatterns)
	if err != nil {
		return Report{}, err
	}
	captureRuntime := map[string]bool{}
	for _, runtime := range options.CaptureRuntimes {
		captureRuntime[runtime] = true
	}

	inspector := inspector{
		options:        options,
		capturePane:    capturePane,
		captureANSI:    captureANSI,
		sleep:          sleep,
		processRuntime: processRuntime,
		ignore:         compiled,
		captureRuntime: captureRuntime,
		runtimeCache:   options.RuntimeCache,
	}
	report := Report{
		Timestamp: time.Now().Format(time.RFC3339),
		Panes:     make([]PaneStatus, len(panes)),
	}
	if len(panes) == 0 {
		return report, nil
	}

	workers := options.MaxConcurrency
	if workers <= 0 {
		workers = defaultMaxConcurrency
	}
	if workers > len(panes) {
		workers = len(panes)
	}

	// Parallelize the inspect loop: each inspectPane is dominated by an
	// `exec.Command("tmux", "capture-pane", ...)` round-trip, so the loop is
	// IO-bound on subprocess fork+exec. tmux itself serializes commands so
	// excessive concurrency just buys scheduler noise. Results are written
	// into report.Panes by index to preserve the caller's pane order.
	type job struct{ idx int }
	jobs := make(chan job, len(panes))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				report.Panes[j.idx] = inspector.inspectPane(panes[j.idx])
			}
		}()
	}
	for i := range panes {
		jobs <- job{idx: i}
	}
	close(jobs)
	wg.Wait()

	if options.RuntimeCache != nil {
		live := make(map[int]struct{}, len(panes))
		for _, p := range panes {
			if p.PanePID > 0 {
				live[p.PanePID] = struct{}{}
			}
		}
		options.RuntimeCache.retain(live)
	}
	return report, nil
}

// idleRegexCache memoizes compiled idle-ignore patterns. The pattern set is
// fixed for a given daemon, so recompiling on every poll (twice a second) is
// wasted work; compiled regexps are immutable and safe to share.
var (
	idleRegexMu    sync.Mutex
	idleRegexCache = map[string][]*regexp.Regexp{}
)

func compileIdlePatterns(extra []string) ([]*regexp.Regexp, error) {
	patterns := append([]string{}, DefaultIdleIgnorePatterns...)
	patterns = append(patterns, extra...)
	key := strings.Join(patterns, "\x00")

	idleRegexMu.Lock()
	cached, ok := idleRegexCache[key]
	idleRegexMu.Unlock()
	if ok {
		return cached, nil
	}

	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, re)
	}

	idleRegexMu.Lock()
	idleRegexCache[key] = compiled
	idleRegexMu.Unlock()
	return compiled, nil
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
		SessionID:      pane.SessionID,
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

	runtime := i.detectRuntime(pane, "")
	status.Runtime = runtime.Runtime
	status.Confidence = runtime.Confidence
	status.Signals = append(status.Signals, runtime.Signals...)
	if !i.shouldCapture(status.Runtime, pane) {
		status.Signals = appendSignal(status.Signals, "capture_skipped")
		return status
	}

	raw, changed, normalizedHash, err := i.captureSamples(pane.PaneID)
	if err != nil {
		status.State = panestate.StateBlocked
		status.Error = err.Error()
		return status
	}
	status.NormalizedHash = normalizedHash

	runtime = i.detectRuntime(pane, raw)
	status.Runtime = runtime.Runtime
	status.Confidence = runtime.Confidence
	status.Signals = append([]string{}, runtime.Signals...)
	classified := panestate.Classify(raw)
	if i.captureANSI != nil && (status.Runtime == RuntimeClaude || status.Runtime == RuntimeCodex) {
		ansi, captureErr := i.captureANSI(pane.PaneID, i.options.Lines)
		if captureErr != nil {
			status.State = panestate.StateBlocked
			status.Error = captureErr.Error()
			status.Signals = appendSignal(status.Signals, "ansi_capture_failed")
			return status
		}
		classified = panestate.ClassifyANSI(raw, ansi)
	}
	status.LastLine = classified.LastLine

	textState := classified.State
	status.State = classified.State
	status.Asking = classified.Asking
	status.Prompt = classified.Prompt
	status.InteractivePrompt = classified.InteractivePrompt
	status.InputReady = classified.State == panestate.StateWaitingInput
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
		status.Idle = idleState(status.State)
	default:
		status.Idle = idleState(status.State)
	}
	if textState == panestate.StateWorking {
		status.Signals = appendSignal(status.Signals, "working_text")
	}
	if textState == panestate.StateIdle || textState == panestate.StateWaitingInput {
		status.Signals = appendSignal(status.Signals, "idle_text")
	}
	status.InputReady = status.State == panestate.StateWaitingInput
	return status
}

func idleState(state string) bool {
	return state == panestate.StateIdle || state == panestate.StateWaitingInput
}

func (i inspector) shouldCapture(runtime string, pane tmux.Pane) bool {
	if i.options.ForceCapturePaneIDs[pane.PaneID] {
		return true
	}
	if len(i.captureRuntime) == 0 {
		return true
	}
	if i.captureRuntime[runtime] {
		return true
	}
	// An interactive remote/shell wrapper (ssh, mosh) hides the real runtime
	// from both pane_current_command and the local process tree — the agent
	// runs on the far end. Its only fingerprint is the pane text, so we must
	// capture it even when the wrapper itself isn't on the runtime allowlist;
	// the second-round text classification then recognizes the nested agent.
	return isWrapperCommand(pane.CurrentCommand)
}

func isWrapperCommand(command string) bool {
	switch strings.ToLower(command) {
	case "ssh", "mosh", "mosh-client":
		return true
	default:
		return false
	}
}

func (i inspector) detectRuntime(pane tmux.Pane, raw string) RuntimeDetection {
	// Fast path: when pane_current_command itself names a known agent (or
	// tmact), the foreground process *is* that runtime, so a process-tree walk
	// can only confirm the same answer — skip the pgrep/ps fork storm. We key
	// off pane_current_command (ground truth for the running program), never
	// the user-assignable window name, and keep the command-precedence used by
	// ClassifyRuntime so a mislabeled window can't override it.
	if rt, ok := commandRuntime(pane.CurrentCommand); ok {
		det := RuntimeDetection{Runtime: rt, Confidence: ConfidenceHigh, Signals: []string{"pane_current_command"}}
		return mergeRuntimeSignals(det, ClassifyRuntime(pane, raw))
	}

	processRuntime := RuntimeDetection{Runtime: RuntimeUnknown, Confidence: ConfidenceLow}
	if i.processRuntime != nil {
		// The process tree rarely changes between ticks, so memoize it keyed by
		// (pid, command); the cache re-walks on command change or after its TTL.
		processRuntime = i.runtimeCache.lookup(pane.PanePID, pane.CurrentCommand, func() RuntimeDetection {
			return i.processRuntime(pane.PanePID)
		})
	}
	if processRuntime.Runtime != RuntimeUnknown {
		return mergeRuntimeSignals(processRuntime, ClassifyRuntime(pane, raw))
	}
	return ClassifyRuntime(pane, raw)
}

// commandRuntime reports the agent named by a pane's foreground command, using
// the same precedence as ClassifyRuntime's pane_current_command checks.
func commandRuntime(command string) (string, bool) {
	cmd := strings.ToLower(command)
	switch {
	case containsAny(cmd, "codex"):
		return RuntimeCodex, true
	case containsAny(cmd, "claude"):
		return RuntimeClaude, true
	case containsAny(cmd, "gemini"):
		return RuntimeGemini, true
	case cmd == "tmact":
		return RuntimeTmact, true
	}
	return "", false
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
	if cmd == "tmact" {
		return RuntimeDetection{Runtime: RuntimeTmact, Confidence: ConfidenceHigh, Signals: []string{"pane_current_command"}}
	}
	if containsAny(window, "tmact") {
		return RuntimeDetection{Runtime: RuntimeTmact, Confidence: ConfidenceHigh, Signals: []string{"window_name"}}
	}
	if isShellCommand(cmd) && looksLikeClaudeCurrentChrome(raw) {
		return RuntimeDetection{Runtime: RuntimeClaude, Confidence: ConfidenceMedium, Signals: []string{"pane_text", "nested_shell"}}
	}
	if isShellCommand(cmd) && looksLikeCodexRunningChrome(raw) {
		return RuntimeDetection{Runtime: RuntimeCodex, Confidence: ConfidenceMedium, Signals: []string{"pane_text", "nested_shell"}}
	}
	if isShellCommand(cmd) {
		return RuntimeDetection{Runtime: RuntimeShell, Confidence: ConfidenceHigh, Signals: []string{"pane_current_command"}}
	}

	if looksLikeVersion(cmd) && containsAny(text, "claude code") {
		return RuntimeDetection{Runtime: RuntimeClaude, Confidence: ConfidenceMedium, Signals: []string{"pane_text", "version_command"}}
	}

	switch {
	case containsAny(text, "openai codex", "codex app"):
		return RuntimeDetection{Runtime: RuntimeCodex, Confidence: ConfidenceMedium, Signals: []string{"pane_text"}}
	case looksLikeCodexRunningChrome(raw):
		return RuntimeDetection{Runtime: RuntimeCodex, Confidence: ConfidenceMedium, Signals: []string{"pane_text", "wrapper_chrome"}}
	case containsAny(text, "claude code", "bypass permissions on", "without interrupting claude") || looksLikeClaudeRunningChrome(text):
		return RuntimeDetection{Runtime: RuntimeClaude, Confidence: ConfidenceMedium, Signals: []string{"pane_text"}}
	case containsAny(text, "gemini"):
		return RuntimeDetection{Runtime: RuntimeGemini, Confidence: ConfidenceMedium, Signals: []string{"pane_text"}}
	}

	if looksLikeShellPrompt(panestate.LastMeaningfulLine(raw)) {
		return RuntimeDetection{Runtime: RuntimeShell, Confidence: ConfidenceLow, Signals: []string{"shell_prompt"}}
	}
	return RuntimeDetection{Runtime: RuntimeUnknown, Confidence: ConfidenceLow}
}

func looksLikeCodexRunningChrome(raw string) bool {
	hasPrompt := false
	hasStatus := false
	for _, line := range panestate.CleanedLines(raw) {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, "›") {
			hasPrompt = true
		}
		if strings.Contains(lower, "context ") && strings.Contains(lower, "% used") && strings.Contains(lower, "window") {
			hasStatus = true
		}
	}
	last := strings.ToLower(panestate.LastMeaningfulLine(raw))
	return hasPrompt && hasStatus && strings.Contains(last, "context ") && strings.Contains(last, "% used") && strings.Contains(last, "window")
}

func looksLikeClaudeCurrentChrome(raw string) bool {
	last := strings.ToLower(panestate.LastMeaningfulLine(raw))
	return strings.Contains(last, "auto mode on (shift+tab to cycle)") && strings.Contains(last, "for agents")
}

func looksLikeClaudeRunningChrome(text string) bool {
	return strings.Contains(text, "auto mode on (shift+tab to cycle)") && strings.Contains(text, "for agents")
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
