package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/mcp"
)

// mcpBridgeEntry allowlists one CLI command for the MCP bridge. Everything not
// listed here (cred run/rm/request, trust-folder, hook init, statusd start,
// stt-set, panels, ask/reply, ...) is never reachable over MCP.
type mcpBridgeEntry struct {
	command      string
	write        bool
	globalTarget bool
	positional   *mcp.BridgeArg
	omit         []string // flags hidden from this command's schema
	fixed        []string
	notes        string
}

// Flags never exposed on any bridged command: workspace trust stays a CLI
// opt-in, the socket is fixed, and following logs would block the call.
var mcpBridgeDeniedFlags = map[string]bool{
	"--trust-folder":  true,
	"--trust-folders": true,
	"--socket-path":   true,
	"--follow":        true,
	"--json":          true, // added automatically
}

var mcpBridgePathFlags = map[string]bool{"--dir": true, "--config": true, "--run-dir": true}

var mcpBridgeListFlags = map[string]bool{"--var": true, "--key": true}

const (
	runDirNote      = "run_dir defaults to .tmact/runs under the user's home; for loops started in a repo pass ~/w/<repo>/.tmact/runs."
	agentConfigNote = "config defaults to examples/agents.yaml under the user's home; pass the repo's agents file, e.g. ~/w/<repo>/examples/agents.yaml."
	sessionArgDesc  = "tmux session name: letters, digits, _ or -"
)

func sessionArg() *mcp.BridgeArg {
	return &mcp.BridgeArg{Key: "session", Description: sessionArgDesc, Required: true, Session: true}
}

var mcpBridgeEntries = []mcpBridgeEntry{
	// Read.
	{command: "ls", omit: []string{"--config"}},
	{command: "capture", omit: []string{"--config"}},
	{command: "status", notes: agentConfigNote},
	{command: "inspect"},
	{command: "detect"},
	{command: "summarize", notes: agentConfigNote},
	{command: "session closed"},
	{command: "loop list", notes: runDirNote},
	{command: "loop status", notes: runDirNote},
	{command: "loop logs", notes: runDirNote},
	{command: "workflow status"},
	{command: "workflow logs"},
	{command: "statusd read"},
	{command: "statusd status"},
	{command: "cred ls"},

	// Write: CLI defaults apply, so most only print a plan unless execute is true.
	{command: "send", write: true, globalTarget: true, omit: []string{"--config"}},
	{command: "dispatch-work", write: true, positional: sessionArg(),
		omit:  []string{"--config", "--wait", "--wait-timeout", "--wait-settle", "--result-lines"},
		notes: "Prints a plan unless execute is true. Follow up with wait_pane instead of --wait."},
	{command: "session create", write: true, positional: sessionArg()},
	{command: "session close", write: true, positional: sessionArg()},
	{command: "session reopen", write: true, positional: sessionArg()},
	{command: "session resume", write: true, positional: sessionArg()},
	{command: "broadcast", write: true, notes: agentConfigNote},
	{command: "loop start", write: true, notes: runDirNote},
	{command: "loop pause", write: true, notes: runDirNote},
	{command: "loop resume", write: true, notes: runDirNote},
	{command: "loop stop", write: true, notes: runDirNote,
		positional: &mcp.BridgeArg{Key: "loop_id", Description: "loop id from tmact_loop_list (or use id/config)"}},
	{command: "loop restart", write: true, notes: runDirNote},
	{command: "workflow start", write: true},
	{command: "workflow pause", write: true},
	{command: "workflow resume", write: true},
	{command: "workflow stop", write: true, omit: []string{"--wait"}, fixed: []string{"--wait"}},
	{command: "workflow retry", write: true},
	{command: "workflow acknowledge", write: true},
}

// mcpBridgeCommands builds bridge specs from the CLI help catalog so tool
// descriptions and parameters track the CLI.
func mcpBridgeCommands() ([]mcp.BridgeCommand, error) {
	var out []mcp.BridgeCommand
	for _, e := range mcpBridgeEntries {
		help, ok := commandHelpFor(e.command)
		if !ok {
			return nil, fmt.Errorf("mcp bridge: %q is not in the help catalog", e.command)
		}
		omit := map[string]bool{}
		for _, f := range e.omit {
			omit[f] = true
		}
		cmd := mcp.BridgeCommand{
			Words:        strings.Fields(e.command),
			Summary:      help.Summary,
			Safety:       help.Safety,
			Notes:        e.notes,
			Write:        e.write,
			GlobalTarget: e.globalTarget,
			FixedArgs:    e.fixed,
			Positional:   e.positional,
		}
		for _, f := range help.Flags {
			flag := longFlagName(f.Name)
			if flag == "--json" {
				cmd.AutoJSON = true
			}
			if flag == "" || mcpBridgeDeniedFlags[flag] || omit[flag] {
				continue
			}
			bf := mcp.BridgeFlag{
				Key:         strings.ReplaceAll(strings.TrimPrefix(flag, "--"), "-", "_"),
				Flag:        flag,
				Kind:        mcp.FlagString,
				Description: f.Description,
				Required:    f.Required,
			}
			switch {
			case f.Value == "":
				bf.Kind = mcp.FlagBool
			case mcpBridgeListFlags[flag]:
				bf.Kind = mcp.FlagList
			}
			if mcpBridgePathFlags[flag] {
				bf.Path = mcp.PathUnderRoots
			}
			cmd.Flags = append(cmd.Flags, bf)
		}
		if !cmd.AutoJSON {
			for _, u := range help.Usage {
				if strings.Contains(u, "--json") {
					cmd.AutoJSON = true
				}
			}
		}
		out = append(out, cmd)
	}
	return out, nil
}

// longFlagName turns "-t, --target" into "--target".
func longFlagName(name string) string {
	for _, part := range strings.Split(name, ",") {
		if p := strings.TrimSpace(part); strings.HasPrefix(p, "--") {
			return p
		}
	}
	return ""
}

// runTmactCLI runs this tmact binary with argv (no shell) from the user's
// home directory, capturing bounded output.
func runTmactCLI(ctx context.Context, argv []string) (mcp.CLIResult, error) {
	exe, err := os.Executable()
	if err != nil {
		return mcp.CLIResult{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return mcp.CLIResult{}, err
	}
	cmd := exec.CommandContext(ctx, exe, argv...)
	cmd.Dir = home
	cmd.Env = os.Environ()
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	res := mcp.CLIResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if ctx.Err() != nil {
		res.TimedOut, res.ExitCode = true, -1
		return res, nil
	}
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		res.ExitCode = exitErr.ExitCode()
	case err != nil:
		return res, err
	}
	return res, nil
}
