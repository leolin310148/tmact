package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leolin310148/tmact/internal/panewait"
	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
	"github.com/leolin310148/tmact/internal/web"
)

const (
	// peerWaitCaptureLines matches the local wait capture depth.
	peerWaitCaptureLines = 200
	// peerWaitRetries tolerates transient network failures so one dropped
	// request does not end a long wait; a gone pane is never retried.
	peerWaitRetries    = 5
	peerWaitRetryDelay = time.Second
)

var paneWaitRunWithDependencies = panewait.RunWithDependencies

// peerWaitDependencies observes a peer pane read-only through its statusd
// /api/pane/capture endpoint. Each sample costs one request: ResolveTarget
// fetches the capture and CapturePane returns it, so both describe the same
// moment.
func peerWaitDependencies(peerName, baseURL string) panewait.Dependencies {
	return peerWaitDependenciesWithWait(peerName, baseURL, panewait.DefaultDependencies().Wait)
}

func peerWaitDependenciesWithWait(peerName, baseURL string, wait func(context.Context, time.Duration) error) panewait.Dependencies {
	deps := panewait.DefaultDependencies()
	deps.Wait = wait
	var latest string
	deps.ResolveTarget = func(ctx context.Context, target string) (tmux.CapturePaneInfo, error) {
		_, pane := statusd.SplitPeerTarget(target)
		capture, err := fetchPeerCaptureWithRetry(ctx, deps.Wait, peerName, baseURL, pane)
		if err != nil {
			return tmux.CapturePaneInfo{}, err
		}
		latest = capture.Captures[web.CaptureModeANSI]
		return tmux.CapturePaneInfo{
			Target:      peerName + statusd.PeerSeparator + capture.Target,
			PaneID:      peerName + statusd.PeerSeparator + capture.PaneID,
			HistorySize: capture.HistorySize,
		}, nil
	}
	deps.CapturePane = func(context.Context, string, int) (string, error) {
		return latest, nil
	}
	deps.IsTargetGone = func(err error) bool {
		return errors.Is(err, web.ErrPeerPaneGone)
	}
	return deps
}

func fetchPeerCaptureWithRetry(ctx context.Context, wait func(context.Context, time.Duration) error, peerName, baseURL, pane string) (web.PaneCapture, error) {
	var lastErr error
	for attempt := 0; attempt <= peerWaitRetries; attempt++ {
		if attempt > 0 {
			if err := wait(ctx, peerWaitRetryDelay); err != nil {
				return web.PaneCapture{}, err
			}
		}
		capture, err := fetchPeerPaneCapture(ctx, peerName, baseURL, pane, peerWaitCaptureLines, web.CaptureModeANSI)
		if err == nil {
			return capture, nil
		}
		if errors.Is(err, web.ErrPeerPaneGone) || ctx.Err() != nil {
			return web.PaneCapture{}, err
		}
		lastErr = err
	}
	return web.PaneCapture{}, fmt.Errorf("%w (after %d attempts)", lastErr, peerWaitRetries+1)
}
