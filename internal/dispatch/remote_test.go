package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/leolin310148/tmact/internal/modelroute"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPostRemoteSendsRequestAndPrefixesTarget(t *testing.T) {
	var got RemoteRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/dispatch-work" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		writeRemoteJSON(t, w, Report{Session: got.Session, Target: "%9", Dir: got.Dir, Agent: got.Agent, Prompt: got.Prompt, Execute: got.Execute})
	}))
	defer srv.Close()

	report, err := PostRemote(context.Background(), srv.Client(), "peer-a", srv.URL, Options{
		Session:      "work",
		Dir:          "/repo",
		Agent:        "codex",
		Model:        "gpt-5.4",
		Prompt:       "go",
		Execute:      true,
		ReadyTimeout: 45 * time.Second,
		ReadySettle:  2 * time.Second,
		TrustFolder:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "gpt-5.4" || got.ReadyTimeout != "45s" || got.ReadySettle != "2s" || !got.TrustFolder {
		t.Fatalf("request durations = %#v", got)
	}
	if report.Peer != "peer-a" || report.Target != "peer-a@%9" {
		t.Fatalf("report = %#v", report)
	}
}

func TestPostRemoteReportsUnsupportedPeer(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer srv.Close()

			_, err := PostRemote(context.Background(), srv.Client(), "peer-a", srv.URL, Options{ReadyTimeout: 30 * time.Second})
			if err == nil || !strings.Contains(err.Error(), "peer does not support remote dispatch; update tmact") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestPostRemoteRejectsUnknownModelBeforeRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer srv.Close()

	_, err := PostRemote(context.Background(), srv.Client(), "peer-a", srv.URL, Options{
		Agent: "codex",
		Model: "gpt-5.4-typo",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported model") {
		t.Fatalf("err = %v", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
}

func TestPostRemoteRejectsWaitBeforeRequest(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer srv.Close()
	_, err := PostRemote(context.Background(), srv.Client(), "peer-a", srv.URL, Options{Wait: true})
	if err == nil || !strings.Contains(err.Error(), "does not support peer waiting") {
		t.Fatalf("err = %v", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
}

func TestRemoteRequestOptionsParsesDurations(t *testing.T) {
	opts, err := (RemoteRequest{Model: "sonnet", ReadyTimeout: "3s", ReadySettle: "500ms", TrustFolder: true}).Options()
	if err != nil {
		t.Fatal(err)
	}
	if opts.Model != "sonnet" || opts.ReadyTimeout != 3*time.Second || opts.ReadySettle != 500*time.Millisecond || !opts.TrustFolder {
		t.Fatalf("opts = %#v", opts)
	}

	if _, err := (RemoteRequest{ReadyTimeout: "nope"}).Options(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}

func TestRemoteURLNormalizesDispatchEndpoint(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{
			name: "http base",
			base: "http://peer.example:7890/status?stale=1",
			want: "http://peer.example:7890/api/dispatch-work",
		},
		{
			name: "websocket base",
			base: "ws://peer.example:7890/ws/snapshot",
			want: "http://peer.example:7890/api/dispatch-work",
		},
		{
			name: "secure websocket base",
			base: "wss://peer.example/ws/snapshot?token=redacted",
			want: "https://peer.example/api/dispatch-work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RemoteURL(tt.base)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("RemoteURL(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}

	if _, err := RemoteURL("peer.example:7890"); err == nil {
		t.Fatal("expected missing scheme error")
	}
	if _, err := RemoteURL("http:///status"); err == nil {
		t.Fatal("expected missing host error")
	}
}

func writeRemoteJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatal(err)
	}
}

func TestPostRemoteAutoModelUsesCallerDecideAndReplacesAgent(t *testing.T) {
	t.Setenv("TMACT_ROUTING_LOG", "off")
	dir := t.TempDir()
	script := `#!/bin/sh
cat > "$ROUTING_TEST_INPUT"
printf '%s\n' '{"model":"test-decide","answers":{"route":{"choice":"profile_0"}},"confidences":{"route":0.9},"gate_passed":true}'
`
	if err := os.WriteFile(filepath.Join(dir, "decide"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DECIDE_CLI_CONFIG", "none")
	t.Setenv("ROUTING_TEST_INPUT", filepath.Join(dir, "input.json"))
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req RemoteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			if req.Execute || !req.ProbeAutoModel {
				t.Fatalf("unsafe probe=%+v", req)
			}
			in := modelroute.Input{Now: time.Now(), Policy: modelroute.Policy, Task: req.Prompt, Candidates: []modelroute.Candidate{{Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium", Tier: "standard"}}}
			in.Files = []modelroute.TaskFile{{Path: "/peer/task.md", Status: "attached", Content: "task available only on peer"}}
			writeRemoteJSON(t, w, Report{AutoModelSupported: true, LaunchOptionsVersion: 2, AgentWasRunning: true, RoutingInput: &in})
		} else {
			if !req.Execute || !req.RestartAgent || req.ProbeAutoModel || req.Agent != "codex" || req.Model != "gpt-6.1-sol" || req.Effort != "medium" {
				t.Fatalf("launch=%+v", req)
			}
			writeRemoteJSON(t, w, Report{Agent: req.Agent, Model: req.Model, Effort: req.Effort, Target: "%9", Execute: true})
		}
	}))
	defer srv.Close()
	report, err := PostRemote(context.Background(), srv.Client(), "peer", srv.URL, Options{AutoModel: true, Session: "work", Dir: "/repo", Prompt: "task", Execute: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "input.json"))
	var supplied modelroute.Input
	if err := json.Unmarshal(raw, &supplied); err != nil {
		t.Fatal(err)
	}
	if len(supplied.Files) != 1 || supplied.Files[0].Content != "task available only on peer" {
		t.Fatal("peer file context was not retained")
	}
	if calls != 2 || report.Selection == nil || report.Target != "peer@%9" {
		t.Fatalf("calls=%d report=%+v", calls, report)
	}
}

func TestPostRemoteAutoModelRejectsOlderPeerBeforeExecution(t *testing.T) {
	for _, opts := range []Options{{AutoModel: true, Agent: "codex"}, {Agent: "codex", Effort: "high"}} {
		t.Run(fmt.Sprint(opts.AutoModel), func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req RemoteRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req.Execute {
					t.Fatal("old peer received execution")
				}
				writeRemoteJSON(t, w, Report{Agent: "codex"})
			}))
			defer srv.Close()
			opts.Execute = true
			if _, err := PostRemote(context.Background(), srv.Client(), "old", srv.URL, opts); err == nil {
				t.Fatal("old peer accepted")
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
		})
	}
}

func TestSavedPlanWireAndRemoteApply(t *testing.T) {
	state := &PlanState{SessionExisted: true, SessionID: "$1", PaneID: "%7", Runtime: "codex", Dir: "/repo"}
	files := []modelroute.TaskFile{{Path: "/repo/task.md", Status: "attached", SHA256: "hash"}}
	opts := Options{Session: "work", Dir: "/repo", Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium", Prompt: "Read task.md", PlanState: state, PlanFiles: &files, Selection: &modelroute.Selection{Status: "selected", Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium"}, Execute: true}
	req := RemoteRequestFromOptions(opts)
	back, err := req.Options()
	if err != nil || back.PlanState == nil || *back.PlanState != *state || back.PlanFiles == nil {
		t.Fatalf("wire=%+v err=%v", back, err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req RemoteRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.ProbeAutoModel || req.PlanState == nil || req.PlanFiles == nil {
			t.Fatalf("unsafe request=%+v", req)
		}
		if calls == 1 && req.Execute {
			t.Fatal("probe executed")
		}
		writeRemoteJSON(t, w, Report{LaunchOptionsVersion: 3, PlanState: state, PlanFiles: files, Agent: req.Agent, Model: req.Model, Effort: req.Effort, Execute: req.Execute})
	}))
	defer srv.Close()
	report, err := PostRemote(context.Background(), srv.Client(), "peer", srv.URL, opts)
	if err != nil || calls != 2 || report.Selection == nil {
		t.Fatalf("calls=%d err=%v report=%+v", calls, err, report)
	}
}
