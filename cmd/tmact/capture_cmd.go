package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/leolin310148/tmact/internal/statusd"
)

type captureReport struct {
	Selector       string `json:"selector"`
	Target         string `json:"target"`
	PaneID         string `json:"pane_id"`
	RequestedLines int    `json:"requested_lines"`
	Text           string `json:"text"`
	HistorySize    int    `json:"history_size"`
	Truncated      bool   `json:"truncated"`
	Cursor         string `json:"cursor"`
	FullSnapshot   bool   `json:"full_snapshot"`
	Reset          bool   `json:"reset"`
	ResetReason    string `json:"reset_reason,omitempty"`
}

func runCapture(args []string, globals globalOptions) error {
	if wantsHelp(args) {
		return printCommandHelp("capture")
	}
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	targetFlag := fs.String("target", "", "exact tmux pane target")
	lines := fs.Int("lines", 120, "number of pane history lines to capture")
	nonEmpty := fs.Bool("non-empty", false, "omit blank rows from captured text")
	after := fs.String("after", "", "opaque cursor from a previous JSON capture")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	afterProvided := false
	fs.Visit(func(current *flag.Flag) {
		if current.Name == "after" {
			afterProvided = true
		}
	})
	if fs.NArg() != 0 {
		return fmt.Errorf("capture does not accept positional arguments: %q", fs.Arg(0))
	}
	if globals.Target != "" && *targetFlag != "" {
		return errors.New("capture accepts exactly one target; use either global -t/--target or capture --target")
	}
	selector := globals.Target
	if selector == "" {
		selector = *targetFlag
	}
	if selector == "" {
		return errors.New("capture requires exactly one target via -t/--target")
	}
	if *lines <= 0 {
		return errors.New("--lines must be positive")
	}
	if afterProvided && *after == "" {
		return errors.New("--after cursor cannot be empty")
	}
	if afterProvided && !*jsonOutput {
		return errors.New("--after requires --json")
	}

	target, err := resolveTarget(selector)
	if err != nil {
		return err
	}
	if peer, _ := statusd.SplitPeerTarget(target); peer != "" {
		return fmt.Errorf("capture does not support peer targets; %q refers to peer %q", target, peer)
	}
	if !isExactPaneTarget(target) {
		return fmt.Errorf("capture requires an exact pane target like %%7 or session:0.0, got %q", target)
	}

	info, err := captureTmuxPaneInfo(target)
	if err != nil {
		return err
	}
	text, err := captureTmuxPane(info.PaneID, *lines)
	if err != nil {
		return err
	}
	if *nonEmpty {
		text = omitBlankRows(text)
	}
	delta := captureDelta{Text: text, FullSnapshot: true}
	if afterProvided {
		delta, err = incrementalCapture(*after, info, *lines, *nonEmpty, text)
		if err != nil {
			return err
		}
	}
	cursor, err := newCaptureCursor(info, *lines, *nonEmpty, text)
	if err != nil {
		return err
	}

	report := captureReport{
		Selector:       selector,
		Target:         info.Target,
		PaneID:         info.PaneID,
		RequestedLines: *lines,
		Text:           delta.Text,
		HistorySize:    info.HistorySize,
		Truncated:      info.HistorySize > *lines,
		Cursor:         cursor,
		FullSnapshot:   delta.FullSnapshot,
		Reset:          delta.Reset,
		ResetReason:    delta.ResetReason,
	}
	if *jsonOutput {
		return printJSON(report)
	}
	_, err = fmt.Fprint(os.Stdout, report.Text)
	return err
}

func isExactPaneTarget(target string) bool {
	if strings.HasPrefix(target, "%") {
		paneID := strings.TrimPrefix(target, "%")
		if paneID == "" {
			return false
		}
		for _, r := range paneID {
			if r < '0' || r > '9' {
				return false
			}
		}
		_, err := strconv.Atoi(paneID)
		return err == nil
	}
	colon := strings.LastIndex(target, ":")
	if colon <= 0 || colon == len(target)-1 {
		return false
	}
	windowPane := target[colon+1:]
	dot := strings.LastIndex(windowPane, ".")
	return dot > 0 && dot < len(windowPane)-1
}

func omitBlankRows(text string) string {
	rows := strings.Split(text, "\n")
	kept := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row) != "" {
			kept = append(kept, row)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	result := strings.Join(kept, "\n")
	if strings.HasSuffix(text, "\n") {
		result += "\n"
	}
	return result
}
