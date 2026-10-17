package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/leolin310148/tmact/internal/mcp"
	"github.com/leolin310148/tmact/internal/peerpane"
	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/tmux"
	"github.com/leolin310148/tmact/internal/web"
)

const defaultMCPAddr = "127.0.0.1:7895"

func runMCP(args []string) error {
	if wantsHelp(args) || len(args) == 0 {
		return printCommandHelp("mcp")
	}
	switch args[0] {
	case "serve":
		return runMCPServe(args[1:])
	case "help", "-h", "--help":
		return printCommandHelp("mcp")
	default:
		return fmt.Errorf("unknown mcp subcommand %q", args[0])
	}
}

func runMCPServe(args []string) error {
	if wantsHelp(args) {
		return printCommandHelp("mcp serve")
	}
	fs := flag.NewFlagSet("mcp serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	addr := fs.String("addr", defaultMCPAddr, "listen address; cloudflared connects here")
	team := fs.String("access-team", "", "Cloudflare Zero Trust team (<team>.cloudflareaccess.com)")
	aud := fs.String("access-aud", "", "Cloudflare Access application AUD tag")
	noAccess := fs.Bool("no-access", false, "skip Access JWT verification (loopback-only local testing)")
	socketPath := fs.String("socket-path", statusd.DefaultSocketPath, "statusd IPC unix socket")
	configPath := fs.String("config", statusd.DefaultFileConfigPath(), "statusd config with peers")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	opts := mcp.Options{Version: version}
	switch {
	case *noAccess:
		if !isLoopbackAddr(*addr) {
			return fmt.Errorf("--no-access requires a loopback --addr, got %q", *addr)
		}
	case *team == "" || *aud == "":
		return errors.New("mcp serve needs --access-team and --access-aud (or --no-access for loopback testing)")
	default:
		opts.Verifier = mcp.NewAccessVerifier(*team, *aud)
	}

	handler := mcp.Handler(mcpDeps(*socketPath, *configPath), opts)
	srv := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("mcp: listening on %s (access=%v)", *addr, opts.Verifier != nil)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func mcpDeps(socketPath, configPath string) mcp.Deps {
	return mcp.Deps{
		Snapshot: func(context.Context) (statusd.Snapshot, error) {
			return statusd.FetchSnapshot(socketPath)
		},
		CaptureLocal: func(ctx context.Context, paneID string, lines int) (string, error) {
			return tmux.CapturePaneContext(ctx, paneID, lines)
		},
		CapturePeer: func(ctx context.Context, peer, paneID string, lines int) (string, error) {
			cfg, err := statusd.LoadFileConfig(configPath)
			if err != nil {
				return "", err
			}
			p, ok := peerpane.LookupConfigPeer(cfg, peer)
			if !ok {
				return "", fmt.Errorf("peer %q is not configured in %s", peer, configPath)
			}
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			capture, err := web.FetchPeerCapture(ctx, nil, p.Name, p.URL, paneID, lines, web.CaptureModePlain)
			if err != nil {
				return "", err
			}
			return capture.Captures[web.CaptureModePlain], nil
		},
	}
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
