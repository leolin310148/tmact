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
	"tmact/internal/runmeta"
	"tmact/internal/statusd"
	"tmact/internal/tmux"
	"tmact/internal/watch"
	"tmact/internal/workflow"
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

type helpFlag struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

type commandHelp struct {
	Command     string     `json:"command"`
	Summary     string     `json:"summary"`
	Usage       []string   `json:"usage,omitempty"`
	Subcommands []string   `json:"subcommands,omitempty"`
	Flags       []helpFlag `json:"flags,omitempty"`
	Examples    []string   `json:"examples,omitempty"`
	Safety      []string   `json:"safety,omitempty"`
	Notes       []string   `json:"notes,omitempty"`
}

type helpManifest struct {
	Name        string        `json:"name"`
	Summary     string        `json:"summary"`
	GlobalFlags []helpFlag    `json:"global_flags,omitempty"`
	Commands    []commandHelp `json:"commands"`
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
	case "workflow":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runWorkflow(args[1:])
	case "watch":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runWatch(args[1:])
	case "commands":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runCommands(args[1:])
	case "help":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runHelp(args[1:])
	case "-h", "--help":
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

func wantsHelp(args []string) bool {
	return len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help")
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

func runSend(args []string, globals globalOptions) error {
	if wantsHelp(args) {
		return printCommandHelp("send")
	}
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

func runDetect(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("detect")
	}
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

func runStatus(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("status")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("statusd")
	}
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
		return printCommandHelp("statusd")
	default:
		return fmt.Errorf("unknown statusd subcommand %q", args[0])
	}
}

func runStatusdStart(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("statusd start")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("statusd once")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("statusd read")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("statusd status")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("inbox")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("summarize")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("broadcast")
	}
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
	if wantsHelp(args) {
		return printCommandHelp("panels")
	}
	if len(args) == 0 {
		return errors.New("panels requires a subcommand: plan or ensure")
	}
	action := args[0]
	if action != "plan" && action != "ensure" {
		return fmt.Errorf("unknown panels subcommand %q", action)
	}
	if wantsHelp(args[1:]) {
		return printCommandHelp("panels " + action)
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
	if wantsHelp(args) {
		if len(args) > 1 {
			return printCommandHelp("loop " + strings.Join(args[1:], " "))
		}
		return printCommandHelp("loop")
	}
	if len(args) > 0 {
		switch args[0] {
		case "status":
			return runRuntimeStatus("loop", args[1:])
		case "stop":
			return runRuntimeStop("loop", args[1:])
		}
	}

	fs := flag.NewFlagSet("loop", flag.ContinueOnError)
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

	runner := loop.NewRunner(cfg, loop.Options{
		DryRun: *dryRun,
		Once:   *once,
	})
	if *once {
		return runner.Run(context.Background())
	}

	return runManagedRunner(*runDir, "loop", *configPath, cfg.Target, cfg.LogPath, func(ctx context.Context) error {
		return runner.Run(ctx)
	})
}

func runWorkflow(args []string) error {
	if wantsHelp(args) {
		if len(args) > 1 {
			return printCommandHelp("workflow " + strings.Join(args[1:], " "))
		}
		return printCommandHelp("workflow")
	}
	if len(args) == 0 {
		return errors.New("workflow requires a subcommand: discuss, status, stop")
	}
	switch args[0] {
	case "discuss":
		return runWorkflowDiscuss(args[1:])
	case "status":
		return runWorkflowStatus(args[1:])
	case "stop":
		return runRuntimeStop("workflow", args[1:])
	default:
		return fmt.Errorf("unknown workflow subcommand %q", args[0])
	}
}

func runWorkflowDiscuss(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow discuss")
	}
	fs := flag.NewFlagSet("workflow discuss", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	configPath := fs.String("config", "", "path to workflow YAML config")
	dryRun := fs.Bool("dry-run", false, "print the next prompt without sending to tmux; this is the default")
	execute := fs.Bool("execute", false, "send prompts to tmux panes")
	once := fs.Bool("once", false, "run one observe/validate/gate/prompt pass and exit")
	runDir := fs.String("run-dir", runmeta.DefaultDir, "directory for runtime metadata")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" {
		return errors.New("--config is required")
	}
	if *dryRun && *execute {
		return errors.New("--dry-run and --execute are mutually exclusive")
	}

	cfg, err := workflow.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	agentCfg, err := agents.LoadConfig(cfg.AgentsConfig)
	if err != nil {
		return err
	}
	bindings, err := workflow.ResolveRoles(cfg, agentCfg)
	if err != nil {
		return err
	}
	if *once {
		runner, err := workflow.NewRunner(cfg, agentCfg, workflow.Options{
			DryRun:     !*execute,
			Once:       true,
			ConfigPath: *configPath,
		})
		if err != nil {
			return err
		}
		return runner.Run(context.Background())
	}
	return runManagedWorkflowRunner(*runDir, *configPath, workflow.TargetSummary(bindings), cfg.LogPath, func(stopRequested func() bool) (*workflow.Runner, error) {
		return workflow.NewRunner(cfg, agentCfg, workflow.Options{
			DryRun:        !*execute,
			Once:          false,
			ConfigPath:    *configPath,
			StopRequested: stopRequested,
		})
	})
}

func runWorkflowStatus(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow status")
	}
	fs := flag.NewFlagSet("workflow status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	runDir := fs.String("run-dir", runmeta.DefaultDir, "directory for runtime metadata")
	configPath := fs.String("config", "", "workflow config path to include local state")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	statuses, err := runmeta.List(*runDir, "workflow", tmactNow())
	if err != nil {
		return err
	}
	report := struct {
		Runs  []runmeta.Status `json:"runs"`
		State *workflow.State  `json:"state,omitempty"`
	}{
		Runs: statuses,
	}
	if *configPath != "" {
		cfg, err := workflow.LoadConfig(*configPath)
		if err != nil {
			return err
		}
		changeDir, err := workflow.ChangeDir(cfg.Change)
		if err != nil {
			return err
		}
		state, err := workflow.LoadState(workflow.StatePath(changeDir))
		if err != nil {
			return err
		}
		report.State = &state
	}
	if *jsonOutput {
		return printJSON(report)
	}
	printRuntimeStatuses(statuses)
	if report.State != nil && report.State.Change != "" {
		printWorkflowState(*report.State)
	}
	return nil
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

func runManagedRunner(runDir string, kind string, configPath string, target string, logPath string, run func(context.Context) error) error {
	startedAt := tmactNow()
	record, err := runmeta.Register(runDir, runmeta.RegisterOptions{
		Kind:       kind,
		ConfigPath: configPath,
		Target:     target,
		LogPath:    logPath,
		Tmux:       currentTmuxInfo(),
		Now:        startedAt,
	})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err = run(ctx)
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

func runManagedWorkflowRunner(runDir string, configPath string, target string, logPath string, newRunner func(func() bool) (*workflow.Runner, error)) error {
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

func printRuntimeStatuses(statuses []runmeta.Status) {
	if len(statuses) == 0 {
		fmt.Println("no registered runs")
		return
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "id\tstatus\tpid\ttarget\tconfig\tlast")
	for _, status := range statuses {
		run := status.Run
		last := ""
		if status.LastEvent != nil {
			last = formatRuntimeEvent(*status.LastEvent)
		}
		fmt.Fprintf(writer, "%s\t%s\t%d\t%s\t%s\t%s\n", run.ID, status.RuntimeStatus, run.PID, run.Target, run.ConfigPath, last)
		if run.Tmux.PaneID != "" {
			window := run.Tmux.WindowName
			if window != "" {
				window = fmt.Sprintf("%d:%s", run.Tmux.WindowIndex, window)
			}
			fmt.Fprintf(writer, "\t\t\tpane:%s\t%s\t%s\n", run.Tmux.PaneID, run.Tmux.Session, window)
		}
		for _, problem := range status.RecentProblems {
			fmt.Fprintf(writer, "\tproblem\t\t\t\t%s\n", formatRuntimeEvent(problem))
		}
	}
	_ = writer.Flush()
}

func printWorkflowState(state workflow.State) {
	fmt.Println()
	fmt.Printf("workflow_state: %s\n", state.Change)
	fmt.Printf("status: %s\n", state.Status)
	if state.Outcome != "" {
		fmt.Printf("outcome: %s\n", state.Outcome)
	}
	if state.Reason != "" {
		fmt.Printf("reason: %s\n", state.Reason)
	}
	fmt.Printf("phase: %s\n", state.Phase)
	fmt.Printf("turn: %d\n", state.Turn)
	if state.PendingRole != "" {
		fmt.Printf("pending_role: %s\n", state.PendingRole)
	}
	if state.ChangeHash != "" {
		fmt.Printf("change_hash: %s\n", state.ChangeHash)
	}
	if state.LastValidation != nil {
		fmt.Printf("openspec_valid: %t\n", state.LastValidation.Passed)
		if state.LastValidation.Stale {
			fmt.Println("openspec_validation: stale")
		}
	}
	if len(state.Gate.Reasons) > 0 {
		fmt.Printf("gate_reasons: %s\n", strings.Join(state.Gate.Reasons, ","))
	}
}

func formatRuntimeEvent(event runmeta.EventSummary) string {
	parts := []string{}
	if !event.Timestamp.IsZero() {
		parts = append(parts, event.Timestamp.Format(time.RFC3339))
	}
	if event.Type != "" {
		parts = append(parts, event.Type)
	}
	if event.Stage != "" {
		parts = append(parts, "stage:"+event.Stage)
	}
	if event.Action != "" {
		parts = append(parts, "action:"+event.Action)
	}
	if event.Cycle > 0 {
		parts = append(parts, fmt.Sprintf("cycle:%d", event.Cycle))
	}
	if event.Status != "" {
		parts = append(parts, "status:"+event.Status)
	}
	if event.Reason != "" {
		parts = append(parts, "reason:"+event.Reason)
	}
	return strings.Join(parts, " ")
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

func runCommands(args []string) error {
	if wantsHelp(args) {
		fmt.Print(`Usage:
  tmact commands [--json]

Print the command catalog. Use --json when another program or LLM needs a
machine-readable list of commands, flags, examples, and safety notes.
`)
		return nil
	}
	fs := flag.NewFlagSet("commands", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(commandManifest())
	}
	printCommandTable(commandManifest().Commands)
	return nil
}

func runHelp(args []string) error {
	jsonOutput := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		filtered = append(filtered, arg)
	}
	if len(filtered) == 0 || wantsHelp(filtered) {
		if jsonOutput {
			return printJSON(commandManifest())
		}
		return usage()
	}
	name := strings.Join(filtered, " ")
	if jsonOutput {
		help, ok := commandHelpFor(name)
		if !ok {
			return fmt.Errorf("unknown help topic %q", name)
		}
		return printJSON(help)
	}
	return printCommandHelp(name)
}

func usage() error {
	fmt.Print(usageText())
	return nil
}

func usageText() string {
	return `tmact - local tmux automation for agent panes

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
  tmact inbox [--config examples/agents.yaml] [--agent z-sample-project] [--role library-maintenance] [--json]
  tmact summarize [--config examples/agents.yaml] [--agent z-sample-project] [--json]
  tmact broadcast [--config examples/agents.yaml] --agent z-sample-project --text "summarize progress" [--enter] [--execute]
  tmact panels plan [--config examples/agents.yaml] [--session IDLL] [--json]
  tmact panels ensure [--config examples/agents.yaml] [--session IDLL] [--execute]
  tmact loop --config examples/night-loop.yaml [--dry-run] [--once] [--assume-idle-on-start]
  tmact loop status [--run-dir .tmact/runs] [--json]
  tmact loop stop (--id ID | --config path)
  tmact workflow discuss --config examples/openspec-workflow.yaml [--dry-run] [--once] [--execute]
  tmact workflow status [--config examples/openspec-workflow.yaml] [--json]
  tmact workflow stop (--id ID | --config path)
  tmact watch --config examples/accept-question-watch.yaml [--dry-run] [--once]
  tmact help [command] [--json]
  tmact commands [--json]

Commands:
  ls        list tmux panes and cache numbered targets for -t
  send      send text, a command, or keys to a selected tmux target
  detect    capture a tmux pane and detect a directory-access prompt
  inspect   detect runtime and idle/running state for tmux panes
  status    summarize configured agent panes
  statusd   maintain a cached tmux pane status snapshot
  inbox     list agent panes that need human intervention
  summarize summarize recent pane and git activity
  broadcast safely send text to selected agent panes
  panels    plan or ensure configured agent tmux panels
  loop      run, inspect, or stop a configurable tmux automation loop
  workflow  run, inspect, or stop a serialized OpenSpec artifact review
  watch     watch a pane and answer allowlisted prompts
  commands  print a machine-readable command catalog for tools and LLMs

Safety:
  send, broadcast, and panels ensure default to dry-run. For loop and watch,
  validate with --dry-run --once before running a live automation.

More help:
  tmact help loop
  tmact help loop status
  tmact commands --json
`
}

func printCommandHelp(name string) error {
	help, ok := commandHelpFor(name)
	if !ok {
		return fmt.Errorf("unknown help topic %q", name)
	}
	fmt.Printf("%s\n\n%s\n", help.Command, help.Summary)
	if len(help.Usage) > 0 {
		fmt.Println("\nUsage:")
		for _, usage := range help.Usage {
			fmt.Printf("  %s\n", usage)
		}
	}
	if len(help.Subcommands) > 0 {
		fmt.Println("\nSubcommands:")
		for _, subcommand := range help.Subcommands {
			fmt.Printf("  %s\n", subcommand)
		}
	}
	if len(help.Flags) > 0 {
		fmt.Println("\nFlags:")
		writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, flag := range help.Flags {
			name := flag.Name
			if flag.Value != "" {
				name += " " + flag.Value
			}
			required := ""
			if flag.Required {
				required = " required"
			}
			fmt.Fprintf(writer, "  %s\t%s%s\n", name, flag.Description, required)
		}
		_ = writer.Flush()
	}
	if len(help.Examples) > 0 {
		fmt.Println("\nExamples:")
		for _, example := range help.Examples {
			fmt.Printf("  %s\n", example)
		}
	}
	if len(help.Safety) > 0 {
		fmt.Println("\nSafety:")
		for _, note := range help.Safety {
			fmt.Printf("  %s\n", note)
		}
	}
	if len(help.Notes) > 0 {
		fmt.Println("\nNotes:")
		for _, note := range help.Notes {
			fmt.Printf("  %s\n", note)
		}
	}
	return nil
}

func printCommandTable(commands []commandHelp) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "command\tsummary")
	for _, command := range commands {
		if strings.Contains(command.Command, " ") {
			continue
		}
		fmt.Fprintf(writer, "%s\t%s\n", command.Command, command.Summary)
	}
	_ = writer.Flush()
}

func commandHelpFor(name string) (commandHelp, bool) {
	normalized := strings.Join(strings.Fields(name), " ")
	for _, help := range commandHelpCatalog() {
		if help.Command == normalized {
			return help, true
		}
	}
	return commandHelp{}, false
}

func commandManifest() helpManifest {
	return helpManifest{
		Name:    "tmact",
		Summary: "Local tmux automation CLI for inspecting panes, sending guarded input, and running loop daemons.",
		GlobalFlags: []helpFlag{
			{Name: "-t, --target", Value: "TARGET", Description: "target selector for send; may be a tmux target or a numbered index from tmact ls"},
		},
		Commands: commandHelpCatalog(),
	}
}

func commandHelpCatalog() []commandHelp {
	return []commandHelp{
		{
			Command: "ls",
			Summary: "List tmux panes and refresh the numbered target cache used by -t.",
			Usage:   []string{"tmact ls [--json]"},
			Flags: []helpFlag{
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact ls", "tmact ls --json"},
			Notes:    []string{"Run this before using a numeric target such as -t 0."},
		},
		{
			Command: "send",
			Summary: "Send text, a command, or tmux keys to one selected pane.",
			Usage: []string{
				`tmact -t TARGET send --text TEXT [--enter] [--clear-line] [--execute]`,
				`tmact -t TARGET send --command COMMAND [--clear-line] [--execute]`,
				`tmact -t TARGET send --key KEY [--key KEY...] [--execute]`,
				`tmact -t TARGET send --keys C-u,Enter [--execute]`,
			},
			Flags: []helpFlag{
				{Name: "--text", Value: "TEXT", Description: "text to paste without Enter unless --enter is set"},
				{Name: "--command", Value: "COMMAND", Description: "command to paste followed by Enter"},
				{Name: "--key", Value: "KEY", Description: "tmux key to send; may be repeated"},
				{Name: "--keys", Value: "CSV", Description: "comma-separated tmux keys"},
				{Name: "--enter", Description: "press Enter after --text"},
				{Name: "--clear-line", Description: "send C-u before text or command"},
				{Name: "--execute", Description: "actually send to tmux; default is dry-run"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{
				`tmact ls`,
				`tmact -t 0 send --command "go test ./..."`,
				`tmact -t work:0.0 send --text "summarize progress" --enter --execute`,
			},
			Safety: []string{"Without --execute this prints the planned send and does not touch tmux."},
		},
		{
			Command: "detect",
			Summary: "Capture a pane and detect a directory-access prompt.",
			Usage:   []string{"tmact detect [--target TARGET] [--lines 120] [--json]"},
			Flags: []helpFlag{
				{Name: "--target", Value: "TARGET", Description: "tmux target pane/window/session to capture"},
				{Name: "--lines", Value: "N", Description: "number of pane history lines to capture"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact detect --target session:0.0 --json"},
		},
		{
			Command: "inspect",
			Summary: "Classify tmux panes by runtime and idle/running/asking state.",
			Usage:   []string{"tmact inspect [--target TARGET | --session SESSION [--window WINDOW] | --all] [--sample 2 --interval 1s] [--json]"},
			Flags: []helpFlag{
				{Name: "--target", Value: "TARGET", Description: "tmux target pane/window to inspect"},
				{Name: "--session", Value: "SESSION", Description: "tmux session to inspect"},
				{Name: "--window", Value: "WINDOW", Description: "tmux window to inspect; combine with --session to avoid ambiguity"},
				{Name: "--all", Description: "inspect every tmux pane"},
				{Name: "--lines", Value: "N", Description: "number of pane history lines to capture"},
				{Name: "--sample", Value: "N", Description: "number of captures per pane for idle/running detection"},
				{Name: "--interval", Value: "DURATION", Description: "delay between samples, for example 1s"},
				{Name: "--idle-ignore", Value: "REGEXP", Description: "line regexp ignored by sample hashing; may be repeated"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact inspect --all", "tmact inspect --target session:0.0 --sample 2 --interval 1s --json"},
			Notes:    []string{"This inspects tmux panes. Use tmact loop status to inspect registered loop daemons."},
		},
		{
			Command: "status",
			Summary: "Summarize configured agent panes from agents.yaml.",
			Usage:   []string{"tmact status [--config examples/agents.yaml] [--agent NAME] [--role ROLE] [--json]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to agent registry YAML config"},
				{Name: "--agent", Value: "NAME", Description: "agent name to include"},
				{Name: "--role", Value: "ROLE", Description: "role to include"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact status --config examples/agents.yaml", "tmact status --agent z-sample-project --json"},
		},
		{
			Command:     "statusd",
			Summary:     "Maintain or read a cached tmux pane status snapshot.",
			Usage:       []string{"tmact statusd start|once|read|status [flags]"},
			Subcommands: []string{"start", "once", "read", "status"},
			Examples:    []string{"tmact statusd once --json", "tmact statusd start --interval 1s --state-path /tmp/tmact-status.json"},
			Notes:       []string{"Use tmact help statusd start for daemon flags."},
		},
		{
			Command:  "statusd start",
			Summary:  "Run the pane status daemon until interrupted.",
			Usage:    []string{"tmact statusd start [--interval 1s] [--state-path PATH] [--no-tmux-options]"},
			Flags:    statusdStartHelpFlags(),
			Examples: []string{"tmact statusd start --interval 1s", "tmact statusd start --once --json"},
		},
		{
			Command:  "statusd once",
			Summary:  "Run one statusd scan and exit.",
			Usage:    []string{"tmact statusd once [--json] [--state-path PATH] [--initial-samples 2]"},
			Flags:    statusdHelpFlags(),
			Examples: []string{"tmact statusd once", "tmact statusd once --json"},
		},
		{
			Command: "statusd read",
			Summary: "Read the latest statusd JSON snapshot from disk.",
			Usage:   []string{"tmact statusd read [--json] [--state-path PATH]"},
			Flags: []helpFlag{
				{Name: "--state-path", Value: "PATH", Description: "latest JSON snapshot path"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact statusd read --state-path /tmp/tmact-status.json"},
		},
		{
			Command: "statusd status",
			Summary: "Print statusd snapshot freshness and summary counts.",
			Usage:   []string{"tmact statusd status [--json] [--state-path PATH]"},
			Flags: []helpFlag{
				{Name: "--state-path", Value: "PATH", Description: "latest JSON snapshot path"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact statusd status"},
		},
		{
			Command:  "inbox",
			Summary:  "List configured agent panes that need human intervention.",
			Usage:    []string{"tmact inbox [--config examples/agents.yaml] [--agent NAME] [--role ROLE] [--json]"},
			Flags:    agentFilterHelpFlags(),
			Examples: []string{"tmact inbox", "tmact inbox --role library-maintenance --json"},
		},
		{
			Command: "summarize",
			Summary: "Summarize recent pane output and git activity for configured agents.",
			Usage:   []string{"tmact summarize [--config examples/agents.yaml] [--agent NAME] [--lines 12] [--commits 5] [--json]"},
			Flags: append(agentSummaryHelpFlags(),
				helpFlag{Name: "--lines", Value: "N", Description: "number of recent pane lines to include"},
				helpFlag{Name: "--commits", Value: "N", Description: "number of recent git commits to include"},
			),
			Examples: []string{"tmact summarize --agent z-sample-project", "tmact summarize --json"},
		},
		{
			Command: "broadcast",
			Summary: "Safely send text to selected configured agent panes.",
			Usage:   []string{`tmact broadcast [--config examples/agents.yaml] (--agent NAME | --role ROLE | --all) --text TEXT [--enter] [--only-idle] [--execute] [--json]`},
			Flags: append(agentFilterHelpFlags(),
				helpFlag{Name: "--all", Description: "send to every configured agent"},
				helpFlag{Name: "--text", Value: "TEXT", Description: "text to send", Required: true},
				helpFlag{Name: "--enter", Description: "press Enter after sending text"},
				helpFlag{Name: "--only-idle", Description: "skip agents that do not appear idle"},
				helpFlag{Name: "--execute", Description: "actually send text to tmux; default is dry-run"},
			),
			Examples: []string{`tmact broadcast --agent z-sample-project --text "summarize progress"`, `tmact broadcast --all --text "status?" --enter --only-idle --execute`},
			Safety:   []string{"Without --execute this prints the planned sends and does not touch tmux."},
		},
		{
			Command:     "panels",
			Summary:     "Plan or reconcile configured agent panes in tmux.",
			Usage:       []string{"tmact panels plan [flags]", "tmact panels ensure [flags]"},
			Subcommands: []string{"plan", "ensure"},
			Examples:    []string{"tmact panels plan --config examples/agents.yaml", "tmact panels ensure --session IDLL --execute"},
			Safety:      []string{"panels plan never changes tmux. panels ensure requires --execute before it applies changes."},
		},
		{
			Command: "panels plan",
			Summary: "Print the tmux panel operations that would be needed.",
			Usage:   []string{"tmact panels plan [--config examples/agents.yaml] [--agent NAME] [--role ROLE] [--session SESSION] [--json]"},
			Flags: append(agentFilterHelpFlags(),
				helpFlag{Name: "--session", Value: "SESSION", Description: "override target tmux session for selected agents"},
			),
			Examples: []string{"tmact panels plan --json"},
		},
		{
			Command: "panels ensure",
			Summary: "Reconcile configured tmux panes, optionally executing the plan.",
			Usage:   []string{"tmact panels ensure [--config examples/agents.yaml] [--agent NAME] [--role ROLE] [--session SESSION] [--execute] [--json]"},
			Flags: append(agentFilterHelpFlags(),
				helpFlag{Name: "--session", Value: "SESSION", Description: "override target tmux session for selected agents"},
				helpFlag{Name: "--execute", Description: "apply planned tmux panel changes"},
			),
			Examples: []string{"tmact panels ensure --session IDLL", "tmact panels ensure --session IDLL --execute"},
			Safety:   []string{"Without --execute this prints the planned changes and does not touch tmux."},
		},
		{
			Command:     "loop",
			Summary:     "Run, inspect, or stop a configurable single-pane automation loop.",
			Usage:       []string{"tmact loop --config PATH [--dry-run] [--once] [--assume-idle-on-start]", "tmact loop status [--run-dir .tmact/runs] [--json]", "tmact loop stop (--id ID | --config PATH)"},
			Subcommands: []string{"status", "stop"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to loop YAML config", Required: true},
				{Name: "--dry-run", Description: "print actions without sending anything to tmux"},
				{Name: "--once", Description: "run one observe/action pass and exit"},
				{Name: "--assume-idle-on-start", Description: "treat the pane as already idle when the loop starts"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			},
			Examples: []string{"tmact loop --config examples/night-loop.yaml --dry-run --once", "tmact loop status", "tmact loop stop --config examples/night-loop.yaml"},
			Safety:   []string{"Loops should stop on permission prompts rather than auto-confirming them. Validate with --dry-run --once first."},
			Notes:    []string{"Long-running loop metadata is stored under .tmact/runs by default."},
		},
		runtimeStatusHelp("loop"),
		runtimeStopHelp("loop"),
		{
			Command:     "workflow",
			Summary:     "Run, inspect, or stop a serialized OpenSpec artifact review workflow.",
			Usage:       []string{"tmact workflow discuss --config PATH [--dry-run] [--once] [--execute]", "tmact workflow status [--config PATH] [--run-dir .tmact/runs] [--json]", "tmact workflow stop (--id ID | --config PATH)"},
			Subcommands: []string{"discuss", "status", "stop"},
			Examples:    []string{"tmact workflow discuss --config examples/openspec-workflow.yaml --dry-run --once", "tmact workflow status --config examples/openspec-workflow.yaml", "tmact workflow stop --config examples/openspec-workflow.yaml"},
			Safety:      []string{"Workflow prompts are dry-run by default. Use --execute only after inspecting the planned prompt and target roles."},
			Notes:       []string{"v1 uses serialized PM -> SWE -> QA -> reviewer review and gates on the full OpenSpec change hash."},
		},
		{
			Command: "workflow discuss",
			Summary: "Run one or more serialized OpenSpec artifact review passes.",
			Usage:   []string{"tmact workflow discuss --config PATH [--dry-run] [--once] [--execute]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to workflow YAML config", Required: true},
				{Name: "--dry-run", Description: "print planned prompts without sending to tmux; default behavior"},
				{Name: "--execute", Description: "send prompts to configured tmux panes"},
				{Name: "--once", Description: "run one observe/validate/gate/prompt pass and exit"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			},
			Examples: []string{"tmact workflow discuss --config examples/openspec-workflow.yaml --dry-run --once", "tmact workflow discuss --config examples/openspec-workflow.yaml --execute"},
			Safety:   []string{"The workflow stops on permission prompts and does not auto-approve tools or filesystem access."},
		},
		{
			Command: "workflow status",
			Summary: "Inspect workflow run metadata and optional local OpenSpec workflow state.",
			Usage:   []string{"tmact workflow status [--config PATH] [--run-dir .tmact/runs] [--json]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "workflow config path; include phase1-state.json details"},
				{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
				{Name: "--json", Description: "print JSON output"},
			},
			Examples: []string{"tmact workflow status", "tmact workflow status --config examples/openspec-workflow.yaml --json"},
		},
		runtimeStopHelp("workflow"),
		{
			Command: "watch",
			Summary: "Run a narrow prompt watcher for allowlisted answers.",
			Usage:   []string{"tmact watch --config PATH [--dry-run] [--once]"},
			Flags: []helpFlag{
				{Name: "--config", Value: "PATH", Description: "path to watch YAML config", Required: true},
				{Name: "--dry-run", Description: "print decisions without sending anything to tmux"},
				{Name: "--once", Description: "run one watch pass and exit"},
			},
			Examples: []string{"tmact watch --config examples/accept-question-watch.yaml --dry-run --once"},
			Safety:   []string{"Watcher configs must keep allow_paths or allow_path_patterns checks in place."},
		},
		{
			Command: "commands",
			Summary: "Print the command catalog for humans or LLM/tooling consumers.",
			Usage:   []string{"tmact commands [--json]"},
			Flags: []helpFlag{
				{Name: "--json", Description: "print machine-readable command metadata"},
			},
			Examples: []string{"tmact commands", "tmact commands --json", "tmact help loop --json"},
		},
	}
}

func statusdHelpFlags() []helpFlag {
	return []helpFlag{
		{Name: "--interval", Value: "DURATION", Description: "scan interval"},
		{Name: "--state-path", Value: "PATH", Description: "latest JSON snapshot path"},
		{Name: "--log-path", Value: "PATH", Description: "optional JSONL daemon log path"},
		{Name: "--tmux-options", Description: "write @ai-* tmux options"},
		{Name: "--no-tmux-options", Description: "only write the state file"},
		{Name: "--capture-lines", Value: "N", Description: "number of pane history lines to capture"},
		{Name: "--initial-samples", Value: "N", Description: "captures per pane before statusd has history"},
		{Name: "--running-debounce", Value: "DURATION", Description: "keep running indicator after changes"},
		{Name: "--stale-after", Value: "DURATION", Description: "mark snapshot stale after this age"},
		{Name: "--idle-ignore", Value: "REGEXP", Description: "line regexp ignored by sample hashing; may be repeated"},
		{Name: "--session", Value: "GLOB", Description: "include sessions matching glob; may be repeated"},
		{Name: "--exclude-session", Value: "GLOB", Description: "exclude sessions matching glob; may be repeated"},
		{Name: "--json", Description: "print JSON output where supported"},
	}
}

func statusdStartHelpFlags() []helpFlag {
	return append([]helpFlag{
		{Name: "--once", Description: "run one scan then exit"},
	}, statusdHelpFlags()...)
}

func agentFilterHelpFlags() []helpFlag {
	return []helpFlag{
		{Name: "--config", Value: "PATH", Description: "path to agent registry YAML config"},
		{Name: "--agent", Value: "NAME", Description: "agent name to include"},
		{Name: "--role", Value: "ROLE", Description: "role to include"},
		{Name: "--json", Description: "print JSON output"},
	}
}

func agentSummaryHelpFlags() []helpFlag {
	return []helpFlag{
		{Name: "--config", Value: "PATH", Description: "path to agent registry YAML config"},
		{Name: "--agent", Value: "NAME", Description: "agent name to summarize; omit for all agents"},
		{Name: "--json", Description: "print JSON output"},
	}
}

func runtimeStatusHelp(kind string) commandHelp {
	return commandHelp{
		Command:  kind + " status",
		Summary:  "Inspect registered " + kind + " run metadata.",
		Usage:    []string{"tmact " + kind + " status [--run-dir .tmact/runs] [--json]"},
		Flags:    []helpFlag{{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"}, {Name: "--json", Description: "print JSON output"}},
		Examples: []string{"tmact " + kind + " status", "tmact " + kind + " status --json"},
		Notes:    []string{"Shows id, runtime status, pid, target, config path, last event, tmux pane, and recent problems."},
	}
}

func runtimeStopHelp(kind string) commandHelp {
	sampleID := kind + "-night-loop-123"
	sampleConfig := "examples/night-loop.yaml"
	return commandHelp{
		Command: kind + " stop",
		Summary: "Stop a registered " + kind + " by id or config path.",
		Usage:   []string{"tmact " + kind + " stop (--id ID | --config PATH) [--run-dir .tmact/runs] [--json]"},
		Flags: []helpFlag{
			{Name: "--id", Value: "ID", Description: "runtime id to stop"},
			{Name: "--config", Value: "PATH", Description: "stop the runtime registered for this config"},
			{Name: "--run-dir", Value: "PATH", Description: "directory for runtime metadata"},
			{Name: "--json", Description: "print JSON output"},
		},
		Examples: []string{"tmact " + kind + " stop --id " + sampleID, "tmact " + kind + " stop --config " + sampleConfig},
		Safety:   []string{"Stops by sending C-c to the recorded tmux pane when available, otherwise interrupts the recorded process."},
	}
}
