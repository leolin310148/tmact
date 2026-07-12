package panestatus

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Process struct {
	PID     int
	PPID    int
	Command string
	Args    string
}

func DetectChildProcessRuntime(parentPID int) RuntimeDetection {
	processes, err := childProcessTree(parentPID, 3)
	if err != nil {
		return RuntimeDetection{Runtime: RuntimeUnknown, Confidence: ConfidenceLow}
	}
	return ClassifyProcessRuntime(processes)
}

func ClassifyProcessRuntime(processes []Process) RuntimeDetection {
	for _, process := range processes {
		if runtime := runtimeFromProcess(process); runtime != RuntimeUnknown {
			return RuntimeDetection{
				Runtime:    runtime,
				Confidence: ConfidenceHigh,
				Signals:    []string{"child_process"},
			}
		}
	}
	return RuntimeDetection{Runtime: RuntimeUnknown, Confidence: ConfidenceLow}
}

func childProcessTree(parentPID int, maxDepth int) ([]Process, error) {
	if parentPID <= 0 || maxDepth <= 0 {
		return nil, nil
	}

	result, err := processesByPID([]int{parentPID})
	if err != nil {
		return nil, err
	}
	frontier := []int{parentPID}
	seen := map[int]bool{parentPID: true}
	for depth := 0; depth < maxDepth && len(frontier) > 0 && len(result) < 64; depth++ {
		children, err := directChildren(frontier)
		if err != nil {
			return result, err
		}
		if len(children) == 0 {
			return result, nil
		}

		var next []int
		for _, child := range children {
			if seen[child.PID] {
				continue
			}
			seen[child.PID] = true
			result = append(result, child)
			next = append(next, child.PID)
		}
		frontier = next
	}
	return result, nil
}

func directChildren(parentPIDs []int) ([]Process, error) {
	childIDs, err := childPIDs(parentPIDs)
	if err != nil {
		return nil, err
	}
	if len(childIDs) == 0 {
		return nil, nil
	}
	return processesByPID(childIDs)
}

func childPIDs(parentPIDs []int) ([]int, error) {
	var childIDs []int
	for _, parentPID := range parentPIDs {
		output, err := commandOutput("pgrep", "-P", strconv.Itoa(parentPID))
		if err != nil {
			if exitCode(err) == 1 {
				continue
			}
			return childIDs, err
		}
		for _, field := range strings.Fields(output) {
			pid, err := strconv.Atoi(field)
			if err != nil {
				continue
			}
			childIDs = append(childIDs, pid)
		}
	}
	return childIDs, nil
}

func processesByPID(pids []int) ([]Process, error) {
	if len(pids) == 0 {
		return nil, nil
	}
	values := make([]string, 0, len(pids))
	for _, pid := range pids {
		values = append(values, strconv.Itoa(pid))
	}
	output, err := commandOutput("ps", "-o", "pid=,ppid=,comm=,command=", "-p", strings.Join(values, ","))
	if err != nil {
		return nil, err
	}
	return parseProcesses(output), nil
}

func parseProcesses(output string) []Process {
	var processes []Process
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		processes = append(processes, Process{
			PID:     pid,
			PPID:    ppid,
			Command: fields[2],
			Args:    strings.Join(fields[3:], " "),
		})
	}
	return processes
}

func runtimeFromProcess(process Process) string {
	for _, candidate := range []string{process.Command, firstArg(process.Args)} {
		switch normalizeCommand(candidate) {
		case "claude":
			return RuntimeClaude
		case "codex", "codex-aarch64-apple-darwin", "codex-aarch64-a":
			return RuntimeCodex
		case "gemini":
			return RuntimeGemini
		case "tmact":
			return RuntimeTmact
		}
	}
	text := strings.ToLower(process.Command + " " + process.Args)
	switch {
	case strings.Contains(text, "claude"):
		return RuntimeClaude
	case strings.Contains(text, "codex"):
		return RuntimeCodex
	case strings.Contains(text, "gemini"):
		return RuntimeGemini
	case strings.Contains(text, "tmact"):
		return RuntimeTmact
	}
	return RuntimeUnknown
}

func firstArg(args string) string {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func normalizeCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	command = filepath.Base(command)
	command = strings.TrimPrefix(command, "-")
	return strings.ToLower(command)
}

func commandOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return -1
	}
	return exitErr.ExitCode()
}
