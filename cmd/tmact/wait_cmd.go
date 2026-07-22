package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/panewait"
	"github.com/leolin310148/tmact/internal/statusd"
)

const (
	defaultWaitSettle       = time.Second
	defaultWaitPollInterval = 500 * time.Millisecond
	defaultWaitTimeout      = 5 * time.Minute
)

var paneWaitRun = panewait.Run

type waitCommandReport struct {
	Selector           string    `json:"selector"`
	Target             string    `json:"target"`
	PaneID             string    `json:"pane_id,omitempty"`
	Until              string    `json:"until"`
	State              string    `json:"state"`
	RawState           string    `json:"raw_state"`
	Reason             string    `json:"reason"`
	ConditionMet       bool      `json:"condition_met"`
	RequireTransition  bool      `json:"require_transition"`
	TransitionObserved bool      `json:"transition_observed"`
	Settle             string    `json:"settle"`
	PollInterval       string    `json:"poll_interval"`
	Timeout            string    `json:"timeout"`
	Samples            int       `json:"samples"`
	LastLine           string    `json:"last_line,omitempty"`
	Signals            []string  `json:"signals,omitempty"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
	Elapsed            string    `json:"elapsed"`
}

func runWait(args []string, globals globalOptions) error {
	if wantsHelp(args) {
		return printCommandHelp("wait")
	}
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	targetFlag := fs.String("target", "", "exact tmux pane target")
	sessionFlag := fs.String("session", "", "exact tmux session whose active pane should be watched")
	until := fs.String("until", "", "terminal condition: input-ready, working, needs-human, or gone")
	requireTransition := fs.Bool("require-transition", false, "require an observed state change before matching the requested condition")
	settle := fs.Duration("settle", defaultWaitSettle, "continuous matching time before returning")
	pollInterval := fs.Duration("poll-interval", defaultWaitPollInterval, "delay between pane observations")
	timeout := fs.Duration("timeout", defaultWaitTimeout, "maximum total wait")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("wait does not accept positional arguments: %q", fs.Arg(0))
	}
	selectors := 0
	for _, selector := range []string{globals.Target, *targetFlag, *sessionFlag} {
		if selector != "" {
			selectors++
		}
	}
	if selectors != 1 {
		return errors.New("wait requires exactly one selector via global -t/--target, wait --target, or wait --session")
	}
	if *until == "" {
		return errors.New("wait requires --until input-ready|working|needs-human|gone")
	}
	switch *until {
	case panewait.UntilInputReady, panewait.UntilWorking, panewait.UntilNeedsHuman, panewait.UntilGone:
	default:
		return fmt.Errorf("unsupported wait condition %q; want input-ready, working, needs-human, or gone", *until)
	}
	if *settle < 0 {
		return errors.New("--settle cannot be negative")
	}
	if *pollInterval <= 0 {
		return errors.New("--poll-interval must be positive")
	}
	if *timeout <= 0 {
		return errors.New("--timeout must be positive")
	}

	selector := globals.Target
	selectorKind := "target"
	if selector == "" {
		selector = *targetFlag
	}
	if *sessionFlag != "" {
		selector = *sessionFlag
		selectorKind = "session"
	}
	if selectorKind == "target" {
		resolved, err := resolveTarget(selector)
		if err != nil {
			return err
		}
		selector = resolved
		if peer, _ := statusd.SplitPeerTarget(selector); peer != "" {
			return fmt.Errorf("wait does not support peer targets; %q refers to peer %q", selector, peer)
		}
		if !isExactPaneTarget(selector) {
			return fmt.Errorf("wait --target requires an exact pane like %%7 or session:0.0, got %q", selector)
		}
	} else {
		if peer, _ := statusd.SplitPeerTarget(selector); peer != "" {
			return fmt.Errorf("wait does not support peer sessions; %q refers to peer %q", selector, peer)
		}
		if strings.HasPrefix(selector, "%") || strings.Contains(selector, ":") {
			return fmt.Errorf("wait --session requires an exact session name, got %q", selector)
		}
	}

	options := panewait.Options{
		Selector:          selector,
		Until:             *until,
		RequireTransition: *requireTransition,
		Settle:            *settle,
		PollInterval:      *pollInterval,
		Timeout:           *timeout,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	report, err := paneWaitRun(ctx, options)
	if err != nil {
		return err
	}
	output := newWaitCommandReport(report, options)
	if *jsonOutput {
		if err := printJSON(output); err != nil {
			return err
		}
	} else {
		fmt.Printf("reason=%s condition=%s state=%s target=%s elapsed=%s\n", output.Reason, output.Until, output.State, output.Target, output.Elapsed)
	}
	if !report.ConditionMet {
		return fmt.Errorf("wait ended before %s: %s", options.Until, report.Reason)
	}
	return nil
}

func newWaitCommandReport(report panewait.Report, options panewait.Options) waitCommandReport {
	return waitCommandReport{
		Selector:           report.Selector,
		Target:             report.Target,
		PaneID:             report.PaneID,
		Until:              report.Until,
		State:              report.State,
		RawState:           report.RawState,
		Reason:             report.Reason,
		ConditionMet:       report.ConditionMet,
		RequireTransition:  options.RequireTransition,
		TransitionObserved: report.TransitionObserved,
		Settle:             options.Settle.String(),
		PollInterval:       options.PollInterval.String(),
		Timeout:            options.Timeout.String(),
		Samples:            report.Samples,
		LastLine:           report.LastLine,
		Signals:            report.Signals,
		StartedAt:          report.StartedAt,
		FinishedAt:         report.FinishedAt,
		Elapsed:            report.Elapsed.String(),
	}
}
