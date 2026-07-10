package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/leolin310148/tmact/internal/runmeta"
)

func printRuntimeStatuses(statuses []runmeta.Status) {
	if len(statuses) == 0 {
		fmt.Println("no registered runs")
		return
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "id\tstatus\tdesired\tphase\tmode\tpid\ttarget\tconfig\tlast")
	for _, status := range statuses {
		run := status.Run
		last := ""
		if status.LastEvent != nil {
			last = formatRuntimeEvent(*status.LastEvent)
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n", run.ID, status.RuntimeStatus, status.DesiredState, displayPhase(run.Phase), loopRunMode(run), run.PID, run.Target, run.ConfigPath, last)
		if run.Tmux.PaneID != "" {
			window := run.Tmux.WindowName
			if window != "" {
				window = fmt.Sprintf("%d:%s", run.Tmux.WindowIndex, window)
			}
			fmt.Fprintf(writer, "\t\t\t\t\t\tpane:%s\t%s\t%s\n", run.Tmux.PaneID, run.Tmux.Session, window)
		}
		for _, problem := range status.RecentProblems {
			fmt.Fprintf(writer, "\tproblem\t\t\t\t\t\t\t%s\n", formatRuntimeEvent(problem))
		}
	}
	_ = writer.Flush()
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
