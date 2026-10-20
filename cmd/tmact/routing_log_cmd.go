package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/leolin310148/tmact/internal/modelroute"
)

func runLogRouting(args []string) error {
	if wantsHelp(args) || containsHelpFlag(args) {
		return printCommandHelp("log routing")
	}
	fs := flag.NewFlagSet("log routing", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	id := fs.String("dispatch-id", "", "filter by dispatch ID")
	limit := fs.Int("limit", 50, "maximum newest records")
	content := fs.Bool("show-content", false, "include private task, attached file snapshots, and decision response")
	jsonOutput := fs.Bool("json", false, "print JSON records")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("log routing takes no positional arguments")
	}
	if *limit <= 0 {
		return fmt.Errorf("--limit must be positive")
	}
	entries, err := modelroute.ReadLog(*id, *limit, *content)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(entries)
	}
	if len(entries) == 0 {
		fmt.Println("No routing records.")
		return nil
	}
	for _, e := range entries {
		fmt.Printf("%s %s %s session=%s", e.Time.Format("2006-01-02T15:04:05Z07:00"), e.Trace.DispatchID, e.Event, e.Trace.Session)
		if e.Selection != nil {
			fmt.Printf(" model=%s effort=%s difficulty=%s tier=%s confidence=%.2f", e.Selection.Model, e.Selection.Effort, e.Selection.Difficulty, e.Selection.SelectedTier, e.Selection.Confidence)
		}
		if e.Outcome != nil {
			fmt.Printf(" status=%s", e.Outcome.Status)
		}
		if e.Error != "" {
			fmt.Printf(" error=%s", e.Error)
		}
		fmt.Println()
		if *content {
			if err := printJSON(e); err != nil {
				return err
			}
		}
	}
	return nil
}
