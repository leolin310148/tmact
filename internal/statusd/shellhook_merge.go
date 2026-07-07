package statusd

import (
	"time"

	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/shellhook"
)

// Shell hook signals added to PaneStatus.Signals when hook data influenced
// (or corroborated) the pane's classification.
const (
	SignalShellHook            = "shell_hook"
	SignalShellHookActive      = "shell_hook_active"
	SignalShellHookActiveStale = "shell_hook_active_stale"
	SignalShellHookCompleted   = "shell_hook_completed"
)

// shellHookPruneGrace keeps unseen-but-recent hook entries alive across
// Prune so an event racing the pane scan is not dropped.
const shellHookPruneGrace = time.Minute

// shellHookActiveGrace is how long an unfinished preexec outranks a capture
// that shows an input-ready prompt. Past it, a visible prompt means the
// command's precmd was lost (emits are fire-and-forget) or the foreground
// program is itself sitting at an input prompt — either way the capture is
// the better signal. Within it, the hook wins so the prompt still on screen
// right after a command starts doesn't mask a fresh preexec.
const shellHookActiveGrace = 10 * time.Second

// ApplyShellHooks overlays opt-in shell preexec/precmd state onto local
// panes, then recomputes sessions and summary. The capture-based heuristics
// stay authoritative wherever they carry stronger evidence:
//
//   - An unfinished preexec marks the pane working/running.
//   - A completed command (matching precmd received) marks the pane
//     idle/input-ready — unless the capture explicitly says working
//     ("working_text"), which means something is still producing output
//     outside the shell's foreground command.
//   - An asking pane is never touched beyond signals: state, running, idle,
//     and input_ready all keep their capture-derived values so summaries,
//     sessions, and tmux options keep surfacing the prompt.
//   - An active command older than shellHookActiveGrace loses to a capture
//     that shows an idle or input-ready prompt (not running): the precmd was
//     lost or the foreground program is back at / waiting on a prompt. It
//     records shell_hook_active_stale instead of shell_hook_active so the
//     snapshot does not look actively driven by the hook.
//   - Panes without hook data (or with a capture error) are untouched, so
//     the existing panestate/capture-hash behavior is the fallback.
func ApplyShellHooks(snapshot Snapshot, states map[string]shellhook.PaneState) Snapshot {
	if len(states) == 0 {
		return snapshot
	}
	changed := false
	for key, pane := range snapshot.Panes {
		if pane.PaneID == "" || pane.Error != "" {
			continue
		}
		state, ok := states[pane.PaneID]
		if !ok {
			continue
		}
		switch {
		case state.Active != nil:
			staleActive := staleActiveCommand(pane, state.Active, snapshot.Timestamp)
			if staleActive {
				pane.Signals = appendSignals(pane.Signals, SignalShellHook, SignalShellHookActiveStale)
			} else {
				pane.Signals = appendSignals(pane.Signals, SignalShellHook, SignalShellHookActive)
			}
			if !pane.Asking && !staleActive {
				pane.Running = true
				pane.Idle = false
				pane.InputReady = false
				pane.State = panestate.StateWorking
			}
		case state.Completed != nil:
			pane.Signals = appendSignals(pane.Signals, SignalShellHook, SignalShellHookCompleted)
			if !pane.Asking && !hasSignal(pane.Signals, "working_text") {
				pane.Running = false
				pane.Idle = true
				pane.InputReady = true
				if pane.State != panestate.StateWaitingInput {
					pane.State = panestate.StateIdle
				}
			}
		default:
			continue
		}
		snapshot.Panes[key] = pane
		changed = true
	}
	if changed {
		snapshot.Sessions = buildSessions(snapshot.Panes, snapshot.Timestamp)
		snapshot.Summary = summarize(snapshot)
	}
	return snapshot
}

// staleActiveCommand reports whether an unfinished preexec should lose to
// the capture: the pane shows an idle or input-ready prompt (not running),
// and the command started long enough ago that the on-screen prompt cannot
// be a leftover from just before it ran. This covers both a plain idle
// prompt and an explicit waiting_input prompt — once the grace has passed a
// visible ready prompt means the precmd was lost, so the capture wins.
func staleActiveCommand(pane PaneStatus, active *shellhook.CommandRecord, now time.Time) bool {
	captureReady := !pane.Running && (pane.Idle || pane.InputReady ||
		pane.State == panestate.StateIdle || pane.State == panestate.StateWaitingInput)
	return captureReady && now.Sub(active.StartedAt) > shellHookActiveGrace
}

func appendSignals(signals []string, add ...string) []string {
	for _, signal := range add {
		if !hasSignal(signals, signal) {
			signals = append(signals, signal)
		}
	}
	return signals
}

func hasSignal(signals []string, want string) bool {
	for _, signal := range signals {
		if signal == want {
			return true
		}
	}
	return false
}
