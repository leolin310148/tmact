package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/loop"
	"github.com/leolin310148/tmact/internal/runmeta"
	"github.com/leolin310148/tmact/internal/tmux"
)

const loopSupervisorSession = "tmact-loops"

var loopWindowNameRE = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

var (
	loopRegistryDir      = runmeta.DefaultRegistryDir
	loopPaneStartCommand = tmux.PaneStartCommand
)

func runLoopValidate(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("loop validate")
	}
	fs := flag.NewFlagSet("loop validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to loop YAML config")
	jsonOutput := fs.Bool("json", false, "print validation result as JSON")
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
	abs, err := filepath.Abs(*configPath)
	if err != nil {
		return err
	}
	result := struct {
		Valid   bool   `json:"valid"`
		Config  string `json:"config"`
		Target  string `json:"target"`
		Actions int    `json:"actions"`
		Flows   int    `json:"flows"`
	}{true, abs, cfg.Target, len(cfg.Actions), len(cfg.Flows)}
	if *jsonOutput {
		return printJSON(result)
	}
	fmt.Printf("valid loop config: %s\ntarget: %s\nactions: %d\nflows: %d\n", abs, cfg.Target, len(cfg.Actions), len(cfg.Flows))
	return nil
}

func runLoopStatus(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("loop status")
	}
	machineWide := !hasFlag(args, "--run-dir")
	fs := flag.NewFlagSet("loop status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runDir := fs.String("run-dir", runmeta.DefaultDir, "limit discovery to one runtime metadata directory")
	id := fs.String("id", "", "show one exact runtime id")
	configPath := fs.String("config", "", "show the active or newest runtime for this config")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	statuses, err := listLoopStatuses(*runDir, machineWide)
	if err != nil {
		return err
	}
	if *id != "" || *configPath != "" {
		selected, err := selectLoopFromStatuses(statuses, *id, *configPath)
		if err != nil {
			return err
		}
		statuses = []runmeta.Status{selected}
	}
	if *jsonOutput {
		return printJSON(statuses)
	}
	printRuntimeStatuses(statuses)
	return nil
}

func runLoopList(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("loop list")
	}
	machineWide := !hasFlag(args, "--run-dir")
	fs := flag.NewFlagSet("loop list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runDir := fs.String("run-dir", runmeta.DefaultDir, "limit results to one runtime metadata directory")
	all := fs.Bool("all", false, "include stopped, errored, and dead loop history")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("loop list does not accept positional arguments")
	}
	statuses, err := listLoopStatuses(*runDir, machineWide)
	if err != nil {
		return err
	}
	if !*all {
		active := statuses[:0]
		for _, status := range statuses {
			if runmeta.Active(status) {
				active = append(active, status)
			}
		}
		statuses = active
	}
	if statuses == nil {
		statuses = []runmeta.Status{}
	}
	if *jsonOutput {
		return printJSON(statuses)
	}
	if len(statuses) == 0 {
		if *all {
			fmt.Println("no registered loops")
			return nil
		}
		fmt.Println("no active loops")
		return nil
	}
	printRuntimeStatuses(statuses)
	return nil
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

func listLoopStatuses(runDir string, machineWide bool) ([]runmeta.Status, error) {
	if !machineWide {
		return runmeta.List(runDir, "loop", tmactNow())
	}
	registryDir, err := loopRegistryDir()
	if err != nil {
		return nil, err
	}
	statuses, err := runmeta.ListRegistry(registryDir, "loop", tmactNow())
	if err != nil {
		return nil, err
	}
	dirs := []string{runDir}
	legacyDirs, err := discoverLegacyLoopRunDirs()
	if err != nil {
		return nil, err
	}
	dirs = append(dirs, legacyDirs...)
	seenDirs := map[string]bool{}
	for _, dir := range dirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		if seenDirs[abs] {
			continue
		}
		seenDirs[abs] = true
		local, err := runmeta.List(abs, "loop", tmactNow())
		if err != nil {
			return nil, err
		}
		for _, status := range local {
			if runmeta.Active(status) {
				if err := runmeta.RegisterLocator(registryDir, status.RunDir, status.Run); err != nil {
					return nil, err
				}
			}
		}
		statuses = append(statuses, local...)
	}
	statuses = dedupeLoopStatuses(statuses)
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Run.StartedAt.After(statuses[j].Run.StartedAt)
	})
	return statuses, nil
}

func dedupeLoopStatuses(statuses []runmeta.Status) []runmeta.Status {
	seen := map[string]bool{}
	result := make([]runmeta.Status, 0, len(statuses))
	for _, status := range statuses {
		key := status.Run.ID + "\x00" + status.RunDir
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, status)
	}
	return result
}

func discoverLegacyLoopRunDirs() ([]string, error) {
	panes, err := listSessionTmuxPanes(loopSupervisorSession)
	if err != nil {
		return nil, nil
	}
	var dirs []string
	for _, pane := range panes {
		command, err := loopPaneStartCommand(pane.PaneID)
		if err != nil {
			continue
		}
		dir := runDirFromLoopStartCommand(command)
		if dir == "" && pane.CurrentPath != "" {
			dir = filepath.Join(pane.CurrentPath, runmeta.DefaultDir)
		}
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

func runDirFromLoopStartCommand(command string) string {
	words := splitShellWords(command)
	for i, word := range words {
		switch {
		case word == "--run-dir" && i+1 < len(words):
			return words[i+1]
		case strings.HasPrefix(word, "--run-dir="):
			return strings.TrimPrefix(word, "--run-dir=")
		}
	}
	return ""
}

func splitShellWords(command string) []string {
	command = strings.TrimSpace(command)
	if len(command) >= 2 && command[0] == '"' && command[len(command)-1] == '"' {
		command = command[1 : len(command)-1]
	}
	var words []string
	var current strings.Builder
	var quote byte
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
				continue
			}
			if ch == '\\' && quote == '"' && i+1 < len(command) {
				i++
				current.WriteByte(command[i])
				continue
			}
			current.WriteByte(ch)
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '\\':
			if i+1 < len(command) {
				i++
				current.WriteByte(command[i])
			}
		case ' ', '\t', '\r', '\n':
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func runLoopStart(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("loop start")
	}
	machineWide := !hasFlag(args, "--run-dir")
	fs := flag.NewFlagSet("loop start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to loop YAML config")
	runDir := fs.String("run-dir", runmeta.DefaultDir, "store metadata in this runtime directory and scope startup idempotency to it")
	dryRun := fs.Bool("dry-run", false, "run the detached loop without sending tmux input")
	assumeIdle := fs.Bool("assume-idle-on-start", false, "treat the target pane as already idle")
	timeout := fs.Duration("timeout", 10*time.Second, "how long to wait for startup registration")
	jsonOutput := fs.Bool("json", false, "print the registered runtime as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}
	if *timeout <= 0 {
		return errors.New("--timeout must be greater than zero")
	}
	if _, err := loop.LoadConfig(*configPath); err != nil {
		return err
	}
	absConfig, err := filepath.Abs(*configPath)
	if err != nil {
		return err
	}
	absRunDir, err := filepath.Abs(*runDir)
	if err != nil {
		return err
	}

	lockDir := absRunDir
	if machineWide {
		lockDir, err = loopRegistryDir()
		if err != nil {
			return err
		}
	}
	release, err := acquireLoopStartLock(lockDir, absConfig)
	if err != nil {
		return err
	}
	defer release()

	if active, ok, err := activeLoopForConfig(absRunDir, machineWide, absConfig); err != nil {
		return err
	} else if ok {
		return printLoopStartResult(active, true, *jsonOutput)
	}

	executable, err := tmactExecutable()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	command := []string{executable, "loop", "run", "--config", absConfig, "--run-dir", absRunDir}
	if *dryRun {
		command = append(command, "--dry-run")
	}
	if *assumeIdle {
		command = append(command, "--assume-idle-on-start")
	}
	window := loopWindowName(absConfig)
	startedAfter := time.Now().Add(-time.Second)
	if _, err := listSessionTmuxPanes(loopSupervisorSession); err != nil {
		err = newTmuxSession(loopSupervisorSession, window, cwd, command)
	} else {
		err = newTmuxWindow(loopSupervisorSession, window, cwd, command)
	}
	if err != nil {
		return fmt.Errorf("start detached loop: %w", err)
	}

	status, err := waitForLoopRegistration(absRunDir, absConfig, startedAfter, *timeout)
	if err != nil {
		return err
	}
	if status.RuntimeStatus == "error" || status.RuntimeStatus == "dead" {
		return fmt.Errorf("detached loop %s failed during startup (%s): %s", status.Run.ID, status.RuntimeStatus, status.Run.Reason)
	}
	return printLoopStartResult(status, false, *jsonOutput)
}

func printLoopStartResult(status runmeta.Status, existed bool, jsonOutput bool) error {
	if jsonOutput {
		return printJSON(struct {
			AlreadyRunning bool           `json:"already_running"`
			Status         runmeta.Status `json:"status"`
		}{existed, status})
	}
	if existed {
		fmt.Printf("loop already active: %s (%s, phase %s, mode %s)\n", status.Run.ID, status.RuntimeStatus, displayPhase(status.Run.Phase), loopRunMode(status.Run))
	} else {
		fmt.Printf("started loop %s in %s (%s, phase %s, mode %s)\n", status.Run.ID, loopSupervisorSession, status.RuntimeStatus, displayPhase(status.Run.Phase), loopRunMode(status.Run))
	}
	fmt.Printf("config: %s\ntarget: %s\n", status.Run.ConfigPath, status.Run.Target)
	return nil
}

func loopRunMode(run runmeta.Run) string {
	if run.Kind != "loop" {
		return "-"
	}
	if run.DryRun {
		return "dry-run"
	}
	return "live"
}

func runLoopControl(args []string, desired string) error {
	topic := "loop pause"
	if desired == runmeta.DesiredRunning {
		topic = "loop resume"
	}
	if wantsHelp(args) {
		return printCommandHelp(topic)
	}
	machineWide := !hasFlag(args, "--run-dir")
	fs := flag.NewFlagSet(topic, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runDir := fs.String("run-dir", runmeta.DefaultDir, "limit discovery and control to one runtime metadata directory")
	id := fs.String("id", "", "runtime id")
	configPath := fs.String("config", "", "select the active runtime for this config")
	timeout := fs.Duration("timeout", 10*time.Second, "how long to wait for the state change")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	selected, err := selectActiveLoop(*runDir, machineWide, *id, *configPath)
	if err != nil {
		return err
	}
	if err := runmeta.WriteControl(selected.RunDir, selected.Run.ID, runmeta.Control{
		DesiredState: desired,
		Reason:       strings.TrimPrefix(topic, "loop "),
		UpdatedAt:    tmactNow(),
	}); err != nil {
		return err
	}
	status, err := waitForLoopPhase(selected.RunDir, selected.Run.ID, desired, *timeout)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(status)
	}
	fmt.Printf("loop %s %s (phase %s)\n", status.Run.ID, desired, displayPhase(status.Run.Phase))
	return nil
}

func runLoopStop(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("loop stop")
	}
	machineWide := !hasFlag(args, "--run-dir")
	fs := flag.NewFlagSet("loop stop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runDir := fs.String("run-dir", runmeta.DefaultDir, "limit discovery and control to one runtime metadata directory")
	id := fs.String("id", "", "runtime id to stop")
	configPath := fs.String("config", "", "stop the active runtime for this config")
	wait := fs.Bool("wait", true, "wait until the runner has stopped")
	noWait := fs.Bool("no-wait", false, "return after recording the stop request")
	timeout := fs.Duration("timeout", 10*time.Second, "maximum wait for a clean stop")
	force := fs.Bool("force", false, "also interrupt the exact process after requesting a clean stop")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	positionalID := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalID = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	remaining := fs.Args()
	if len(remaining) > 1 || (positionalID != "" && len(remaining) != 0) {
		return errors.New("loop stop accepts at most one positional LOOP_ID")
	}
	if positionalID == "" && len(remaining) == 1 {
		positionalID = remaining[0]
	}
	if positionalID != "" {
		if *id != "" {
			return errors.New("LOOP_ID and --id are mutually exclusive")
		}
		*id = positionalID
	}
	if *id != "" && *configPath != "" {
		return errors.New("LOOP_ID/--id and --config are mutually exclusive")
	}
	if *id == "" && *configPath == "" {
		return errors.New("LOOP_ID, --id, or --config is required")
	}
	selected, err := selectActiveLoop(*runDir, machineWide, *id, *configPath)
	if err != nil {
		return err
	}
	if err := requestLoopStop(selected.RunDir, selected.Run, *force); err != nil {
		return err
	}
	if *noWait {
		*wait = false
	}
	if !*wait {
		if *jsonOutput {
			return printJSON(struct {
				ID            string `json:"id"`
				StopRequested bool   `json:"stop_requested"`
			}{selected.Run.ID, true})
		}
		fmt.Printf("requested stop for loop %s\n", selected.Run.ID)
		return nil
	}
	status, err := waitForLoopTerminal(selected.RunDir, selected.Run.ID, *timeout)
	if err != nil {
		if !*force {
			return fmt.Errorf("%w; retry with --force if the runner is stuck", err)
		}
		return err
	}
	if *jsonOutput {
		return printJSON(status)
	}
	fmt.Printf("stopped loop %s (%s, reason %s)\n", status.Run.ID, status.RuntimeStatus, status.Run.Reason)
	return nil
}

func requestLoopStop(runDir string, run runmeta.Run, force bool) error {
	if err := runmeta.WriteControl(runDir, run.ID, runmeta.Control{
		DesiredState: runmeta.DesiredStopped,
		Reason:       "operator_request",
		UpdatedAt:    tmactNow(),
	}); err != nil {
		return err
	}
	if !force {
		return nil
	}
	process, err := os.FindProcess(run.PID)
	if err == nil {
		err = process.Signal(os.Interrupt)
	}
	if err != nil && run.Tmux.PaneID != "" {
		return sendTmuxKeys(run.Tmux.PaneID, []string{"C-c"})
	}
	return err
}

func runLoopRestart(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("loop restart")
	}
	machineWide := !hasFlag(args, "--run-dir")
	fs := flag.NewFlagSet("loop restart", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "", "path to loop YAML config")
	runDir := fs.String("run-dir", runmeta.DefaultDir, "limit discovery and restart to one runtime metadata directory")
	dryRun := fs.Bool("dry-run", false, "run the restarted loop without sending tmux input")
	live := fs.Bool("live", false, "restart in live mode even if the previous run was dry-run")
	assumeIdle := fs.Bool("assume-idle-on-start", false, "treat the target pane as already idle")
	timeout := fs.Duration("timeout", 10*time.Second, "stop and startup timeout")
	jsonOutput := fs.Bool("json", false, "print startup result as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}
	if *dryRun && *live {
		return errors.New("--dry-run and --live are mutually exclusive")
	}
	absConfig, err := filepath.Abs(*configPath)
	if err != nil {
		return err
	}
	restartDryRun := false
	selectedRunDir := *runDir
	if previous, err := selectLoop(*runDir, machineWide, "", absConfig); err == nil {
		restartDryRun = previous.Run.DryRun
		selectedRunDir = previous.RunDir
	}
	if active, ok, err := activeLoopForConfig(*runDir, machineWide, absConfig); err != nil {
		return err
	} else if ok {
		restartDryRun = active.Run.DryRun
		selectedRunDir = active.RunDir
		if err := requestLoopStop(active.RunDir, active.Run, false); err != nil {
			return err
		}
		if _, err := waitForLoopTerminal(active.RunDir, active.Run.ID, *timeout); err != nil {
			return err
		}
	}
	if *dryRun {
		restartDryRun = true
	}
	if *live {
		restartDryRun = false
	}
	startArgs := []string{"--config", absConfig, "--run-dir", selectedRunDir, "--timeout", timeout.String()}
	if restartDryRun {
		startArgs = append(startArgs, "--dry-run")
	}
	if *assumeIdle {
		startArgs = append(startArgs, "--assume-idle-on-start")
	}
	if *jsonOutput {
		startArgs = append(startArgs, "--json")
	}
	return runLoopStart(startArgs)
}

func runLoopLogs(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("loop logs")
	}
	machineWide := !hasFlag(args, "--run-dir")
	fs := flag.NewFlagSet("loop logs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runDir := fs.String("run-dir", runmeta.DefaultDir, "limit discovery to one runtime metadata directory")
	id := fs.String("id", "", "runtime id")
	configPath := fs.String("config", "", "select the newest runtime for this config")
	lines := fs.Int("lines", 50, "number of existing log lines to print")
	follow := fs.Bool("follow", false, "follow new log events until interrupted or the loop stops")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *lines < 0 {
		return errors.New("--lines cannot be negative")
	}
	selected, err := selectLoop(*runDir, machineWide, *id, *configPath)
	if err != nil {
		return err
	}
	if selected.Run.LogPath == "" {
		return fmt.Errorf("loop %s has no log_path", selected.Run.ID)
	}
	offset, err := printLogTail(selected.Run.LogPath, *lines)
	if err != nil {
		return err
	}
	if !*follow {
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return followLoopLog(ctx, selected.RunDir, selected.Run.ID, selected.Run.LogPath, offset)
}

func printLogTail(path string, count int) (int64, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text != "" && count > 0 {
		lines := strings.Split(text, "\n")
		if len(lines) > count {
			lines = lines[len(lines)-count:]
		}
		fmt.Println(strings.Join(lines, "\n"))
	}
	return int64(len(data)), nil
}

func followLoopLog(ctx context.Context, runDir, id, path string, offset int64) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			file, err := os.Open(path)
			if errors.Is(err, os.ErrNotExist) {
				if run, readErr := runmeta.Read(runDir, id); readErr == nil {
					status, _ := runmeta.BuildStatus(run, tmactNow())
					if !runmeta.Active(status) {
						return nil
					}
				}
				continue
			}
			if err != nil {
				return err
			}
			info, err := file.Stat()
			if err != nil {
				file.Close()
				return err
			}
			if info.Size() < offset {
				offset = 0
			}
			if _, err := file.Seek(offset, io.SeekStart); err != nil {
				file.Close()
				return err
			}
			written, err := io.Copy(os.Stdout, file)
			file.Close()
			if err != nil {
				return err
			}
			offset += written
			if run, err := runmeta.Read(runDir, id); err == nil {
				status, _ := runmeta.BuildStatus(run, tmactNow())
				if !runmeta.Active(status) && offset >= info.Size() {
					return nil
				}
			}
		}
	}
}

func selectLoop(runDir string, machineWide bool, id, configPath string) (runmeta.Status, error) {
	statuses, err := listLoopStatuses(runDir, machineWide)
	if err != nil {
		return runmeta.Status{}, err
	}
	return selectLoopFromStatuses(statuses, id, configPath)
}

func selectLoopFromStatuses(statuses []runmeta.Status, id, configPath string) (runmeta.Status, error) {
	if id == "" && configPath != "" {
		abs, err := filepath.Abs(configPath)
		if err != nil {
			return runmeta.Status{}, err
		}
		// List is newest-first. Prefer an active run, otherwise let read-only
		// commands such as logs select the newest historical run.
		var active []runmeta.Status
		var newest *runmeta.Status
		for _, status := range statuses {
			if status.Run.ConfigPath != abs {
				continue
			}
			if newest == nil {
				candidate := status
				newest = &candidate
			}
			if runmeta.Active(status) {
				active = append(active, status)
			}
		}
		switch len(active) {
		case 1:
			return active[0], nil
		case 0:
			if newest != nil {
				return *newest, nil
			}
			return runmeta.Status{}, errors.New("run not found")
		default:
			return runmeta.Status{}, errors.New("multiple active runs matched; use --id")
		}
	}
	return runmeta.SelectStatus(statuses, id, configPath)
}

func selectActiveLoop(runDir string, machineWide bool, id, configPath string) (runmeta.Status, error) {
	status, err := selectLoop(runDir, machineWide, id, configPath)
	if err != nil {
		return runmeta.Status{}, err
	}
	if !runmeta.Active(status) {
		return runmeta.Status{}, fmt.Errorf("loop %s is not active (status: %s)", status.Run.ID, status.RuntimeStatus)
	}
	return status, nil
}

func activeLoopForConfig(runDir string, machineWide bool, configPath string) (runmeta.Status, bool, error) {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		return runmeta.Status{}, false, err
	}
	statuses, err := listLoopStatuses(runDir, machineWide)
	if err != nil {
		return runmeta.Status{}, false, err
	}
	var matches []runmeta.Status
	for _, status := range statuses {
		if status.Run.ConfigPath == abs && runmeta.Active(status) {
			matches = append(matches, status)
		}
	}
	switch len(matches) {
	case 0:
		return runmeta.Status{}, false, nil
	case 1:
		return matches[0], true, nil
	default:
		return runmeta.Status{}, false, errors.New("multiple active runs matched; use --id")
	}
}

func waitForLoopRegistration(runDir, configPath string, startedAfter time.Time, timeout time.Duration) (runmeta.Status, error) {
	deadline := time.Now().Add(timeout)
	for {
		statuses, err := runmeta.List(runDir, "loop", tmactNow())
		if err != nil {
			return runmeta.Status{}, err
		}
		for _, status := range statuses {
			if status.Run.ConfigPath == configPath && !status.Run.StartedAt.Before(startedAfter) {
				return status, nil
			}
		}
		if time.Now().After(deadline) {
			return runmeta.Status{}, fmt.Errorf("timed out after %s waiting for detached loop to register", timeout)
		}
		tmactSleep(100 * time.Millisecond)
	}
}

func waitForLoopPhase(runDir, id, desired string, timeout time.Duration) (runmeta.Status, error) {
	deadline := time.Now().Add(timeout)
	for {
		run, err := runmeta.Read(runDir, id)
		if err != nil {
			return runmeta.Status{}, err
		}
		status, err := buildLoopStatus(runDir, run)
		if err != nil {
			return runmeta.Status{}, err
		}
		if !runmeta.Active(status) {
			return status, nil
		}
		if desired == runmeta.DesiredPaused && run.Phase == "paused" {
			return status, nil
		}
		if desired == runmeta.DesiredRunning && run.Phase != "paused" {
			return status, nil
		}
		if time.Now().After(deadline) {
			return runmeta.Status{}, fmt.Errorf("timed out after %s waiting for loop %s to become %s", timeout, id, desired)
		}
		tmactSleep(100 * time.Millisecond)
	}
}

func waitForLoopTerminal(runDir, id string, timeout time.Duration) (runmeta.Status, error) {
	deadline := time.Now().Add(timeout)
	for {
		run, err := runmeta.Read(runDir, id)
		if err != nil {
			return runmeta.Status{}, err
		}
		status, err := buildLoopStatus(runDir, run)
		if err != nil {
			return runmeta.Status{}, err
		}
		if !runmeta.Active(status) {
			return status, nil
		}
		if time.Now().After(deadline) {
			return runmeta.Status{}, fmt.Errorf("timed out after %s waiting for loop %s to stop", timeout, id)
		}
		tmactSleep(100 * time.Millisecond)
	}
}

func buildLoopStatus(runDir string, run runmeta.Run) (runmeta.Status, error) {
	status, err := runmeta.BuildStatus(run, tmactNow())
	if err != nil {
		return runmeta.Status{}, err
	}
	if control, err := runmeta.ReadControl(runDir, run.ID); err == nil {
		status.DesiredState = control.DesiredState
	}
	if abs, err := filepath.Abs(runDir); err == nil {
		status.RunDir = abs
	}
	return status, nil
}

func acquireLoopStartLock(runDir, configPath string) (func(), error) {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(configPath))
	path := filepath.Join(runDir, ".start-"+hex.EncodeToString(sum[:8])+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) && staleStartLock(path) {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return nil, removeErr
		}
		file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	}
	if errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("another loop start is already in progress for %s", configPath)
	}
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
	return func() {
		_ = file.Close()
		_ = os.Remove(path)
	}, nil
}

func staleStartLock(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	if time.Since(info.ModTime()) > 30*time.Second {
		return true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return !runmeta.ProcessAlive(pid)
}

func loopWindowName(configPath string) string {
	base := strings.TrimSuffix(filepath.Base(configPath), filepath.Ext(configPath))
	base = strings.Trim(loopWindowNameRE.ReplaceAllString(base, "-"), "-")
	if base == "" {
		base = "loop"
	}
	if len(base) > 40 {
		base = base[:40]
	}
	return base
}

func displayPhase(phase string) string {
	if phase == "" {
		return "unknown"
	}
	return phase
}
