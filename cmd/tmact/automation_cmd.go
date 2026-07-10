package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/leolin310148/tmact/internal/loop"
	"github.com/leolin310148/tmact/internal/runmeta"
	"github.com/leolin310148/tmact/internal/watch"
)

func runLoop(args []string) error {
	if wantsHelp(args) {
		if len(args) > 1 {
			return printCommandHelp("loop " + strings.Join(args[1:], " "))
		}
		return printCommandHelp("loop")
	}
	if len(args) > 0 {
		switch args[0] {
		case "start":
			return runLoopStart(args[1:])
		case "run":
			return runLoopForeground(args[1:])
		case "validate":
			return runLoopValidate(args[1:])
		case "status":
			return runLoopStatus(args[1:])
		case "stop":
			return runLoopStop(args[1:])
		case "pause":
			return runLoopControl(args[1:], runmeta.DesiredPaused)
		case "resume":
			return runLoopControl(args[1:], runmeta.DesiredRunning)
		case "restart":
			return runLoopRestart(args[1:])
		case "logs":
			return runLoopLogs(args[1:])
		}
		if !strings.HasPrefix(args[0], "-") {
			return fmt.Errorf("unknown loop subcommand %q", args[0])
		}
	}
	// Backward compatibility: the historical foreground form was
	// `tmact loop --config ...`. Keep it as an alias for `loop run`.
	return runLoopForeground(args)
}

func runLoopForeground(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("loop run")
	}
	fs := flag.NewFlagSet("loop run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to loop YAML config")
	dryRun := fs.Bool("dry-run", false, "print actions without sending anything to tmux")
	once := fs.Bool("once", false, "run one observe/action pass and exit")
	assumeIdleOnStart := fs.Bool("assume-idle-on-start", false, "treat the pane as already idle when the loop starts")
	runDir := fs.String("run-dir", runmeta.DefaultDir, "directory for runtime metadata")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}

	cfg, err := loop.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	if *assumeIdleOnStart {
		cfg.AssumeIdleOnStart = true
	}

	options := loop.Options{
		DryRun: *dryRun,
		Once:   *once,
	}
	if *once {
		runner := loop.NewRunner(cfg, options)
		return runner.Run(context.Background())
	}

	return runManagedRunner(*runDir, "loop", *configPath, cfg.Target, cfg.LogPath, *dryRun, func(ctx context.Context, record runmeta.Run) error {
		options.Control = func() (string, error) {
			control, err := runmeta.ReadControl(*runDir, record.ID)
			if errors.Is(err, os.ErrNotExist) {
				return runmeta.DesiredRunning, nil
			}
			return control.DesiredState, err
		}
		options.Heartbeat = func(phase string) error {
			return runmeta.Heartbeat(*runDir, record, phase, tmactNow())
		}
		runner := loop.NewRunner(cfg, options)
		return runner.Run(ctx)
	})
}

func runWatch(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("watch")
	}
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to watch YAML config")
	dryRun := fs.Bool("dry-run", false, "print decisions without sending anything to tmux")
	once := fs.Bool("once", false, "run one watch pass and exit")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}

	cfg, err := watch.LoadConfig(*configPath)
	if err != nil {
		return err
	}

	runner := watch.NewRunner(cfg, watch.Options{
		DryRun: *dryRun,
		Once:   *once,
	})
	return runner.Run(context.Background())
}

func runManagedRunner(runDir string, kind string, configPath string, target string, logPath string, dryRun bool, run func(context.Context, runmeta.Run) error) error {
	startedAt := tmactNow()
	record, err := runmeta.Register(runDir, runmeta.RegisterOptions{
		Kind:       kind,
		ConfigPath: configPath,
		Target:     target,
		LogPath:    logPath,
		DryRun:     dryRun,
		Tmux:       currentTmuxInfo(),
		Now:        startedAt,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err = run(ctx, record)
	status := "stopped"
	reason := "complete"
	if errors.Is(err, loop.ErrStopRequested) {
		err = nil
		reason = "requested"
	} else if errors.Is(err, context.Canceled) {
		err = nil
		reason = "interrupted"
	} else if err != nil {
		status = "error"
		reason = err.Error()
	}
	if finishErr := runmeta.Finish(runDir, record, status, reason, tmactNow()); finishErr != nil && err == nil {
		err = finishErr
	}
	return err
}

type managedWorkflowRunner interface {
	Run(context.Context) error
}

func runManagedWorkflowRunner(runDir string, configPath string, target string, logPath string, newRunner func(func() bool) (managedWorkflowRunner, error)) error {
	startedAt := tmactNow()
	record, err := runmeta.Register(runDir, runmeta.RegisterOptions{
		Kind:       "workflow",
		ConfigPath: configPath,
		Target:     target,
		LogPath:    logPath,
		Tmux:       currentTmuxInfo(),
		Now:        startedAt,
	})
	if err != nil {
		return err
	}
	stopRequested := func() bool {
		latest, err := runmeta.Read(runDir, record.ID)
		return err == nil && latest.Status == "stopping"
	}
	runner, err := newRunner(stopRequested)
	if err != nil {
		_ = runmeta.Finish(runDir, record, "error", err.Error(), tmactNow())
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err = runner.Run(ctx)
	status := "stopped"
	reason := "complete"
	if errors.Is(err, context.Canceled) {
		err = nil
		reason = "interrupted"
	} else if err != nil {
		status = "error"
		reason = err.Error()
	}
	if finishErr := runmeta.Finish(runDir, record, status, reason, tmactNow()); finishErr != nil && err == nil {
		err = finishErr
	}
	return err
}

func runRuntimeStatus(kind string, args []string) error {
	if wantsHelp(args) {
		return printCommandHelp(kind + " status")
	}
	fs := flag.NewFlagSet(kind+" status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runDir := fs.String("run-dir", runmeta.DefaultDir, "directory for runtime metadata")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	statuses, err := runmeta.List(*runDir, kind, tmactNow())
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(statuses)
	}
	printRuntimeStatuses(statuses)
	return nil
}

func runRuntimeStop(kind string, args []string) error {
	if wantsHelp(args) {
		return printCommandHelp(kind + " stop")
	}
	fs := flag.NewFlagSet(kind+" stop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runDir := fs.String("run-dir", runmeta.DefaultDir, "directory for runtime metadata")
	id := fs.String("id", "", "runtime id to stop")
	configPath := fs.String("config", "", "stop the runtime registered for this config")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	statuses, err := runmeta.List(*runDir, kind, tmactNow())
	if err != nil {
		return err
	}
	selected, err := runmeta.SelectStatus(statuses, *id, *configPath)
	if err != nil {
		return err
	}
	if selected.RuntimeStatus != "running" && selected.RuntimeStatus != "stopping" {
		return fmt.Errorf("%s %s is not running (status: %s)", kind, selected.Run.ID, selected.RuntimeStatus)
	}
	record := selected.Run

	stoppedBy := "process"
	if record.Tmux.PaneID != "" {
		if err := sendTmuxKeys(record.Tmux.PaneID, []string{"C-c"}); err != nil {
			return err
		}
		stoppedBy = "tmux"
	} else {
		process, err := os.FindProcess(record.PID)
		if err != nil {
			return err
		}
		if err := process.Signal(os.Interrupt); err != nil {
			return err
		}
	}
	if err := runmeta.Mark(*runDir, record, "stopping", stoppedBy, tmactNow()); err != nil {
		return err
	}
	if *jsonOutput {
		record.Status = "stopping"
		record.Reason = stoppedBy
		return printJSON(record)
	}
	fmt.Printf("sent stop to %s %s via %s\n", kind, record.ID, stoppedBy)
	return nil
}

func currentTmuxInfo() runmeta.TmuxInfo {
	paneID := os.Getenv("TMUX_PANE")
	if paneID == "" {
		return runmeta.TmuxInfo{}
	}
	info := runmeta.TmuxInfo{PaneID: paneID}
	panes, err := listTargetTmuxPanes(paneID)
	if err != nil || len(panes) == 0 {
		return info
	}
	pane := panes[0]
	info.Session = pane.Session
	info.WindowIndex = pane.WindowIndex
	info.WindowName = pane.WindowName
	info.PaneIndex = pane.PaneIndex
	return info
}
