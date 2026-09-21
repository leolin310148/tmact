package dispatch

import (
	"fmt"
	"time"

	"github.com/leolin310148/tmact/internal/foldertrust"
	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/prompt"
)

func waitReady(opts Options, deps Deps, target string, trustedFolder bool) (bool, error) {
	deadline := deps.Now().Add(opts.ReadyTimeout)
	var readySince time.Time
	for {
		panes, err := deps.ListSessionPanes(opts.Session)
		if err != nil {
			return trustedFolder, fmt.Errorf("wait for %s: %w", opts.Agent, err)
		}
		pane, ok := findPane(panes, target)
		if !ok {
			return trustedFolder, fmt.Errorf("wait for %s: pane %s disappeared", opts.Agent, target)
		}
		raw, err := deps.CapturePane(target, captureLines)
		if err != nil {
			return trustedFolder, fmt.Errorf("wait for %s: %w", opts.Agent, err)
		}
		classified, err := classifyPane(deps, target, raw)
		if err != nil {
			return trustedFolder, fmt.Errorf("wait for %s: %w", opts.Agent, err)
		}
		runtime := detectRuntime(deps, pane, raw)
		if classified.Asking {
			if opts.TrustFolder && classified.InteractivePrompt != nil && classified.InteractivePrompt.Type == prompt.TypeTrustFolder {
				if trustedFolder {
					if !deps.Now().Before(deadline) {
						return trustedFolder, fmt.Errorf("%s trust-folder prompt remained after it was accepted", opts.Agent)
					}
					deps.Sleep(pollInterval)
					continue
				}
				result, err := foldertrust.AcceptPrompt(foldertrust.Options{
					Target: target,
					Dir:    opts.Dir,
					Agent:  opts.Agent,
				}, pane, raw, runtime, deps.SendKeys)
				if err != nil {
					return trustedFolder, err
				}
				if !result.Accepted {
					return trustedFolder, fmt.Errorf("%s trust-folder prompt was detected but not accepted", opts.Agent)
				}
				trustedFolder = true
				readySince = time.Time{}
				deps.Sleep(pollInterval)
				continue
			}
			return trustedFolder, fmt.Errorf("%s startup is waiting on a prompt (%s); refusing to auto-confirm", opts.Agent, promptKind(classified))
		}
		if runtime == opts.Agent && isReadyToDispatch(opts, deps, classified) {
			now := deps.Now()
			if opts.ReadySettle <= 0 {
				return trustedFolder, nil
			}
			if readySince.IsZero() {
				readySince = now
			}
			if now.Sub(readySince) >= opts.ReadySettle {
				return trustedFolder, nil
			}
		} else {
			readySince = time.Time{}
		}
		if !deps.Now().Before(deadline) {
			return trustedFolder, fmt.Errorf("%s did not become ready within %s (runtime=%s state=%s)", opts.Agent, opts.ReadyTimeout, runtime, classified.State)
		}
		sleep := pollInterval
		if !readySince.IsZero() {
			remaining := opts.ReadySettle - deps.Now().Sub(readySince)
			if remaining > 0 && remaining < sleep {
				sleep = remaining
			}
		}
		deps.Sleep(sleep)
	}
}

// isReadyState allowlists the states a freshly launched agent may be dispatched
// into. Anything else — an unrecognized full-screen dialog, an operator draft,
// a blocked or unknown pane — must keep waiting rather than receive keystrokes:
// a screen this build cannot classify is exactly where a dispatched prompt can
// answer a question nobody meant to answer.
func isReadyState(state string) bool {
	switch state {
	case panestate.StateWaitingInput, panestate.StateIdle:
		return true
	default:
		return false
	}
}

// isReadyToDispatch adds the one state outside that allowlist a launch may
// still use: a Codex limit screen an explicit, still-matching quota resume
// already vouched for, the same exception dispatchExisting makes.
func isReadyToDispatch(opts Options, deps Deps, classified panestate.Result) bool {
	if isReadyState(classified.State) {
		return true
	}
	if classified.State != panestate.StateWaitingQuota {
		return false
	}
	return validateQuotaResume(opts, deps, classified) == nil
}

func readyDetail(opts Options) string {
	detail := ""
	if opts.ReadySettle > 0 {
		detail = fmt.Sprintf("wait up to %s for %s to be ready, then stable for %s", opts.ReadyTimeout, opts.Agent, opts.ReadySettle)
	} else {
		detail = fmt.Sprintf("wait up to %s for %s to be ready", opts.ReadyTimeout, opts.Agent)
	}
	if opts.TrustFolder {
		detail += "; accept only an exact-cwd trust-folder prompt"
	}
	return detail
}
