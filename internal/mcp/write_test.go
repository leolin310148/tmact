package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/statusd"
)

var testRoots = []string{"~/w", "~/ops"}

type writeRec struct {
	dispatched []dispatch.Options
	peerOf     []string
	sent       []string
}

func writeDeps(t *testing.T, home string, rec *writeRec, snaps ...statusd.Snapshot) Deps {
	t.Helper()
	deps := testDeps(new([]captureCall))
	if len(snaps) > 0 {
		i := 0
		deps.Snapshot = func(context.Context) (statusd.Snapshot, error) {
			s := snaps[min(i, len(snaps)-1)]
			i++
			return s, nil
		}
	}
	deps.Home = func() (string, error) { return home, nil }
	deps.Dispatch = func(_ context.Context, o dispatch.Options) (dispatch.Report, error) {
		rec.dispatched = append(rec.dispatched, o)
		rec.peerOf = append(rec.peerOf, "")
		return dispatch.Report{Session: o.Session, Target: o.Session + ":0.0", Dir: o.Dir, Agent: o.Agent,
			Steps: []dispatch.Step{{Name: "send-prompt", Status: dispatch.StatusOK}}}, nil
	}
	deps.DispatchPeer = func(_ context.Context, peer string, o dispatch.Options) (dispatch.Report, error) {
		rec.dispatched = append(rec.dispatched, o)
		rec.peerOf = append(rec.peerOf, peer)
		return dispatch.Report{Session: o.Session, Target: o.Session + ":0.0", Dir: o.Dir, Agent: o.Agent}, nil
	}
	deps.SendLocal = func(_ context.Context, pane, text string) error {
		rec.sent = append(rec.sent, pane+"|"+text)
		return nil
	}
	deps.SendPeer = func(_ context.Context, peer, pane, text string) error {
		rec.sent = append(rec.sent, peer+"@"+pane+"|"+text)
		return nil
	}
	deps.Sleep = func(context.Context, time.Duration) error { return nil }
	return deps
}

func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	for _, d := range []string{"w/repo", "ops", "etc"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/", filepath.Join(home, "w", "escape")); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestWriteToolsOnlyWithRoots(t *testing.T) {
	cs := connectRoots(t, testDeps(new([]captureCall)), testRoots)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if tool.Name == "dispatch_work" || tool.Name == "send_text" || tool.Name == "answer_prompt" {
			a := tool.Annotations
			if a == nil || a.ReadOnlyHint || a.DestructiveHint == nil || !*a.DestructiveHint {
				t.Fatalf("%s annotations = %+v", tool.Name, a)
			}
			if strings.Contains(string(mustJSON(t, tool.InputSchema)), "trust") {
				t.Fatalf("%s exposes a trust field", tool.Name)
			}
		}
	}
	if got := strings.Join(names, ","); got != "answer_prompt,capture_pane,dispatch_work,list_panes,list_sessions,send_text,wait_pane" {
		t.Fatalf("tools = %s", got)
	}
}

func TestDispatchWorkLocal(t *testing.T) {
	home := tempHome(t)
	rec := &writeRec{}
	cs := connectRoots(t, writeDeps(t, home, rec), testRoots)
	var out dispatchOut
	res := call(t, cs, "dispatch_work", map[string]any{
		"session": "fix-1", "dir": "~/w/repo", "agent": "codex", "prompt": "run tests",
	}, &out)
	if res.IsError {
		t.Fatalf("error: %v", res.Content[0].(*sdk.TextContent).Text)
	}
	o := rec.dispatched[0]
	if !o.Execute || o.TrustFolder || o.Wait || o.ReadyTimeout != dispatchReadyTimeout {
		t.Fatalf("opts = %+v", o)
	}
	if want, _ := filepath.EvalSymlinks(filepath.Join(home, "w", "repo")); o.Dir != want {
		t.Fatalf("dir = %q want %q", o.Dir, want)
	}
	if out.Pane != "fix-1:0.0" || len(out.Steps) != 1 {
		t.Fatalf("out = %+v", out)
	}
}

func TestDispatchWorkPeerKeepsTildeDir(t *testing.T) {
	rec := &writeRec{}
	cs := connectRoots(t, writeDeps(t, t.TempDir(), rec), testRoots)
	var out dispatchOut
	call(t, cs, "dispatch_work", map[string]any{"session": "s", "dir": "~/ops/x/../y", "agent": "claude", "prompt": "p", "peer": "mini"}, &out)
	if rec.peerOf[0] != "mini" || rec.dispatched[0].Dir != "~/ops/y" || out.Pane != "mini@s:0.0" {
		t.Fatalf("rec = %+v out = %+v", rec, out)
	}
}

func TestDispatchWorkRejects(t *testing.T) {
	home := tempHome(t)
	cases := map[string]map[string]any{
		"dotdot":      {"dir": "~/w/../etc"},
		"absolute":    {"dir": "/Users/x/w/repo"},
		"other root":  {"dir": "~/etc"},
		"prefix only": {"dir": "~/wat"},
		"symlink":     {"dir": "~/w/escape"},
		"missing":     {"dir": "~/w/nope"},
		"dot session": {"session": "a.b"},
		"at session":  {"session": "x@y"},
		"shell agent": {"agent": "shell"},
		"empty":       {"prompt": " "},
		"peer cont":   {"peer": "mini", "continue_conversation": true},
		"trust field": {"trust_folder": true},
		"execute":     {"execute": false},
	}
	for name, override := range cases {
		t.Run(name, func(t *testing.T) {
			rec := &writeRec{}
			cs := connectRoots(t, writeDeps(t, home, rec), testRoots)
			args := map[string]any{"session": "ok", "dir": "~/w/repo", "agent": "codex", "prompt": "p"}
			for k, v := range override {
				args[k] = v
			}
			if res := call(t, cs, "dispatch_work", args, nil); !res.IsError {
				t.Fatalf("accepted %v", args)
			}
			if len(rec.dispatched) != 0 {
				t.Fatalf("dispatched %+v", rec.dispatched)
			}
		})
	}
}

func agentSnap(mut func(*statusd.PaneStatus)) statusd.Snapshot {
	s := fakeSnapshot()
	p := statusd.PaneStatus{Target: "work:0.0", PaneID: "%1", Session: "work", Runtime: "codex", State: "idle", Idle: true, InputReady: true}
	if mut != nil {
		mut(&p)
	}
	s.Panes["work:0.0"] = p
	return s
}

func TestSendTextGuards(t *testing.T) {
	cases := map[string]func(*statusd.PaneStatus){
		"shell":   func(p *statusd.PaneStatus) { p.Runtime = "shell" },
		"asking":  func(p *statusd.PaneStatus) { p.Asking = true },
		"running": func(p *statusd.PaneStatus) { p.Running, p.InputReady, p.Idle = true, false, false },
		"stale":   func(p *statusd.PaneStatus) { p.Stale = true },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			rec := &writeRec{}
			cs := connectRoots(t, writeDeps(t, t.TempDir(), rec, agentSnap(mut)), testRoots)
			if res := call(t, cs, "send_text", map[string]any{"pane": "work:0.0", "text": "hi"}, nil); !res.IsError {
				t.Fatal("accepted")
			}
			if len(rec.sent) != 0 {
				t.Fatalf("sent %v", rec.sent)
			}
		})
	}
}

func TestSendTextLocalAndPeer(t *testing.T) {
	rec := &writeRec{}
	idle := agentSnap(nil)
	busy := agentSnap(func(p *statusd.PaneStatus) { p.Running, p.InputReady, p.State = true, false, "working" })
	peerIdle := fakeSnapshot()
	pp := peerIdle.Panes["mini@side:0.0"]
	pp.Asking, pp.Prompt, pp.State, pp.Idle, pp.InputReady = false, nil, "idle", true, true
	peerIdle.Panes["mini@side:0.0"] = pp

	cs := connectRoots(t, writeDeps(t, t.TempDir(), rec, idle, busy), testRoots)
	var out sendTextOut
	call(t, cs, "send_text", map[string]any{"pane": "%1", "text": "next"}, &out)
	if rec.sent[0] != "%1|next" || !out.Accepted || out.StateAfter != "working" {
		t.Fatalf("rec=%v out=%+v", rec.sent, out)
	}

	cs = connectRoots(t, writeDeps(t, t.TempDir(), rec, peerIdle), testRoots)
	call(t, cs, "send_text", map[string]any{"pane": "mini@side:0.0", "text": "go"}, &out)
	if rec.sent[1] != "mini@%3|go" || out.Accepted {
		t.Fatalf("rec=%v out=%+v", rec.sent, out)
	}
}

func TestWaitPane(t *testing.T) {
	busy := agentSnap(func(p *statusd.PaneStatus) { p.Running, p.InputReady, p.State = true, false, "working" })
	idle := agentSnap(nil)
	asking := agentSnap(func(p *statusd.PaneStatus) { p.Asking, p.State = true, "asking" })
	gone := fakeSnapshot()
	delete(gone.Panes, "work:0.0")

	run := func(snaps ...statusd.Snapshot) waitOut {
		t.Helper()
		clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		deps := writeDeps(t, t.TempDir(), &writeRec{}, snaps...)
		deps.Now = func() time.Time { return clock }
		deps.Sleep = func(context.Context, time.Duration) error { clock = clock.Add(time.Second); return nil }
		cs := connect(t, deps)
		var out waitOut
		if res := call(t, cs, "wait_pane", map[string]any{"pane": "work:0.0", "timeout_seconds": 5}, &out); res.IsError {
			t.Fatalf("error %v", res.Content)
		}
		return out
	}
	if out := run(busy, busy, idle); out.Outcome != "idle" || out.WaitedSeconds < 3 {
		t.Fatalf("idle: %+v", out)
	}
	if out := run(busy, asking); out.Outcome != "asking" {
		t.Fatalf("asking: %+v", out)
	}
	if out := run(busy); out.Outcome != "timeout" || out.State != "working" {
		t.Fatalf("timeout: %+v", out)
	}
	if out := run(busy, gone); out.Outcome != "gone" {
		t.Fatalf("gone: %+v", out)
	}
}

func TestNormalizeRoots(t *testing.T) {
	got, err := NormalizeRoots([]string{"~/w/", " ~/ops", ""})
	if err != nil || strings.Join(got, ",") != "~/w,~/ops" {
		t.Fatalf("got %v %v", got, err)
	}
	for _, bad := range []string{"/etc", "~", "~/../x", "w"} {
		if _, err := NormalizeRoots([]string{bad}); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
