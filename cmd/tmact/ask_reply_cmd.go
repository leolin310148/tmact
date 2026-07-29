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

	"github.com/leolin310148/tmact/internal/askreply"
	"github.com/leolin310148/tmact/internal/dispatch"
)

const askExpiryDispatchAllowance = time.Minute

type askReport struct {
	Status       string          `json:"status"`
	QuestionID   string          `json:"question_id,omitempty"`
	Session      string          `json:"session"`
	Prompt       string          `json:"prompt"`
	Timeout      string          `json:"timeout"`
	ReplyCommand string          `json:"reply_command"`
	Dispatch     dispatch.Report `json:"dispatch"`
	Reply        *askreply.Reply `json:"reply,omitempty"`
}

func runAsk(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("ask")
	}

	var session string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		session = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	dir := fs.String("dir", "", "working directory; sets cwd when the session is created")
	agent := fs.String("agent", "", "agent to launch: "+strings.Join(dispatch.SupportedAgents(), "|"))
	model := fs.String("model", "", dispatchModelHelp())
	promptText := fs.String("prompt", "", "question or task sent to the answering agent")
	readyTimeout := fs.Duration("ready-timeout", 30*time.Second, "max wait for the agent to become ready")
	readySettle := fs.Duration("ready-settle", dispatch.DefaultReadySettleDelay, "stable idle time after ready before sending the prompt")
	timeout := fs.Duration("timeout", askreply.DefaultTimeout, "max wait for an explicit tmact reply after dispatch")
	trustFolder := fs.Bool("trust-folder", false, "accept a Claude/Codex trust prompt only when pane cwd exactly matches --dir")
	storeDir := fs.String("store-dir", "", "ask mailbox directory; defaults to a private temporary runtime directory")
	execute := fs.Bool("execute", false, "actually dispatch and wait; default is dry-run")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if session == "" && fs.NArg() > 0 {
		session = fs.Arg(0)
	}
	if session == "" {
		return errors.New("ask requires a session name as the first argument")
	}
	if *dir == "" {
		return errors.New("ask requires --dir")
	}
	if *agent == "" {
		return errors.New("ask requires --agent")
	}
	if strings.TrimSpace(*promptText) == "" {
		return errors.New("ask requires --prompt")
	}
	if *timeout <= 0 {
		return errors.New("--timeout must be positive")
	}
	if *readyTimeout <= 0 {
		return errors.New("--ready-timeout must be positive")
	}
	if *readySettle < 0 {
		return errors.New("--ready-settle cannot be negative")
	}

	store, err := askreply.New(*storeDir)
	if err != nil {
		return err
	}
	questionID := "<question-id>"
	replyCommand := buildReplyCommand(questionID, store.Dir, *storeDir != "")
	report := askReport{
		Status:       dispatch.StatusPlanned,
		Session:      session,
		Prompt:       *promptText,
		Timeout:      timeout.String(),
		ReplyCommand: replyCommand,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var request askreply.Request
	if *execute {
		expiry := *timeout + *readyTimeout + *readySettle + askExpiryDispatchAllowance
		request, err = store.Create(session, *dir, *agent, os.Getenv("TMUX_PANE"), expiry)
		if err != nil {
			return err
		}
		questionID = request.ID
		replyCommand = buildReplyCommand(questionID, store.Dir, *storeDir != "")
		report.QuestionID = questionID
		report.ReplyCommand = replyCommand
	}

	opts := dispatch.Options{
		Session:      session,
		Dir:          *dir,
		Agent:        *agent,
		Model:        *model,
		Prompt:       buildAskPrompt(*promptText, questionID, replyCommand),
		Execute:      *execute,
		ReadyTimeout: *readyTimeout,
		ReadySettle:  *readySettle,
		TrustFolder:  *trustFolder,
		Context:      ctx,
	}
	report.Dispatch, err = dispatchRun(opts)
	if err != nil {
		if *execute {
			_ = store.Cancel(request.ID, "dispatch failed")
			report.Status = dispatch.StatusFailed
			if outputErr := printAskOutput(report, *jsonOutput); outputErr != nil {
				return outputErr
			}
		}
		return err
	}
	if !*execute {
		return printAskOutput(report, *jsonOutput)
	}

	waitCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	reply, err := store.Wait(waitCtx, request.ID)
	if err != nil {
		report.Status = dispatch.StatusFailed
		if outputErr := printAskOutput(report, *jsonOutput); outputErr != nil {
			return outputErr
		}
		return err
	}
	report.Status = "answered"
	report.Reply = &reply
	return printAskOutput(report, *jsonOutput)
}

func runReply(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("reply")
	}
	var questionID string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		questionID = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("reply", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	text := fs.String("text", "", "reply text")
	file := fs.String("file", "", "read reply text from this file")
	storeDir := fs.String("store-dir", "", "ask mailbox directory; defaults to a private temporary runtime directory")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if questionID == "" && fs.NArg() > 0 {
		questionID = fs.Arg(0)
	}
	if questionID == "" {
		return errors.New("reply requires a question id as the first argument")
	}
	if (*text == "") == (*file == "") {
		return errors.New("reply requires exactly one of --text or --file")
	}
	body := *text
	if *file != "" {
		info, err := os.Stat(*file)
		if err != nil {
			return fmt.Errorf("read reply file: %w", err)
		}
		if info.Size() > askreply.MaxReplyBytes {
			return fmt.Errorf("reply file exceeds %d bytes", askreply.MaxReplyBytes)
		}
		data, err := os.ReadFile(*file)
		if err != nil {
			return fmt.Errorf("read reply file: %w", err)
		}
		body = string(data)
	}
	store, err := askreply.New(*storeDir)
	if err != nil {
		return err
	}
	reply, err := store.Reply(questionID, body)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(reply)
	}
	fmt.Printf("replied: %s\n", reply.QuestionID)
	return nil
}

func buildAskPrompt(promptText, questionID, replyCommand string) string {
	return strings.TrimSpace(promptText) + "\n\n" +
		"tmact reply protocol (required):\n" +
		"- This request's question ID is " + questionID + ".\n" +
		"- Before finishing, send your result back to the waiting asker by running:\n  " + replyCommand + "\n" +
		"- Replace FINAL ANSWER with a concise result. For multiline output, use --file PATH instead of --text.\n" +
		"- Run tmact reply exactly once; do not merely mention the command in your final chat response."
}

func buildReplyCommand(questionID, storeDir string, includeStore bool) string {
	command := "tmact reply " + questionID + ` --text "FINAL ANSWER"`
	if includeStore {
		command += " --store-dir " + quoteShellArg(storeDir)
	}
	return command
}

func quoteShellArg(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || strings.ContainsRune("_@%+=:,./-", r))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func printAskOutput(report askReport, jsonOutput bool) error {
	if jsonOutput {
		return printJSON(report)
	}
	prefix := ""
	if report.Status == dispatch.StatusPlanned {
		prefix = "dry-run: "
	}
	id := report.QuestionID
	if id == "" {
		id = "<question-id>"
	}
	fmt.Printf("%sask %s [session=%s status=%s timeout=%s]\n", prefix, id, report.Session, report.Status, report.Timeout)
	if report.Dispatch.Target != "" {
		fmt.Printf("  target: %s\n", report.Dispatch.Target)
	}
	fmt.Printf("  reply command: %s\n", report.ReplyCommand)
	for _, step := range report.Dispatch.Steps {
		fmt.Printf("  [%s] %s\n", step.Status, step.Name)
	}
	if report.Reply != nil {
		fmt.Println("reply:")
		fmt.Print(report.Reply.Text)
		if !strings.HasSuffix(report.Reply.Text, "\n") {
			fmt.Println()
		}
	}
	return nil
}
