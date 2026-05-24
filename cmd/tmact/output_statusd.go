package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
)

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
