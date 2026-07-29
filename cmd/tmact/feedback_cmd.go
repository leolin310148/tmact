package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultFeedbackCategory = "ux"
	defaultFeedbackLimit    = 50
	feedbackFileName        = "feedback.jsonl"
)

var validFeedbackCategories = map[string]struct{}{
	"ux":      {},
	"bug":     {},
	"feature": {},
	"docs":    {},
	"perf":    {},
}

type feedbackEntry struct {
	Time     time.Time `json:"time"`
	Version  string    `json:"version"`
	Category string    `json:"category"`
	Command  string    `json:"command"`
	Message  string    `json:"message"`
}

type feedbackAddResult struct {
	Path  string        `json:"path"`
	Entry feedbackEntry `json:"entry"`
}

func runFeedback(args []string) error {
	if len(args) == 0 || wantsHelp(args) {
		return printCommandHelp("feedback")
	}

	switch args[0] {
	case "list":
		return runFeedbackList(args[1:])
	case "path":
		return runFeedbackPath(args[1:])
	case "add":
		if len(args) == 1 || (len(args) == 2 && (args[1] == "-h" || args[1] == "--help")) {
			return printCommandHelp("feedback")
		}
		return runFeedbackAdd(args[1:])
	default:
		return runFeedbackAdd(args)
	}
}

func runFeedbackAdd(args []string) error {
	category := defaultFeedbackCategory
	command := ""
	jsonOutput := false
	messageParts := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			messageParts = append(messageParts, args[i+1:]...)
			i = len(args)
		case arg == "--category":
			if i+1 >= len(args) {
				return errors.New("--category requires a value (ux, bug, feature, docs, or perf)")
			}
			i++
			category = args[i]
		case strings.HasPrefix(arg, "--category="):
			category = strings.TrimPrefix(arg, "--category=")
		case arg == "--command":
			if i+1 >= len(args) {
				return errors.New("--command requires a value")
			}
			i++
			command = args[i]
		case strings.HasPrefix(arg, "--command="):
			command = strings.TrimPrefix(arg, "--command=")
		case arg == "--json":
			jsonOutput = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown feedback flag %q", arg)
		default:
			messageParts = append(messageParts, arg)
		}
	}

	category = strings.TrimSpace(category)
	if _, ok := validFeedbackCategories[category]; !ok {
		return errors.New("--category must be one of: ux, bug, feature, docs, perf")
	}
	command = strings.TrimSpace(command)
	message := strings.TrimSpace(strings.Join(messageParts, " "))
	if message == "" {
		return errors.New("feedback message is required")
	}

	path, err := defaultFeedbackPath()
	if err != nil {
		return err
	}
	entry := feedbackEntry{
		Time:     tmactNow().UTC(),
		Version:  version,
		Category: category,
		Command:  command,
		Message:  message,
	}
	if err := appendFeedback(path, entry); err != nil {
		return err
	}

	if jsonOutput {
		return printJSON(feedbackAddResult{Path: path, Entry: entry})
	}
	fmt.Printf("Feedback recorded in %s\n", path)
	return nil
}

func runFeedbackList(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("feedback")
	}
	fs := flag.NewFlagSet("feedback list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	limit := fs.Int("limit", defaultFeedbackLimit, "number of newest entries to show")
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected feedback list argument %q", fs.Arg(0))
	}
	if *limit <= 0 {
		return errors.New("--limit must be greater than zero")
	}

	path, err := defaultFeedbackPath()
	if err != nil {
		return err
	}
	entries, err := readFeedback(path, *limit)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(entries)
	}
	if len(entries) == 0 {
		fmt.Println("No feedback recorded yet.")
		return nil
	}
	for _, entry := range entries {
		command := ""
		if entry.Command != "" {
			command = " " + entry.Command
		}
		message := strings.ReplaceAll(entry.Message, "\n", `\n`)
		fmt.Printf("%s [%s]%s %s\n", entry.Time.Local().Format(time.RFC3339), entry.Category, command, message)
	}
	return nil
}

func runFeedbackPath(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("feedback")
	}
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			return fmt.Errorf("unknown feedback path flag %q", arg)
		}
	}
	path, err := defaultFeedbackPath()
	if err != nil {
		return err
	}
	if jsonOutput {
		return printJSON(map[string]string{"path": path})
	}
	fmt.Println(path)
	return nil
}

func defaultFeedbackPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for feedback: %w", err)
	}
	return filepath.Join(home, ".tmact", feedbackFileName), nil
}

func appendFeedback(path string, entry feedbackEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode feedback: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create feedback directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open feedback file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure feedback file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("append feedback: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close feedback file: %w", err)
	}
	return nil
}

func readFeedback(path string, limit int) ([]feedbackEntry, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []feedbackEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open feedback file: %w", err)
	}
	defer file.Close()

	initialCapacity := min(limit, defaultFeedbackLimit)
	entries := make([]feedbackEntry, 0, initialCapacity)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var entry feedbackEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("decode feedback line %s: %w", strconv.Itoa(lineNumber), err)
		}
		if len(entries) == limit {
			copy(entries, entries[1:])
			entries[len(entries)-1] = entry
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read feedback file: %w", err)
	}
	return entries, nil
}
