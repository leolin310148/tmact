package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"tmact/internal/agents"
	"tmact/internal/loop"
	"tmact/internal/panestatus"
	"tmact/internal/prompt"
	agentstate "tmact/internal/state"
	"tmact/internal/statusd"
	"tmact/internal/tmux"
	"tmact/internal/watch"
	"tmact/internal/workflow"

	"gopkg.in/yaml.v3"
)

type detectResult struct {
	Target string                  `json:"target"`
	Found  bool                    `json:"found"`
	Prompt *prompt.DirectoryAccess `json:"prompt,omitempty"`
	Error  string                  `json:"error,omitempty"`
}

type globalOptions struct {
	Target string
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

type targetCache struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Panes       []listPaneRow `json:"panes"`
}

type sendReport struct {
	Selector  string   `json:"selector"`
	Target    string   `json:"target"`
	Mode      string   `json:"mode"`
	Text      string   `json:"text,omitempty"`
	Keys      []string `json:"keys,omitempty"`
	Enter     bool     `json:"enter,omitempty"`
	ClearLine bool     `json:"clear_line,omitempty"`
	Execute   bool     `json:"execute"`
}

type repeatedStrings []string

func (r *repeatedStrings) String() string {
	return strings.Join(*r, ",")
}

func (r *repeatedStrings) Set(value string) error {
	if value == "" {
		return errors.New("value cannot be empty")
	}
	*r = append(*r, value)
	return nil
}

type optionalInt struct {
	value int
	set   bool
}

func (i *optionalInt) String() string {
	if !i.set {
		return ""
	}
	return strconv.Itoa(i.value)
}

func (i *optionalInt) Set(value string) error {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return err
	}
	if parsed < 0 {
		return errors.New("value cannot be negative")
	}
	i.value = parsed
	i.set = true
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var (
	listAllTmuxPanes    = tmux.ListAllPanes
	listTargetTmuxPanes = tmux.ListPanes
	pasteTmuxText       = tmux.PasteText
	sendTmuxKeys        = tmux.SendKeys
	tmactNow            = time.Now
)

const targetCacheMaxAge = 30 * time.Minute

func run(args []string) error {
	globals, args, err := parseGlobalArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return usage()
	}

	switch args[0] {
	case "ls":
		if globals.Target != "" {
			return errors.New("global -t/--target is not valid with ls")
		}
		return runList(args[1:])
	case "send":
		return runSend(args[1:], globals)
	case "detect":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runDetect(args[1:])
	case "inspect":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runInspect(args[1:])
	case "status":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runStatus(args[1:])
	case "statusd":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runStatusd(args[1:])
	case "state":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runState(args[1:])
	case "inbox":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runInbox(args[1:])
	case "summarize":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runSummarize(args[1:])
	case "broadcast":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runBroadcast(args[1:])
	case "panels":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runPanels(args[1:])
	case "loop":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runLoop(args[1:])
	case "watch":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runWatch(args[1:])
	case "workflow":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runWorkflow(args[1:])
	case "help", "-h", "--help":
		return usage()
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usageText())
	}
}

func parseGlobalArgs(args []string) (globalOptions, []string, error) {
	var opts globalOptions
	for len(args) > 0 {
		arg := args[0]
		switch {
		case arg == "-t" || arg == "--target":
			if len(args) < 2 || args[1] == "" {
				return opts, args, fmt.Errorf("%s requires a value", arg)
			}
			opts.Target = args[1]
			args = args[2:]
		case strings.HasPrefix(arg, "-t="):
			opts.Target = strings.TrimPrefix(arg, "-t=")
			args = args[1:]
		case strings.HasPrefix(arg, "--target="):
			opts.Target = strings.TrimPrefix(arg, "--target=")
			args = args[1:]
		default:
			return opts, args, nil
		}
		if opts.Target == "" {
			return opts, args, errors.New("target cannot be empty")
		}
	}
	return opts, args, nil
}

func runList(args []string) error {
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

func runSend(args []string, globals globalOptions) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	text := fs.String("text", "", "text to send")
	command := fs.String("command", "", "command to send followed by Enter")
	var keyFlags repeatedStrings
	fs.Var(&keyFlags, "key", "tmux key to send; may be repeated")
	keysCSV := fs.String("keys", "", "comma-separated tmux keys to send")
	enter := fs.Bool("enter", false, "press Enter after sending text")
	clearLine := fs.Bool("clear-line", false, "send C-u before text or command")
	execute := fs.Bool("execute", false, "actually send to tmux; default is dry-run")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if globals.Target == "" {
		return errors.New("global -t/--target is required for send")
	}

	keys, err := collectKeys(keyFlags, *keysCSV)
	if err != nil {
		return err
	}
	modeCount := 0
	mode := ""
	if *text != "" {
		modeCount++
		mode = "text"
	}
	if *command != "" {
		modeCount++
		mode = "command"
	}
	if len(keys) > 0 {
		modeCount++
		mode = "keys"
	}
	if modeCount != 1 {
		return errors.New("send requires exactly one of --text, --command, --key, or --keys")
	}
	if mode == "keys" && (*enter || *clearLine) {
		return errors.New("--enter and --clear-line are only valid with --text or --command")
	}

	target, err := resolveTarget(globals.Target)
	if err != nil {
		return err
	}

	report := sendReport{
		Selector:  globals.Target,
		Target:    target,
		Mode:      mode,
		Keys:      keys,
		Enter:     *enter || mode == "command",
		ClearLine: *clearLine,
		Execute:   *execute,
	}
	switch mode {
	case "text":
		report.Text = *text
	case "command":
		report.Text = *command
	}

	if *execute {
		if err := executeSend(report); err != nil {
			return err
		}
	}
	if *jsonOutput {
		return printJSON(report)
	}
	printSendReport(report)
	return nil
}

func collectKeys(keyFlags []string, keysCSV string) ([]string, error) {
	var keys []string
	for _, key := range keyFlags {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errors.New("key cannot be empty")
		}
		keys = append(keys, key)
	}
	if keysCSV == "" {
		return keys, nil
	}
	for _, part := range strings.Split(keysCSV, ",") {
		key := strings.TrimSpace(part)
		if key == "" {
			return nil, fmt.Errorf("invalid empty key in %q", keysCSV)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func executeSend(report sendReport) error {
	if report.ClearLine {
		if err := sendTmuxKeys(report.Target, []string{"C-u"}); err != nil {
			return err
		}
	}
	if report.Mode == "keys" {
		return sendTmuxKeys(report.Target, report.Keys)
	}
	return pasteTmuxText(report.Target, report.Text, report.Enter)
}

func resolveTarget(selector string) (string, error) {
	index, err := strconv.Atoi(selector)
	if err != nil {
		return selector, nil
	}
	if index < 0 {
		return "", fmt.Errorf("target index %d is invalid", index)
	}
	cache, err := readTargetCache()
	if err != nil {
		return "", err
	}
	if tmactNow().Sub(cache.GeneratedAt) > targetCacheMaxAge {
		return "", fmt.Errorf("target cache is older than %s; run `tmact ls` again", targetCacheMaxAge)
	}
	if index >= len(cache.Panes) {
		return "", fmt.Errorf("target index %d not found; run `tmact ls` again", index)
	}
	row := cache.Panes[index]
	if _, err := listTargetTmuxPanes(row.Target); err != nil {
		return "", fmt.Errorf("cached target %d (%s) is no longer available; run `tmact ls` again: %w", index, row.Target, err)
	}
	return row.Target, nil
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

func targetCachePath() string {
	return filepath.Join(".cache", "tmact-targets.json")
}

func writeTargetCache(cache targetCache) error {
	path := targetCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func readTargetCache() (targetCache, error) {
	var cache targetCache
	data, err := os.ReadFile(targetCachePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cache, errors.New("target cache not found; run `tmact ls` first")
		}
		return cache, err
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		return cache, fmt.Errorf("read target cache: %w", err)
	}
	return cache, nil
}

func runState(args []string) error {
	if len(args) == 0 {
		return errors.New("state requires a subcommand: get, set, transition, or event")
	}

	switch args[0] {
	case "get":
		return runStateGet(args[1:])
	case "set":
		return runStateSet(args[1:])
	case "transition":
		return runStateTransition(args[1:])
	case "event":
		return runStateEvent(args[1:])
	default:
		return fmt.Errorf("unknown state subcommand %q", args[0])
	}
}

func runStateGet(args []string) error {
	fs := flag.NewFlagSet("state get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	path := fs.String("path", "", "path to status.yaml")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("--path is required")
	}

	status, err := agentstate.Load(*path)
	if err != nil {
		return err
	}
	data, err := status.Data()
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(data)
	}
	encoded, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	fmt.Print(string(encoded))
	return nil
}

func runStateSet(args []string) error {
	fs := flag.NewFlagSet("state set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	path := fs.String("path", "", "path to status.yaml")
	stateName := fs.String("state", "", "state to write")
	owner := fs.String("owner", "", "owner to write")
	stage := fs.String("stage", "", "stage to write")
	var cycle optionalInt
	fs.Var(&cycle, "cycle", "cycle number to write")
	var blockers repeatedStrings
	fs.Var(&blockers, "blocker", "blocker text to write; may be repeated")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("--path is required")
	}
	if *stateName == "" {
		return errors.New("--state is required")
	}

	update := agentStateUpdate(*stateName, *owner, *stage, cycle, blockers)
	data, event, err := agentstate.Set(*path, update)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(map[string]interface{}{"status": data, "event": event})
	}
	printStateChange(*path, data, event)
	return nil
}

func runStateTransition(args []string) error {
	fs := flag.NewFlagSet("state transition", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	path := fs.String("path", "", "path to status.yaml")
	from := fs.String("from", "", "required current state")
	to := fs.String("to", "", "state to transition to")
	owner := fs.String("owner", "", "owner to write")
	stage := fs.String("stage", "", "stage to write")
	var cycle optionalInt
	fs.Var(&cycle, "cycle", "cycle number to write")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("--path is required")
	}
	if *from == "" {
		return errors.New("--from is required")
	}
	if *to == "" {
		return errors.New("--to is required")
	}

	update := agentStateUpdate(*to, *owner, *stage, cycle, nil)
	data, event, err := agentstate.Transition(*path, *from, update)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(map[string]interface{}{"status": data, "event": event})
	}
	printStateChange(*path, data, event)
	return nil
}

func runStateEvent(args []string) error {
	fs := flag.NewFlagSet("state event", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	path := fs.String("path", "", "path to status.yaml")
	kind := fs.String("kind", "", "event kind")
	stage := fs.String("stage", "", "stage name")
	agent := fs.String("agent", "", "agent name")
	message := fs.String("message", "", "event message")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("--path is required")
	}
	if *kind == "" {
		return errors.New("--kind is required")
	}

	event := agentstate.Event{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Kind:      *kind,
		Path:      *path,
		Stage:     *stage,
		Agent:     *agent,
		Message:   *message,
	}
	if err := agentstate.AppendEvent(*path, event); err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(event)
	}
	fmt.Printf("event: %s\npath: %s\n", event.Kind, event.Path)
	return nil
}

func agentStateUpdate(stateName string, owner string, stage string, cycle optionalInt, blockers repeatedStrings) agentstate.Update {
	update := agentstate.Update{
		State:       stateName,
		Owner:       owner,
		Stage:       stage,
		SetBlockers: blockers != nil,
		Blockers:    blockers,
	}
	if cycle.set {
		update.Cycle = &cycle.value
	}
	return update
}

func runDetect(args []string) error {
	fs := flag.NewFlagSet("detect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	target := fs.String("target", "z_sample-project_sample:0.0", "tmux target pane/window/session to capture")
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

func runStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to agent registry YAML config")
	agentName := fs.String("agent", "", "agent name to include")
	role := fs.String("role", "", "role to include")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadAgentConfig(*configPath)
	if err != nil {
		return err
	}
	cfg, err = agents.FilterConfig(cfg, agents.Filter{Agent: *agentName, Role: *role})
	if err != nil {
		return err
	}
	report := agents.Collect(cfg)
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	printStatusReport(report)
	return nil
}

func runStatusd(args []string) error {
	if len(args) == 0 {
		return errors.New("statusd requires a subcommand: start, once, read, status, stop")
	}
	switch args[0] {
	case "start":
		return runStatusdStart(args[1:])
	case "once":
		return runStatusdOnce(args[1:])
	case "read":
		return runStatusdRead(args[1:])
	case "status":
		return runStatusdStatus(args[1:])
	case "stop":
		return errors.New("statusd stop is not available without background mode; stop the foreground process instead")
	case "help", "-h", "--help":
		return statusdUsage()
	default:
		return fmt.Errorf("unknown statusd subcommand %q", args[0])
	}
}

func runStatusdStart(args []string) error {
	fs := flag.NewFlagSet("statusd start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	flags := statusdFlags(fs)
	once := fs.Bool("once", false, "run one scan then exit")

	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := flags.config()
	if err := validateStatusdConfig(cfg); err != nil {
		return err
	}

	daemon := statusd.NewDaemon(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *once {
		snapshot, err := daemon.RunOnce(ctx)
		if *flags.JSON {
			if printErr := printJSON(snapshot); printErr != nil && err == nil {
				err = printErr
			}
		}
		return err
	}
	if *flags.JSON {
		return errors.New("--json is only valid with --once for statusd start")
	}
	err := daemon.Start(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func runStatusdOnce(args []string) error {
	fs := flag.NewFlagSet("statusd once", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	flags := statusdFlags(fs)

	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := flags.config()
	if err := validateStatusdConfig(cfg); err != nil {
		return err
	}

	snapshot, err := statusd.NewDaemon(cfg).RunOnce(context.Background())
	if *flags.JSON {
		if printErr := printJSON(snapshot); printErr != nil && err == nil {
			err = printErr
		}
	} else {
		printStatusdSnapshot(snapshot, tmactNow())
	}
	return err
}

func runStatusdRead(args []string) error {
	fs := flag.NewFlagSet("statusd read", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	statePath := fs.String("state-path", statusd.DefaultStatePath, "latest JSON snapshot path")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	snapshot, err := statusd.ReadSnapshot(*statePath)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(snapshot)
	}
	printStatusdSnapshot(snapshot, tmactNow())
	return nil
}

func runStatusdStatus(args []string) error {
	fs := flag.NewFlagSet("statusd status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	statePath := fs.String("state-path", statusd.DefaultStatePath, "latest JSON snapshot path")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	snapshot, err := statusd.ReadSnapshot(*statePath)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(snapshot)
	}
	now := tmactNow()
	fmt.Printf("state_path: %s\n", *statePath)
	fmt.Printf("last_update: %s\n", snapshot.Timestamp.Format(time.RFC3339))
	fmt.Printf("age: %s\n", formatAge(now.Sub(snapshot.Timestamp)))
	fmt.Printf("stale: %t\n", snapshot.IsStale(now))
	fmt.Printf("panes: %d\n", snapshot.Summary.Panes)
	fmt.Printf("sessions: %d\n", snapshot.Summary.Sessions)
	fmt.Printf("errors: %d\n", snapshot.Summary.Errors)
	return nil
}

type statusdFlagValues struct {
	Config         statusd.Config
	JSON           *bool
	NoTmuxOptions  *bool
	IdleIgnore     repeatedStrings
	IncludeSession repeatedStrings
	ExcludeSession repeatedStrings
}

func statusdFlags(fs *flag.FlagSet) *statusdFlagValues {
	values := &statusdFlagValues{Config: statusd.Config{TmuxOptions: true}}
	fs.DurationVar(&values.Config.Interval, "interval", statusd.DefaultInterval, "scan interval")
	fs.StringVar(&values.Config.StatePath, "state-path", statusd.DefaultStatePath, "latest JSON snapshot path")
	fs.StringVar(&values.Config.LogPath, "log-path", "", "optional JSONL daemon log path")
	fs.BoolVar(&values.Config.TmuxOptions, "tmux-options", true, "write @ai-* tmux options")
	values.NoTmuxOptions = fs.Bool("no-tmux-options", false, "only write the state file")
	fs.IntVar(&values.Config.CaptureLines, "capture-lines", statusd.DefaultCaptureLines, "number of pane history lines to capture")
	fs.IntVar(&values.Config.InitialSamples, "initial-samples", statusd.DefaultInitialSamples, "captures per pane before statusd has history")
	fs.DurationVar(&values.Config.RunningDebounce, "running-debounce", statusd.DefaultRunningDebounce, "keep running indicator after changes")
	fs.DurationVar(&values.Config.StaleAfter, "stale-after", statusd.DefaultStaleAfter, "mark snapshot stale after this age")
	fs.Var(&values.IdleIgnore, "idle-ignore", "regexp for lines ignored by sample hashing; may be repeated")
	fs.Var(&values.IncludeSession, "session", "include sessions matching glob; may be repeated")
	fs.Var(&values.ExcludeSession, "exclude-session", "exclude sessions matching glob; may be repeated")
	values.JSON = fs.Bool("json", false, "print JSON output")
	return values
}

func (v *statusdFlagValues) config() statusd.Config {
	cfg := v.Config
	if *v.NoTmuxOptions {
		cfg.TmuxOptions = false
	}
	cfg.IdleIgnorePatterns = v.IdleIgnore
	cfg.IncludeSessions = v.IncludeSession
	cfg.ExcludeSessions = v.ExcludeSession
	return cfg
}

func validateStatusdConfig(cfg statusd.Config) error {
	if cfg.Interval <= 0 {
		return errors.New("--interval must be positive")
	}
	if cfg.CaptureLines <= 0 {
		return errors.New("--capture-lines must be positive")
	}
	if cfg.InitialSamples <= 0 {
		return errors.New("--initial-samples must be positive")
	}
	if cfg.RunningDebounce <= 0 {
		return errors.New("--running-debounce must be positive")
	}
	if cfg.StaleAfter <= 0 {
		return errors.New("--stale-after must be positive")
	}
	return nil
}

func statusdUsage() error {
	fmt.Fprint(os.Stderr, `Usage:
  tmact statusd start [--interval 1s] [--state-path /tmp/tmact-status.json] [--no-tmux-options]
  tmact statusd once [--json] [--state-path /tmp/tmact-status.json] [--initial-samples 2]
  tmact statusd read [--json] [--state-path /tmp/tmact-status.json]
  tmact statusd status [--state-path /tmp/tmact-status.json]
`)
	return nil
}

func runInbox(args []string) error {
	fs := flag.NewFlagSet("inbox", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to agent registry YAML config")
	agentName := fs.String("agent", "", "agent name to include")
	role := fs.String("role", "", "role to include")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadAgentConfig(*configPath)
	if err != nil {
		return err
	}
	cfg, err = agents.FilterConfig(cfg, agents.Filter{Agent: *agentName, Role: *role})
	if err != nil {
		return err
	}
	inbox := agents.InboxFromReport(agents.Collect(cfg))
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(inbox)
	}

	printInbox(inbox)
	return nil
}

func runSummarize(args []string) error {
	fs := flag.NewFlagSet("summarize", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to agent registry YAML config")
	agentName := fs.String("agent", "", "agent name to summarize; omit for all agents")
	lines := fs.Int("lines", 12, "number of recent pane lines to include")
	commits := fs.Int("commits", 5, "number of recent git commits to include")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadAgentConfig(*configPath)
	if err != nil {
		return err
	}
	report, err := agents.Summarize(cfg, *agentName, *lines, *commits)
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	printSummary(report)
	return nil
}

func runBroadcast(args []string) error {
	fs := flag.NewFlagSet("broadcast", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to agent registry YAML config")
	agentName := fs.String("agent", "", "agent name to send to")
	role := fs.String("role", "", "role to send to")
	all := fs.Bool("all", false, "send to every configured agent")
	text := fs.String("text", "", "text to send")
	enter := fs.Bool("enter", false, "press Enter after sending text")
	execute := fs.Bool("execute", false, "actually send text to tmux; default is dry-run")
	onlyIdle := fs.Bool("only-idle", false, "skip agents that do not appear idle")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadAgentConfig(*configPath)
	if err != nil {
		return err
	}
	report, err := agents.Broadcast(cfg, agents.BroadcastOptions{
		Agent:    *agentName,
		Role:     *role,
		All:      *all,
		Text:     *text,
		Enter:    *enter,
		Execute:  *execute,
		OnlyIdle: *onlyIdle,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	printBroadcast(report)
	return nil
}

func runPanels(args []string) error {
	if len(args) == 0 {
		return errors.New("panels requires a subcommand: plan or ensure")
	}
	action := args[0]
	if action != "plan" && action != "ensure" {
		return fmt.Errorf("unknown panels subcommand %q", action)
	}

	fs := flag.NewFlagSet("panels "+action, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to agent registry YAML config")
	agentName := fs.String("agent", "", "agent name to include")
	role := fs.String("role", "", "role to include")
	session := fs.String("session", "", "override target tmux session for selected agents")
	execute := fs.Bool("execute", false, "apply planned tmux panel changes")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if action == "plan" && *execute {
		return errors.New("--execute is only valid with panels ensure")
	}

	cfg, err := loadAgentConfig(*configPath)
	if err != nil {
		return err
	}
	cfg, err = agents.FilterConfig(cfg, agents.Filter{Agent: *agentName, Role: *role})
	if err != nil {
		return err
	}

	opts := agents.PanelOptions{Session: *session, Execute: action == "ensure" && *execute}
	var report agents.PanelReport
	if action == "ensure" {
		report, err = agents.EnsurePanels(cfg, opts)
	} else {
		report, err = agents.PlanPanels(cfg, opts)
	}
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	printPanelReport(report)
	return nil
}

func loadAgentConfig(configPath string) (agents.Config, error) {
	resolved, err := agents.ResolveConfigPath(configPath)
	if err != nil {
		return agents.Config{}, err
	}
	return agents.LoadConfig(resolved)
}

func runLoop(args []string) error {
	fs := flag.NewFlagSet("loop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to loop YAML config")
	dryRun := fs.Bool("dry-run", false, "print actions without sending anything to tmux")
	once := fs.Bool("once", false, "run one observe/action pass and exit")
	assumeIdleOnStart := fs.Bool("assume-idle-on-start", false, "treat the pane as already idle when the loop starts")

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

	runner := loop.NewRunner(cfg, loop.Options{
		DryRun: *dryRun,
		Once:   *once,
	})
	return runner.Run(context.Background())
}

func runWatch(args []string) error {
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

func runWorkflow(args []string) error {
	fs := flag.NewFlagSet("workflow", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to workflow YAML config")
	dryRun := fs.Bool("dry-run", false, "print workflow actions without sending anything to tmux")
	once := fs.Bool("once", false, "run one workflow observe/action pass and exit")
	assumeIdleOnStart := fs.Bool("assume-idle-on-start", false, "treat the pane as already idle when the workflow starts")
	startStage := fs.String("start-stage", "", "start from the named workflow stage")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}

	cfg, err := workflow.LoadConfig(*configPath)
	if err != nil {
		return err
	}

	runner := workflow.NewRunner(cfg, workflow.Options{
		DryRun:            *dryRun,
		Once:              *once,
		AssumeIdleOnStart: *assumeIdleOnStart,
		StartStage:        *startStage,
	})
	return runner.Run(context.Background())
}

func printDetectResult(result detectResult, jsonOutput bool) {
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(result)
		return
	}

	if result.Error != "" {
		fmt.Printf("target: %s\nerror: %s\n", result.Target, result.Error)
		return
	}
	if !result.Found || result.Prompt == nil {
		fmt.Printf("target: %s\nfound: false\n", result.Target)
		return
	}

	fmt.Printf("target: %s\nfound: true\n", result.Target)
	fmt.Printf("title: %s\n", result.Prompt.Title)
	if result.Prompt.Path != "" {
		fmt.Printf("path: %s\n", result.Prompt.Path)
	}
	if len(result.Prompt.Paths) > 1 {
		fmt.Printf("paths: %s\n", strings.Join(result.Prompt.Paths, ", "))
	}
	if result.Prompt.Question != "" {
		fmt.Printf("question: %s\n", result.Prompt.Question)
	}
	if result.Prompt.SelectedOption != nil {
		fmt.Printf("selected: %d. %s\n", result.Prompt.SelectedOption.Number, result.Prompt.SelectedOption.Label)
	}
}

func printJSON(value interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printPaneRows(rows []listPaneRow) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "#\ttarget\tsession\twindow\tpane\tcommand\tcwd")
	for _, row := range rows {
		window := fmt.Sprintf("%d:%s", row.WindowIndex, row.WindowName)
		fmt.Fprintf(writer, "%d\t%s\t%s\t%s\t%d\t%s\t%s\n", row.Index, row.Target, row.Session, window, row.PaneIndex, row.CurrentCommand, row.CurrentPath)
	}
	_ = writer.Flush()
}

func printSendReport(report sendReport) {
	prefix := ""
	if !report.Execute {
		prefix = "dry-run: would "
	}
	switch report.Mode {
	case "keys":
		fmt.Printf("%ssend keys to %s: %s\n", prefix, report.Target, strings.Join(report.Keys, ","))
	case "command":
		if report.ClearLine {
			fmt.Printf("%sclear line and send command to %s: %s\n", prefix, report.Target, report.Text)
			return
		}
		fmt.Printf("%ssend command to %s: %s\n", prefix, report.Target, report.Text)
	case "text":
		enter := ""
		if report.Enter {
			enter = " and Enter"
		}
		if report.ClearLine {
			fmt.Printf("%sclear line and send text%s to %s: %s\n", prefix, enter, report.Target, report.Text)
			return
		}
		fmt.Printf("%ssend text%s to %s: %s\n", prefix, enter, report.Target, report.Text)
	}
}

func printStateChange(path string, data map[string]interface{}, event agentstate.Event) {
	fmt.Printf("path: %s\n", path)
	if stateName, ok := data["state"].(string); ok {
		fmt.Printf("state: %s\n", stateName)
	}
	if event.Kind != "" {
		fmt.Printf("event: %s\n", event.Kind)
	}
}

func printStatusReport(report agents.Report) {
	fmt.Printf("ts: %s\n", report.Timestamp)
	for _, agent := range report.Agents {
		fmt.Printf("%s\t%s\t%s", agent.Name, agent.Target, agent.State)
		if agent.Git != nil {
			dirty := "clean"
			if agent.Git.Dirty {
				dirty = "dirty"
			}
			if agent.Git.Error != "" {
				fmt.Printf("\tgit:%s", agent.Git.Error)
			} else if agent.Git.Branch != "" {
				fmt.Printf("\tgit:%s/%s", agent.Git.Branch, dirty)
			} else {
				fmt.Printf("\tgit:%s", dirty)
			}
		}
		if agent.LastLine != "" {
			fmt.Printf("\t%s", agent.LastLine)
		}
		if agent.Error != "" {
			fmt.Printf("\terror:%s", agent.Error)
		}
		fmt.Println()
	}
}

func printStatusdSnapshot(snapshot statusd.Snapshot, now time.Time) {
	fmt.Printf("ts: %s age: %s stale: %t\n", snapshot.Timestamp.Format(time.RFC3339), formatAge(now.Sub(snapshot.Timestamp)), snapshot.IsStale(now))
	sessions := make([]statusd.SessionStatus, 0, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Session < sessions[j].Session
	})
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, session := range sessions {
		promptType := ""
		if pane, ok := snapshot.Panes[session.ActiveTarget]; ok && pane.Prompt != nil {
			promptType = pane.Prompt.Type
		}
		if promptType != "" {
			fmt.Fprintf(writer, "%s\t%s\t%s\ttag:%s\trunning:%t\tasking:%t\tprompt:%s\n", session.ActiveTarget, session.Runtime, session.State, session.Tag, session.Running, session.Asking, promptType)
			continue
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\ttag:%s\trunning:%t\tasking:%t\n", session.ActiveTarget, session.Runtime, session.State, session.Tag, session.Running, session.Asking)
	}
	_ = writer.Flush()
	if len(snapshot.Errors) > 0 {
		fmt.Printf("errors: %d\n", len(snapshot.Errors))
	}
}

func formatAge(age time.Duration) string {
	if age < 0 {
		age = 0
	}
	if age < time.Second {
		return age.Truncate(time.Millisecond).String()
	}
	return age.Truncate(100 * time.Millisecond).String()
}

func printInspectReport(report panestatus.Report) {
	fmt.Printf("ts: %s\n", report.Timestamp)
	for _, pane := range report.Panes {
		fmt.Printf("%s\t%s\t%s\tidle:%t", pane.Target, pane.Runtime, pane.State, pane.Idle)
		if pane.InputReady {
			fmt.Printf("\tinput_ready:%t", pane.InputReady)
		}
		if pane.InteractivePrompt != nil {
			fmt.Printf("\tprompt:%s", pane.InteractivePrompt.Type)
		}
		if pane.CurrentCommand != "" {
			fmt.Printf("\tcmd:%s", pane.CurrentCommand)
		}
		if pane.CWD != "" {
			fmt.Printf("\tcwd:%s", pane.CWD)
		}
		if pane.LastLine != "" {
			fmt.Printf("\t%s", pane.LastLine)
		}
		if pane.Error != "" {
			fmt.Printf("\terror:%s", pane.Error)
		}
		fmt.Println()
	}
}

func printInbox(inbox agents.Inbox) {
	fmt.Printf("ts: %s\n", inbox.Timestamp)
	if len(inbox.Items) == 0 {
		fmt.Println("inbox: empty")
		return
	}
	for _, item := range inbox.Items {
		fmt.Printf("%s\t%s\t%s\t%s", item.Agent, item.Target, item.Kind, item.Severity)
		if item.Summary != "" {
			fmt.Printf("\t%s", item.Summary)
		}
		fmt.Println()
	}
}

func printSummary(report agents.SummaryReport) {
	fmt.Printf("ts: %s\n", report.Timestamp)
	for _, summary := range report.Summaries {
		fmt.Printf("\n%s\t%s\t%s\n", summary.Name, summary.Target, summary.State)
		if summary.Role != "" || summary.Type != "" {
			fmt.Printf("role: %s\ttype: %s\n", summary.Role, summary.Type)
		}
		if summary.Git != nil {
			printGitSummary(summary.Git)
		}
		if summary.Error != "" {
			fmt.Printf("error: %s\n", summary.Error)
		}
		if summary.NextAction != "" {
			fmt.Printf("next: %s\n", summary.NextAction)
		}
		if len(summary.LastLines) > 0 {
			fmt.Println("recent:")
			for _, line := range summary.LastLines {
				fmt.Printf("  %s\n", line)
			}
		}
	}
}

func printGitSummary(git *agents.GitSummary) {
	if git.Error != "" {
		fmt.Printf("git: %s\n", git.Error)
		return
	}
	dirty := "clean"
	if git.Dirty {
		dirty = "dirty"
	}
	if git.Branch != "" {
		fmt.Printf("git: %s/%s\n", git.Branch, dirty)
	} else {
		fmt.Printf("git: %s\n", dirty)
	}
	if len(git.ChangedFiles) > 0 {
		fmt.Printf("changed: %d files\n", len(git.ChangedFiles))
	}
	if len(git.RecentCommits) > 0 {
		fmt.Println("commits:")
		for _, commit := range git.RecentCommits {
			fmt.Printf("  %s %s\n", commit.Hash, commit.Subject)
		}
	}
}

func printBroadcast(report agents.BroadcastReport) {
	fmt.Printf("ts: %s\n", report.Timestamp)
	if report.DryRun {
		fmt.Println("mode: dry-run")
	} else {
		fmt.Println("mode: execute")
	}
	for _, result := range report.Results {
		fmt.Printf("%s\t%s\t%s", result.Agent, result.Target, result.Status)
		if result.State != "" {
			fmt.Printf("\tstate:%s", result.State)
		}
		if result.Reason != "" {
			fmt.Printf("\treason:%s", result.Reason)
		}
		if result.Error != "" {
			fmt.Printf("\terror:%s", result.Error)
		}
		fmt.Println()
	}
}

func printPanelReport(report agents.PanelReport) {
	fmt.Printf("ts: %s\n", report.Timestamp)
	if report.DryRun {
		fmt.Println("mode: dry-run")
	} else {
		fmt.Println("mode: execute")
	}
	for _, op := range report.Operations {
		fmt.Printf("%s\t%s\t%s\t%s", op.Agent, op.Action, op.Target, op.Status)
		if len(op.Command) > 0 {
			fmt.Printf("\tcmd:%s", strings.Join(op.Command, " "))
		}
		if op.Error != "" {
			fmt.Printf("\terror:%s", op.Error)
		}
		fmt.Println()
	}
}

func usage() error {
	fmt.Fprint(os.Stderr, usageText())
	return nil
}

func usageText() string {
	return `tmact minimal POC

Usage:
  tmact ls [--json]
  tmact -t 0 send --command "go test ./..." [--execute]
  tmact -t 0 send --text "summarize progress" [--enter] [--execute]
  tmact -t 0 send --key Enter [--execute]
  tmact -t 0 send --keys C-u,Enter [--execute]
  tmact detect [--target z_sample-project_sample:0.0] [--lines 120] [--json]
  tmact inspect [--target z_sample-project:0.0 | --session z_sample-project | --all] [--sample 2 --interval 1s] [--json]
  tmact status [--config examples/agents.yaml] [--agent z-sample-project] [--role library-maintenance] [--json]
  tmact statusd start|once|read|status [--state-path /tmp/tmact-status.json]
  tmact state get --path .agent-inbox/features/example/status.yaml [--json]
  tmact state set --path .agent-inbox/features/example/status.yaml --state planning [--owner OWNER] [--stage STAGE]
  tmact state transition --path .agent-inbox/features/example/status.yaml --from planning --to implementation
  tmact state event --path .agent-inbox/features/example/status.yaml --kind note [--message TEXT]
  tmact inbox [--config examples/agents.yaml] [--agent z-sample-project] [--role library-maintenance] [--json]
  tmact summarize [--config examples/agents.yaml] [--agent z-sample-project] [--json]
  tmact broadcast [--config examples/agents.yaml] --agent z-sample-project --text "summarize progress" [--enter] [--execute]
  tmact panels plan [--config examples/agents.yaml] [--session IDLL] [--json]
  tmact panels ensure [--config examples/agents.yaml] [--session IDLL] [--execute]
  tmact loop --config examples/night-loop.yaml [--dry-run] [--once] [--assume-idle-on-start]
  tmact watch --config examples/accept-question-watch.yaml [--dry-run] [--once]
  tmact workflow --config examples/simple-improvement-workflow.yaml [--dry-run] [--once] [--assume-idle-on-start] [--start-stage name]

Commands:
  ls        list tmux panes and cache numbered targets for -t
  send      send text, a command, or keys to a selected tmux target
  detect    capture a tmux pane and detect a directory-access prompt
  inspect   detect runtime and idle/running state for tmux panes
  status    summarize configured agent panes
  statusd   maintain a cached tmux pane status snapshot
  state     read and update agent-inbox workflow status files
  inbox     list agent panes that need human intervention
  summarize summarize recent pane and git activity
  broadcast safely send text to selected agent panes
  panels    plan or ensure configured agent tmux panels
  loop      run a configurable tmux automation loop
  watch     watch a pane and answer allowlisted prompts
  workflow  run a staged prompt workflow such as agent-inbox feature work
`
}
