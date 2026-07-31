package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
)

func peerTestSnapshot() statusd.Snapshot {
	return statusd.Snapshot{
		Panes: map[string]statusd.PaneStatus{
			"%40": {Target: "%40", Session: "puni-gw", WindowIndex: 0, Window: "main", PaneIndex: 0, CurrentCommand: "zsh", CWD: "/repo/gw"},
			"%12": {Target: "%12", Session: "alpha", WindowIndex: 1, Window: "edit", PaneIndex: 2, CurrentCommand: "claude", CWD: "/repo/alpha"},
			"%99": {Target: "%99", Session: "beta", CurrentCommand: "codex", CWD: "/repo/beta", Peer: "downstream"},
		},
	}
}

func TestPeerPaneRowsDropsPanesThePeerMergedFromItsOwnPeers(t *testing.T) {
	rows := peerPaneRows(peerTestSnapshot(), "studio", time.Unix(0, 0))

	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (the downstream-merged pane must be dropped): %#v", len(rows), rows)
	}
	for _, row := range rows {
		if strings.Contains(row.Target, "downstream") || row.Session == "beta" {
			t.Fatalf("row leaked a pane the peer merged from its own peers: %#v", row)
		}
	}
}

func TestPeerPaneRowsQualifiesTargetsAndSortsStably(t *testing.T) {
	rows := peerPaneRows(peerTestSnapshot(), "studio", time.Unix(0, 0))

	// Sorted by session, so alpha precedes puni-gw regardless of map order.
	if rows[0].Session != "alpha" || rows[1].Session != "puni-gw" {
		t.Fatalf("unsorted rows: %#v", rows)
	}
	if rows[0].Target != "studio@%12" || rows[1].Target != "studio@%40" {
		t.Fatalf("targets not peer-qualified: %#v", rows)
	}
	if rows[0].Index != 0 || rows[1].Index != 1 {
		t.Fatalf("indexes not renumbered: %#v", rows)
	}
	if rows[1].WindowName != "main" || rows[1].CurrentCommand != "zsh" || rows[1].CurrentPath != "/repo/gw" {
		t.Fatalf("row fields not carried over: %#v", rows[1])
	}
	// A peer-qualified target must survive cache resolution without a local
	// tmux probe, which is what makes `-t N` usable after `ls --peer`.
	if peer, rest := statusd.SplitPeerTarget(rows[1].Target); peer != "studio" || rest != "%40" {
		t.Fatalf("SplitPeerTarget(%q) = %q, %q", rows[1].Target, peer, rest)
	}
}

func TestResolvePeerConfigPrefersDispatchPeersOverSnapshotPeers(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "statusd.json")
	writeText(t, configPath, `{
		"peers":[{"name":"peer-a","url":"http://snapshot-peer.example:7890"}],
		"dispatch_peers":[{"name":"peer-a","url":"http://dispatch-peer.example:7890"}]
	}`)

	peer, err := resolvePeerConfig("peer-a", configPath)
	if err != nil {
		t.Fatal(err)
	}
	if peer.URL != "http://dispatch-peer.example:7890" {
		t.Fatalf("url = %s", peer.URL)
	}
}

func TestResolvePeerConfigFallsBackToSnapshotPeers(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "statusd.json")
	writeText(t, configPath, `{"peers":[{"name":"peer-a","url":"http://peer-a.example:7890"}]}`)

	peer, err := resolvePeerConfig("peer-a", configPath)
	if err != nil {
		t.Fatal(err)
	}
	if peer.URL != "http://peer-a.example:7890" {
		t.Fatalf("url = %s", peer.URL)
	}
}

func TestResolvePeerConfigErrorsWhenPeerMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "statusd.json")
	writeText(t, configPath, `{"dispatch_peers":[{"name":"other","url":"http://other.example:7890"}]}`)

	_, err := resolvePeerConfig("peer-a", configPath)
	if err == nil || !strings.Contains(err.Error(), "not found in dispatch_peers or peers") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchPeerSnapshotReadsSnapshotEndpoint(t *testing.T) {
	var gotPath, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewEncoder(w).Encode(peerTestSnapshot())
	}))
	defer server.Close()

	snap, err := fetchPeerSnapshot(t.Context(), server.Client(), "studio", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/snapshot" || gotMethod != http.MethodGet {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if len(snap.Panes) != 3 {
		t.Fatalf("panes = %d", len(snap.Panes))
	}
}

func TestFetchPeerSnapshotReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := fetchPeerSnapshot(t.Context(), server.Client(), "studio", server.URL)
	if err == nil || !strings.Contains(err.Error(), "no panes to report") {
		t.Fatalf("err = %v", err)
	}
}

func TestListPeerNeverTouchesLocalTmux(t *testing.T) {
	defer stubCLIHooks(t)()
	t.Chdir(t.TempDir())

	listAllTmuxPanes = func() ([]tmux.Pane, error) {
		t.Fatal("ls --peer must not enumerate local tmux panes")
		return nil, nil
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(peerTestSnapshot())
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "statusd.json")
	writeText(t, configPath, `{"dispatch_peers":[{"name":"studio","url":"`+server.URL+`"}]}`)

	out, err := captureRun(t, "ls", "--peer", "studio", "--config", configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"studio@%12", "studio@%40", "puni-gw", "alpha"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
	if strings.Contains(out, "beta") {
		t.Fatalf("output leaked a downstream-merged pane: %s", out)
	}
}
