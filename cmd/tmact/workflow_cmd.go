package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/leolin310148/tmact/internal/workflow"
	"gopkg.in/yaml.v3"
)

const workflowSupervisorSession = "tmact-workflows"

var workflowWindowRE = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)
var workflowProcessAlive = processAlive

func runWorkflow(args []string) error {
	if wantsHelp(args) {
		topic := "workflow"
		if len(args) > 1 {
			topic += " " + strings.Join(args[1:], " ")
		}
		return printCommandHelp(topic)
	}
	if len(args) == 0 {
		return errors.New("workflow requires a subcommand: example, validate, plan, run, start, status, logs, pause, resume, retry, resolve, report, stop")
	}
	switch args[0] {
	case "example":
		return runWorkflowExample(args[1:])
	case "validate":
		return runWorkflowValidate(args[1:])
	case "plan":
		return runWorkflowPlan(args[1:])
	case "run":
		return runWorkflowRun(args[1:])
	case "start":
		return runWorkflowStart(args[1:])
	case "status":
		return runWorkflowStatus(args[1:])
	case "logs":
		return runWorkflowLogs(args[1:])
	case "pause":
		return runWorkflowControl(args[1:], "paused")
	case "resume":
		return runWorkflowControl(args[1:], "running")
	case "retry":
		return runWorkflowRetry(args[1:])
	case "resolve":
		return runWorkflowResolve(args[1:])
	case "report":
		return runWorkflowReport(args[1:])
	case "stop":
		return runWorkflowStop(args[1:])
	default:
		return fmt.Errorf("unknown workflow subcommand %q", args[0])
	}
}

func runWorkflowExample(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow example")
	}
	fs := flag.NewFlagSet("workflow example", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	profile := fs.String("profile", "", "built-in profile (openspec)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("workflow example does not accept positional arguments")
	}
	switch *profile {
	case "":
		fmt.Print(workflowGenericExampleYAML)
	case "openspec":
		fmt.Print(workflowOpenSpecProfileYAML)
	default:
		return fmt.Errorf("unknown workflow profile %q", *profile)
	}
	return nil
}

type workflowConfigFlags struct {
	config *string
	vars   *repeatedStrings
	json   *bool
}

func addWorkflowConfigFlags(fs *flag.FlagSet, jsonFlag bool) workflowConfigFlags {
	f := workflowConfigFlags{config: fs.String("config", "", "workflow YAML config")}
	var values repeatedStrings
	fs.Var(&values, "var", "typed variable assignment key=value (repeatable)")
	f.vars = &values
	if jsonFlag {
		f.json = fs.Bool("json", false, "print JSON output")
	}
	return f
}
func loadWorkflowFlags(f workflowConfigFlags) (workflow.Loaded, error) {
	if *f.config == "" {
		return workflow.Loaded{}, errors.New("--config is required")
	}
	vars, err := workflow.ParseAssignments(*f.vars)
	if err != nil {
		return workflow.Loaded{}, err
	}
	return workflow.Load(*f.config, vars)
}

func runWorkflowValidate(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow validate")
	}
	fs := flag.NewFlagSet("workflow validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	f := addWorkflowConfigFlags(fs, true)
	if err := fs.Parse(args); err != nil {
		return err
	}
	loaded, err := loadWorkflowFlags(f)
	if err != nil {
		return err
	}
	result := map[string]any{"valid": true, "config": loaded.Config.ConfigPath, "config_hash": loaded.Hash, "stages": len(loaded.Config.Stages), "revisions": len(loaded.Config.Revisions)}
	if *f.json {
		return printJSON(result)
	}
	fmt.Printf("valid workflow v2: %s\nconfig_hash: %s\nstages: %d\n", loaded.Config.ConfigPath, loaded.Hash, len(loaded.Config.Stages))
	return nil
}
func runWorkflowPlan(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow plan")
	}
	fs := flag.NewFlagSet("workflow plan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	f := addWorkflowConfigFlags(fs, true)
	if err := fs.Parse(args); err != nil {
		return err
	}
	loaded, err := loadWorkflowFlags(f)
	if err != nil {
		return err
	}
	plan, err := workflow.BuildPlan(loaded)
	if err != nil {
		return err
	}
	if *f.json {
		return printJSON(plan)
	}
	printWorkflowPlan(plan)
	return nil
}
func printWorkflowPlan(plan workflow.Plan) {
	fmt.Printf("workflow plan %s\nworkspace: %s\nconfig_hash: %s\n", plan.RunID, plan.Workspace, plan.ConfigHash)
	for i, s := range plan.Stages {
		needs := ""
		if len(s.Needs) > 0 {
			needs = " needs=" + strings.Join(s.Needs, ",")
		}
		actor := ""
		if s.Actor != "" {
			actor = " actor=" + s.Actor
		}
		fmt.Printf("%d. %s [%s]%s%s\n", i+1, s.ID, s.Type, needs, actor)
	}
}

func runWorkflowRun(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow run")
	}
	fs := flag.NewFlagSet("workflow run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	f := addWorkflowConfigFlags(fs, false)
	id := fs.String("id", "", "internal: resume a config snapshot by run id")
	storeDir := fs.String("store-dir", "", "workflow state root")
	once := fs.Bool("once", false, "run one scheduling pass")
	execute := fs.Bool("execute", false, "perform command and tmux side effects")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var loaded workflow.Loaded
	var err error
	if *id != "" {
		root := *storeDir
		if root == "" {
			root = workflow.DefaultStoreDir
		}
		store, state, findErr := workflow.Find(root, *id, "")
		if findErr != nil {
			return findErr
		}
		loaded, err = workflow.LoadSnapshot(store, state)
	} else {
		loaded, err = loadWorkflowFlags(f)
	}
	if err != nil {
		return err
	}
	if !*execute {
		plan, err := workflow.BuildPlan(loaded)
		if err != nil {
			return err
		}
		printWorkflowPlan(plan)
		fmt.Println("dry-run: add --execute to perform side effects")
		return nil
	}
	root := *storeDir
	if root == "" {
		root = filepath.Join(loaded.Config.Workspace.Root, workflow.DefaultStoreDir)
	}
	engine, err := workflow.NewEngine(loaded, root, true)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return engine.Run(ctx, *once)
}

func runWorkflowStart(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow start")
	}
	fs := flag.NewFlagSet("workflow start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	f := addWorkflowConfigFlags(fs, false)
	execute := fs.Bool("execute", false, "create the detached runner and perform live side effects")
	timeout := fs.Duration("timeout", 10*time.Second, "startup wait")
	if err := fs.Parse(args); err != nil {
		return err
	}
	loaded, err := loadWorkflowFlags(f)
	if err != nil {
		return err
	}
	plan, err := workflow.BuildPlan(loaded)
	if err != nil {
		return err
	}
	if !*execute {
		printWorkflowPlan(plan)
		fmt.Printf("detached session: %s\ndry-run: add --execute to start\n", workflowSupervisorSession)
		return nil
	}
	root := filepath.Join(loaded.Config.Workspace.Root, workflow.DefaultStoreDir)
	release, err := acquireWorkflowStartLock(root, plan.RunID)
	if err != nil {
		return err
	}
	defer release()
	if existing, readErr := workflow.NewStore(root, plan.RunID).Read(); readErr == nil && existing.Desired == "stopped" {
		return fmt.Errorf("workflow %s has a stop request; run workflow resume before starting it", existing.RunID)
	} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	engine, err := workflow.NewEngine(loaded, root, true)
	if err != nil {
		return err
	}
	state, err := engine.Store.Read()
	if err != nil {
		return err
	}
	if state.Desired == "stopped" {
		return fmt.Errorf("workflow %s has a stop request; run workflow resume before starting it", state.RunID)
	}
	if state.Status == "stopped" || state.Status == "failed" || state.Status == "blocked" || state.Status == "succeeded" {
		fmt.Printf("workflow already terminal: %s (%s); use retry or change the config\n", state.RunID, state.Status)
		return nil
	}
	if state.PID != 0 && state.PID != os.Getpid() && workflowProcessAlive(state.PID) {
		fmt.Printf("workflow already active: %s (%s)\n", state.RunID, state.Status)
		return nil
	}
	executable, err := tmactExecutable()
	if err != nil {
		return err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	command := []string{executable, "workflow", "run", "--id", state.RunID, "--store-dir", absRoot, "--execute"}
	window := workflowWindowRE.ReplaceAllString(state.RunID, "-")
	if _, err := listSessionTmuxPanes(workflowSupervisorSession); err != nil {
		err = newTmuxSession(workflowSupervisorSession, window, loaded.Config.Workspace.Root, command)
	} else {
		err = newTmuxWindow(workflowSupervisorSession, window, loaded.Config.Workspace.Root, command)
	}
	if err != nil {
		return fmt.Errorf("start detached workflow: %w", err)
	}
	deadline := time.Now().Add(*timeout)
	for time.Now().Before(deadline) {
		fresh, e := engine.Store.Read()
		if e == nil && fresh.PID != os.Getpid() {
			fmt.Printf("started workflow %s in %s\n", fresh.RunID, workflowSupervisorSession)
			return nil
		}
		tmactSleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for workflow %s to start", state.RunID)
}

func acquireWorkflowStartLock(root, id string) (func(), error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(root, ".start-"+id+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, fmt.Errorf("workflow %s start is already in progress", id)
	}
	if err != nil {
		return nil, err
	}
	_ = file.Close()
	return func() { _ = os.Remove(path) }, nil
}
func workflowSelection(fs *flag.FlagSet) (id, config, store *string) {
	id = fs.String("id", "", "workflow run id")
	config = fs.String("config", "", "workflow config path")
	store = fs.String("store-dir", "", "workflow state root")
	return
}
func selectWorkflow(id, config, root string) (workflow.Store, workflow.State, error) {
	if id != "" && config != "" {
		return workflow.Store{}, workflow.State{}, errors.New("--id and --config are mutually exclusive")
	}
	if root == "" {
		root = storeRootForConfig(config)
	}
	return workflow.Find(root, id, config)
}
func storeRootForConfig(config string) string {
	if config == "" {
		return workflow.DefaultStoreDir
	}
	raw, err := os.ReadFile(config)
	if err != nil {
		return workflow.DefaultStoreDir
	}
	var header struct {
		Workspace struct {
			Root string `yaml:"root"`
		} `yaml:"workspace"`
	}
	if yaml.Unmarshal(raw, &header) != nil {
		return workflow.DefaultStoreDir
	}
	root := header.Workspace.Root
	if root == "" {
		root = "."
	}
	if !filepath.IsAbs(root) {
		abs, _ := filepath.Abs(filepath.Join(filepath.Dir(config), root))
		root = abs
	}
	return filepath.Join(root, workflow.DefaultStoreDir)
}

func runWorkflowStatus(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow status")
	}
	fs := flag.NewFlagSet("workflow status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	id, config, root := workflowSelection(fs)
	jsonOut := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, state, err := selectWorkflow(*id, *config, *root)
	if err != nil {
		return err
	}
	if *jsonOut {
		return printJSON(state)
	}
	printWorkflowStateV2(state)
	return nil
}
func printWorkflowStateV2(state workflow.State) {
	fmt.Printf("workflow: %s\nstatus: %s\ndesired: %s\nconfig_hash: %s\nworkspace: %s\n", state.RunID, state.Status, state.Desired, state.ConfigHash, state.Workspace)
	for _, s := range workflow.SortedStageStates(state) {
		extra := ""
		if s.Outcome != "" {
			extra += " outcome=" + s.Outcome
		}
		if s.DispatchID != "" {
			extra += " dispatch=" + s.DispatchID
		}
		if s.Error != "" {
			extra += " error=" + s.Error
		}
		fmt.Printf("%-24s %-15s attempt=%d%s\n", s.ID, s.Status, s.Attempt, extra)
	}
}

func runWorkflowLogs(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow logs")
	}
	fs := flag.NewFlagSet("workflow logs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	id, config, root := workflowSelection(fs)
	follow := fs.Bool("follow", false, "follow new events")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, _, err := selectWorkflow(*id, *config, *root)
	if err != nil {
		return err
	}
	offset, err := copyWorkflowLog(store.EventsPath(), 0)
	if err != nil {
		return err
	}
	if !*follow {
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(250 * time.Millisecond):
			next, e := copyWorkflowLog(store.EventsPath(), offset)
			if e != nil {
				return e
			}
			offset = next
		}
	}
}
func copyWorkflowLog(path string, offset int64) (int64, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return offset, nil
	}
	if err != nil {
		return offset, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return offset, err
	}
	if info.Size() < offset {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	n, err := io.Copy(os.Stdout, f)
	return offset + n, err
}

func runWorkflowControl(args []string, desired string) error {
	topic := "workflow pause"
	if desired == "running" {
		topic = "workflow resume"
	}
	if wantsHelp(args) {
		return printCommandHelp(topic)
	}
	fs := flag.NewFlagSet(topic, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	id, config, root := workflowSelection(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, state, err := selectWorkflow(*id, *config, *root)
	if err != nil {
		return err
	}
	err = store.Update(func(s *workflow.State) error {
		s.Desired = desired
		if desired == "running" {
			s.Status = "running"
			s.Reason = ""
			s.FinishedAt = time.Time{}
			for id, ss := range s.Stages {
				if ss.Status == workflow.StageBlocked && strings.Contains(ss.Error, "prompt") {
					ss.Status = workflow.StagePending
					ss.Error = ""
					s.Stages[id] = ss
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Printf("workflow %s desired=%s (was %s)\n", state.RunID, desired, state.Desired)
	return nil
}
func runWorkflowRetry(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow retry")
	}
	fs := flag.NewFlagSet("workflow retry", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	id := fs.String("id", "", "workflow run id")
	stage := fs.String("stage", "", "stage id")
	root := fs.String("store-dir", workflow.DefaultStoreDir, "workflow state root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" || *stage == "" {
		return errors.New("--id and --stage are required")
	}
	if err := workflow.RetryStage(*root, *id, *stage); err != nil {
		return err
	}
	fmt.Printf("workflow %s stage %s scheduled for retry\n", *id, *stage)
	return nil
}
func runWorkflowResolve(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow resolve")
	}
	fs := flag.NewFlagSet("workflow resolve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	id, config, root := workflowSelection(fs)
	stage := fs.String("stage", "", "human stage id")
	outcome := fs.String("outcome", "", "allowed outcome")
	var inputs repeatedStrings
	fs.Var(&inputs, "input", "input key=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *stage == "" || *outcome == "" {
		return errors.New("--stage and --outcome are required")
	}
	values, err := workflow.ParseAssignments(inputs)
	if err != nil {
		return err
	}
	if *root == "" {
		*root = storeRootForConfig(*config)
	}
	if err := workflow.ResolveHuman(*root, *id, *config, *stage, *outcome, values); err != nil {
		return err
	}
	fmt.Printf("resolved human stage %s with %s\n", *stage, *outcome)
	return nil
}
func runWorkflowReport(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow report")
	}
	fs := flag.NewFlagSet("workflow report", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dispatchID := fs.String("dispatch-id", "", "durable dispatch id")
	outcome := fs.String("outcome", "", "allowed outcome")
	body := fs.String("body", "", "report summary")
	root := fs.String("store-dir", workflow.DefaultStoreDir, "workflow state root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dispatchID == "" || *outcome == "" {
		return errors.New("--dispatch-id and --outcome are required")
	}
	report, err := workflow.ApplyReport(*root, *dispatchID, *outcome, *body)
	if err != nil {
		return err
	}
	fmt.Printf("workflow_report: %s\n", report.ID)
	return nil
}
func runWorkflowStop(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("workflow stop")
	}
	fs := flag.NewFlagSet("workflow stop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	id, config, root := workflowSelection(fs)
	wait := fs.Bool("wait", false, "wait for cooperative shutdown")
	timeout := fs.Duration("timeout", 10*time.Second, "maximum wait")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*wait {
		return errors.New("workflow stop requires --wait")
	}
	store, state, err := selectWorkflow(*id, *config, *root)
	if err != nil {
		return err
	}
	if err := store.Update(func(s *workflow.State) error { s.Desired = "stopped"; return nil }); err != nil {
		return err
	}
	fresh, err := store.Read()
	if err != nil {
		return err
	}
	if fresh.PID == 0 || !workflowProcessAlive(fresh.PID) {
		if err := store.Update(func(s *workflow.State) error {
			s.Status = "stopped"
			s.Reason = "operator_request_runner_not_alive"
			s.FinishedAt = time.Now()
			s.PID = 0
			return nil
		}); err != nil {
			return err
		}
		fmt.Printf("stopped workflow %s (runner not alive)\n", state.RunID)
		return nil
	}
	deadline := time.Now().Add(*timeout)
	for time.Now().Before(deadline) {
		fresh, e := store.Read()
		if e != nil {
			return e
		}
		if fresh.Status == "stopped" || fresh.Status == "succeeded" || fresh.Status == "failed" || fresh.Status == "blocked" {
			fmt.Printf("stopped workflow %s (%s)\n", state.RunID, fresh.Status)
			return nil
		}
		tmactSleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for workflow %s to stop", state.RunID)
}
