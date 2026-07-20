package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/statusd"
)

var localPaneIDPattern = regexp.MustCompile(`^%[0-9]+$`)

type paneDiffCache struct {
	mu sync.Mutex
	m  map[string]paneDiffEntry
}

type paneDiffEntry struct {
	lines  []string
	cursor string
}

type paneDiffMsg struct {
	T      string           `json:"t"`
	From   int              `json:"from"`
	Lines  []string         `json:"lines"`
	Q      *prompt.Question `json:"q,omitempty"`
	Cursor string           `json:"cursor"`
}

func (s *Server) handlePaneDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	pane := r.URL.Query().Get("pane")
	if !localPaneIDPattern.MatchString(pane) {
		writeJSONError(w, http.StatusBadRequest, `invalid "pane" parameter, expected a local tmux pane id like %12`)
		return
	}
	captureLines := wsCaptureLines
	if raw := r.URL.Query().Get("lines"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeJSONError(w, http.StatusBadRequest, `invalid "lines" parameter, expected a positive integer`)
			return
		}
		captureLines = parsed
	}

	started := time.Now()
	captureCtx, cancel := context.WithTimeout(r.Context(), s.paneCaptureTimeout())
	content, err := s.captureContext()(captureCtx, pane, captureLines)
	cancel()
	elapsed := time.Since(started)
	if err != nil {
		s.logf("pane diff capture error pane=%s duration=%s err=%v", pane, elapsed.Round(time.Millisecond), err)
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	if elapsed >= peerSlowRequest {
		s.logf("pane diff capture slow pane=%s duration=%s", pane, elapsed.Round(time.Millisecond))
	}

	cursor := paneDiffCursor(content)
	lines := strings.Split(content, "\n")
	from, tail, unchanged := s.paneDiff.diff(pane, r.URL.Query().Get("cursor"), lines, cursor)
	if unchanged {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, paneDiffMsg{
		T:      "patch",
		From:   from,
		Lines:  tail,
		Q:      prompt.DetectQuestion(content),
		Cursor: cursor,
	})
}

func (s *Server) handlePaneInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	pane := r.URL.Query().Get("pane")
	if !localPaneIDPattern.MatchString(pane) {
		writeJSONError(w, http.StatusBadRequest, `invalid "pane" parameter, expected a local tmux pane id like %12`)
		return
	}
	defer r.Body.Close()
	var m inputMsg
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if err := s.applyInput(pane, m); err != nil {
		s.logf("pane input apply error pane=%s type=%s err=%v", pane, m.T, err)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (c *paneDiffCache) diff(pane, requestCursor string, next []string, nextCursor string) (from int, tail []string, unchanged bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]paneDiffEntry{}
	}
	prev, ok := c.m[pane]
	c.m[pane] = paneDiffEntry{lines: append([]string(nil), next...), cursor: nextCursor}
	if requestCursor != "" && ok && requestCursor == prev.cursor {
		if requestCursor == nextCursor {
			return 0, nil, true
		}
		p := 0
		for p < len(prev.lines) && p < len(next) && prev.lines[p] == next[p] {
			p++
		}
		return p, append([]string(nil), next[p:]...), false
	}
	return 0, append([]string(nil), next...), false
}

func paneDiffCursor(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "v1:" + hex.EncodeToString(sum[:])
}

func (s *Server) handleRemotePaneWS(w http.ResponseWriter, r *http.Request, peer statusd.Peer, pane string) {
	if !localPaneIDPattern.MatchString(pane) {
		writeJSONError(w, http.StatusBadRequest, `invalid remote pane id, expected a tmux pane id like %12`)
		return
	}
	if _, err := peerPaneURL(peer.URL, "/api/pane/diff", pane); err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("invalid peer URL %q: %v", peer.URL, err))
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	started := time.Now()
	s.logf("peer pane stream open peer=%s pane=%s", peer.Name, pane)
	defer s.logf("peer pane stream closed peer=%s pane=%s duration=%s", peer.Name, pane, time.Since(started).Round(time.Millisecond))
	defer conn.CloseNow()
	conn.SetReadLimit(wsReadLimit)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var writeMu sync.Mutex
	write := func(m outMsg) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		writeCtx, writeCancel := context.WithTimeout(ctx, wsWriteTimeout)
		defer writeCancel()
		return wsjson.Write(writeCtx, conn, m)
	}

	go func() {
		t := time.NewTicker(wsPingInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pingCtx, pingCancel := context.WithTimeout(ctx, wsPingTimeout)
				err := conn.Ping(pingCtx)
				pingCancel()
				if err != nil {
					s.logf("peer pane stream ping failed peer=%s pane=%s err=%v", peer.Name, pane, err)
					cancel()
					return
				}
			}
		}
	}()

	inputSent := make(chan struct{}, 1)
	go func() {
		defer cancel()
		for {
			var m inputMsg
			if err := wsjson.Read(ctx, conn, &m); err != nil {
				s.logf("peer pane stream browser read ended peer=%s pane=%s err=%v", peer.Name, pane, err)
				return
			}
			// Input bound for a peer pane is still a human acting in this
			// server's web UI.
			s.recordHumanActivity()
			if err := s.postPeerPaneInput(ctx, peer, pane, m); err != nil {
				_ = write(outMsg{T: "error", S: err.Error()})
				continue
			}
			select {
			case inputSent <- struct{}{}:
			default:
			}
		}
	}()

	s.pollPeerPaneDiff(ctx, peer, pane, write, inputSent)
}

func (s *Server) pollPeerPaneDiff(ctx context.Context, peer statusd.Peer, pane string, write func(outMsg) error, inputSent <-chan struct{}) {
	cursor := ""
	delay := time.Duration(0)
	unchanged := 0
	errBackoff := 500 * time.Millisecond
	for {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-inputSent:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				delay = 200 * time.Millisecond
				continue
			case <-timer.C:
			}
		}

		patch, ok, err := s.getPeerPaneDiff(ctx, peer, pane, cursor)
		if err != nil {
			_ = write(outMsg{T: "error", S: err.Error()})
			delay = errBackoff
			if errBackoff < 2*time.Second {
				errBackoff *= 2
				if errBackoff > 2*time.Second {
					errBackoff = 2 * time.Second
				}
			}
			continue
		}
		errBackoff = 500 * time.Millisecond
		if ok {
			cursor = patch.Cursor
			unchanged = 0
			if write(outMsg{T: patch.T, From: patch.From, Lines: patch.Lines, Q: patch.Q}) != nil {
				return
			}
			delay = 200 * time.Millisecond
			continue
		}

		unchanged++
		switch {
		case unchanged < 5:
			delay = 200 * time.Millisecond
		case unchanged < 20:
			delay = 500 * time.Millisecond
		default:
			delay = time.Second
		}
	}
}

func (s *Server) getPeerPaneDiff(ctx context.Context, peer statusd.Peer, pane, cursor string) (paneDiffMsg, bool, error) {
	upstream, err := peerPaneURL(peer.URL, "/api/pane/diff", pane)
	if err != nil {
		return paneDiffMsg{}, false, fmt.Errorf("invalid peer URL %q: %v", peer.URL, err)
	}
	u, err := url.Parse(upstream)
	if err != nil {
		return paneDiffMsg{}, false, err
	}
	q := u.Query()
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	u.RawQuery = q.Encode()

	reqCtx, cancel := context.WithTimeout(ctx, peerRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
	if err != nil {
		return paneDiffMsg{}, false, err
	}
	started := time.Now()
	resp, err := s.httpClient().Do(req)
	elapsed := time.Since(started)
	if err != nil {
		if ctx.Err() == nil {
			s.logf("peer diff error peer=%s pane=%s duration=%s err=%v", peer.Name, pane, elapsed.Round(time.Millisecond), err)
		}
		return paneDiffMsg{}, false, fmt.Errorf("peer %s diff request failed: %v", peer.Name, err)
	}
	defer resp.Body.Close()
	if elapsed >= peerSlowRequest {
		s.logf("peer diff slow peer=%s pane=%s status=%d duration=%s", peer.Name, pane, resp.StatusCode, elapsed.Round(time.Millisecond))
	}
	switch resp.StatusCode {
	case http.StatusNoContent:
		return paneDiffMsg{}, false, nil
	case http.StatusOK:
		var patch paneDiffMsg
		if err := json.NewDecoder(resp.Body).Decode(&patch); err != nil {
			s.logf("peer diff decode error peer=%s pane=%s status=%d duration=%s err=%v", peer.Name, pane, resp.StatusCode, elapsed.Round(time.Millisecond), err)
			return paneDiffMsg{}, false, fmt.Errorf("peer %s diff response invalid: %v", peer.Name, err)
		}
		if patch.T == "" {
			patch.T = "patch"
		}
		return patch, true, nil
	case http.StatusNotFound:
		s.logf("peer diff unsupported peer=%s pane=%s status=%d duration=%s", peer.Name, pane, resp.StatusCode, elapsed.Round(time.Millisecond))
		return paneDiffMsg{}, false, fmt.Errorf("peer %s does not support /api/pane/diff; please update the peer tmact", peer.Name)
	default:
		s.logf("peer diff bad status peer=%s pane=%s status=%d duration=%s", peer.Name, pane, resp.StatusCode, elapsed.Round(time.Millisecond))
		return paneDiffMsg{}, false, fmt.Errorf("peer %s diff returned HTTP %d", peer.Name, resp.StatusCode)
	}
}

func (s *Server) postPeerPaneInput(ctx context.Context, peer statusd.Peer, pane string, m inputMsg) error {
	upstream, err := peerPaneURL(peer.URL, "/api/pane/input", pane)
	if err != nil {
		return fmt.Errorf("invalid peer URL %q: %v", peer.URL, err)
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(m); err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, peerRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, upstream, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	started := time.Now()
	resp, err := s.httpClient().Do(req)
	elapsed := time.Since(started)
	if err != nil {
		if ctx.Err() == nil {
			s.logf("peer input error peer=%s pane=%s type=%s duration=%s err=%v", peer.Name, pane, m.T, elapsed.Round(time.Millisecond), err)
		}
		return fmt.Errorf("peer %s input request failed: %v", peer.Name, err)
	}
	defer resp.Body.Close()
	if elapsed >= peerSlowRequest {
		s.logf("peer input slow peer=%s pane=%s type=%s status=%d duration=%s", peer.Name, pane, m.T, resp.StatusCode, elapsed.Round(time.Millisecond))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.logf("peer input bad status peer=%s pane=%s type=%s status=%d duration=%s", peer.Name, pane, m.T, resp.StatusCode, elapsed.Round(time.Millisecond))
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("peer %s does not support /api/pane/input; please update the peer tmact", peer.Name)
		}
		return fmt.Errorf("peer %s input returned HTTP %d", peer.Name, resp.StatusCode)
	}
	return nil
}

func peerPaneURL(base, path, pane string) (string, error) {
	q := url.Values{}
	q.Set("pane", pane)
	return peerHTTPURL(base, path, q)
}
