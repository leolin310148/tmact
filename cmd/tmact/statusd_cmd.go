package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/stt"
	"github.com/leolin310148/tmact/internal/tmux"
	"github.com/leolin310148/tmact/internal/web"
)

func runSTTSet(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("stt-set")
	}
	fs := flag.NewFlagSet("stt-set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	provider := fs.String("provider", stt.DefaultProvider, "speech-to-text provider")
	apiKey := fs.String("api-key", "", "provider API key")
	model := fs.String("model", stt.DefaultModel, "speech-to-text model")
	endpoint := fs.String("endpoint", stt.DefaultEndpoint, "transcription API endpoint")
	configPath := fs.String("config", "", "provider config path")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	cfg := stt.ProviderConfig{
		Provider: *provider,
		APIKey:   *apiKey,
		Model:    *model,
		Endpoint: *endpoint,
	}
	if err := cfg.NormalizeAndValidate(); err != nil {
		return err
	}
	if err := stt.SaveProvider(*configPath, cfg); err != nil {
		return err
	}
	path := *configPath
	if path == "" {
		var err error
		path, err = stt.DefaultProviderPath()
		if err != nil {
			return err
		}
	}
	if *jsonOutput {
		return printJSON(map[string]string{
			"path":     path,
			"provider": cfg.Provider,
			"model":    cfg.Model,
			"endpoint": cfg.Endpoint,
		})
	}
	fmt.Fprintf(os.Stdout, "wrote STT provider config to %s (provider %s, model %s)\n", path, cfg.Provider, cfg.Model)
	return nil
}

func runStatusd(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("statusd")
	}
	if len(args) == 0 {
		return errors.New("statusd requires a subcommand: start, once, read, status, stop")
	}
	switch args[0] {
	case "start":
		return runStatusdStart(args[1:])
	case "once":
		return runStatusdOnce(args[1:])
	case "read":
		return runStatusdRead(args[1:])
	case "status":
		return runStatusdStatus(args[1:])
	case "stop":
		return errors.New("statusd stop is not available without background mode; stop the foreground process instead")
	case "help", "-h", "--help":
		return printCommandHelp("statusd")
	default:
		return fmt.Errorf("unknown statusd subcommand %q", args[0])
	}
}

func runStatusdStart(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("statusd start")
	}
	fs := flag.NewFlagSet("statusd start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	flags := statusdFlags(fs)
	once := fs.Bool("once", false, "run one scan then exit")
	webAddr := fs.String("web-addr", "", "serve the read-only web UI on this address (e.g. 0.0.0.0:7890)")
	agentUsage := fs.Bool("agent-usage", true, "serve the web agent quota/rate-limit usage panel (reads agent OAuth creds read-only)")
	agentCost := fs.Bool("agent-cost", true, "compute token-spend (cost) in the usage panel; disable on machines that should not compute/contribute cost")
	configPath := fs.String("config", statusd.DefaultFileConfigPath(), "statusd config file (JSON); auto-created with defaults if missing")

	if err := fs.Parse(args); err != nil {
		return err
	}

	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	var fileCfg statusd.FileConfig
	if !*once && *configPath != "" {
		loaded, created, err := statusd.LoadOrCreateFileConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "statusd: ignoring config %s: %v\n", *configPath, err)
		} else {
			fileCfg = loaded
			if created {
				fmt.Fprintf(os.Stderr, "statusd: seeded default config at %s\n", *configPath)
			}
		}
	}

	cfg := flags.config()
	applyFileConfig(&cfg, webAddr, fileCfg, set)
	// Agent-usage panel: CLI flag wins; otherwise honor the file value.
	usageEnabled := *agentUsage
	if !set["agent-usage"] && fileCfg.AgentUsage != nil {
		usageEnabled = *fileCfg.AgentUsage
	}
	// Token-spend (cost): same precedence, independent toggle.
	spendEnabled := *agentCost
	if !set["agent-cost"] && fileCfg.AgentCost != nil {
		spendEnabled = *fileCfg.AgentCost
	}
	if err := validateStatusdConfig(cfg); err != nil {
		return err
	}
	cfg.Logf = func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "statusd: "+format+"\n", args...)
	}

	daemon := statusd.NewDaemon(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if *once {
		if *webAddr != "" {
			return errors.New("--web-addr cannot be combined with --once")
		}
		snapshot, err := daemon.RunOnce(ctx)
		if *flags.JSON {
			if printErr := printJSON(snapshot); printErr != nil && err == nil {
				err = printErr
			}
		}
		return err
	}
	if *flags.JSON {
		return errors.New("--json is only valid with --once for statusd start")
	}

	// Always serve the unix socket so CLI read/status can reach the daemon.
	// The TCP --web-addr is additional and optional.
	server := &web.Server{
		Addr:                     *webAddr,
		SocketPath:               cfg.SocketPath,
		Store:                    daemon.Store(),
		CapturePane:              tmux.CapturePaneANSI,
		BuildTime:                buildVersionInfo().Time,
		Peers:                    cfg.Peers,
		CostPeers:                cfg.CostPeers,
		UsageEnabled:             usageEnabled,
		SpendEnabled:             spendEnabled,
		UsageInterval:            cfg.UsageInterval,
		SpendInterval:            cfg.SpendInterval,
		WebPushVAPIDPublicKey:    fileCfg.WebPushVAPIDPublicKey,
		WebPushVAPIDPrivateKey:   fileCfg.WebPushVAPIDPrivateKey,
		WebPushVAPIDSubject:      fileCfg.WebPushVAPIDSubject,
		WebPushSubscriptionsPath: fileCfg.WebPushSubscriptionsPath,
	}
	go func() {
		if err := server.Serve(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "statusd server stopped: %v\n", err)
		}
	}()
	fmt.Fprintf(os.Stderr, "statusd IPC listening on %s\n", cfg.SocketPath)
	if *webAddr != "" {
		fmt.Fprintf(os.Stderr, "statusd web UI listening on %s\n", *webAddr)
	}

	err := daemon.Start(ctx)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// applyFileConfig overlays values from the on-disk config onto cfg for any
// flag the user did not pass explicitly. Precedence: CLI flag > file > default.
func applyFileConfig(cfg *statusd.Config, webAddr *string, file statusd.FileConfig, set map[string]bool) {
	if !set["web-addr"] && file.WebAddr != "" {
		*webAddr = file.WebAddr
	}
	if !set["interval"] {
		if d := file.IntervalDuration(); d > 0 {
			cfg.Interval = d
		}
	}
	if !set["socket-path"] && file.SocketPath != "" {
		cfg.SocketPath = file.SocketPath
	}
	if !set["log-path"] && file.LogPath != "" {
		cfg.LogPath = file.LogPath
	}
	if !set["tmux-options"] && !set["no-tmux-options"] && file.TmuxOptions != nil {
		cfg.TmuxOptions = *file.TmuxOptions
	}
	if !set["pane-cols"] && file.PaneCols != nil {
		cfg.PaneCols = *file.PaneCols
	}
	if !set["pane-rows"] && file.PaneRows != nil {
		cfg.PaneRows = *file.PaneRows
	}
	if len(cfg.Peers) == 0 && len(file.Peers) > 0 {
		for _, p := range file.Peers {
			if p.Name == "" || p.URL == "" {
				continue
			}
			cfg.Peers = append(cfg.Peers, statusd.Peer{Name: p.Name, URL: p.URL})
		}
	}
	if len(cfg.CostPeers) == 0 && len(file.CostPeers) > 0 {
		for _, p := range file.CostPeers {
			if p.Name == "" || p.URL == "" {
				continue
			}
			cfg.CostPeers = append(cfg.CostPeers, statusd.Peer{Name: p.Name, URL: p.URL})
		}
	}
	if cfg.PeerInterval == 0 {
		if d := file.PeerIntervalDuration(); d > 0 {
			cfg.PeerInterval = d
		}
	}
	if cfg.PeerTimeout == 0 {
		if d := file.PeerTimeoutDuration(); d > 0 {
			cfg.PeerTimeout = d
		}
	}
	if cfg.UsageInterval == 0 {
		if d := file.UsageIntervalDuration(); d > 0 {
			cfg.UsageInterval = d
		}
	}
	if cfg.SpendInterval == 0 {
		if d := file.SpendIntervalDuration(); d > 0 {
			cfg.SpendInterval = d
		}
	}
}

func runStatusdOnce(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("statusd once")
	}
	fs := flag.NewFlagSet("statusd once", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	flags := statusdFlags(fs)

	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := flags.config()
	if err := validateStatusdConfig(cfg); err != nil {
		return err
	}

	snapshot, err := statusd.NewDaemon(cfg).RunOnce(context.Background())
	if *flags.JSON {
		if printErr := printJSON(snapshot); printErr != nil && err == nil {
			err = printErr
		}
	} else {
		printStatusdSnapshot(snapshot, tmactNow())
	}
	return err
}

func runStatusdRead(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("statusd read")
	}
	fs := flag.NewFlagSet("statusd read", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	socketPath := fs.String("socket-path", statusd.DefaultSocketPath, "daemon IPC unix socket")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	snapshot, err := fetchOrLiveSnapshot(*socketPath)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(snapshot)
	}
	printStatusdSnapshot(snapshot, tmactNow())
	return nil
}

func runStatusdStatus(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("statusd status")
	}
	fs := flag.NewFlagSet("statusd status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	socketPath := fs.String("socket-path", statusd.DefaultSocketPath, "daemon IPC unix socket")
	jsonOutput := fs.Bool("json", false, "print JSON output")

	if err := fs.Parse(args); err != nil {
		return err
	}
	snapshot, err := fetchOrLiveSnapshot(*socketPath)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(snapshot)
	}
	now := tmactNow()
	fmt.Printf("socket_path: %s\n", *socketPath)
	fmt.Printf("last_update: %s\n", snapshot.Timestamp.Format(time.RFC3339))
	fmt.Printf("age: %s\n", formatAge(now.Sub(snapshot.Timestamp)))
	fmt.Printf("stale: %t\n", snapshot.IsStale(now))
	fmt.Printf("panes: %d\n", snapshot.Summary.Panes)
	fmt.Printf("sessions: %d\n", snapshot.Summary.Sessions)
	fmt.Printf("errors: %d\n", snapshot.Summary.Errors)
	return nil
}

// fetchOrLiveSnapshot tries the daemon over the unix socket first; if the
// daemon is down, it falls back to a one-shot live capture. The fallback is
// noticeably slower (multi-sample capture for valid classification) so we log
// to stderr before doing it.
func fetchOrLiveSnapshot(socketPath string) (statusd.Snapshot, error) {
	snap, err := statusd.FetchSnapshot(socketPath)
	if err == nil {
		return snap, nil
	}
	if !errors.Is(err, statusd.ErrDaemonUnavailable) {
		return statusd.Snapshot{}, err
	}
	fmt.Fprintln(os.Stderr, "statusd daemon not running; capturing live (this may take ~1s)")
	cfg := statusd.Config{TmuxOptions: false}
	return statusd.NewDaemon(cfg).RunOnce(context.Background())
}

type statusdFlagValues struct {
	Config         statusd.Config
	JSON           *bool
	NoTmuxOptions  *bool
	IdleIgnore     repeatedStrings
	IncludeSession repeatedStrings
	ExcludeSession repeatedStrings
}

func statusdFlags(fs *flag.FlagSet) *statusdFlagValues {
	values := &statusdFlagValues{Config: statusd.Config{TmuxOptions: true}}
	fs.DurationVar(&values.Config.Interval, "interval", statusd.DefaultInterval, "scan interval")
	fs.StringVar(&values.Config.SocketPath, "socket-path", statusd.DefaultSocketPath, "daemon IPC unix socket")
	fs.StringVar(&values.Config.LogPath, "log-path", "", "optional JSONL daemon log path")
	fs.BoolVar(&values.Config.TmuxOptions, "tmux-options", true, "write @ai-* tmux options")
	values.NoTmuxOptions = fs.Bool("no-tmux-options", false, "skip writing @ai-* tmux options")
	fs.IntVar(&values.Config.CaptureLines, "capture-lines", statusd.DefaultCaptureLines, "number of pane history lines to capture")
	fs.IntVar(&values.Config.InitialSamples, "initial-samples", statusd.DefaultInitialSamples, "captures per pane before statusd has history")
	fs.DurationVar(&values.Config.RunningDebounce, "running-debounce", statusd.DefaultRunningDebounce, "keep running indicator after changes")
	fs.DurationVar(&values.Config.StaleAfter, "stale-after", statusd.DefaultStaleAfter, "mark snapshot stale after this age")
	fs.IntVar(&values.Config.PaneCols, "pane-cols", statusd.DefaultPaneCols, "fixed tmux window width (0 disables sweep)")
	fs.IntVar(&values.Config.PaneRows, "pane-rows", statusd.DefaultPaneRows, "fixed tmux window height (0 disables sweep)")
	fs.Var(&values.IdleIgnore, "idle-ignore", "regexp for lines ignored by sample hashing; may be repeated")
	fs.Var(&values.IncludeSession, "session", "include sessions matching glob; may be repeated")
	fs.Var(&values.ExcludeSession, "exclude-session", "exclude sessions matching glob; may be repeated")
	values.JSON = fs.Bool("json", false, "print JSON output")
	return values
}

func (v *statusdFlagValues) config() statusd.Config {
	cfg := v.Config
	if *v.NoTmuxOptions {
		cfg.TmuxOptions = false
	}
	cfg.IdleIgnorePatterns = v.IdleIgnore
	cfg.IncludeSessions = v.IncludeSession
	cfg.ExcludeSessions = v.ExcludeSession
	return cfg
}

func validateStatusdConfig(cfg statusd.Config) error {
	if cfg.Interval <= 0 {
		return errors.New("--interval must be positive")
	}
	if cfg.CaptureLines <= 0 {
		return errors.New("--capture-lines must be positive")
	}
	if cfg.InitialSamples <= 0 {
		return errors.New("--initial-samples must be positive")
	}
	if cfg.RunningDebounce <= 0 {
		return errors.New("--running-debounce must be positive")
	}
	if cfg.StaleAfter <= 0 {
		return errors.New("--stale-after must be positive")
	}
	if err := validatePeerNames(cfg.Peers, cfg.CostPeers); err != nil {
		return err
	}
	return nil
}

func validatePeerNames(peers, costPeers []statusd.Peer) error {
	seen := map[string]string{}
	for _, p := range peers {
		if p.Name == "" {
			return errors.New("peer name cannot be empty")
		}
		if p.URL == "" {
			return fmt.Errorf("peer %q URL cannot be empty", p.Name)
		}
		if prev, ok := seen[p.Name]; ok && prev != p.URL {
			return fmt.Errorf("peer %q has conflicting URLs %q and %q", p.Name, prev, p.URL)
		}
		seen[p.Name] = p.URL
	}
	for _, p := range costPeers {
		if p.Name == "" {
			return errors.New("cost peer name cannot be empty")
		}
		if p.URL == "" {
			return fmt.Errorf("cost peer %q URL cannot be empty", p.Name)
		}
		if prev, ok := seen[p.Name]; ok && prev != p.URL {
			return fmt.Errorf("peer %q has conflicting pane/cost URLs %q and %q", p.Name, prev, p.URL)
		}
		seen[p.Name] = p.URL
	}
	return nil
}

func statusdUsage() error {
	fmt.Fprint(os.Stderr, `Usage:
  tmact statusd start [--interval 1s] [--socket-path /tmp/tmact-statusd.sock] [--no-tmux-options]
  tmact statusd once [--json] [--initial-samples 2]
  tmact statusd read [--json] [--socket-path /tmp/tmact-statusd.sock]
  tmact statusd status [--socket-path /tmp/tmact-statusd.sock]
`)
	return nil
}
