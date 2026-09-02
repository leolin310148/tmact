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

const (
	askExpiryDispatchAllowance = time.Minute
	// askWaiterRecheckDelay is how long a mailbox-delivered follow-up waits
	// before confirming the answerer is still blocked in `reply --wait`. If
	// that waiter vanished without answering, the follow-up is re-delivered
	// through the pane so it is never silently stranded in the mailbox.
	askWaiterRecheckDelay = 300 * time.Millisecond

	askStatusAnswered = "answered"
	askStatusClosed   = "closed"

	deliveryMailboxThenPane = "mailbox+pane"
)

type askReport struct {
	Status          string            `json:"status"`
	QuestionID      string            `json:"question_id,omitempty"`
	Session         string            `json:"session"`
	Prompt          string            `json:"prompt,omitempty"`
	Timeout         string            `json:"timeout,omitempty"`
	Delivery        string            `json:"delivery,omitempty"`
	ReplyCommand    string            `json:"reply_command,omitempty"`
	FollowUpCommand string            `json:"follow_up_command,omitempty"`
	Dispatch        *dispatch.Report  `json:"dispatch,omitempty"`
	Reply           *askreply.Message `json:"reply,omitempty"`
	AnswererWaiting bool              `json:"answerer_waiting"`
	Closed          bool              `json:"closed"`
}

type replyReport struct {
	Status   string            `json:"status"`
	Reply    askreply.Message  `json:"reply"`
	Closed   bool              `json:"closed"`
	Timeout  string            `json:"timeout,omitempty"`
	FollowUp *askreply.Message `json:"follow_up,omitempty"`
}

type askFlags struct {
	session      string
	dir          string
	agent        string
	model        string
	prompt       string
	thread       string
	closeThread  bool
	readyTimeout time.Duration
	readySettle  time.Duration
	timeout      time.Duration
	trustFolder  bool
	storeDir     string
	execute      bool
	jsonOutput   bool
}

func runAsk(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("ask")
	}

	var f askFlags
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		f.session = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&f.dir, "dir", "", "working directory; sets cwd when the session is created")
	fs.StringVar(&f.agent, "agent", "", "agent to launch: "+strings.Join(dispatch.SupportedAgents(), "|"))
	fs.StringVar(&f.model, "model", "", dispatchModelHelp())
	fs.StringVar(&f.prompt, "prompt", "", "question, task, or follow-up sent to the answering agent")
	fs.StringVar(&f.thread, "thread", "", "continue an existing question ID instead of dispatching a new one")
	fs.BoolVar(&f.closeThread, "close", false, "with --thread: close the question so no further replies are accepted")
	fs.DurationVar(&f.readyTimeout, "ready-timeout", 30*time.Second, "max wait for the agent to become ready")
	fs.DurationVar(&f.readySettle, "ready-settle", dispatch.DefaultReadySettleDelay, "stable idle time after ready before sending the prompt")
	fs.DurationVar(&f.timeout, "timeout", askreply.DefaultTimeout, "max wait for the next explicit tmact reply")
	fs.BoolVar(&f.trustFolder, "trust-folder", false, "accept a Claude/Codex trust prompt only when pane cwd exactly matches --dir")
	fs.StringVar(&f.storeDir, "store-dir", "", "ask mailbox directory; defaults to a private temporary runtime directory")
	fs.BoolVar(&f.execute, "execute", false, "actually dispatch and wait; default is dry-run")
	fs.BoolVar(&f.jsonOutput, "json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if f.session == "" && fs.NArg() > 0 {
		f.session = fs.Arg(0)
	}
	if f.timeout <= 0 {
		return errors.New("--timeout must be positive")
	}
	if f.readyTimeout <= 0 {
		return errors.New("--ready-timeout must be positive")
	}
	if f.readySettle < 0 {
		return errors.New("--ready-settle cannot be negative")
	}
	if f.thread != "" {
		return runAskThread(f)
	}
	if f.closeThread {
		return errors.New("--close requires --thread")
	}
	if f.session == "" {
		return errors.New("ask requires a session name as the first argument")
	}
	if f.dir == "" {
		return errors.New("ask requires --dir")
	}
	if f.agent == "" {
		return errors.New("ask requires --agent")
	}
	if strings.TrimSpace(f.prompt) == "" {
		return errors.New("ask requires --prompt")
	}
	return runAskNew(f)
}

func runAskNew(f askFlags) error {
	store, err := askreply.New(f.storeDir)
	if err != nil {
		return err
	}
	questionID := "<question-id>"
	report := askReport{
		Status:   dispatch.StatusPlanned,
		Session:  f.session,
		Prompt:   f.prompt,
		Timeout:  f.timeout.String(),
		Delivery: askreply.DeliveryPane,
	}
	report.ReplyCommand, report.FollowUpCommand = buildProtocolCommands(questionID, store.Dir, f.storeDir != "")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var request askreply.Request
	if f.execute {
		expiry := f.timeout + f.readyTimeout + f.readySettle + askExpiryDispatchAllowance
		request, err = store.Create(f.session, f.dir, f.agent, os.Getenv("TMUX_PANE"), expiry)
		if err != nil {
			return err
		}
		questionID = request.ID
		report.QuestionID = questionID
		report.ReplyCommand, report.FollowUpCommand = buildProtocolCommands(questionID, store.Dir, f.storeDir != "")
	}

	opts := dispatch.Options{
		Session:      f.session,
		Dir:          f.dir,
		Agent:        f.agent,
		Model:        f.model,
		Prompt:       buildAskPrompt(f.prompt, questionID, report.ReplyCommand),
		Execute:      f.execute,
		ReadyTimeout: f.readyTimeout,
		ReadySettle:  f.readySettle,
		TrustFolder:  f.trustFolder,
		Context:      ctx,
	}
	dispatchReport, err := dispatchRun(opts)
	report.Dispatch = &dispatchReport
	if err != nil {
		if f.execute {
			_ = store.Cancel(request.ID, "dispatch failed")
			report.Status = dispatch.StatusFailed
			if outputErr := printAskOutput(report, f.jsonOutput); outputErr != nil {
				return outputErr
			}
		}
		return err
	}
	if !f.execute {
		return printAskOutput(report, f.jsonOutput)
	}
	if _, err := store.Post(request.ID, askreply.Message{
		From: askreply.RoleAsker, Kind: askreply.KindPrompt,
		Delivery: askreply.DeliveryPane, Target: dispatchReport.Target,
	}, time.Time{}); err != nil && !errors.Is(err, askreply.ErrAlreadyFinalized) {
		// A very fast final reply may already have closed the thread; the
		// await below still returns it. Any other failure is a broken mailbox.
		_ = store.Cancel(request.ID, "record dispatch failed")
		return fmt.Errorf("record dispatched prompt: %w", err)
	}
	return awaitAskAnswer(ctx, store, report, request.ID, 0, f.timeout, f.jsonOutput)
}

// runAskThread continues or closes an existing question. Session, directory,
// and agent come from the persisted request, so only --prompt is needed.
func runAskThread(f askFlags) error {
	switch {
	case f.session != "":
		return errors.New("--thread continues the question's recorded session; do not pass a session name")
	case f.dir != "" || f.agent != "" || f.model != "":
		return errors.New("--thread reuses the question's recorded dir and agent; do not pass --dir, --agent, or --model")
	case f.trustFolder:
		return errors.New("--trust-folder only applies when launching a new agent, not with --thread")
	}
	store, err := askreply.New(f.storeDir)
	if err != nil {
		return err
	}
	if f.closeThread {
		if strings.TrimSpace(f.prompt) != "" {
			return errors.New("--close does not take --prompt")
		}
		thread, err := store.Load(f.thread)
		if err != nil {
			return err
		}
		if err := store.Close(f.thread, askreply.RoleAsker, "closed by asker"); err != nil {
			return err
		}
		return printAskOutput(askReport{
			Status: askStatusClosed, QuestionID: f.thread, Session: thread.Request.Session, Closed: true,
		}, f.jsonOutput)
	}
	if strings.TrimSpace(f.prompt) == "" {
		return errors.New("ask --thread requires --prompt (or --close)")
	}

	thread, err := store.Load(f.thread)
	if err != nil {
		return err
	}
	if thread.Closed != nil {
		return fmt.Errorf("question %s is closed (%s by %s); start a new ask", f.thread, thread.Closed.Reason, thread.Closed.By)
	}
	lastAnswerSeq := thread.LastSeq(askreply.RoleAnswerer)
	hasWaiter, err := store.HasWaiter(f.thread)
	if err != nil {
		return err
	}
	report := askReport{
		Status:     dispatch.StatusPlanned,
		QuestionID: f.thread,
		Session:    thread.Request.Session,
		Prompt:     f.prompt,
		Timeout:    f.timeout.String(),
	}
	report.ReplyCommand, report.FollowUpCommand = buildProtocolCommands(f.thread, store.Dir, f.storeDir != "")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	waitUntil := tmactNow().Add(f.timeout + f.readyTimeout + f.readySettle + askExpiryDispatchAllowance)

	if hasWaiter {
		report.Delivery = askreply.DeliveryMailbox
		if !f.execute {
			return printAskOutput(report, f.jsonOutput)
		}
		posted, err := store.Post(f.thread, askreply.Message{
			From: askreply.RoleAsker, Kind: askreply.KindPrompt,
			Text: f.prompt, Delivery: askreply.DeliveryMailbox,
		}, waitUntil)
		if err != nil {
			return err
		}
		tmactSleep(askWaiterRecheckDelay)
		stillWaiting, err := store.HasWaiter(f.thread)
		if err != nil {
			return err
		}
		if !stillWaiting {
			// The waiter removes its marker as soon as it returns, so a
			// missing marker also means "consumed"; the ack tells them apart.
			acked, err := store.Acknowledged(f.thread, posted.Seq)
			if err != nil {
				return err
			}
			current, err := store.Load(f.thread)
			if err != nil {
				return err
			}
			if !acked && current.LastSeq(askreply.RoleAnswerer) == lastAnswerSeq && current.Closed == nil {
				// The waiter gave up between our check and our post. Fall back
				// to the pane so the follow-up is not stranded.
				report.Delivery = deliveryMailboxThenPane
				hasWaiter = false
			}
		}
	}
	if !hasWaiter {
		if report.Delivery == "" {
			report.Delivery = askreply.DeliveryPane
		}
		opts := dispatch.Options{
			Session:      thread.Request.Session,
			Target:       thread.LastTarget(),
			Dir:          thread.Request.Dir,
			Agent:        thread.Request.Agent,
			Prompt:       buildFollowUpPrompt(f.prompt, f.thread, report.ReplyCommand),
			Execute:      f.execute,
			ReadyTimeout: f.readyTimeout,
			ReadySettle:  f.readySettle,
			NoClear:      true,
			Context:      ctx,
		}
		dispatchReport, err := dispatchRun(opts)
		report.Dispatch = &dispatchReport
		if err != nil {
			// Leave the thread open: the asker can retry once the pane is idle.
			if f.execute {
				report.Status = dispatch.StatusFailed
				if outputErr := printAskOutput(report, f.jsonOutput); outputErr != nil {
					return outputErr
				}
			}
			return err
		}
		if !f.execute {
			return printAskOutput(report, f.jsonOutput)
		}
		if _, err := store.Post(f.thread, askreply.Message{
			From: askreply.RoleAsker, Kind: askreply.KindPrompt,
			Delivery: askreply.DeliveryPane, Target: dispatchReport.Target,
		}, waitUntil); err != nil && !errors.Is(err, askreply.ErrAlreadyFinalized) {
			return fmt.Errorf("record dispatched follow-up: %w", err)
		}
	}
	return awaitAskAnswer(ctx, store, report, f.thread, lastAnswerSeq, f.timeout, f.jsonOutput)
}

func awaitAskAnswer(ctx context.Context, store *askreply.Store, report askReport, id string, afterSeq int, timeout time.Duration, jsonOutput bool) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	reply, err := store.AwaitAnswer(waitCtx, id, afterSeq)
	if err != nil {
		report.Status = dispatch.StatusFailed
		if outputErr := printAskOutput(report, jsonOutput); outputErr != nil {
			return outputErr
		}
		return err
	}
	report.Status = askStatusAnswered
	report.Reply = &reply
	report.AnswererWaiting = reply.Kind == askreply.KindQuestion
	report.Closed = reply.Kind == askreply.KindFinal
	return printAskOutput(report, jsonOutput)
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
	wait := fs.Bool("wait", false, "after replying, block until the asker sends a follow-up and print it")
	timeout := fs.Duration("timeout", askreply.DefaultReplyWaitTimeout, "max wait for the asker's follow-up with --wait")
	final := fs.Bool("final", false, "mark this reply final and close the question to further messages")
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
	if *wait && *final {
		return errors.New("--wait and --final are mutually exclusive")
	}
	if *timeout <= 0 {
		return errors.New("--timeout must be positive")
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
	kind := askreply.KindAnswer
	switch {
	case *wait:
		kind = askreply.KindQuestion
	case *final:
		kind = askreply.KindFinal
	}
	message, err := store.Post(questionID, askreply.Message{
		From: askreply.RoleAnswerer, Kind: kind, Text: body, Delivery: askreply.DeliveryMailbox,
	}, time.Time{})
	if err != nil {
		return err
	}
	report := replyReport{Status: "replied", Reply: message, Closed: *final}
	if *final {
		if err := store.Close(questionID, askreply.RoleAnswerer, "final reply"); err != nil && !errors.Is(err, askreply.ErrAlreadyFinalized) {
			return err
		}
	}
	if !*wait {
		return printReplyOutput(report, *jsonOutput)
	}

	report.Timeout = timeout.String()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	waitCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	followUp, err := store.AwaitPrompt(waitCtx, questionID, message.Seq)
	if err != nil {
		report.Status = dispatch.StatusFailed
		if outputErr := printReplyOutput(report, *jsonOutput); outputErr != nil {
			return outputErr
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("no follow-up within %s; finish this turn normally, the asker can still continue in this pane", *timeout)
		}
		return err
	}
	report.Status = askStatusAnswered
	report.FollowUp = &followUp
	return printReplyOutput(report, *jsonOutput)
}

func buildAskPrompt(promptText, questionID, replyCommand string) string {
	return strings.TrimSpace(promptText) + "\n\n" +
		"tmact reply protocol (required):\n" +
		"- This request's question ID is " + questionID + ".\n" +
		"- Before finishing, send your result back to the waiting asker by running:\n  " + replyCommand + "\n" +
		"- Replace FINAL ANSWER with a concise result. For multiline output, use --file PATH instead of --text.\n" +
		"- Run tmact reply exactly once per turn; do not merely mention the command in your final chat response.\n" +
		"- If you cannot finish without clarification from the asker, run the same command with --wait (optionally --timeout 10m); it prints the asker's follow-up and that counts as this turn's reply.\n" +
		"- The asker may send follow-ups to this pane later; answer each with tmact reply " + questionID + " again."
}

func buildFollowUpPrompt(promptText, questionID, replyCommand string) string {
	return strings.TrimSpace(promptText) + "\n\n" +
		"tmact reply protocol (required): this is a follow-up on question ID " + questionID + ". " +
		"Before finishing, run exactly once:\n  " + replyCommand + "\n" +
		"Use --file PATH for multiline output, or add --wait to block for the asker's next message instead of ending your turn."
}

func buildProtocolCommands(questionID, storeDir string, includeStore bool) (replyCommand, followUpCommand string) {
	replyCommand = buildReplyCommand(questionID, storeDir, includeStore)
	followUpCommand = "tmact ask --thread " + questionID + ` --prompt "FOLLOW-UP" --execute`
	if includeStore {
		followUpCommand += " --store-dir " + quoteShellArg(storeDir)
	}
	return replyCommand, followUpCommand
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
	if report.Status == askStatusClosed {
		fmt.Printf("closed: %s [session=%s]\n", report.QuestionID, report.Session)
		return nil
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
	if report.Dispatch != nil && report.Dispatch.Target != "" {
		fmt.Printf("  target: %s\n", report.Dispatch.Target)
	}
	if report.Delivery != "" {
		fmt.Printf("  delivery: %s\n", report.Delivery)
	}
	fmt.Printf("  reply command: %s\n", report.ReplyCommand)
	if report.Dispatch != nil {
		for _, step := range report.Dispatch.Steps {
			fmt.Printf("  [%s] %s\n", step.Status, step.Name)
		}
	}
	if report.Reply != nil {
		switch report.Reply.Kind {
		case askreply.KindQuestion:
			fmt.Printf("reply (seq %d, kind=question; the answerer is waiting for your follow-up):\n", report.Reply.Seq)
		case askreply.KindFinal:
			fmt.Printf("reply (seq %d, kind=final; question closed):\n", report.Reply.Seq)
		default:
			fmt.Printf("reply (seq %d, kind=%s):\n", report.Reply.Seq, report.Reply.Kind)
		}
		fmt.Print(report.Reply.Text)
		if !strings.HasSuffix(report.Reply.Text, "\n") {
			fmt.Println()
		}
		if !report.Closed {
			fmt.Printf("follow-up: %s\n", report.FollowUpCommand)
		}
	}
	return nil
}

func printReplyOutput(report replyReport, jsonOutput bool) error {
	if jsonOutput {
		return printJSON(report)
	}
	suffix := ""
	if report.Closed {
		suffix = "; question closed"
	}
	fmt.Printf("replied: %s (seq %d%s)\n", report.Reply.QuestionID, report.Reply.Seq, suffix)
	if report.FollowUp != nil {
		fmt.Printf("follow-up from asker (seq %d):\n", report.FollowUp.Seq)
		fmt.Print(report.FollowUp.Text)
		if !strings.HasSuffix(report.FollowUp.Text, "\n") {
			fmt.Println()
		}
	}
	return nil
}
