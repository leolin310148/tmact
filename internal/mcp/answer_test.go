package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/prompt"
	"github.com/leolin310148/tmact/internal/statusd"
)

const approvalScreen = "\nDo you want to proceed?\n  1. Yes\n❯ 2. No\n"

func answerDeps(t *testing.T, screen string, keys *[]string, snap statusd.Snapshot) Deps {
	t.Helper()
	deps := writeDeps(t, t.TempDir(), &writeRec{}, snap)
	deps.CaptureLocal = func(context.Context, string, int) (string, error) { return screen, nil }
	deps.CapturePeer = func(context.Context, string, string, int) (string, error) { return screen, nil }
	deps.KeysLocal = func(_ context.Context, pane string, k []string) error {
		*keys = append(*keys, pane+"|"+strings.Join(k, ","))
		return nil
	}
	deps.KeysPeer = func(_ context.Context, peer, pane string, k []string) error {
		*keys = append(*keys, peer+"@"+pane+"|"+strings.Join(k, ","))
		return nil
	}
	return deps
}

func askingSnap() statusd.Snapshot {
	return agentSnap(func(p *statusd.PaneStatus) { p.Asking, p.Idle, p.InputReady, p.State = true, false, false, "asking" })
}

func approvalQuestion(t *testing.T, screen string) string {
	t.Helper()
	d := prompt.Detect(screen)
	if d == nil {
		t.Fatal("fixture is not a prompt")
	}
	if d.Question != "" {
		return d.Question
	}
	return d.Title
}

func TestAnswerPromptPressesDigit(t *testing.T) {
	var keys []string
	cs := connectRoots(t, answerDeps(t, approvalScreen, &keys, askingSnap()), testRoots)
	var out promptAnswerOut
	res := call(t, cs, "answer_prompt", map[string]any{
		"pane": "work:0.0", "option_number": 1,
		"expected_question": "  " + approvalQuestion(t, approvalScreen) + " ", "expected_option_label": "yes",
	}, &out)
	if res.IsError {
		t.Fatalf("error: %+v", res.Content)
	}
	if len(keys) != 1 || keys[0] != "%1|1" || out.Option != 1 {
		t.Fatalf("keys=%v out=%+v", keys, out)
	}
}

func TestAnswerPromptPeer(t *testing.T) {
	var keys []string
	snap := fakeSnapshot()
	snap.Panes["mini@ops:0.0"] = statusd.PaneStatus{Target: "ops:0.0", PaneID: "mini@%3", Session: "mini@ops", Peer: "mini", Runtime: "claude", Asking: true}
	cs := connectRoots(t, answerDeps(t, approvalScreen, &keys, snap), testRoots)
	res := call(t, cs, "answer_prompt", map[string]any{
		"pane": "mini@%3", "option_number": 2,
		"expected_question": approvalQuestion(t, approvalScreen), "expected_option_label": "No",
	}, nil)
	if res.IsError || len(keys) != 1 || keys[0] != "mini@%3|2" {
		t.Fatalf("isError=%v keys=%v", res.IsError, keys)
	}
}

func TestAnswerPromptRefuses(t *testing.T) {
	q := approvalQuestion(t, approvalScreen)
	cases := map[string]struct {
		screen string
		snap   statusd.Snapshot
		args   map[string]any
	}{
		"no prompt now":   {"$ \n", askingSnap(), map[string]any{"option_number": 1, "expected_question": q, "expected_option_label": "Yes"}},
		"question moved":  {approvalScreen, askingSnap(), map[string]any{"option_number": 1, "expected_question": "Apply this patch?", "expected_option_label": "Yes"}},
		"empty question":  {approvalScreen, askingSnap(), map[string]any{"option_number": 1, "expected_question": "", "expected_option_label": "Yes"}},
		"label mismatch":  {approvalScreen, askingSnap(), map[string]any{"option_number": 1, "expected_question": q, "expected_option_label": "No"}},
		"missing option":  {approvalScreen, askingSnap(), map[string]any{"option_number": 3, "expected_question": q, "expected_option_label": "Yes"}},
		"shell pane":      {approvalScreen, agentSnap(func(p *statusd.PaneStatus) { p.Runtime = "shell" }), map[string]any{"option_number": 1, "expected_question": q, "expected_option_label": "Yes"}},
		"workspace trust": {"Do you trust the contents of this directory?\n› 1. Yes, continue\n  2. No, quit\n", askingSnap(), map[string]any{"option_number": 1, "expected_question": "Do you trust the contents of this directory?", "expected_option_label": "Yes, continue"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var keys []string
			cs := connectRoots(t, answerDeps(t, c.screen, &keys, c.snap), testRoots)
			c.args["pane"] = "work:0.0"
			if res := call(t, cs, "answer_prompt", c.args, nil); !res.IsError {
				t.Fatal("answered")
			}
			if len(keys) != 0 {
				t.Fatalf("pressed %v", keys)
			}
		})
	}
}

func TestAnswerPromptNeedsRoots(t *testing.T) {
	cs := connect(t, testDeps(new([]captureCall)))
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "answer_prompt" {
			t.Fatal("answer_prompt registered on a read-only server")
		}
	}
}
