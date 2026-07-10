package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/dispatch"
)

func TestDispatchWithoutPeerUsesLocalRun(t *testing.T) {
	defer stubCLIHooks(t)()

	dir := t.TempDir()
	called := false
	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		called = true
		if opts.Session != "work" || opts.Dir != dir || opts.Agent != "codex" || opts.Prompt != "go" || opts.Execute {
			t.Fatalf("opts = %#v", opts)
		}
		return dispatch.Report{Session: opts.Session, Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt, Execute: opts.Execute}, nil
	}

	out, err := captureRun(t, "dispatch-work", "work", "--dir", dir, "--agent", "codex", "--prompt", "go")
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("dispatchRun was not called")
	}
	if !strings.Contains(out, "dry-run: dispatch-work work") {
		t.Fatalf("output = %s", out)
	}
}

func TestDispatchPeerReadsConfigAndUsesRemoteRun(t *testing.T) {
	defer stubCLIHooks(t)()

	configPath := filepath.Join(t.TempDir(), "statusd.json")
	writeText(t, configPath, `{
		"peers":[{"name":"peer-a","url":"http://snapshot-peer.example:7890"}],
		"dispatch_peers":[{"name":"peer-a","url":"http://dispatch-peer.example:7890"}]
	}`)

	dispatchRun = func(dispatch.Options) (dispatch.Report, error) {
		t.Fatal("local dispatchRun must not be called for --peer")
		return dispatch.Report{}, nil
	}
	dispatchRemoteRun = func(_ context.Context, _ *http.Client, peerName, peerURL string, opts dispatch.Options) (dispatch.Report, error) {
		if peerName != "peer-a" || peerURL != "http://dispatch-peer.example:7890" {
			t.Fatalf("peer = %s %s", peerName, peerURL)
		}
		if opts.Dir != "/peer/repo" || opts.ReadyTimeout != 45*time.Second || opts.ReadySettle != 2*time.Second || !opts.Execute || !opts.TrustFolder {
			t.Fatalf("opts = %#v", opts)
		}
		return dispatch.Report{Peer: peerName, Session: opts.Session, Target: "peer-a@%7", Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt, Execute: opts.Execute}, nil
	}

	out, err := captureRun(t, "dispatch-work", "work", "--peer", "peer-a", "--config", configPath, "--dir", "/peer/repo", "--agent", "codex", "--prompt", "go", "--ready-timeout", "45s", "--ready-settle", "2s", "--trust-folder", "--execute")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dispatch-work work", "peer: peer-a", "target: peer-a@%7"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestDispatchPeerFallsBackToSnapshotPeers(t *testing.T) {
	defer stubCLIHooks(t)()

	configPath := filepath.Join(t.TempDir(), "statusd.json")
	writeText(t, configPath, `{"peers":[{"name":"peer-a","url":"http://peer-a.example:7890"}]}`)

	dispatchRemoteRun = func(_ context.Context, _ *http.Client, peerName, peerURL string, opts dispatch.Options) (dispatch.Report, error) {
		if peerName != "peer-a" || peerURL != "http://peer-a.example:7890" {
			t.Fatalf("peer = %s %s", peerName, peerURL)
		}
		return dispatch.Report{Peer: peerName, Session: opts.Session, Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt}, nil
	}

	if _, err := captureRun(t, "dispatch-work", "work", "--peer", "peer-a", "--config", configPath, "--dir", "/peer/repo", "--agent", "codex", "--prompt", "go"); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchPeerErrorsWhenPeerMissing(t *testing.T) {
	defer stubCLIHooks(t)()

	configPath := filepath.Join(t.TempDir(), "statusd.json")
	writeText(t, configPath, `{"dispatch_peers":[{"name":"other","url":"http://other.example:7890"}]}`)

	_, err := captureRun(t, "dispatch-work", "work", "--peer", "peer-a", "--config", configPath, "--dir", "/peer/repo", "--agent", "codex", "--prompt", "go")
	if err == nil || !strings.Contains(err.Error(), `peer "peer-a" not found in dispatch_peers or peers`) {
		t.Fatalf("err = %v", err)
	}
}

func writeText(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
