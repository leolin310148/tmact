package statusd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		"peer-a": {Snapshot: remote, Reachable: true, FetchedAt: time.Now()},
	}

	merged := MergePeers(local, peers)

	if _, ok := merged.Panes["main:0.0"]; !ok {
		t.Fatalf("local pane main:0.0 dropped after merge")
	}
	if pane, ok := merged.Panes["peer-a@probe:0.0"]; !ok {
		t.Fatalf("expected merged key peer-a@probe:0.0; got keys %v", keys(merged.Panes))
	} else {
		if pane.Peer != "peer-a" {
			t.Fatalf("pane.Peer = %q want peer-a", pane.Peer)
		}
		if pane.Target != "peer-a@probe:0.0" {
			t.Fatalf("pane.Target = %q want peer-a@probe:0.0", pane.Target)
		}
		if pane.Session != "peer-a@probe" {
			t.Fatalf("pane.Session = %q want peer-a@probe", pane.Session)
		}
		if pane.PaneID != "peer-a@%0" {
			t.Fatalf("pane.PaneID = %q want peer-a@%%0", pane.PaneID)
		}
	}
	if sess, ok := merged.Sessions["peer-a@probe"]; !ok {
		t.Fatalf("expected merged session peer-a@probe; got %v", keys(merged.Sessions))
	} else {
		if sess.Peer != "peer-a" || sess.Session != "peer-a@probe" || sess.ActiveTarget != "peer-a@probe:0.0" {
			t.Fatalf("session = %#v", sess)
		}
	}
	if got := len(merged.Errors); got != 1 {
		t.Fatalf("expected 1 propagated error, got %d (%v)", got, merged.Errors)
	}
	if merged.Errors[0].Scope != "peer:peer-a:tmux" || merged.Errors[0].Target != "peer-a@probe:0.0" {
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

func TestMergePeersKeepsLastSnapshotOnFetchError(t *testing.T) {
	local := Snapshot{
		Sessions: map[string]SessionStatus{},
		Panes:    map[string]PaneStatus{},
	}
	remote := Snapshot{
		Sessions: map[string]SessionStatus{
			"probe": {Session: "probe", ActiveTarget: "probe:0.0"},
		},
		Panes: map[string]PaneStatus{
			"probe:0.0": {Target: "probe:0.0", Session: "probe", PaneID: "%0"},
		},
	}
	merged := MergePeers(local, map[string]PeerSnapshot{
		"peer-a": {Snapshot: remote, Err: errors.New("dial: refused"), FetchedAt: time.Now()},
	})

	pane, ok := merged.Panes["peer-a@probe:0.0"]
	if !ok {
		t.Fatalf("expected stale peer pane to stay visible; got %v", keys(merged.Panes))
	}
	if !pane.Stale || pane.Peer != "peer-a" {
		t.Fatalf("pane = %#v, want stale peer-a pane", pane)
	}
	session, ok := merged.Sessions["peer-a@probe"]
	if !ok || !session.Stale {
		t.Fatalf("session = %#v ok=%v, want stale peer session", session, ok)
	}
	if len(merged.Errors) != 1 || merged.Errors[0].Scope != "peer:peer-a" {
		t.Fatalf("errors = %#v, want peer:peer-a fetch error", merged.Errors)
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
		"":                 {"", ""},
		"main:0.0":         {"", "main:0.0"},
		"peer-a@probe:0.0": {"peer-a", "probe:0.0"},
		"peer-a@%0":        {"peer-a", "%0"},
		"@bad":             {"", "@bad"}, // leading @ -> no peer name; treat as local
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

func TestPeerFetcherPreservesLastSnapshotOnHTTPError(t *testing.T) {
	fail := false
	remote := Snapshot{
		Sessions: map[string]SessionStatus{"r": {Session: "r"}},
		Panes:    map[string]PaneStatus{"r:0.0": {Target: "r:0.0", Session: "r", PaneID: "%0"}},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(remote)
	}))
	defer srv.Close()

	f := NewPeerFetcher([]Peer{{Name: "remote", URL: srv.URL}}, time.Second, time.Second)
	ctx := context.Background()
	f.fetchOnce(ctx, Peer{Name: "remote", URL: srv.URL})
	fail = true
	f.fetchOnce(ctx, Peer{Name: "remote", URL: srv.URL})

	got := f.Latest()["remote"]
	if got.Err == nil || got.Reachable {
		t.Fatalf("got %#v, want unreachable error state", got)
	}
	if len(got.Snapshot.Panes) != 1 {
		t.Fatalf("snapshot panes = %#v, want last successful snapshot preserved", got.Snapshot.Panes)
	}
}

func TestPeerFetcherLogsPeerStateChanges(t *testing.T) {
	var logs []string
	f := NewPeerFetcher([]Peer{{Name: "peer-a", URL: "http://example.test"}}, time.Second, time.Second)
	f.SetLogger(func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})

	f.storeError("peer-a", errors.New("context deadline exceeded"))
	f.storeError("peer-a", errors.New("context deadline exceeded"))
	f.storeError("peer-a", errors.New("connection refused"))
	f.store("peer-a", PeerSnapshot{Reachable: true, FetchedAt: time.Now()})

	if len(logs) != 3 {
		t.Fatalf("logs = %#v, want first failure, changed failure, and recovery", logs)
	}
	if !strings.Contains(logs[0], "peer peer-a unreachable: context deadline exceeded") {
		t.Fatalf("first log = %q", logs[0])
	}
	if !strings.Contains(logs[1], "peer peer-a unreachable: connection refused") {
		t.Fatalf("second log = %q", logs[1])
	}
	if logs[2] != "peer peer-a reachable again" {
		t.Fatalf("third log = %q", logs[2])
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
