package dispatch

import (
	"fmt"
	"time"

	"github.com/leolin310148/tmact/internal/foldertrust"
	"github.com/leolin310148/tmact/internal/panestate"
	"github.com/leolin310148/tmact/internal/prompt"
)

func waitReady(opts Options, deps Deps, target string) (bool, error) {
	deadline := deps.Now().Add(opts.ReadyTimeout)
	var readySince time.Time
	trustedFolder := false
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
		if runtime == opts.Agent && classified.State != panestate.StateWorking {
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
