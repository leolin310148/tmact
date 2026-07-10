// Package foldertrust implements narrowly scoped handling of Claude/Codex
// workspace-trust prompts. Trust is never implicit: callers must opt in and
// provide the exact directory expected in the target pane.
package foldertrust

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	DefaultTimeout = 30 * time.Second
	pollInterval   = 250 * time.Millisecond
	readySettle    = time.Second
)

type Options struct {
	Target  string
	Dir     string
	Agent   string
	Timeout time.Duration
	DryRun  bool
}

type Result struct {
	Target       string `json:"target"`
	Dir          string `json:"dir"`
	Agent        string `json:"agent"`
	PromptFound  bool   `json:"prompt_found"`
	Accepted     bool   `json:"accepted"`
	DryRun       bool   `json:"dry_run,omitempty"`
	OptionNumber int    `json:"option_number,omitempty"`
	OptionLabel  string `json:"option_label,omitempty"`
}

type Deps struct {
	ListPanes      func(string) ([]tmux.Pane, error)
	CapturePane    func(string, int) (string, error)
	SendKeys       func(string, []string) error
	ProcessRuntime func(int) panestatus.RuntimeDetection
	StartCommand   func(string) (string, error)
	Now            func() time.Time
	Sleep          func(time.Duration)
}

func DefaultDeps() Deps {
	return Deps{
		ListPanes:      tmux.ListPanes,
		CapturePane:    tmux.CapturePane,
		SendKeys:       tmux.SendKeys,
		ProcessRuntime: panestatus.DetectChildProcessRuntime,
		StartCommand:   tmux.PaneStartCommand,
		Now:            time.Now,
		Sleep:          time.Sleep,
	}
}

func Run(ctx context.Context, options Options) (Result, error) {
	return RunWithDeps(ctx, options, DefaultDeps())
}

func RunWithDeps(ctx context.Context, options Options, deps Deps) (Result, error) {
	result := Result{Target: options.Target, Dir: options.Dir, Agent: options.Agent, DryRun: options.DryRun}
	if options.Target == "" {
		return result, errors.New("target is required")
	}
	if options.Agent != panestatus.RuntimeClaude && options.Agent != panestatus.RuntimeCodex {
		return result, fmt.Errorf("auto trust only supports claude or codex, got %q", options.Agent)
	}
	dir, err := canonicalDirectory(options.Dir)
	if err != nil {
		return result, err
	}
	options.Dir = dir
	result.Dir = dir
	if options.Timeout <= 0 {
		options.Timeout = DefaultTimeout
	}
	deadline := deps.Now().Add(options.Timeout)
	var readySince time.Time
	var accepted *Result
	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		panes, err := deps.ListPanes(options.Target)
		if err != nil {
			return result, fmt.Errorf("inspect target %s: %w", options.Target, err)
		}
		if len(panes) == 0 {
			return result, fmt.Errorf("target %s has no panes", options.Target)
		}
		pane := panes[0]
		raw, err := deps.CapturePane(options.Target, 120)
		if err != nil {
			return result, fmt.Errorf("capture target %s: %w", options.Target, err)
		}
		runtime := detectRuntime(deps, pane, raw)
		if runtime == panestatus.RuntimeUnknown && deps.StartCommand != nil {
			if command, err := deps.StartCommand(options.Target); err == nil {
				runtime = runtimeFromStartCommand(command)
			}
		}
		classified := panestate.Classify(raw)
		if classified.InteractivePrompt != nil {
			if classified.InteractivePrompt.Type != prompt.TypeTrustFolder {
				return result, fmt.Errorf("%s is waiting on %s, not a trust-folder prompt; refusing to answer", options.Target, classified.InteractivePrompt.Type)
			}
			if accepted != nil {
				if !deps.Now().Before(deadline) {
					return *accepted, errors.New("trust-folder prompt remained after the affirmative option was sent")
				}
				deps.Sleep(pollInterval)
				continue
			}
			matched, err := AcceptPrompt(options, pane, raw, runtime, deps.SendKeys)
			if err != nil {
				return result, err
			}
			if options.DryRun {
				return matched, nil
			}
			accepted = &matched
			deps.Sleep(pollInterval)
			continue
		}
		if accepted != nil {
			return *accepted, nil
		}

		ready := classified.State == panestate.StateWaitingInput || classified.State == panestate.StateIdle
		if runtime == options.Agent && ready {
			now := deps.Now()
			if readySince.IsZero() {
				readySince = now
			} else if now.Sub(readySince) >= readySettle {
				return result, nil
			}
		} else {
			readySince = time.Time{}
		}
		if !deps.Now().Before(deadline) {
			return result, fmt.Errorf("no trust-folder prompt or ready %s runtime appeared within %s", options.Agent, options.Timeout)
		}
		deps.Sleep(pollInterval)
	}
}

// AcceptPrompt validates and answers one currently visible trust prompt. It
// returns PromptFound=false without side effects when raw is not a trust prompt.
func AcceptPrompt(options Options, pane tmux.Pane, raw, runtime string, sendKeys func(string, []string) error) (Result, error) {
	result := Result{Target: options.Target, Dir: options.Dir, Agent: options.Agent, DryRun: options.DryRun}
	detected := prompt.Detect(raw)
	if detected == nil || detected.Type != prompt.TypeTrustFolder {
		return result, nil
	}
	result.PromptFound = true
	if options.Agent != panestatus.RuntimeClaude && options.Agent != panestatus.RuntimeCodex {
		return result, fmt.Errorf("auto trust only supports claude or codex, got %q", options.Agent)
	}
	dir, err := canonicalDirectory(options.Dir)
	if err != nil {
		return result, err
	}
	result.Dir = dir
	if runtime != options.Agent {
		return result, fmt.Errorf("trust prompt runtime is %q, expected %q; refusing to answer", runtime, options.Agent)
	}
	paneDir, err := canonicalDirectory(pane.CurrentPath)
	if err != nil {
		return result, fmt.Errorf("resolve pane cwd %q: %w", pane.CurrentPath, err)
	}
	if paneDir != dir {
		return result, fmt.Errorf("pane cwd %s does not exactly match trusted directory %s; refusing to answer", paneDir, dir)
	}
	option, optionIndex, err := affirmativeOption(detected)
	if err != nil {
		return result, err
	}
	result.OptionNumber = option.Number
	result.OptionLabel = option.Label
	if options.DryRun {
		return result, nil
	}
	if sendKeys == nil {
		return result, errors.New("send keys is required")
	}
	keys := selectionKeys(detected, optionIndex, option.Number)
	if err := sendKeys(options.Target, keys); err != nil {
		return result, fmt.Errorf("accept trust-folder prompt: %w", err)
	}
	result.Accepted = true
	return result, nil
}

func canonicalDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("directory is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func detectRuntime(deps Deps, pane tmux.Pane, raw string) string {
	if deps.ProcessRuntime != nil {
		detected := deps.ProcessRuntime(pane.PanePID)
		if detected.Runtime != "" && detected.Runtime != panestatus.RuntimeUnknown {
			return detected.Runtime
		}
	}
	return panestatus.ClassifyRuntime(pane, raw).Runtime
}

func runtimeFromStartCommand(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return panestatus.RuntimeUnknown
	}
	first := strings.Trim(fields[0], "'\"")
	switch strings.ToLower(filepath.Base(first)) {
	case "claude":
		return panestatus.RuntimeClaude
	case "codex", "codex-aarch64-apple-darwin", "codex-aarch64-a":
		return panestatus.RuntimeCodex
	default:
		return panestatus.RuntimeUnknown
	}
}

func affirmativeOption(detected *prompt.Prompt) (prompt.Option, int, error) {
	var matches []int
	for index, option := range detected.Options {
		label := strings.ToLower(strings.TrimSpace(option.Label))
		if isNegativeTrustLabel(label) {
			continue
		}
		if isPositiveTrustLabel(label) {
			matches = append(matches, index)
		}
	}
	if len(matches) != 1 {
		return prompt.Option{}, -1, fmt.Errorf("trust-folder prompt has %d unambiguous affirmative options; refusing to answer", len(matches))
	}
	index := matches[0]
	return detected.Options[index], index, nil
}

func isNegativeTrustLabel(label string) bool {
	return strings.HasPrefix(label, "no") ||
		strings.Contains(label, "don't") ||
		strings.Contains(label, "do not") ||
		strings.Contains(label, "not trust") ||
		strings.Contains(label, "exit") ||
		strings.Contains(label, "quit") ||
		strings.Contains(label, "cancel")
}

func isPositiveTrustLabel(label string) bool {
	return strings.HasPrefix(label, "yes") ||
		strings.HasPrefix(label, "trust") ||
		strings.Contains(label, " trust ") ||
		strings.HasPrefix(label, "continue") ||
		strings.HasPrefix(label, "proceed") ||
		strings.HasPrefix(label, "allow")
}

func selectionKeys(detected *prompt.Prompt, affirmativeIndex, affirmativeNumber int) []string {
	selectedIndex := -1
	for index, option := range detected.Options {
		if option.Selected {
			selectedIndex = index
			break
		}
	}
	if selectedIndex < 0 {
		return []string{strconv.Itoa(affirmativeNumber)}
	}
	keys := []string{}
	delta := affirmativeIndex - selectedIndex
	key := "Down"
	if delta < 0 {
		key = "Up"
		delta = -delta
	}
	for range delta {
		keys = append(keys, key)
	}
	return append(keys, "Enter")
}
