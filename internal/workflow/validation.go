package workflow

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

type ValidationResult struct {
	Command      string    `json:"command"`
	Args         []string  `json:"args"`
	ChangeHash   string    `json:"change_hash"`
	Passed       bool      `json:"passed"`
	Stale        bool      `json:"stale"`
	ExitCode     int       `json:"exit_code"`
	Stdout       string    `json:"stdout,omitempty"`
	Stderr       string    `json:"stderr,omitempty"`
	Error        string    `json:"error,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Artifacts    []string  `json:"artifacts,omitempty"`
	HashAfterRun string    `json:"hash_after_run,omitempty"`
}

type Validator func(context.Context, string, string) (ValidationResult, error)

func RunOpenSpecValidation(ctx context.Context, change string, changeDir string) (ValidationResult, error) {
	started := time.Now()
	hashBefore, artifacts, hashErr := HashChangeDir(changeDir)
	if hashErr != nil {
		return ValidationResult{
			Command:    "openspec",
			Args:       []string{"validate", change, "--strict"},
			ExitCode:   -1,
			Error:      hashErr.Error(),
			StartedAt:  started,
			FinishedAt: time.Now(),
		}, hashErr
	}

	cmd := exec.CommandContext(ctx, "openspec", "validate", change, "--strict")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	finished := time.Now()

	exitCode := 0
	passed := err == nil
	if err != nil {
		exitCode = -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
	}
	hashAfter, _, afterErr := HashChangeDir(changeDir)
	stale := afterErr == nil && hashAfter != hashBefore
	result := ValidationResult{
		Command:      "openspec",
		Args:         []string{"validate", change, "--strict"},
		ChangeHash:   hashBefore,
		Passed:       passed && !stale,
		Stale:        stale,
		ExitCode:     exitCode,
		Stdout:       stdout.String(),
		Stderr:       stderr.String(),
		StartedAt:    started,
		FinishedAt:   finished,
		Artifacts:    artifacts,
		HashAfterRun: hashAfter,
	}
	if err != nil {
		result.Error = err.Error()
	}
	if afterErr != nil && err == nil {
		result.Passed = false
		result.Error = afterErr.Error()
		return result, afterErr
	}
	return result, nil
}
