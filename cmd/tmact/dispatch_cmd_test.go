package main

import (
	"context"
	"encoding/json"
	"errors"
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
		if opts.Session != "work" || opts.Dir != dir || opts.Agent != "codex" || opts.Model != "gpt-5.4" || opts.Prompt != "go" || opts.Execute {
			t.Fatalf("opts = %#v", opts)
		}
		return dispatch.Report{Session: opts.Session, Dir: opts.Dir, Agent: opts.Agent, Model: opts.Model, Prompt: opts.Prompt, Execute: opts.Execute}, nil
	}

	out, err := captureRun(t, "dispatch-work", "work", "--dir", dir, "--agent", "codex", "--model", "gpt-5.4", "--prompt", "go")
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("dispatchRun was not called")
	}
	if !strings.Contains(out, "dry-run: dispatch-work work") {
		t.Fatalf("output = %s", out)
	}
	if !strings.Contains(out, "model=gpt-5.4") {
		t.Fatalf("output = %s", out)
	}
}

func TestDispatchWaitFlagsAndStructuredJSON(t *testing.T) {
	defer stubCLIHooks(t)()
	dir := t.TempDir()
	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		if !opts.Wait || !opts.Execute || opts.WaitTimeout != 2*time.Minute || opts.WaitSettle != 3*time.Second || opts.ResultLines != 44 || opts.Context == nil {
			t.Fatalf("opts = %#v", opts)
		}
		return dispatch.Report{
			Session: opts.Session, Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt, Execute: true,
			Wait:   &dispatch.WaitReport{Status: dispatch.StatusOK, Timeout: "2m0s", Settle: "3s", ResultLines: 44, Outcome: &dispatch.WaitOutcome{Reason: "condition_met", ConditionMet: true}},
			Result: &dispatch.ResultReport{Lines: 44, Text: "done\n"},
		}, nil
	}
	out, err := captureRun(t, "dispatch-work", "work", "--dir", dir, "--agent", "codex", "--prompt", "go", "--wait", "--wait-timeout", "2m", "--wait-settle", "3s", "--result-lines", "44", "--execute", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got["wait"] == nil || got["result"] == nil {
		t.Fatalf("JSON missing wait/result: %s", out)
	}
}

func TestDispatchJSONWithoutWaitKeepsWaitFieldsAbsent(t *testing.T) {
	defer stubCLIHooks(t)()
	dir := t.TempDir()
	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		return dispatch.Report{Session: opts.Session, Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt}, nil
	}
	out, err := captureRun(t, "dispatch-work", "work", "--dir", dir, "--agent", "codex", "--prompt", "go", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["wait"]; ok {
		t.Fatalf("unexpected wait field: %s", out)
	}
	if _, ok := got["result"]; ok {
		t.Fatalf("unexpected result field: %s", out)
	}
}

func TestDispatchWaitRejectsPeerBeforeRequest(t *testing.T) {
	defer stubCLIHooks(t)()
	dispatchRemoteRun = func(context.Context, *http.Client, string, string, dispatch.Options) (dispatch.Report, error) {
		t.Fatal("peer request must not be made")
		return dispatch.Report{}, nil
	}
	_, err := captureRun(t, "dispatch-work", "work", "--peer", "peer-a", "--dir", "/repo", "--agent", "codex", "--prompt", "go", "--wait")
	if err == nil || !strings.Contains(err.Error(), "does not support peer waiting") {
		t.Fatalf("err = %v", err)
	}
}

func TestDispatchWaitSpecificFlagsRequireWait(t *testing.T) {
	defer stubCLIHooks(t)()
	_, err := captureRun(t, "dispatch-work", "work", "--dir", t.TempDir(), "--agent", "codex", "--prompt", "go", "--result-lines", "20")
	if err == nil || !strings.Contains(err.Error(), "requires --wait") {
		t.Fatalf("err = %v", err)
	}
}

func TestDispatchWaitBlockerPrintsStructuredJSONBeforeError(t *testing.T) {
	defer stubCLIHooks(t)()
	dispatchRun = func(opts dispatch.Options) (dispatch.Report, error) {
		return dispatch.Report{
			Session: opts.Session, Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt,
			Wait: &dispatch.WaitReport{
				Status: dispatch.StatusFailed, Baseline: &dispatch.WaitBaseline{Accepted: true},
				Outcome: &dispatch.WaitOutcome{Reason: "needs_human"},
			},
		}, errors.New("dispatch wait ended before input-ready: needs_human")
	}
	out, err := captureRun(t, "dispatch-work", "work", "--dir", t.TempDir(), "--agent", "codex", "--prompt", "go", "--wait", "--json")
	if err == nil || !strings.Contains(err.Error(), "needs_human") {
		t.Fatalf("err = %v", err)
	}
	var got map[string]any
	if jsonErr := json.Unmarshal([]byte(out), &got); jsonErr != nil || got["wait"] == nil {
		t.Fatalf("output = %q, JSON error = %v", out, jsonErr)
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
		if opts.Dir != "/peer/repo" || opts.Model != "gpt-5.4" || opts.ReadyTimeout != 45*time.Second || opts.ReadySettle != 2*time.Second || !opts.Execute || !opts.TrustFolder {
			t.Fatalf("opts = %#v", opts)
		}
		return dispatch.Report{Peer: peerName, Session: opts.Session, Target: "peer-a@%7", Dir: opts.Dir, Agent: opts.Agent, Prompt: opts.Prompt, Execute: opts.Execute}, nil
	}

	out, err := captureRun(t, "dispatch-work", "work", "--peer", "peer-a", "--config", configPath, "--dir", "/peer/repo", "--agent", "codex", "--model", "gpt-5.4", "--prompt", "go", "--ready-timeout", "45s", "--ready-settle", "2s", "--trust-folder", "--execute")
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
