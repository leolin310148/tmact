package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/dispatch"
)

var errTestDispatch = errors.New("dispatch validation failed")

func TestHandleDispatchWorkRunsInjectedDispatcher(t *testing.T) {
	var got dispatch.Options
	s := &Server{
		DispatchRun: func(opts dispatch.Options) (dispatch.Report, error) {
			got = opts
			return dispatch.Report{
				Session: opts.Session,
				Target:  "%8",
				Dir:     opts.Dir,
				Agent:   opts.Agent,
				Prompt:  opts.Prompt,
				Execute: opts.Execute,
				Steps:   []dispatch.Step{{Name: "send-prompt", Status: dispatch.StatusOK}},
			}, nil
		},
	}
	body := bytes.NewBufferString(`{"session":"work","dir":"/repo","agent":"codex","prompt":"go","execute":true,"ready_timeout":"45s","ready_settle":"2s"}`)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/dispatch-work", body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got.Session != "work" || got.Dir != "/repo" || got.Agent != "codex" || got.Prompt != "go" || !got.Execute {
		t.Fatalf("opts = %#v", got)
	}
	if got.ReadyTimeout != 45*time.Second || got.ReadySettle != 2*time.Second {
		t.Fatalf("durations = %s %s", got.ReadyTimeout, got.ReadySettle)
	}
	var report dispatch.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Target != "%8" || len(report.Steps) != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestHandleDispatchWorkRejectsMethodAndBadJSON(t *testing.T) {
	handler := (&Server{}).Handler()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/dispatch-work", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/dispatch-work", strings.NewReader("{")))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid JSON body") {
		t.Fatalf("bad JSON status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleDispatchWorkReturnsDispatchValidationError(t *testing.T) {
	s := &Server{
		DispatchRun: func(dispatch.Options) (dispatch.Report, error) {
			return dispatch.Report{}, errTestDispatch
		},
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/dispatch-work", strings.NewReader(`{"session":"work","dir":"/repo","agent":"codex","prompt":"go"}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), errTestDispatch.Error()) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
