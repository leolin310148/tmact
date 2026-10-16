package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/leolin310148/tmact/internal/tmux"
)

// Capture modes served by /api/pane/capture. They match the local tmux
// helpers the CLI uses, so a peer capture classifies exactly like a local one.
const (
	// CaptureModePlain is tmux.CapturePane: no escapes, wrapped rows joined.
	CaptureModePlain = "plain"
	// CaptureModeStyled is tmux.CapturePaneStyled: escapes, wrapped rows joined.
	CaptureModeStyled = "styled"
	// CaptureModeANSI is tmux.CapturePaneANSIContext: escapes, rows unjoined.
	CaptureModeANSI = "ansi"

	paneCapturePath     = "/api/pane/capture"
	maxPaneCaptureLines = 5000
)

// ErrPeerPaneGone reports that the peer no longer has the requested pane.
var ErrPeerPaneGone = errors.New("peer pane is gone")

// PaneCapture is the read-only /api/pane/capture response.
type PaneCapture struct {
	Target      string            `json:"target"`
	PaneID      string            `json:"pane_id"`
	HistorySize int               `json:"history_size"`
	Captures    map[string]string `json:"captures"`
}

func (s *Server) paneInfo() func(context.Context, string) (tmux.CapturePaneInfo, error) {
	if s.PaneInfo != nil {
		return s.PaneInfo
	}
	return tmux.CapturePaneInfoForTargetContext
}

func (s *Server) captureMode() func(context.Context, string, int, string) (string, error) {
	if s.CapturePaneMode != nil {
		return s.CapturePaneMode
	}
	return func(ctx context.Context, target string, lines int, mode string) (string, error) {
		switch mode {
		case CaptureModePlain:
			return tmux.CapturePaneContext(ctx, target, lines)
		case CaptureModeStyled:
			return tmux.CapturePaneStyledContext(ctx, target, lines)
		default:
			return tmux.CapturePaneANSIContext(ctx, target, lines)
		}
	}
}

// handlePaneCapture serves one bounded, read-only capture of a local pane so
// a peer's `tmact capture` and `tmact wait` can observe it. It never sends
// input.
func (s *Server) handlePaneCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	query := r.URL.Query()
	pane := query.Get("pane")
	if !localPaneIDPattern.MatchString(pane) {
		writeJSONError(w, http.StatusBadRequest, `invalid "pane" parameter, expected a local tmux pane id like %12`)
		return
	}
	lines, err := strconv.Atoi(query.Get("lines"))
	if err != nil || lines <= 0 || lines > maxPaneCaptureLines {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf(`invalid "lines" parameter, expected 1-%d`, maxPaneCaptureLines))
		return
	}
	modes, err := parseCaptureModes(query.Get("modes"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.paneCaptureTimeout())
	defer cancel()
	info, err := s.paneInfo()(ctx, pane)
	if err != nil {
		writePaneCaptureError(w, err)
		return
	}
	out := PaneCapture{Target: info.Target, PaneID: info.PaneID, HistorySize: info.HistorySize, Captures: map[string]string{}}
	for _, mode := range modes {
		text, err := s.captureMode()(ctx, info.PaneID, lines, mode)
		if err != nil {
			writePaneCaptureError(w, err)
			return
		}
		out.Captures[mode] = text
	}
	writeJSON(w, http.StatusOK, out)
}

func parseCaptureModes(raw string) ([]string, error) {
	if raw == "" {
		return nil, errors.New(`missing "modes" parameter`)
	}
	var modes []string
	seen := map[string]bool{}
	for _, mode := range strings.Split(raw, ",") {
		switch mode {
		case CaptureModePlain, CaptureModeStyled, CaptureModeANSI:
		default:
			return nil, fmt.Errorf("unknown capture mode %q", mode)
		}
		if !seen[mode] {
			seen[mode] = true
			modes = append(modes, mode)
		}
	}
	return modes, nil
}

// writePaneCaptureError uses 410 for a vanished pane: 404 is what an older
// peer without this route returns, and must keep meaning "update the peer".
func writePaneCaptureError(w http.ResponseWriter, err error) {
	if tmux.IsTargetGoneError(err) {
		writeJSONError(w, http.StatusGone, err.Error())
		return
	}
	writeJSONError(w, http.StatusBadGateway, err.Error())
}

// FetchPeerCapture reads a pane on the peer at baseURL through its
// /api/pane/capture endpoint. A missing pane returns ErrPeerPaneGone.
func FetchPeerCapture(ctx context.Context, client *http.Client, peerName, baseURL, pane string, lines int, modes ...string) (PaneCapture, error) {
	if client == nil {
		client = http.DefaultClient
	}
	q := url.Values{}
	q.Set("pane", pane)
	q.Set("lines", strconv.Itoa(lines))
	q.Set("modes", strings.Join(modes, ","))
	upstream, err := peerHTTPURL(baseURL, paneCapturePath, q)
	if err != nil {
		return PaneCapture{}, fmt.Errorf("invalid peer %s URL %q: %w", peerName, baseURL, err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, peerRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, upstream, nil)
	if err != nil {
		return PaneCapture{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return PaneCapture{}, fmt.Errorf("peer %s capture request failed: %w", peerName, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusGone:
		return PaneCapture{}, fmt.Errorf("%w: %s@%s", ErrPeerPaneGone, peerName, pane)
	case http.StatusNotFound:
		return PaneCapture{}, fmt.Errorf("peer %s does not support %s; please update the peer tmact", peerName, paneCapturePath)
	default:
		var body struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if body.Error != "" {
			return PaneCapture{}, fmt.Errorf("peer %s capture returned HTTP %d: %s", peerName, resp.StatusCode, body.Error)
		}
		return PaneCapture{}, fmt.Errorf("peer %s capture returned HTTP %d", peerName, resp.StatusCode)
	}
	var out PaneCapture
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return PaneCapture{}, fmt.Errorf("peer %s capture response invalid: %w", peerName, err)
	}
	for _, mode := range modes {
		if _, ok := out.Captures[mode]; !ok {
			return PaneCapture{}, fmt.Errorf("peer %s capture response is missing %s output", peerName, mode)
		}
	}
	return out, nil
}
