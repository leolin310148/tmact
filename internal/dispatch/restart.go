package dispatch

import (
	"fmt"
	"time"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/panestatus"
	"github.com/leolin310148/tmact/internal/tmux"
)

// dispatchRestart replaces only an explicitly idle Claude/Codex. Decide must
// succeed before /exit; the old process must exit before any shell command is
// pasted. A blocker or timeout stops the operation without escalation to kill.
func dispatchRestart(opts Options, deps Deps, report Report, pane tmux.Pane, target, oldAgent string, classified panestate.Result) (Report, error) {
	if classified.Asking || classified.State != panestate.StateWaitingInput && classified.State != panestate.StateIdle {
		return report, fmt.Errorf("session %s is not input-ready (%s); refusing to replace %s", opts.Session, classified.State, oldAgent)
	}
	report.AgentWasRunning = true
	var err error
	opts, report, err = prepareAutoLaunch(opts, deps, report)
	if err != nil || report.RoutingInput != nil {
		return report, err
	}
	opts.AutoModel = false
	opts.ProbeAutoModel = false
	plan := opts
	plan.Execute = false
	planned, err := dispatchLaunch(plan, deps, report, target)
	if err != nil {
		return planned, err
	}
	steps := []Step{{Name: "exit-agent", Detail: "send /exit to idle " + oldAgent, Status: StatusPlanned}, {Name: "wait-shell", Detail: "confirm old agent exited in the same pane", Status: StatusPlanned}}
	if !opts.Execute {
		planned.Steps = append(steps, planned.Steps...)
		return planned, nil
	}
	previous := opts
	previous.Agent = oldAgent
	if err := settleExistingAgent(previous, deps, pane, target); err != nil {
		return report, err
	}
	// Refresh pane metadata after the decision: stale current_command data
	// must not let /exit land in a shell if the old process already left.
	panes, err := deps.ListSessionPanes(opts.Session)
	if err != nil {
		return report, err
	}
	fresh, ok := findPane(panes, target)
	if !ok {
		return report, fmt.Errorf("pane %s disappeared before /exit", target)
	}
	raw, err := deps.CapturePane(target, captureLines)
	if err != nil {
		return report, err
	}
	if runtime := detectRuntime(deps, fresh, raw); runtime != oldAgent {
		return report, fmt.Errorf("agent runtime changed before /exit; refusing to replace")
	}
	if deps.CapturePaneANSI == nil {
		return report, fmt.Errorf("styled capture required before replacing an agent")
	}
	current, draft, err := captureInput(deps, target)
	if err != nil {
		return report, err
	}
	if current.Asking || draft || current.State != panestate.StateWaitingInput && current.State != panestate.StateIdle {
		return report, fmt.Errorf("agent changed state or contains a draft before /exit; refusing to replace")
	}
	if err = deps.PasteText(target, "/exit", false); err == nil {
		err = waitExitInput(opts, deps, target)
		if err == nil {
			err = deps.SendKeys(target, []string{"Enter"})
		}
	}

	if err != nil {
		steps[0].Status = StatusFailed
		report.Steps = steps
		return report, fmt.Errorf("exit %s: %w", oldAgent, err)
	}
	steps[0].Status = StatusOK
	if err = waitShell(opts, deps, target, oldAgent); err != nil {
		steps[1].Status = StatusFailed
		report.Steps = steps
		return report, err
	}
	steps[1].Status = StatusOK
	report, err = dispatchLaunch(opts, deps, report, target)
	report.Steps = append(steps, report.Steps...)
	return report, err
}

func waitShell(opts Options, deps Deps, target, oldAgent string) error {
	deadline := deps.Now().Add(opts.ReadyTimeout)
	for {
		if opts.Context != nil && opts.Context.Err() != nil {
			return opts.Context.Err()
		}
		panes, err := deps.ListSessionPanes(opts.Session)
		if err != nil {
			return err
		}
		pane, ok := findPane(panes, target)
		if !ok {
			return fmt.Errorf("pane %s disappeared while exiting %s", target, oldAgent)
		}
		raw, err := deps.CapturePane(target, captureLines)
		if err != nil {
			return err
		}
		runtime := detectRuntime(deps, pane, raw)
		if runtime == panestatus.RuntimeShell {
			return nil
		}
		if runtime != oldAgent {
			return fmt.Errorf("unexpected runtime %s while exiting %s; refusing to launch", runtime, oldAgent)
		}
		classified, err := classifyPane(deps, target, raw)
		if err != nil {
			return err
		}
		if classified.Asking {
			return fmt.Errorf("%s is waiting on a prompt while exiting; resolve it manually", oldAgent)
		}
		if !deps.Now().Before(deadline) {
			return fmt.Errorf("%s did not return to shell within %s; no new agent launched", oldAgent, opts.ReadyTimeout)
		}
		deps.Sleep(pollInterval)
	}
}

// A paste can return before the TUI paints. Wait for our exact command without
// repeating the paste, and never press Enter while an approval prompt appears.
func waitExitInput(opts Options, deps Deps, target string) error {
	deadline := deps.Now().Add(3 * time.Second)
	for {
		if opts.Context != nil && opts.Context.Err() != nil {
			return opts.Context.Err()
		}
		raw, err := deps.CapturePane(target, captureLines)
		if err != nil {
			return err
		}
		classified, err := classifyPane(deps, target, raw)
		if err != nil {
			return err
		}
		if classified.Asking {
			return fmt.Errorf("agent raised a prompt before /exit Enter; resolve it manually")
		}
		if promptInInputBox(raw, "/exit") {
			return nil
		}
		if !deps.Now().Before(deadline) {
			return fmt.Errorf("cannot confirm /exit in the old agent input; refusing Enter")
		}
		deps.Sleep(100 * time.Millisecond)
	}
}
