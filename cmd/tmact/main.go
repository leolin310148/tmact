package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/leolin310148/tmact/internal/agents"
	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/runmeta"
	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
	"github.com/leolin310148/tmact/internal/workflow"
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

// version is the tmact build version. It defaults to "dev" and can be
// overridden at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

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
	case "stt-set":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runSTTSet(args[1:])
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
	case "dispatch-work":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runDispatch(args[1:])
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
	case "version", "-v", "--version", "-version":
		if globals.Target != "" {
			return errors.New("global -t/--target is currently supported with send")
		}
		return runVersion(args[1:])
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

func printDispatchReport(report dispatch.Report) {
	prefix := ""
	if !report.Execute {
		prefix = "dry-run: "
	}
	fmt.Printf("%sdispatch-work %s [agent=%s dir=%s]\n", prefix, report.Session, report.Agent, report.Dir)
	if report.Target != "" {
		fmt.Printf("  target: %s\n", report.Target)
	}
	fmt.Printf("  session existed: %t  agent already running: %t\n", report.SessionExisted, report.AgentWasRunning)
	for _, step := range report.Steps {
		line := fmt.Sprintf("  [%s] %s", step.Status, step.Name)
		if step.Detail != "" {
			line += " - " + step.Detail
		}
		fmt.Println(line)
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

func printImplementationWorkflowState(state workflow.ImplementationState) {
	fmt.Println()
	fmt.Printf("implementation_state: %s\n", state.Change)
	fmt.Printf("status: %s\n", state.Status)
	if state.Outcome != "" {
		fmt.Printf("outcome: %s\n", state.Outcome)
	}
	if state.Reason != "" {
		fmt.Printf("reason: %s\n", state.Reason)
	}
	fmt.Printf("phase: %s\n", state.Phase)
	fmt.Printf("turn: %d\n", state.Turn)
	if state.PendingStage != "" {
		fmt.Printf("pending_stage: %s\n", state.PendingStage)
	}
	if state.PendingRole != "" {
		fmt.Printf("pending_role: %s\n", state.PendingRole)
	}
	if state.AcceptedChangeHash != "" {
		fmt.Printf("accepted_change_hash: %s\n", state.AcceptedChangeHash)
	}
	if state.CurrentChangeHash != "" {
		fmt.Printf("current_change_hash: %s\n", state.CurrentChangeHash)
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

func runDispatch(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("dispatch-work")
	}

	var session string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		session = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("dispatch-work", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dir := fs.String("dir", "", "working directory; sets cwd when the session is created")
	agent := fs.String("agent", "", "agent to launch: "+strings.Join(dispatch.SupportedAgents(), "|"))
	promptText := fs.String("prompt", "", "prompt text to send to the agent")
	readyTimeout := fs.Duration("ready-timeout", 30*time.Second, "max wait for the agent to become ready")
	readySettle := fs.Duration("ready-settle", dispatch.DefaultReadySettleDelay, "stable idle time after ready before sending the prompt")
	execute := fs.Bool("execute", false, "actually create, launch, and send; default is dry-run")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if session == "" && fs.NArg() > 0 {
		session = fs.Arg(0)
	}
	if session == "" {
		return errors.New("dispatch-work requires a session name as the first argument")
	}
	if *dir == "" {
		return errors.New("dispatch-work requires --dir")
	}
	if *agent == "" {
		return errors.New("dispatch-work requires --agent")
	}
	if *promptText == "" {
		return errors.New("dispatch-work requires --prompt")
	}

	report, err := dispatch.Run(dispatch.Options{
		Session:      session,
		Dir:          *dir,
		Agent:        *agent,
		Prompt:       *promptText,
		Execute:      *execute,
		ReadyTimeout: *readyTimeout,
		ReadySettle:  *readySettle,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(report)
	}
	printDispatchReport(report)
	return nil
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

type versionInfo struct {
	Version   string `json:"version"`
	Revision  string `json:"revision,omitempty"`
	Time      string `json:"time,omitempty"`
	Modified  bool   `json:"modified,omitempty"`
	GoVersion string `json:"go_version,omitempty"`
}

func buildVersionInfo() versionInfo {
	info := versionInfo{Version: version}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	info.GoVersion = bi.GoVersion
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.time":
			info.Time = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	return info
}

func (v versionInfo) String() string {
	var b strings.Builder
	b.WriteString("tmact ")
	b.WriteString(v.Version)
	if v.Revision != "" {
		rev := v.Revision
		if len(rev) > 12 {
			rev = rev[:12]
		}
		b.WriteString(" (")
		b.WriteString(rev)
		if v.Modified {
			b.WriteString("-dirty")
		}
		b.WriteString(")")
	}
	if v.Time != "" {
		b.WriteString(" built ")
		b.WriteString(v.Time)
	}
	if v.GoVersion != "" {
		b.WriteString(" with ")
		b.WriteString(v.GoVersion)
	}
	return b.String()
}

func runVersion(args []string) error {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "-h", "--help", "help":
			fmt.Print(`Usage:
  tmact version [--json]
  tmact -v | --version

Print the tmact build version. When the binary was built from a Git
checkout, the VCS revision, commit time, and dirty flag are included.
`)
			return nil
		default:
			return fmt.Errorf("unknown version flag %q", arg)
		}
	}
	info := buildVersionInfo()
	if jsonOutput {
		return printJSON(info)
	}
	fmt.Println(info.String())
	return nil
}
