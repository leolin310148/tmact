package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// The CLI bridge exposes allowlisted tmact subcommands as MCP tools. Specs are
// built from the CLI help catalog in cmd/tmact; this file only turns a spec
// into a schema, validates arguments, and runs the binary without a shell.

const (
	bridgeCallTimeout = 80 * time.Second
	bridgeOutputLimit = 64 << 10
)

// Flag kinds.
const (
	FlagBool   = "bool"
	FlagString = "string"
	FlagList   = "list" // repeatable string flag
)

// Path rules for string flags.
const (
	PathNone = ""
	// PathUnderRoots requires a "~/..." path under the dispatch roots. On the
	// MCP host it is resolved through symlinks; with --peer it is passed in
	// "~/" form for the peer to expand.
	PathUnderRoots = "under_roots"
)

// BridgeFlag is one CLI flag exposed as a tool parameter.
type BridgeFlag struct {
	Key         string // JSON parameter name, e.g. "ready_timeout"
	Flag        string // CLI flag, e.g. "--ready-timeout"
	Kind        string
	Path        string
	Description string
	Required    bool
}

// BridgeCommand is one allowlisted tmact subcommand.
type BridgeCommand struct {
	Words   []string // e.g. ["loop", "list"]
	Summary string
	Safety  []string
	Notes   string // extra guidance appended to the description
	Write   bool
	// GlobalTarget adds a "target" parameter emitted as "-t X" before the
	// subcommand words (the send command reads the global flag only).
	GlobalTarget bool
	AutoJSON     bool     // always pass --json and parse stdout
	FixedArgs    []string // always appended, e.g. workflow stop's --wait
	Positional   *BridgeArg
	Flags        []BridgeFlag
}

// BridgeArg is a single positional argument.
type BridgeArg struct {
	Key         string
	Description string
	Required    bool
	Session     bool // validate as a tmux session name
}

// CLIResult is the outcome of one tmact invocation.
type CLIResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

// ToolName returns the MCP tool name for a command.
func (c BridgeCommand) ToolName() string {
	return "tmact_" + strings.NewReplacer("-", "_", " ", "_").Replace(strings.Join(c.Words, "_"))
}

func (c BridgeCommand) schema() map[string]any {
	props := map[string]any{}
	var required []string
	if c.GlobalTarget {
		props["target"] = map[string]any{"type": "string", "description": "pane target: tmux target, %id, index from tmact_ls, or peer@target"}
		required = append(required, "target")
	}
	if a := c.Positional; a != nil {
		props[a.Key] = map[string]any{"type": "string", "description": a.Description}
		if a.Required {
			required = append(required, a.Key)
		}
	}
	for _, f := range c.Flags {
		desc := f.Description
		if f.Path == PathUnderRoots {
			desc += " (must be a ~/ path under the allowed roots)"
		}
		switch f.Kind {
		case FlagBool:
			props[f.Key] = map[string]any{"type": "boolean", "description": desc}
		case FlagList:
			props[f.Key] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": desc}
		default:
			props[f.Key] = map[string]any{"type": []string{"string", "number"}, "description": desc}
		}
		if f.Required {
			required = append(required, f.Key)
		}
	}
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		sort.Strings(required)
		s["required"] = required
	}
	return s
}

func (c BridgeCommand) description() string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "CLI passthrough for `tmact %s`: %s", strings.Join(c.Words, " "), c.Summary)
	for _, s := range c.Safety {
		b.WriteString(" Safety: " + s)
	}
	if c.Notes != "" {
		b.WriteString(" " + c.Notes)
	}
	return b.String()
}

func addBridgeTools(s *sdk.Server, t tools, cmds []BridgeCommand, write bool) {
	for _, c := range cmds {
		if c.Write != write {
			continue
		}
		ann := &sdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPtr(false)}
		if c.Write {
			ann = writeAnnotations
		}
		c := c
		s.AddTool(&sdk.Tool{
			Name:        c.ToolName(),
			Description: c.description(),
			InputSchema: c.schema(),
			Annotations: ann,
		}, func(ctx context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return t.runBridge(ctx, c, req.Params.Arguments), nil
		})
	}
}

func (t tools) runBridge(ctx context.Context, c BridgeCommand, raw json.RawMessage) *sdk.CallToolResult {
	argv, err := t.bridgeArgv(c, raw)
	if err != nil {
		return toolError(err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, bridgeCallTimeout)
	defer cancel()
	res, err := t.deps.RunCLI(ctx, argv)
	if err != nil {
		return toolError(fmt.Sprintf("run tmact: %v", err))
	}
	out := map[string]any{
		"note":      untrustedNote,
		"command":   "tmact " + strings.Join(argv, " "),
		"exit_code": res.ExitCode,
	}
	if res.TimedOut {
		out["timed_out"] = true
	}
	var parsed any
	if c.AutoJSON && json.Unmarshal([]byte(res.Stdout), &parsed) == nil {
		out["result"] = parsed
	} else if s := strings.TrimSpace(res.Stdout); s != "" {
		out["observed_stdout"] = wrapObserved(truncate(s))
	}
	if s := strings.TrimSpace(res.Stderr); s != "" {
		out["observed_stderr"] = wrapObserved(truncate(s))
	}
	text, _ := json.Marshal(out)
	return &sdk.CallToolResult{
		Content:           []sdk.Content{&sdk.TextContent{Text: string(text)}},
		StructuredContent: out,
		IsError:           res.ExitCode != 0 || res.TimedOut,
	}
}

// bridgeArgv validates arguments against the spec and builds the argv.
func (t tools) bridgeArgv(c BridgeCommand, raw json.RawMessage) ([]string, error) {
	args := map[string]json.RawMessage{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("arguments must be an object: %v", err)
		}
	}
	known := map[string]bool{}
	if c.GlobalTarget {
		known["target"] = true
	}
	if c.Positional != nil {
		known[c.Positional.Key] = true
	}
	flags := map[string]BridgeFlag{}
	for _, f := range c.Flags {
		known[f.Key] = true
		flags[f.Key] = f
	}
	for k := range args {
		if !known[k] {
			return nil, fmt.Errorf("unknown parameter %q", k)
		}
	}

	var argv []string
	if c.GlobalTarget {
		target, err := stringArg(args, "target")
		if err != nil || target == "" {
			return nil, fmt.Errorf("target is required")
		}
		if strings.HasPrefix(target, "-") {
			return nil, fmt.Errorf("target must not start with '-'")
		}
		argv = append(argv, "-t", target)
	}
	argv = append(argv, c.Words...)

	if a := c.Positional; a != nil {
		v, err := stringArg(args, a.Key)
		if err != nil {
			return nil, err
		}
		switch {
		case v == "" && a.Required:
			return nil, fmt.Errorf("%s is required", a.Key)
		case strings.HasPrefix(v, "-"):
			return nil, fmt.Errorf("%s must not start with '-'", a.Key)
		case v != "" && a.Session && !sessionNamePattern.MatchString(v):
			return nil, fmt.Errorf("%s must match %s", a.Key, sessionNamePattern)
		}
		if v != "" {
			argv = append(argv, v)
		}
	}

	onPeer := false
	if _, ok := flags["peer"]; ok {
		p, err := stringArg(args, "peer")
		if err != nil {
			return nil, err
		}
		onPeer = p != ""
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		if _, ok := flags[k]; ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, f := range c.Flags {
		if _, ok := args[f.Key]; !ok && f.Required {
			return nil, fmt.Errorf("%s is required", f.Key)
		}
	}
	for _, k := range keys {
		f := flags[k]
		switch f.Kind {
		case FlagBool:
			var b bool
			if err := json.Unmarshal(args[k], &b); err != nil {
				return nil, fmt.Errorf("%s must be a boolean", k)
			}
			if b {
				argv = append(argv, f.Flag)
			}
		case FlagList:
			var vs []string
			if err := json.Unmarshal(args[k], &vs); err != nil {
				return nil, fmt.Errorf("%s must be an array of strings", k)
			}
			for _, v := range vs {
				argv = append(argv, f.Flag+"="+v)
			}
		default:
			v, err := stringArg(args, k)
			if err != nil {
				return nil, err
			}
			if f.Path == PathUnderRoots {
				if v, err = t.bridgePath(v, onPeer && f.Key == "dir"); err != nil {
					return nil, fmt.Errorf("%s: %w", k, err)
				}
			}
			argv = append(argv, f.Flag+"="+v)
		}
	}
	argv = append(argv, c.FixedArgs...)
	if c.AutoJSON {
		argv = append(argv, "--json")
	}
	return argv, nil
}

func (t tools) bridgePath(v string, peer bool) (string, error) {
	if len(t.roots) == 0 {
		return "", fmt.Errorf("no dispatch roots are configured, so path arguments are not allowed")
	}
	p, err := allowedDir(v, t.roots)
	if err != nil {
		return "", err
	}
	if peer {
		return p, nil
	}
	home, err := t.deps.Home()
	if err != nil {
		return "", err
	}
	return resolveLocalDir(p, t.roots, home)
}

// stringArg reads a string (or number) argument; missing yields "".
func stringArg(args map[string]json.RawMessage, key string) (string, error) {
	raw, ok := args[key]
	if !ok || string(raw) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s), nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String(), nil
	}
	return "", fmt.Errorf("%s must be a string", key)
}

func wrapObserved(s string) string {
	return captureOpen + "\n" + s + "\n" + captureClose
}

func truncate(s string) string {
	if len(s) <= bridgeOutputLimit {
		return s
	}
	return s[len(s)-bridgeOutputLimit:] + "\n[truncated to the last 64 KiB]"
}

func toolError(msg string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: msg}}, IsError: true}
}
