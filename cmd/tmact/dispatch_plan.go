package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/modelroute"
)

// Plans are private, explicit artifacts, never an implicit decision cache.
type dispatchPlan struct {
	SourcePath string                `json:"-"`
	Version    int                   `json:"version"`
	Command    string                `json:"command"`
	CreatedAt  time.Time             `json:"created_at"`
	Args       []string              `json:"args"`
	Report     dispatch.Report       `json:"dispatch"`
	Files      []modelroute.TaskFile `json:"files,omitempty"`
	Digest     string                `json:"sha256"`
}

func (p dispatchPlan) digest() string {
	p.Digest = ""
	p.SourcePath = ""
	b, _ := json.Marshal(p)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

// Apply accepts no task overrides: the reviewed arguments are the authority.
func expandApplyPlan(command string, args []string) ([]string, *dispatchPlan, error) {
	apply := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--apply-plan" || strings.HasPrefix(a, "--apply-plan=") {
			apply = true
			break
		}
		if strings.HasPrefix(a, "-") && !strings.Contains(a, "=") && !planBoolFlag(a) {
			i++
		}
	}
	if !apply {
		return args, nil, nil
	}
	var path string
	execute, jsonOutput := false, false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--apply-plan":
			i++
			if i >= len(args) {
				return nil, nil, fmt.Errorf("--apply-plan requires a file")
			}
			path = args[i]
		case strings.HasPrefix(a, "--apply-plan="):
			path = strings.TrimPrefix(a, "--apply-plan=")
		case a == "--execute":
			execute = true
		case a == "--json":
			jsonOutput = true
		default:
			return nil, nil, fmt.Errorf("--apply-plan accepts only --execute and --json; regenerate the plan to change task options")
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var p dispatchPlan
	if err = json.Unmarshal(b, &p); err != nil {
		return nil, nil, fmt.Errorf("invalid plan: %w", err)
	}
	if p.Version != 1 || p.Command != command || p.Digest != p.digest() || p.Report.PlanState == nil {
		return nil, nil, fmt.Errorf("invalid, modified, or incompatible plan; regenerate it")
	}
	p.SourcePath = path
	dir := p.Report.Dir
	if p.Report.PlanState.Dir != "" {
		dir = p.Report.PlanState.Dir
	}
	var files []modelroute.TaskFile
	if p.Report.Peer == "" {
		files, err = planFiles(p.Report.Prompt, dir)
	} else {
		files = p.Files
	}
	if err != nil {
		return nil, nil, err
	}
	want, _ := json.Marshal(p.Files)
	got, _ := json.Marshal(files)
	if string(want) != string(got) {
		return nil, nil, fmt.Errorf("plan is stale: referenced files changed; regenerate it")
	}
	expanded := append([]string{}, p.Args...)
	expanded = append(expanded, "--auto-model=false", "--dir", p.Report.Dir, "--agent", p.Report.Agent)
	if p.Report.Model != "" {
		expanded = append(expanded, "--model", p.Report.Model)
	}
	if p.Report.Effort != "" {
		expanded = append(expanded, "--effort", p.Report.Effort)
	}
	if p.Report.Peer == "" && p.Report.PlanState.PaneID != "" {
		expanded = append(expanded, "--target", p.Report.PlanState.PaneID)
	}
	if execute {
		expanded = append(expanded, "--execute")
	}
	if jsonOutput {
		expanded = append(expanded, "--json")
	}
	return expanded, &p, nil
}

func planBoolFlag(a string) bool {
	switch a {
	case "--execute", "--json", "--auto-model", "--no-clear", "--queue", "--wait", "--trust-folder", "--close", "--await":
		return true
	}
	return false
}

func planFiles(prompt, dir string) ([]modelroute.TaskFile, error) {
	return modelroute.FingerprintPromptFiles(prompt, dir)
}

func writeDispatchPlan(command string, args []string, report dispatch.Report) (string, error) {
	folder, err := os.MkdirTemp("", "tmact-plan-")
	if err != nil {
		return "", err
	}
	path := filepath.Join(folder, "plan.json")
	saved := false
	defer func() {
		if !saved {
			os.RemoveAll(folder)
		}
	}()
	if report.Execute {
		return "", fmt.Errorf("plans are dry-run only")
	}
	if report.PlanState == nil {
		report.PlanState = &dispatch.PlanState{SessionExisted: report.SessionExisted}
	}
	// Exclude the ask protocol and capability placeholder from the saved task.
	dir := report.Dir
	if report.PlanState.Dir != "" {
		dir = report.PlanState.Dir
	}
	files := report.PlanFiles
	if report.Peer == "" {
		files, err = planFiles(report.Prompt, dir)
	}
	if err != nil {
		return "", err
	}

	// The saved fingerprints must describe the same files used during planning,
	// including the routing decision (which can take several seconds).
	if report.PlanFiles != nil {
		a, _ := json.Marshal(files)
		b, _ := json.Marshal(report.PlanFiles)
		if string(a) != string(b) {
			return "", fmt.Errorf("plan is stale: referenced files changed while planning; run dry-run again")
		}
	}
	if report.Selection != nil && report.Selection.Status == "selected" {
		if report.Peer == "" {
			current := modelroute.CollectPromptFiles(report.Prompt, dir)
			for i := range current {
				current[i].Content = ""
			}
			a, _ := json.Marshal(current)
			b, _ := json.Marshal(report.Selection.Files)
			if string(a) != string(b) {
				return "", fmt.Errorf("plan is stale: routing attachments changed while deciding; run dry-run again")
			}
		} else {
			for _, routed := range report.Selection.Files {
				for _, fingerprint := range files {
					if routed.Path == fingerprint.Path && (routed.Status != fingerprint.Status || !routed.Truncated && routed.SHA256 != fingerprint.SHA256) {
						return "", fmt.Errorf("plan is stale: peer attachments changed while deciding; run dry-run again")
					}
				}
			}
		}
	}
	clean := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--json" || strings.HasPrefix(a, "--json=") {
			continue
		}
		clean = append(clean, a)
		if strings.HasPrefix(a, "-") && !strings.Contains(a, "=") && !planBoolFlag(a) && i+1 < len(args) {
			i++
			clean = append(clean, args[i])
		}
	}
	p := dispatchPlan{Version: 1, Command: command, CreatedAt: time.Now(), Args: clean, Report: report, Files: files}
	p.Digest = p.digest()
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	// Never truncate an existing artifact or follow an output symlink.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, err = f.Write(append(b, '\n'))
	closeErr := f.Close()
	if err != nil {
		os.Remove(path)
		return "", err
	}
	if closeErr != nil {
		os.Remove(path)
		return "", closeErr
	}
	abs, _ := filepath.Abs(path)
	fmt.Fprintf(os.Stderr, "Saved plan: %s\nApply: %s\n", abs, planApplyCommand(command, abs))
	saved = true
	return abs, nil
}

func planApplyCommand(command, path string) string {
	quoted := "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
	return "tmact " + command + " --apply-plan " + quoted + " --execute"
}
