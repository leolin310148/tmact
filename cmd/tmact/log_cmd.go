package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/logsearch"
	"github.com/leolin310148/tmact/internal/logstats"
	"github.com/leolin310148/tmact/internal/sessionlog"
)

func runLog(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		return printCommandHelp("log")
	}
	switch args[0] {
	case "search":
		return runLogSearch(args[1:])
	case "stats":
		return runLogStats(args[1:])
	case "doctor":
		return runLogDoctor(args[1:])
	default:
		return fmt.Errorf("unknown log subcommand %q (want search, stats, or doctor)", args[0])
	}
}

func runLogStats(args []string) error {
	if wantsHelp(args) || containsHelpFlag(args) {
		return printCommandHelp("log stats")
	}
	fs := flag.NewFlagSet("log stats", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	sinceValue := fs.String("since", "", "relative duration or RFC3339 lower timestamp bound")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("log stats does not accept positional arguments")
	}
	since, err := parseLogSince(*sinceValue, tmactNow())
	if err != nil {
		return err
	}
	report, err := logstats.Stats(logstats.Options{Since: since})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(report)
	}
	printLogStatsReport(report)
	return nil
}

func runLogDoctor(args []string) error {
	if wantsHelp(args) || containsHelpFlag(args) {
		return printCommandHelp("log doctor")
	}
	fs := flag.NewFlagSet("log doctor", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("log doctor does not accept positional arguments")
	}
	report, err := logstats.Doctor(logstats.Options{})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(report)
	}
	printLogDoctorReport(report)
	return nil
}

func runLogSearch(args []string) error {
	if wantsHelp(args) || containsHelpFlag(args) {
		return printCommandHelp("log search")
	}
	fs := flag.NewFlagSet("log search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var providerNames repeatedStrings
	fs.Var(&providerNames, "provider", "provider to search (claude or codex); may be repeated")
	sinceValue := fs.String("since", "", "relative duration or RFC3339 lower timestamp bound")
	cwdValue := fs.String("cwd", "", "exact normalized working directory")
	kindValue := fs.String("kind", "", "normalized record kind")
	limit := fs.Int("limit", logsearch.DefaultLimit, "maximum newest matches")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	showContent := fs.Bool("show-content", false, "include private normalized prompt and tool content")
	reordered, err := reorderLogSearchArgs(args)
	if err != nil {
		return err
	}
	if err := fs.Parse(reordered); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("log search requires exactly one non-empty QUERY")
	}
	query := fs.Arg(0)
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("log search requires exactly one non-empty QUERY")
	}
	providers := make([]sessionlog.Provider, 0, len(providerNames))
	for _, name := range providerNames {
		providers = append(providers, sessionlog.Provider(strings.ToLower(name)))
	}
	since, err := parseLogSince(*sinceValue, tmactNow())
	if err != nil {
		return err
	}
	cwd := ""
	if *cwdValue != "" {
		cwd, err = filepath.Abs(*cwdValue)
		if err != nil {
			return fmt.Errorf("resolve --cwd: %w", err)
		}
		cwd = filepath.Clean(cwd)
	}
	report, err := logsearch.Search(logsearch.Options{
		Query:       query,
		Providers:   providers,
		Since:       since,
		CWD:         cwd,
		Kind:        sessionlog.Kind(*kindValue),
		Limit:       *limit,
		ShowContent: *showContent,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(report)
	}
	printLogSearchReport(report, *showContent)
	return nil
}

func containsHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func reorderLogSearchArgs(args []string) ([]string, error) {
	valueFlags := map[string]bool{
		"--provider": true, "--since": true, "--cwd": true, "--kind": true, "--limit": true,
	}
	var flags, positional []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			positional = append(positional, args[index+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			name := arg
			if equals := strings.IndexByte(name, '='); equals >= 0 {
				name = name[:equals]
			}
			if valueFlags[name] && !strings.Contains(arg, "=") {
				if index+1 >= len(args) {
					return nil, fmt.Errorf("%s requires a value", arg)
				}
				index++
				flags = append(flags, args[index])
			}
			continue
		}
		positional = append(positional, arg)
	}
	flags = append(flags, "--")
	return append(flags, positional...), nil
}

func parseLogSince(value string, now time.Time) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	if duration, err := time.ParseDuration(value); err == nil {
		if duration <= 0 {
			return time.Time{}, fmt.Errorf("--since duration must be greater than zero")
		}
		return now.Add(-duration), nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid --since %q (want duration such as 24h or RFC3339 timestamp)", value)
}

func printLogSearchReport(report logsearch.Report, showContent bool) {
	for _, match := range report.Matches {
		when := match.Timestamp
		if when == "" {
			when = "unknown-time"
		}
		fields := []string{when, string(match.Provider), string(match.Kind)}
		if match.Role != "" {
			fields = append(fields, "role="+match.Role)
		}
		if match.SessionID != "" {
			fields = append(fields, "session="+match.SessionID)
		}
		if match.CWD != "" {
			fields = append(fields, "cwd="+match.CWD)
		}
		if match.Tool != "" {
			fields = append(fields, "tool="+match.Tool)
		}
		if match.Event != "" {
			fields = append(fields, "event="+match.Event)
		}
		if match.Model != "" {
			fields = append(fields, "model="+match.Model)
		}
		if match.CommandVerb != "" {
			command := match.CommandVerb
			if match.CommandSubcommand != "" {
				command += " " + match.CommandSubcommand
			}
			fields = append(fields, "command="+command)
		}
		if match.ExitCode != nil {
			fields = append(fields, fmt.Sprintf("exit_code=%d", *match.ExitCode))
		}
		if match.DurationMS != nil {
			fields = append(fields, fmt.Sprintf("duration_ms=%g", *match.DurationMS))
		}
		fmt.Println(strings.Join(fields, " "))
		if showContent && match.Content != "" {
			fmt.Printf("  content: %s", strings.ReplaceAll(match.Content, "\n", "\n  "))
			if match.ContentTruncated {
				fmt.Print(" [truncated]")
			}
			fmt.Println()
		}
	}
	if len(report.Matches) == 0 {
		fmt.Println("No matches.")
	}
	fmt.Println("Coverage:")
	for _, coverage := range report.Coverage {
		fmt.Printf("  %s sources=%d lines=%d records=%d malformed=%d unknown=%d oversized=%d errors=%d\n",
			coverage.Provider, coverage.Sources, coverage.Lines, coverage.Records,
			coverage.Malformed, coverage.Unknown, coverage.Oversized, len(coverage.Errors))
		for _, coverageErr := range coverage.Errors {
			path := coverageErr.Path
			if path == "" {
				path = "-"
			}
			fmt.Printf("    %s %s: %s\n", coverageErr.Stage, path, coverageErr.Error)
		}
	}
}

func printLogStatsReport(report logstats.Report) {
	if report.Since != "" {
		fmt.Printf("Since: %s\n", report.Since)
	}
	fmt.Printf("Records: %d\n", report.Records)
	printLogBuckets("Providers", report.Providers)
	printLogBuckets("Tools", report.Tools)
	printLogBuckets("Commands", report.Commands)
	printLogBuckets("Subcommands", report.Subcommands)
	fmt.Printf("Index: status=%s entries=%d hits=%d misses=%d appended=%d rebuilt=%d removed=%d\n",
		report.Index.Status, report.Index.Entries, report.Index.Hits, report.Index.Misses,
		report.Index.Appended, report.Index.Rebuilt, report.Index.Removed)
	printLogScanErrors(report.Errors)
}

func printLogDoctorReport(report logstats.DoctorReport) {
	fmt.Printf("Files: discovered=%d indexed=%d cache_hits=%d parsed=%d failed=%d\n",
		report.Files.Discovered, report.Files.Indexed, report.Files.CacheHits,
		report.Files.Parsed, report.Files.Failed)
	fmt.Printf("Records: lines=%d records=%d known=%d unknown=%d malformed=%d oversized=%d skipped=%d\n",
		report.Records.Lines, report.Records.Records, report.Records.Known,
		report.Records.Unknown, report.Records.Malformed, report.Records.Oversized, report.Records.Skipped)
	fmt.Println("Schema coverage:")
	for _, coverage := range report.SchemaCoverage {
		fmt.Printf("  %s files=%d records=%d known=%d unknown=%d malformed=%d oversized=%d skipped=%d\n",
			coverage.Provider, coverage.Files, coverage.Records, coverage.Known,
			coverage.Unknown, coverage.Malformed, coverage.Oversized, coverage.Skipped)
	}
	fmt.Printf("Cache: healthy=%t status=%s entries=%d hits=%d misses=%d appended=%d rebuilt=%d removed=%d path=%s\n",
		report.Cache.Healthy, report.Cache.Status, report.Cache.Entries, report.Cache.Hits,
		report.Cache.Misses, report.Cache.Appended, report.Cache.Rebuilt, report.Cache.Removed, report.Cache.Path)
	printLogScanErrors(report.Errors)
}

func printLogBuckets(title string, buckets []logstats.Bucket) {
	fmt.Printf("%s:\n", title)
	if len(buckets) == 0 {
		fmt.Println("  (none)")
		return
	}
	for _, bucket := range buckets {
		fmt.Printf("  %s %d\n", bucket.Name, bucket.Count)
	}
}

func printLogScanErrors(errors []logstats.ScanError) {
	if len(errors) == 0 {
		return
	}
	fmt.Println("Errors:")
	for _, scanErr := range errors {
		fields := []string{scanErr.Stage}
		if scanErr.Provider != "" {
			fields = append(fields, string(scanErr.Provider))
		}
		if scanErr.Path != "" {
			fields = append(fields, scanErr.Path)
		}
		fmt.Printf("  %s: %s\n", strings.Join(fields, " "), scanErr.Error)
	}
}
