package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/dispatch"
	"github.com/leolin310148/tmact/internal/statusd"
)

var (
	dispatchRun       = dispatch.Run
	dispatchRemoteRun = dispatch.PostRemote
)

func runDispatch(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("dispatch-work")
	}

	var session string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		session = args[0]
		args = args[1:]
	}

	fs := flag.NewFlagSet("dispatch-work", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dir := fs.String("dir", "", "working directory; sets cwd when the session is created")
	agent := fs.String("agent", "", "agent to launch: "+strings.Join(dispatch.SupportedAgents(), "|"))
	model := fs.String("model", "", dispatchModelHelp())
	promptText := fs.String("prompt", "", "prompt text to send to the agent")
	readyTimeout := fs.Duration("ready-timeout", 30*time.Second, "max wait for the agent to become ready")
	readySettle := fs.Duration("ready-settle", dispatch.DefaultReadySettleDelay, "stable idle time after ready before sending the prompt")
	trustFolder := fs.Bool("trust-folder", false, "accept a Claude/Codex trust prompt only when pane cwd exactly matches --dir")
	execute := fs.Bool("execute", false, "actually create, launch, and send; default is dry-run")
	peerName := fs.String("peer", "", "dispatch on the named statusd dispatch_peer from config")
	configPath := fs.String("config", statusd.DefaultFileConfigPath(), "statusd config file containing dispatch_peers")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if session == "" && fs.NArg() > 0 {
		session = fs.Arg(0)
	}
	if session == "" {
		return errors.New("dispatch-work requires a session name as the first argument")
	}
	if *dir == "" {
		return errors.New("dispatch-work requires --dir")
	}
	if *agent == "" {
		return errors.New("dispatch-work requires --agent")
	}
	if *promptText == "" {
		return errors.New("dispatch-work requires --prompt")
	}

	opts := dispatch.Options{
		Session:      session,
		Dir:          *dir,
		Agent:        *agent,
		Model:        *model,
		Prompt:       *promptText,
		Execute:      *execute,
		ReadyTimeout: *readyTimeout,
		ReadySettle:  *readySettle,
		TrustFolder:  *trustFolder,
	}
	var report dispatch.Report
	var err error
	if *peerName != "" {
		report, err = runRemoteDispatch(*peerName, *configPath, opts)
	} else {
		report, err = dispatchRun(opts)
	}
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(report)
	}
	printDispatchReport(report)
	return nil
}

func dispatchModelHelp() string {
	return fmt.Sprintf("model to use (claude: %s; codex: %s)",
		strings.Join(dispatch.SupportedModels("claude"), "|"),
		strings.Join(dispatch.SupportedModels("codex"), "|"))
}

func runRemoteDispatch(peerName, configPath string, opts dispatch.Options) (dispatch.Report, error) {
	if configPath == "" {
		return dispatch.Report{}, errors.New("dispatch-work --peer requires --config or a default statusd config path")
	}
	cfg, err := statusd.LoadFileConfig(configPath)
	if err != nil {
		return dispatch.Report{}, fmt.Errorf("load statusd config %s: %w", configPath, err)
	}
	if p, ok := findPeerConfig(cfg.DispatchPeers, peerName); ok {
		if p.URL == "" {
			return dispatch.Report{}, fmt.Errorf("peer %q has empty url in %s", peerName, configPath)
		}
		return dispatchRemoteRun(context.Background(), &http.Client{}, p.Name, p.URL, opts)
	}
	if p, ok := findPeerConfig(cfg.Peers, peerName); ok {
		if p.URL == "" {
			return dispatch.Report{}, fmt.Errorf("peer %q has empty url in %s", peerName, configPath)
		}
		return dispatchRemoteRun(context.Background(), &http.Client{}, p.Name, p.URL, opts)
	}
	return dispatch.Report{}, fmt.Errorf("peer %q not found in dispatch_peers or peers in %s", peerName, configPath)
}

func findPeerConfig(peers []statusd.PeerFileConfig, name string) (statusd.PeerFileConfig, bool) {
	for _, p := range peers {
		if p.Name == name {
			return p, true
		}
	}
	return statusd.PeerFileConfig{}, false
}
