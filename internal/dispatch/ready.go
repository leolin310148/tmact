package dispatch

import (
	"fmt"
	"time"

	"github.com/leolin310148/tmact/internal/panestate"
)

func waitReady(opts Options, deps Deps, target string) error {
	deadline := deps.Now().Add(opts.ReadyTimeout)
	var readySince time.Time
	for {
		panes, err := deps.ListSessionPanes(opts.Session)
		if err != nil {
			return fmt.Errorf("wait for %s: %w", opts.Agent, err)
		}
		pane, ok := findPane(panes, target)
		if !ok {
			return fmt.Errorf("wait for %s: pane %s disappeared", opts.Agent, target)
		}
		raw, err := deps.CapturePane(target, captureLines)
		if err != nil {
			return fmt.Errorf("wait for %s: %w", opts.Agent, err)
		}
		classified := panestate.Classify(raw)
		if classified.Asking {
			return fmt.Errorf("%s startup is waiting on a prompt (%s); refusing to auto-confirm", opts.Agent, promptKind(classified))
		}
		runtime := detectRuntime(deps, pane, raw)
		if runtime == opts.Agent && classified.State != panestate.StateWorking {
			now := deps.Now()
			if opts.ReadySettle <= 0 {
				return nil
			}
			if readySince.IsZero() {
				readySince = now
			}
			if now.Sub(readySince) >= opts.ReadySettle {
				return nil
			}
		} else {
			readySince = time.Time{}
		}
		if !deps.Now().Before(deadline) {
			return fmt.Errorf("%s did not become ready within %s (runtime=%s state=%s)", opts.Agent, opts.ReadyTimeout, runtime, classified.State)
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
	if opts.ReadySettle > 0 {
		return fmt.Sprintf("wait up to %s for %s to be ready, then stable for %s", opts.ReadyTimeout, opts.Agent, opts.ReadySettle)
	}
	return fmt.Sprintf("wait up to %s for %s to be ready", opts.ReadyTimeout, opts.Agent)
}
