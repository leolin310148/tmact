package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/tmux"
)

func paneCaptureServer(t *testing.T, infoErr error) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer((&Server{
		PaneInfo: func(_ context.Context, target string) (tmux.CapturePaneInfo, error) {
			if infoErr != nil {
				return tmux.CapturePaneInfo{}, infoErr
			}
			return tmux.CapturePaneInfo{Target: "work:0.1", PaneID: target, HistorySize: 12}, nil
		},
		CapturePaneMode: func(_ context.Context, target string, lines int, mode string) (string, error) {
			return mode + ":" + target + ":" + strings.Repeat("x", lines), nil
		},
		CapturePane: func(string, int) (string, error) {
			t.Fatal("capture endpoint must not use the web stream capture")
			return "", nil
		},
	}).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchPeerCaptureReturnsRequestedModes(t *testing.T) {
	srv := paneCaptureServer(t, nil)
	got, err := FetchPeerCapture(context.Background(), srv.Client(), "mini", srv.URL, "%7", 3, CaptureModePlain, CaptureModeStyled)
	if err != nil {
		t.Fatal(err)
	}
	if got.Target != "work:0.1" || got.PaneID != "%7" || got.HistorySize != 12 {
		t.Fatalf("info = %+v", got)
	}
	if got.Captures[CaptureModePlain] != "plain:%7:xxx" || got.Captures[CaptureModeStyled] != "styled:%7:xxx" {
		t.Fatalf("captures = %#v", got.Captures)
	}
	if _, ok := got.Captures[CaptureModeANSI]; ok {
		t.Fatalf("unrequested mode returned: %#v", got.Captures)
	}
}

func TestFetchPeerCaptureReportsGonePane(t *testing.T) {
	srv := paneCaptureServer(t, errors.New("can't find pane: %7"))
	_, err := FetchPeerCapture(context.Background(), srv.Client(), "mini", srv.URL, "%7", 3, CaptureModeANSI)
	if !errors.Is(err, ErrPeerPaneGone) {
		t.Fatalf("err = %v, want ErrPeerPaneGone", err)
	}
}

func TestFetchPeerCaptureExplainsOldPeer(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, err := FetchPeerCapture(context.Background(), srv.Client(), "mini", srv.URL, "%7", 3, CaptureModeANSI)
	if err == nil || !strings.Contains(err.Error(), "update the peer tmact") || errors.Is(err, ErrPeerPaneGone) {
		t.Fatalf("err = %v, want update-peer message", err)
	}
}

func TestPaneCaptureRejectsBadParameters(t *testing.T) {
	srv := paneCaptureServer(t, nil)
	for name, query := range map[string]string{
		"peer pane":    "pane=mini@%257&lines=3&modes=plain",
		"no lines":     "pane=%257&modes=plain",
		"huge lines":   "pane=%257&lines=99999&modes=plain",
		"no modes":     "pane=%257&lines=3",
		"unknown mode": "pane=%257&lines=3&modes=raw",
	} {
		resp, err := srv.Client().Get(srv.URL + "/api/pane/capture?" + query)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", name, resp.StatusCode)
		}
	}
	resp, err := srv.Client().Post(srv.URL+"/api/pane/capture?pane=%257&lines=3&modes=plain", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", resp.StatusCode)
	}
}
