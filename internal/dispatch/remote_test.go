package dispatch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		Prompt:       "go",
		Execute:      true,
		ReadyTimeout: 45 * time.Second,
		ReadySettle:  2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ReadyTimeout != "45s" || got.ReadySettle != "2s" {
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

func TestRemoteRequestOptionsParsesDurations(t *testing.T) {
	opts, err := (RemoteRequest{ReadyTimeout: "3s", ReadySettle: "500ms"}).Options()
	if err != nil {
		t.Fatal(err)
	}
	if opts.ReadyTimeout != 3*time.Second || opts.ReadySettle != 500*time.Millisecond {
		t.Fatalf("opts = %#v", opts)
	}

	if _, err := (RemoteRequest{ReadyTimeout: "nope"}).Options(); err == nil {
		t.Fatal("expected invalid duration error")
	}
}

func writeRemoteJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatal(err)
	}
}
