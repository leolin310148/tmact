package web

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/leolin310148/tmact/internal/prompt"
)

// paneIDPattern restricts pane targets to canonical tmux pane ids (%12). Acting
// strictly on a pane id means a request value can never be read as a tmux flag.
var paneIDPattern = regexp.MustCompile(`^%[0-9]+$`)

// inputMsg is a client-to-server WebSocket message.
//
// Older clients may still send "resize" frames; the server ignores them — the
// statusd daemon enforces a fixed tmux window size so the scrollback grid
// stays consistent across devices, and the browser does the visual wrap.
type inputMsg struct {
	T string `json:"t"`           // "text", "send", "key", "clear" (or legacy "resize", ignored)
	S string `json:"s,omitempty"` // literal text for "text"/"send"
	K string `json:"k,omitempty"` // tmux key name for "key"
}

// outMsg is a server-to-client WebSocket message.
//
//   - "patch" — incremental pane update. Lines[From:] replaces the client's
//     buffer from index From onwards. From=0 with the full line list is the
//     initial snapshot; From=len(lastLines) with empty Lines means append
//     nothing (used on connect when capture is empty).
//   - "error" — server-side error text in S, no buffer change.
type outMsg struct {
	T     string   `json:"t"`
	S     string   `json:"s,omitempty"`
	From  int      `json:"from,omitempty"`
	Lines []string `json:"lines,omitempty"`
	// Q is the interactive menu the pane is waiting on, when one is detected.
	// It rides along with each "patch" so the browser can offer quick-answer
	// buttons; nil (omitted) means there is no question to answer.
	Q *prompt.Question `json:"q,omitempty"`
}

func (s *Server) handlePaneWS(w http.ResponseWriter, r *http.Request) {
	pane := r.URL.Query().Get("pane")
	if !paneIDPattern.MatchString(pane) {
		writeJSONError(w, http.StatusBadRequest, `invalid "pane" parameter, expected a tmux pane id like %12`)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(wsReadLimit)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Periodic ping/pong so a dead connection (laptop sleep, NAT drop, proxy
	// timeout) is noticed within wsPingInterval rather than waiting for the
	// next browser input. Each Ping waits up to wsPingTimeout for the pong.
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
					cancel()
					return
				}
			}
		}
	}()

	var writeMu sync.Mutex
	write := func(m outMsg) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return wsjson.Write(ctx, conn, m)
	}

	// Reader goroutine: relay browser input into the pane.
	go func() {
		defer cancel()
		for {
			var m inputMsg
			if err := wsjson.Read(ctx, conn, &m); err != nil {
				return
			}
			if err := s.applyInput(pane, m); err != nil {
				_ = write(outMsg{T: "error", S: err.Error()})
			}
		}
	}()

	// Main loop: stream pane output to the browser.
	ticker := time.NewTicker(wsCaptureInterval)
	defer ticker.Stop()

	last := ""
	var lastLines []string
	push := func() bool {
		content, err := s.capture()(pane, wsCaptureLines)
		if err != nil {
			_ = write(outMsg{T: "error", S: err.Error()})
			return false
		}
		if content == last {
			return true
		}
		last = content
		next := strings.Split(content, "\n")
		// Find longest common prefix line count; the client only needs the
		// diverging tail. Typical AI-agent output advances one line per tick,
		// so this collapses a 400-line capture to a 1-line patch.
		p := 0
		for p < len(lastLines) && p < len(next) && lastLines[p] == next[p] {
			p++
		}
		tail := append([]string(nil), next[p:]...)
		lastLines = next
		return write(outMsg{
			T:     "patch",
			From:  p,
			Lines: tail,
			Q:     prompt.DetectQuestion(content),
		}) == nil
	}

	if !push() {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !push() {
				return
			}
		}
	}
}

// applyInput validates and relays one browser input message into the pane.
func (s *Server) applyInput(target string, m inputMsg) error {
	switch m.T {
	case "text":
		if m.S == "" {
			return nil
		}
		return s.sendText()(target, m.S, false)
	case "send":
		if m.S == "" {
			return nil
		}
		return s.sendText()(target, m.S, true)
	case "key":
		if !keyAllowed(m.K) {
			return fmt.Errorf("key not allowed: %q", m.K)
		}
		return s.sendKey()(target, m.K)
	case "clear":
		return s.clearPane()(target)
	case "resize":
		// Tolerated for backwards compatibility — older browser bundles still
		// send this on connect. statusd owns window sizing now.
		return nil
	default:
		return fmt.Errorf("unknown message type: %q", m.T)
	}
}

var allowedNamedKeys = map[string]bool{
	"Enter": true, "BSpace": true, "Tab": true, "BTab": true, "Escape": true,
	"Up": true, "Down": true, "Left": true, "Right": true,
	"Home": true, "End": true, "PageUp": true, "PageDown": true,
	"Delete": true, "Space": true,
}

// keyAllowed gates the "key" message against a fixed set of tmux key names plus
// Ctrl+<letter> combos, so an arbitrary string can never reach send-keys.
func keyAllowed(k string) bool {
	if allowedNamedKeys[k] {
		return true
	}
	if len(k) == 3 && k[0] == 'C' && k[1] == '-' && k[2] >= 'a' && k[2] <= 'z' {
		return true
	}
	return false
}
