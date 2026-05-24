package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/tmux"
)

type detectResult struct {
	Target string                  `json:"target"`
	Found  bool                    `json:"found"`
	Prompt *prompt.DirectoryAccess `json:"prompt,omitempty"`
	Error  string                  `json:"error,omitempty"`
}

type listPaneRow struct {
	Index          int       `json:"index"`
	Target         string    `json:"target"`
	Session        string    `json:"session"`
	WindowIndex    int       `json:"window_index"`
	WindowName     string    `json:"window_name"`
	PaneIndex      int       `json:"pane_index"`
	CurrentCommand string    `json:"current_command"`
	CurrentPath    string    `json:"current_path"`
	Active         bool      `json:"active"`
	InMode         bool      `json:"in_mode"`
	GeneratedAt    time.Time `json:"-"`
}

func runList(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("ls")
	}
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	panes, err := listAllTmuxPanes()
	if err != nil {
		return err
	}
	rows := paneRows(panes, tmactNow())
	cache := targetCache{GeneratedAt: tmactNow(), Panes: rows}
	if err := writeTargetCache(cache); err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(cache)
	}
	printPaneRows(rows)
	return nil
}

func runDetect(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("detect")
	}
	fs := flag.NewFlagSet("detect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	target := fs.String("target", "sample:0.0", "tmux target pane/window/session to capture")
	lines := fs.Int("lines", 120, "number of pane history lines to capture")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return errors.New("--target cannot be empty")
	}

	captured, err := tmux.CapturePane(*target, *lines)
	result := detectResult{Target: *target}
	if err != nil {
		result.Error = err.Error()
		printDetectResult(result, *jsonOutput)
		return err
	}

	detected := prompt.DetectDirectoryAccess(captured)
	if detected != nil {
		result.Found = true
		result.Prompt = detected
	}

	printDetectResult(result, *jsonOutput)
	if !result.Found {
		return nil
	}
	return nil
}

func runInspect(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("inspect")
	}
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	target := fs.String("target", "", "tmux target pane/window to inspect")
	session := fs.String("session", "", "tmux session to inspect")
	window := fs.String("window", "", "tmux window to inspect; combine with --session to avoid ambiguity")
	all := fs.Bool("all", false, "inspect every tmux pane")
	lines := fs.Int("lines", 120, "number of pane history lines to capture")
	samples := fs.Int("sample", 1, "number of captures per pane for idle/running detection")
	interval := fs.Duration("interval", 0, "delay between samples, for example 1s")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	var idleIgnore repeatedStrings
	fs.Var(&idleIgnore, "idle-ignore", "regexp for lines ignored by sample hashing; may be repeated")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *lines <= 0 {
		return errors.New("--lines must be positive")
	}
	if *samples <= 0 {
		return errors.New("--sample must be positive")
	}
	if *samples == 1 && *interval != 0 {
		return errors.New("--interval is only useful when --sample is greater than 1")
	}
	if *interval < 0 {
		return errors.New("--interval cannot be negative")
	}
	selectors := 0
	for _, selected := range []bool{*target != "", *session != "" || *window != "", *all} {
		if selected {
			selectors++
		}
	}
	if selectors > 1 {
		return errors.New("choose only one selector: --target, --session/--window, or --all")
	}

	report, err := panestatus.Inspect(panestatus.Options{
		Target:             *target,
		Session:            *session,
		Window:             *window,
		All:                *all,
		Lines:              *lines,
		Samples:            *samples,
		Interval:           *interval,
		IdleIgnorePatterns: idleIgnore,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	printInspectReport(report)
	return nil
}

func paneRows(panes []tmux.Pane, generatedAt time.Time) []listPaneRow {
	rows := make([]listPaneRow, 0, len(panes))
	for index, pane := range panes {
		rows = append(rows, listPaneRow{
			Index:          index,
			Target:         paneTarget(pane),
			Session:        pane.Session,
			WindowIndex:    pane.WindowIndex,
			WindowName:     pane.WindowName,
			PaneIndex:      pane.PaneIndex,
			CurrentCommand: pane.CurrentCommand,
			CurrentPath:    pane.CurrentPath,
			Active:         pane.Active,
			InMode:         pane.InMode,
			GeneratedAt:    generatedAt,
		})
	}
	return rows
}

func paneTarget(pane tmux.Pane) string {
	if pane.PaneID != "" {
		return pane.PaneID
	}
	return fmt.Sprintf("%s:%d.%d", pane.Session, pane.WindowIndex, pane.PaneIndex)
}
