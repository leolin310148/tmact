package web

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

const (
	uploadsGCInterval = 1 * time.Hour
	uploadsMaxAge     = 7 * 24 * time.Hour
)

// runUploadsGC sweeps the paste/upload staging dirs hourly, deleting files
// older than uploadsMaxAge. Both dirs only ever hold transient handoff
// artifacts (the agent has already consumed the path it was sent), so a hard
// age cutoff is enough — no LRU, no quota tracking. Skips silently if a dir
// does not exist or cannot be read; logs only when removal fails.
func (s *Server) runUploadsGC(ctx context.Context) {
	sweep := func() {
		s.sweepDir(s.pasteImageDir())
		s.sweepDir(s.uploadDir())
	}
	sweep()

	t := time.NewTicker(uploadsGCInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweep()
		}
	}
}

func (s *Server) sweepDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-uploadsMaxAge)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := os.Remove(path); err != nil {
			s.logf("uploads-gc: remove %s: %v", path, err)
		}
	}
}
