package statusd

import (
	"fmt"
	"strings"
)

// EnforcePaneSize resizes every detached tmux window that isn't already at
// cfg.PaneCols x cfg.PaneRows. Attached windows are skipped — tmux would
// immediately re-size them to whatever the attached terminal reports.
// Returns the number of windows resized and a multi-error of per-window
// failures (never blocks the daemon loop).
func EnforcePaneSize(cfg Config) (int, error) {
	if cfg.PaneCols <= 0 || cfg.PaneRows <= 0 {
		return 0, nil
	}
	sizes, err := cfg.ListWindowSizes()
	if err != nil {
		return 0, err
	}
	var (
		resized int
		errs    []string
	)
	for _, w := range sizes {
		if w.Attached {
			continue
		}
		if w.Width == cfg.PaneCols && w.Height == cfg.PaneRows {
			continue
		}
		if err := cfg.ResizeWindow(w.WindowID, cfg.PaneCols, cfg.PaneRows); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", w.WindowID, err))
			continue
		}
		resized++
	}
	if len(errs) > 0 {
		return resized, fmt.Errorf("resize window: %s", strings.Join(errs, "; "))
	}
	return resized, nil
}
