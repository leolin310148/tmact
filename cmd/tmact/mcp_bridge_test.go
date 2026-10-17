package main

import (
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/mcp"
)

func TestMCPBridgeCommandsResolve(t *testing.T) {
	cmds, err := mcpBridgeCommands()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]mcp.BridgeCommand{}
	for _, c := range cmds {
		byName[c.ToolName()] = c
		for _, f := range c.Flags {
			if mcpBridgeDeniedFlags[f.Flag] {
				t.Fatalf("%s exposes denied flag %s", c.ToolName(), f.Flag)
			}
			if mcpBridgePathFlags[f.Flag] && f.Path != mcp.PathUnderRoots {
				t.Fatalf("%s %s is not root-checked", c.ToolName(), f.Flag)
			}
		}
	}
	for _, never := range []string{"tmact_cred_run", "tmact_cred_rm", "tmact_cred_request", "tmact_trust_folder",
		"tmact_hook_init", "tmact_statusd_start", "tmact_stt_set", "tmact_panels_ensure", "tmact_ask", "tmact_reply"} {
		if _, ok := byName[never]; ok {
			t.Fatalf("%s must not be bridged", never)
		}
	}
	dw := byName["tmact_dispatch_work"]
	if !dw.Write || dw.Positional == nil || !dw.Positional.Session || !dw.AutoJSON {
		t.Fatalf("dispatch-work spec = %+v", dw)
	}
	for _, f := range dw.Flags {
		if f.Flag == "--wait" || f.Flag == "--config" {
			t.Fatalf("dispatch-work exposes %s", f.Flag)
		}
	}
	if send := byName["tmact_send"]; !send.GlobalTarget || !send.Write {
		t.Fatalf("send spec = %+v", send)
	}
	if ws := byName["tmact_workflow_stop"]; strings.Join(ws.FixedArgs, " ") != "--wait" || len(ws.Flags) != 2 {
		t.Fatalf("workflow stop spec = %+v", ws)
	}
	if !byName["tmact_cred_ls"].AutoJSON || byName["tmact_summarize"].Write {
		t.Fatal("cred ls / summarize spec wrong")
	}
	var keyKind string
	for _, f := range byName["tmact_send"].Flags {
		if f.Flag == "--key" {
			keyKind = f.Kind
		}
	}
	if keyKind != mcp.FlagList {
		t.Fatalf("send --key kind = %q", keyKind)
	}
}
