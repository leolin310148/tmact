package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/leolin310148/tmact/internal/agents"
	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/runmeta"
	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/workflow"
)

func printJSON(value interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
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
