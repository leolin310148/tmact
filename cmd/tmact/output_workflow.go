package main

import (
	"fmt"
	"strings"

	"github.com/leolin310148/tmact/internal/workflow"
)

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
