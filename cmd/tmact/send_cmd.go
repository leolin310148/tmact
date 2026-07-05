package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/leolin310148/tmact/internal/peerpane"
	"github.com/leolin310148/tmact/internal/statusd"
)

var sendPeerPaneInput = func(ctx context.Context, peer statusd.Peer, target string, report sendReport) error {
	client := peerpane.Client{Peer: peer, HTTPClient: &http.Client{}}
	if report.ClearLine {
		if err := client.SendKeys(ctx, target, []string{"C-u"}); err != nil {
			return err
		}
	}
	if report.Mode == "keys" {
		return client.SendKeys(ctx, target, report.Keys)
	}
	return client.SendText(ctx, target, report.Text, report.Enter)
}

type sendReport struct {
	Selector     string   `json:"selector"`
	Target       string   `json:"target"`
	Peer         string   `json:"peer,omitempty"`
	RemoteTarget string   `json:"remote_target,omitempty"`
	Mode         string   `json:"mode"`
	Text         string   `json:"text,omitempty"`
	Keys         []string `json:"keys,omitempty"`
	Enter        bool     `json:"enter,omitempty"`
	ClearLine    bool     `json:"clear_line,omitempty"`
	Execute      bool     `json:"execute"`
}

func runSend(args []string, globals globalOptions) error {
	if wantsHelp(args) {
		return printCommandHelp("send")
	}
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	text := fs.String("text", "", "text to send")
	command := fs.String("command", "", "command to send followed by Enter")
	var keyFlags repeatedStrings
	fs.Var(&keyFlags, "key", "tmux key to send; may be repeated")
	keysCSV := fs.String("keys", "", "comma-separated tmux keys to send")
	enter := fs.Bool("enter", false, "press Enter after sending text")
	clearLine := fs.Bool("clear-line", false, "send C-u before text or command")
	execute := fs.Bool("execute", false, "actually send to tmux; default is dry-run")
	peerName := fs.String("peer", "", "send through the named statusd peer from config")
	configPath := fs.String("config", statusd.DefaultFileConfigPath(), "statusd config file containing peers")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if globals.Target == "" {
		return errors.New("global -t/--target is required for send")
	}

	keys, err := collectKeys(keyFlags, *keysCSV)
	if err != nil {
		return err
	}
	modeCount := 0
	mode := ""
	if *text != "" {
		modeCount++
		mode = "text"
	}
	if *command != "" {
		modeCount++
		mode = "command"
	}
	if len(keys) > 0 {
		modeCount++
		mode = "keys"
	}
	if modeCount != 1 {
		return errors.New("send requires exactly one of --text, --command, --key, or --keys")
	}
	if mode == "keys" && (*enter || *clearLine) {
		return errors.New("--enter and --clear-line are only valid with --text or --command")
	}

	target, err := resolveTarget(globals.Target)
	if err != nil {
		return err
	}
	peer, remoteTarget, err := sendPeerTarget(target, *peerName)
	if err != nil {
		return err
	}
	displayTarget := target
	if peer != "" {
		displayTarget = peer + "@" + remoteTarget
	}

	report := sendReport{
		Selector:  globals.Target,
		Target:    displayTarget,
		Peer:      peer,
		Mode:      mode,
		Keys:      keys,
		Enter:     *enter || mode == "command",
		ClearLine: *clearLine,
		Execute:   *execute,
	}
	if peer != "" {
		report.RemoteTarget = remoteTarget
	}
	switch mode {
	case "text":
		report.Text = *text
	case "command":
		report.Text = *command
	}

	if *execute {
		if err := executeSend(report, *configPath); err != nil {
			return err
		}
	}
	if *jsonOutput {
		return printJSON(report)
	}
	printSendReport(report)
	return nil
}

func collectKeys(keyFlags []string, keysCSV string) ([]string, error) {
	var keys []string
	for _, key := range keyFlags {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errors.New("key cannot be empty")
		}
		keys = append(keys, key)
	}
	if keysCSV == "" {
		return keys, nil
	}
	for _, part := range strings.Split(keysCSV, ",") {
		key := strings.TrimSpace(part)
		if key == "" {
			return nil, fmt.Errorf("invalid empty key in %q", keysCSV)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func sendPeerTarget(target, explicitPeer string) (peer string, remoteTarget string, err error) {
	embeddedPeer, rest := statusd.SplitPeerTarget(target)
	if explicitPeer != "" {
		if embeddedPeer != "" && embeddedPeer != explicitPeer {
			return "", "", fmt.Errorf("target peer %q conflicts with --peer %q", embeddedPeer, explicitPeer)
		}
		peer = explicitPeer
		remoteTarget = rest
	} else {
		peer = embeddedPeer
		remoteTarget = rest
	}
	if peer == "" {
		return "", "", nil
	}
	if remoteTarget == "" || !strings.HasPrefix(remoteTarget, "%") {
		return "", "", fmt.Errorf("peer target must be a tmux pane id like peer@%%12, got %q", target)
	}
	return peer, remoteTarget, nil
}

func executeSend(report sendReport, configPath string) error {
	if report.Peer != "" {
		peer, err := peerpane.LoadConfigPeer(configPath, report.Peer)
		if err != nil {
			return err
		}
		return sendPeerPaneInput(context.Background(), peer, report.RemoteTarget, report)
	}
	if report.ClearLine {
		if err := sendTmuxKeys(report.Target, []string{"C-u"}); err != nil {
			return err
		}
	}
	if report.Mode == "keys" {
		return sendTmuxKeys(report.Target, report.Keys)
	}
	return pasteTmuxText(report.Target, report.Text, report.Enter)
}
