package statusd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMergePeersPrefixesIDsAndSetsPeer(t *testing.T) {
	local := Snapshot{
		Version:  1,
		Sessions: map[string]SessionStatus{"main": {Session: "main", ActiveTarget: "main:0.0"}},
		Panes: map[string]PaneStatus{
			"main:0.0": {Target: "main:0.0", Session: "main", PaneID: "%5"},
		},
	}
	remote := Snapshot{
		Version: 1,
		Sessions: map[string]SessionStatus{
			"probe": {Session: "probe", ActiveTarget: "probe:0.0"},
		},
		Panes: map[string]PaneStatus{
			"probe:0.0": {Target: "probe:0.0", Session: "probe", PaneID: "%0"},
		},
		Errors: []SnapshotError{{Scope: "tmux", Target: "probe:0.0", Error: "remote boom"}},
	}
	peers := map[string]PeerSnapshot{
		"z13": {Snapshot: remote, Reachable: true, FetchedAt: time.Now()},
	}

	merged := MergePeers(local, peers)

	if _, ok := merged.Panes["main:0.0"]; !ok {
		t.Fatalf("local pane main:0.0 dropped after merge")
	}
	if pane, ok := merged.Panes["z13@probe:0.0"]; !ok {
		t.Fatalf("expected merged key z13@probe:0.0; got keys %v", keys(merged.Panes))
	} else {
		if pane.Peer != "z13" {
			t.Fatalf("pane.Peer = %q want z13", pane.Peer)
		}
		if pane.Target != "z13@probe:0.0" {
			t.Fatalf("pane.Target = %q want z13@probe:0.0", pane.Target)
		}
		if pane.Session != "z13@probe" {
			t.Fatalf("pane.Session = %q want z13@probe", pane.Session)
		}
		if pane.PaneID != "z13@%0" {
			t.Fatalf("pane.PaneID = %q want z13@%%0", pane.PaneID)
		}
	}
	if sess, ok := merged.Sessions["z13@probe"]; !ok {
		t.Fatalf("expected merged session z13@probe; got %v", keys(merged.Sessions))
	} else {
		if sess.Peer != "z13" || sess.Session != "z13@probe" || sess.ActiveTarget != "z13@probe:0.0" {
			t.Fatalf("session = %#v", sess)
		}
	}
	if got := len(merged.Errors); got != 1 {
		t.Fatalf("expected 1 propagated error, got %d (%v)", got, merged.Errors)
	}
	if merged.Errors[0].Scope != "peer:z13:tmux" || merged.Errors[0].Target != "z13@probe:0.0" {
		t.Fatalf("propagated error = %#v", merged.Errors[0])
	}
	if merged.Summary.Sessions != 2 || merged.Summary.Panes != 2 {
		t.Fatalf("summary = %#v", merged.Summary)
	}
}

func TestMergePeersRecordsFetchErrorAndSkipsUnreachable(t *testing.T) {
	local := Snapshot{
		Sessions: map[string]SessionStatus{},
		Panes:    map[string]PaneStatus{},
	}
	peers := map[string]PeerSnapshot{
		"down":  {Err: errors.New("dial: refused"), FetchedAt: time.Now()},
		"slow":  {Reachable: false, FetchedAt: time.Now()},
		"empty": {Reachable: true, Snapshot: Snapshot{Sessions: map[string]SessionStatus{}, Panes: map[string]PaneStatus{}}},
	}
	merged := MergePeers(local, peers)
	if len(merged.Panes) != 0 || len(merged.Sessions) != 0 {
		t.Fatalf("expected no panes/sessions, got panes=%v sessions=%v", merged.Panes, merged.Sessions)
	}
	if len(merged.Errors) != 1 || merged.Errors[0].Scope != "peer:down" {
		t.Fatalf("expected one peer:down error, got %v", merged.Errors)
	}
}

func TestMergePeersEmptyMapReturnsLocalUnchanged(t *testing.T) {
	local := Snapshot{
		Sessions: map[string]SessionStatus{"a": {Session: "a"}},
		Panes:    map[string]PaneStatus{"a:0.0": {Target: "a:0.0", Session: "a"}},
		Summary:  Summary{Sessions: 1, Panes: 1},
	}
	merged := MergePeers(local, nil)
	if len(merged.Sessions) != 1 || len(merged.Panes) != 1 {
		t.Fatalf("local snapshot mutated by no-op merge: %#v", merged)
	}
}

func TestSplitPeerTarget(t *testing.T) {
	cases := map[string][2]string{
		"":              {"", ""},
		"main:0.0":      {"", "main:0.0"},
		"z13@probe:0.0": {"z13", "probe:0.0"},
		"z13@%0":        {"z13", "%0"},
		"@bad":          {"", "@bad"}, // leading @ -> no peer name; treat as local
	}
	for input, want := range cases {
		gotPeer, gotRest := SplitPeerTarget(input)
		if gotPeer != want[0] || gotRest != want[1] {
			t.Fatalf("SplitPeerTarget(%q) = (%q, %q) want (%q, %q)", input, gotPeer, gotRest, want[0], want[1])
		}
	}
}

func TestPeerFetcherFetchesAndCachesRemoteSnapshot(t *testing.T) {
	remote := Snapshot{
		Version:  1,
		Sessions: map[string]SessionStatus{"r": {Session: "r"}},
		Panes:    map[string]PaneStatus{"r:0.0": {Target: "r:0.0", Session: "r", PaneID: "%0"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/snapshot") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(remote)
	}))
	defer srv.Close()

	f := NewPeerFetcher([]Peer{{Name: "remote", URL: srv.URL}}, 10*time.Millisecond, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx)

	if !waitFor(time.Second, func() bool {
		s, ok := f.Latest()["remote"]
		return ok && s.Reachable && s.Err == nil && len(s.Snapshot.Panes) == 1
	}) {
		t.Fatalf("peer never reported reachable snapshot: %#v", f.Latest())
	}
}

func TestPeerFetcherRecordsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	f := NewPeerFetcher([]Peer{{Name: "broken", URL: srv.URL}}, 10*time.Millisecond, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.Start(ctx)
	if !waitFor(time.Second, func() bool {
		s, ok := f.Latest()["broken"]
		return ok && s.Err != nil && !s.Reachable
	}) {
		t.Fatalf("expected an error to be recorded for broken peer; got %#v", f.Latest())
	}
}

func waitFor(d time.Duration, pred func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return pred()
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
