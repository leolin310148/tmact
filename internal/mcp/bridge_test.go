package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var testBridge = []BridgeCommand{
	{Words: []string{"ls"}, Summary: "list panes", AutoJSON: true,
		Flags: []BridgeFlag{{Key: "peer", Flag: "--peer", Kind: FlagString}}},
	{Words: []string{"loop", "list"}, Summary: "list loops", AutoJSON: true,
		Flags: []BridgeFlag{{Key: "run_dir", Flag: "--run-dir", Kind: FlagString, Path: PathUnderRoots}}},
	{Words: []string{"send"}, Summary: "send input", Write: true, GlobalTarget: true,
		Flags: []BridgeFlag{
			{Key: "text", Flag: "--text", Kind: FlagString},
			{Key: "key", Flag: "--key", Kind: FlagList},
			{Key: "execute", Flag: "--execute", Kind: FlagBool},
		}},
	{Words: []string{"dispatch-work"}, Summary: "dispatch", Write: true, AutoJSON: true,
		Positional: &BridgeArg{Key: "session", Required: true, Session: true},
		Flags: []BridgeFlag{
			{Key: "dir", Flag: "--dir", Kind: FlagString, Path: PathUnderRoots, Required: true},
			{Key: "peer", Flag: "--peer", Kind: FlagString},
		}},
	{Words: []string{"workflow", "stop"}, Summary: "stop", Write: true, FixedArgs: []string{"--wait"},
		Flags: []BridgeFlag{{Key: "id", Flag: "--id", Kind: FlagString}}},
}

type cliRec struct {
	argv [][]string
	res  CLIResult
}

func bridgeDeps(t *testing.T, home string, rec *cliRec) Deps {
	deps := writeDeps(t, home, &writeRec{})
	deps.Bridge = testBridge
	deps.RunCLI = func(_ context.Context, argv []string) (CLIResult, error) {
		rec.argv = append(rec.argv, argv)
		return rec.res, nil
	}
	return deps
}

func TestBridgeToolsListed(t *testing.T) {
	for _, tc := range []struct {
		roots []string
		want  []string
		gone  []string
	}{
		{testRoots, []string{"tmact_ls", "tmact_loop_list", "tmact_send", "tmact_dispatch_work", "tmact_workflow_stop"}, nil},
		{nil, []string{"tmact_ls", "tmact_loop_list"}, []string{"tmact_send", "tmact_dispatch_work"}},
	} {
		cs := connectRoots(t, bridgeDeps(t, t.TempDir(), &cliRec{}), tc.roots)
		res, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		tools := map[string]*sdk.Tool{}
		for _, tool := range res.Tools {
			tools[tool.Name] = tool
		}
		for _, n := range tc.want {
			tool := tools[n]
			if tool == nil {
				t.Fatalf("roots=%v: missing %s", tc.roots, n)
			}
			write := n == "tmact_send" || n == "tmact_dispatch_work" || n == "tmact_workflow_stop"
			if tool.Annotations.ReadOnlyHint == write {
				t.Fatalf("%s readOnly=%v", n, tool.Annotations.ReadOnlyHint)
			}
		}
		for _, n := range tc.gone {
			if tools[n] != nil {
				t.Fatalf("roots=%v: %s listed", tc.roots, n)
			}
		}
	}
}

func TestBridgeArgv(t *testing.T) {
	home := tempHome(t)
	repo, _ := filepath.EvalSymlinks(filepath.Join(home, "w", "repo"))
	cases := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"tmact_ls", nil, "ls --json"},
		{"tmact_ls", map[string]any{"peer": "mini"}, "ls --peer=mini --json"},
		{"tmact_send", map[string]any{"target": "work:0.0", "text": "hi", "key": []string{"Enter", "C-c"}},
			"-t work:0.0 send --key=Enter --key=C-c --text=hi"},
		{"tmact_send", map[string]any{"target": "%3", "text": "go", "execute": true}, "-t %3 send --execute --text=go"},
		{"tmact_dispatch_work", map[string]any{"session": "fix", "dir": "~/w/repo"}, "dispatch-work fix --dir=" + repo + " --json"},
		{"tmact_dispatch_work", map[string]any{"session": "fix", "dir": "~/w/repo", "peer": "mini"}, "dispatch-work fix --dir=~/w/repo --peer=mini --json"},
		{"tmact_loop_list", map[string]any{"run_dir": "~/w/repo"}, "loop list --run-dir=" + repo + " --json"},
		{"tmact_workflow_stop", map[string]any{"id": "wf1"}, "workflow stop --id=wf1 --wait"},
	}
	for _, tc := range cases {
		rec := &cliRec{res: CLIResult{Stdout: `{"ok":true}`}}
		cs := connectRoots(t, bridgeDeps(t, home, rec), testRoots)
		res := call(t, cs, tc.tool, tc.args, nil)
		if res.IsError {
			t.Fatalf("%s %v: %s", tc.tool, tc.args, res.Content[0].(*sdk.TextContent).Text)
		}
		if got := strings.Join(rec.argv[0], " "); got != tc.want {
			t.Fatalf("%s argv = %q want %q", tc.tool, got, tc.want)
		}
	}
}

func TestBridgeRejects(t *testing.T) {
	home := tempHome(t)
	cases := []struct {
		tool string
		args map[string]any
	}{
		{"tmact_ls", map[string]any{"trust_folder": true}},
		{"tmact_send", map[string]any{"text": "no target"}},
		{"tmact_send", map[string]any{"target": "-x", "text": "flag injection"}},
		{"tmact_send", map[string]any{"target": "%1", "execute": "yes"}},
		{"tmact_dispatch_work", map[string]any{"dir": "~/w/repo"}},
		{"tmact_dispatch_work", map[string]any{"session": "a.b", "dir": "~/w/repo"}},
		{"tmact_dispatch_work", map[string]any{"session": "--execute", "dir": "~/w/repo"}},
		{"tmact_dispatch_work", map[string]any{"session": "s", "dir": "/etc"}},
		{"tmact_dispatch_work", map[string]any{"session": "s", "dir": "~/w/escape"}},
		{"tmact_dispatch_work", map[string]any{"session": "s", "dir": "~/w/../etc", "peer": "mini"}},
		{"tmact_loop_list", map[string]any{"run_dir": "~/etc"}},
	}
	for _, tc := range cases {
		rec := &cliRec{}
		cs := connectRoots(t, bridgeDeps(t, home, rec), testRoots)
		if res := call(t, cs, tc.tool, tc.args, nil); !res.IsError {
			t.Fatalf("%s %v accepted", tc.tool, tc.args)
		}
		if len(rec.argv) != 0 {
			t.Fatalf("%s %v ran %v", tc.tool, tc.args, rec.argv)
		}
	}
}

func TestBridgeResult(t *testing.T) {
	rec := &cliRec{res: CLIResult{Stdout: `{"panes":[1]}`}}
	cs := connectRoots(t, bridgeDeps(t, t.TempDir(), rec), testRoots)
	var out map[string]any
	call(t, cs, "tmact_ls", nil, &out)
	if r, _ := json.Marshal(out["result"]); string(r) != `{"panes":[1]}` {
		t.Fatalf("result = %s", r)
	}

	rec.res = CLIResult{Stdout: "plain text", Stderr: "boom", ExitCode: 2}
	res := call(t, cs, "tmact_workflow_stop", map[string]any{"id": "x"}, nil)
	text := res.Content[0].(*sdk.TextContent).Text
	if !res.IsError || !strings.Contains(text, "boom") || !strings.Contains(text, "observed terminal output") {
		t.Fatalf("exit 2 result = %v %s", res.IsError, text)
	}

	rec.res = CLIResult{TimedOut: true, ExitCode: -1}
	res = call(t, cs, "tmact_ls", nil, nil)
	if !res.IsError || !strings.Contains(res.Content[0].(*sdk.TextContent).Text, "timed_out") {
		t.Fatalf("timeout not surfaced: %+v", res)
	}
}
