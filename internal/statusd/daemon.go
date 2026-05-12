package statusd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Daemon struct {
	cfg Config
	mem *Memory
}

func NewDaemon(cfg Config) *Daemon {
	return &Daemon{cfg: cfg.withDefaults(), mem: NewMemory()}
}

func (d *Daemon) RunOnce(ctx context.Context) (Snapshot, error) {
	snapshot, scanErr := BuildSnapshot(ctx, d.cfg, d.mem)
	if scanErr != nil {
		if d.cfg.LogPath != "" {
			_ = appendLog(d.cfg.LogPath, snapshot)
		}
		return snapshot, scanErr
	}
	if d.cfg.TmuxOptions {
		if err := PublishTmuxOptions(d.cfg, snapshot); err != nil {
			snapshot.addError("tmux_options", "", err)
			if scanErr == nil {
				scanErr = err
			}
		}
	}
	if err := WriteSnapshot(d.cfg.StatePath, snapshot); err != nil {
		if scanErr != nil {
			return snapshot, scanErr
		}
		return snapshot, err
	}
	if d.cfg.LogPath != "" {
		_ = appendLog(d.cfg.LogPath, snapshot)
	}
	return snapshot, scanErr
}

func (d *Daemon) Start(ctx context.Context) error {
	for {
		_, _ = d.RunOnce(ctx)

		timer := time.NewTimer(d.cfg.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func appendLog(path string, snapshot Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	return encoder.Encode(snapshot)
}
