package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
)

func TestCommandBackgroundRunsAsJobAndReturnsOutput(t *testing.T) {
	release := make(chan struct{})
	s := &Server{
		RunCommandCaptured: func(target, command string, maxOutput int) (tmux.CommandResult, error) {
			if target != "%7" || command != "printf hello" || maxOutput != maxCommandOutput {
				t.Fatalf("runner got (%q, %q, %d)", target, command, maxOutput)
			}
			<-release
			return tmux.CommandResult{Output: "hello", ExitCode: 0}, nil
		},
	}
	handler := s.Handler()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, sessionJSONRequest(
		http.MethodPost,
		"/api/command",
		`{"pane":"%7","command":"printf hello","mode":"background"}`,
	))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start status = %d body=%s", rec.Code, rec.Body.String())
	}
	var started struct {
		Job commandJob `json:"job"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Job.Status != "running" || len(started.Job.ID) != 32 {
		t.Fatalf("started job = %#v", started.Job)
	}

	close(release)
	deadline := time.Now().Add(time.Second)
	for {
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/command?id="+started.Job.ID, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("result status = %d body=%s", rec.Code, rec.Body.String())
		}
		var result struct {
			Job commandJob `json:"job"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Job.Status == "finished" {
			if result.Job.Output != "hello" || result.Job.ExitCode == nil || *result.Job.ExitCode != 0 {
				t.Fatalf("finished job = %#v", result.Job)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command job did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCommandTmuxReturnsNewPane(t *testing.T) {
	s := &Server{
		RunCommandInNewSession: func(target, command string) (string, string, error) {
			if target != "%9" || command != "make test" {
				t.Fatalf("runner got (%q, %q)", target, command)
			}
			return "work-run", "%12", nil
		},
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, sessionJSONRequest(
		http.MethodPost,
		"/api/command",
		`{"pane":"%9","command":"make test","mode":"tmux"}`,
	))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"pane_id":"%12"`) ||
		!strings.Contains(rec.Body.String(), `"session":"work-run"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestCommandStartValidatesRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"invalid pane", `{"pane":"work:0.0","command":"true","mode":"background"}`, http.StatusBadRequest},
		{"blank command", `{"pane":"%1","command":" ","mode":"background"}`, http.StatusBadRequest},
		{"invalid mode", `{"pane":"%1","command":"true","mode":"window"}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			(&Server{}).Handler().ServeHTTP(rec, sessionJSONRequest(http.MethodPost, "/api/command", tc.body))
			if rec.Code != tc.want {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCommandStartProxiesToPanePeer(t *testing.T) {
	peer := httptest.NewServer((&Server{
		RunCommandInNewSession: func(target, command string) (string, string, error) {
			if target != "%4" || command != "go test ./..." {
				t.Fatalf("peer runner got (%q, %q)", target, command)
			}
			return "remote-run", "%8", nil
		},
	}).Handler())
	defer peer.Close()

	hub := (&Server{Peers: []statusd.Peer{{Name: "mini", URL: peer.URL}}}).Handler()
	rec := httptest.NewRecorder()
	hub.ServeHTTP(rec, sessionJSONRequest(
		http.MethodPost,
		"/api/command?peer=mini",
		`{"pane":"%4","command":"go test ./...","mode":"tmux"}`,
	))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"pane_id":"%8"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
