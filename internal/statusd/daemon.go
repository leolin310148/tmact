package statusd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Daemon struct {
	cfg         Config
	mem         *Memory
	store       *Store
	optionCache *TmuxOptionCache
	peers       *PeerFetcher
}

func NewDaemon(cfg Config) *Daemon {
	cfg = cfg.withDefaults()
	d := &Daemon{cfg: cfg, mem: NewMemory(), store: NewStore(), optionCache: NewTmuxOptionCache()}
	if len(cfg.Peers) > 0 {
		d.peers = NewPeerFetcher(cfg.Peers, cfg.PeerInterval, cfg.PeerTimeout)
	}
	return d
}

// Peers returns the peer fetcher, or nil when no peers are configured.
func (d *Daemon) Peers() *PeerFetcher { return d.peers }

// Store returns the in-memory snapshot store the daemon publishes to. The web
// server and IPC handlers read from this; nothing is written to disk.
func (d *Daemon) Store() *Store {
	return d.store
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
		if err := PublishTmuxOptions(d.cfg, snapshot, d.optionCache); err != nil {
			snapshot.addError("tmux_options", "", err)
			if scanErr == nil {
				scanErr = err
			}
		}
	}
	if _, err := EnforcePaneSize(d.cfg); err != nil {
		snapshot.addError("pane_size", "", err)
	}
	// PublishTmuxOptions / EnforcePaneSize run before the merge so they only
	// ever touch local tmux. Merging remote peers afterwards keeps /api/snapshot
	// and the web UI in sync without confusing the local tmux integration.
	if d.peers != nil {
		snapshot = MergePeers(snapshot, d.peers.Latest())
	}
	d.store.Publish(snapshot)
	if d.cfg.LogPath != "" {
		_ = appendLog(d.cfg.LogPath, snapshot)
	}
	return snapshot, scanErr
}

func (d *Daemon) Start(ctx context.Context) error {
	if d.peers != nil {
		d.peers.Start(ctx)
	}
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
