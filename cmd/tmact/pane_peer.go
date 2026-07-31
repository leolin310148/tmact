package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
)

// peerListTimeout bounds the single read-only snapshot fetch. `ls --peer` is an
// on-demand query, not federation: nothing is merged into a local snapshot and
// no polling loop is started, so a peer that is down costs one timeout.
const peerListTimeout = 10 * time.Second

// resolvePeerConfig finds a peer by name, preferring dispatch_peers over peers
// so a dispatch-only peer is listable without granting it snapshot federation.
func resolvePeerConfig(peerName, configPath string) (statusd.PeerFileConfig, error) {
	if configPath == "" {
		return statusd.PeerFileConfig{}, fmt.Errorf("ls --peer requires --config or a default statusd config path")
	}
	cfg, err := statusd.LoadFileConfig(configPath)
	if err != nil {
		return statusd.PeerFileConfig{}, fmt.Errorf("load statusd config %s: %w", configPath, err)
	}
	for _, list := range [][]statusd.PeerFileConfig{cfg.DispatchPeers, cfg.Peers} {
		if p, ok := findPeerConfig(list, peerName); ok {
			if p.URL == "" {
				return statusd.PeerFileConfig{}, fmt.Errorf("peer %q has empty url in %s", peerName, configPath)
			}
			return p, nil
		}
	}
	return statusd.PeerFileConfig{}, fmt.Errorf("peer %q not found in dispatch_peers or peers in %s", peerName, configPath)
}

func fetchPeerSnapshot(ctx context.Context, client *http.Client, peerName, baseURL string) (statusd.Snapshot, error) {
	if client == nil {
		client = http.DefaultClient
	}
	reqCtx, cancel := context.WithTimeout(ctx, peerListTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, statusd.PeerSnapshotURL(baseURL), nil)
	if err != nil {
		return statusd.Snapshot{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return statusd.Snapshot{}, fmt.Errorf("peer %s snapshot request failed: %w", peerName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusServiceUnavailable {
			return statusd.Snapshot{}, fmt.Errorf("peer %s has no panes to report (HTTP 503)", peerName)
		}
		return statusd.Snapshot{}, fmt.Errorf("peer %s snapshot returned HTTP %d", peerName, resp.StatusCode)
	}
	var snap statusd.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return statusd.Snapshot{}, fmt.Errorf("peer %s snapshot response invalid: %w", peerName, err)
	}
	return snap, nil
}

// peerPaneRows converts a peer snapshot into list rows. Panes the peer itself
// merged in from its own peers are dropped: `ls --peer NAME` answers "what runs
// on NAME", not "what NAME can see". Targets are returned peer-qualified so
// they can never be mistaken for a local pane id.
func peerPaneRows(snap statusd.Snapshot, peerName string, now time.Time) []listPaneRow {
	panes := make([]statusd.PaneStatus, 0, len(snap.Panes))
	for _, pane := range snap.Panes {
		if pane.Peer != "" {
			continue
		}
		panes = append(panes, pane)
	}
	sort.Slice(panes, func(i, j int) bool {
		if panes[i].Session != panes[j].Session {
			return panes[i].Session < panes[j].Session
		}
		if panes[i].WindowIndex != panes[j].WindowIndex {
			return panes[i].WindowIndex < panes[j].WindowIndex
		}
		if panes[i].PaneIndex != panes[j].PaneIndex {
			return panes[i].PaneIndex < panes[j].PaneIndex
		}
		return panes[i].Target < panes[j].Target
	})
	rows := make([]listPaneRow, 0, len(panes))
	for i, pane := range panes {
		rows = append(rows, listPaneRow{
			Index:          i,
			Target:         peerName + statusd.PeerSeparator + pane.Target,
			Session:        pane.Session,
			WindowIndex:    pane.WindowIndex,
			WindowName:     pane.Window,
			PaneIndex:      pane.PaneIndex,
			CurrentCommand: pane.CurrentCommand,
			CurrentPath:    pane.CWD,
			GeneratedAt:    now,
		})
	}
	return rows
}
