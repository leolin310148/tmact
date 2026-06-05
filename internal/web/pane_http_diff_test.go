package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPaneDiffFirstRequestReturnsFullPatch(t *testing.T) {
	srv := httptest.NewServer((&Server{
		CapturePane: func(target string, lines int) (string, error) {
			if target != "%7" || lines != wsCaptureLines {
				t.Fatalf("capture got (%q, %d)", target, lines)
			}
			return "line 1\nline 2", nil
		},
	}).Handler())
	defer srv.Close()

	rec := getPaneDiff(t, srv.URL, "%7", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got paneDiffMsg
	decodeJSON(t, rec, &got)
	if got.T != "patch" || got.From != 0 || strings.Join(got.Lines, "|") != "line 1|line 2" || got.Cursor == "" {
		t.Fatalf("diff = %+v, want full patch with cursor", got)
	}
}

func TestPaneDiffSameCursorUnchangedReturnsNoContent(t *testing.T) {
	s := &Server{CapturePane: func(string, int) (string, error) { return "same", nil }}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	first := getPaneDiff(t, srv.URL, "%7", "")
	var seed paneDiffMsg
	decodeJSON(t, first, &seed)

	second := getPaneDiff(t, srv.URL, "%7", seed.Cursor)
	if second.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", second.Code, second.Body.String())
	}
}

func TestPaneDiffChangedCaptureReturnsTailPatch(t *testing.T) {
	captures := []string{"a\nb\nc", "a\nb\nd\ne"}
	s := &Server{CapturePane: func(string, int) (string, error) {
		out := captures[0]
		if len(captures) > 1 {
			captures = captures[1:]
		}
		return out, nil
	}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	first := getPaneDiff(t, srv.URL, "%7", "")
	var seed paneDiffMsg
	decodeJSON(t, first, &seed)

	second := getPaneDiff(t, srv.URL, "%7", seed.Cursor)
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", second.Code)
	}
	var got paneDiffMsg
	decodeJSON(t, second, &got)
	if got.From != 2 || strings.Join(got.Lines, "|") != "d|e" {
		t.Fatalf("diff = %+v, want from=2 tail d/e", got)
	}
}

func TestPaneDiffStaleCursorReturnsFullPatch(t *testing.T) {
	srv := httptest.NewServer((&Server{
		CapturePane: func(string, int) (string, error) { return "fresh\nbody", nil },
	}).Handler())
	defer srv.Close()

	rec := getPaneDiff(t, srv.URL, "%7", "stale")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got paneDiffMsg
	decodeJSON(t, rec, &got)
	if got.From != 0 || strings.Join(got.Lines, "|") != "fresh|body" {
		t.Fatalf("diff = %+v, want full patch", got)
	}
}

func TestPaneDiffRejectsFederatedPane(t *testing.T) {
	srv := httptest.NewServer((&Server{}).Handler())
	defer srv.Close()

	rec := getPaneDiff(t, srv.URL, "peer@%7", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPaneInputValidatesAndAppliesInput(t *testing.T) {
	got := make(chan inputMsg, 1)
	srv := httptest.NewServer((&Server{
		SendKey: func(target, key string) error {
			if target != "%7" {
				t.Fatalf("target = %q, want %%7", target)
			}
			got <- inputMsg{T: "key", K: key}
			return nil
		},
	}).Handler())
	defer srv.Close()

	body := bytes.NewBufferString(`{"t":"key","k":"C-c"}`)
	resp, err := http.Post(srv.URL+"/api/pane/input?pane=%257", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if m := <-got; m.K != "C-c" {
		t.Fatalf("input = %+v, want C-c", m)
	}
}

func TestPaneInputRejectsInvalidPaneAndKey(t *testing.T) {
	srv := httptest.NewServer((&Server{}).Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/pane/input?pane=peer@%257", "application/json", bytes.NewBufferString(`{"t":"resize"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid pane status = %d, want 400", resp.StatusCode)
	}

	resp, err = http.Post(srv.URL+"/api/pane/input?pane=%257", "application/json", bytes.NewBufferString(`{"t":"key","k":"Dangerous"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid key status = %d, want 400", resp.StatusCode)
	}
}

type testHTTPResponse struct {
	Code int
	Body *bytes.Buffer
}

func getPaneDiff(t *testing.T, base, pane, cursor string) testHTTPResponse {
	t.Helper()
	q := url.Values{}
	q.Set("pane", pane)
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	resp, err := http.Get(base + "/api/pane/diff?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body bytes.Buffer
	if _, err := io.Copy(&body, resp.Body); err != nil {
		t.Fatal(err)
	}
	return testHTTPResponse{Code: resp.StatusCode, Body: &body}
}

func decodeJSON(t *testing.T, rec testHTTPResponse, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("decode JSON: %v; body=%s", err, rec.Body.String())
	}
}
