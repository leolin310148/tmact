package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"
)

// statusdPprofEnv enables the profiling endpoint without editing the service
// definition: `launchctl setenv` (or a systemd drop-in) plus a restart.
const statusdPprofEnv = "TMACT_STATUSD_PPROF_ADDR"

// resolvePprofAddr picks the --pprof-addr flag, falling back to the env var,
// and refuses anything but a loopback host: profiles expose heap contents
// (pane text included), so unlike --web-addr this must never face the LAN.
func resolvePprofAddr(flagValue string, flagSet bool) (string, error) {
	addr := flagValue
	if !flagSet {
		addr = os.Getenv(statusdPprofEnv)
	}
	if addr == "" {
		return "", nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("pprof addr %q: %w", addr, err)
	}
	if host == "localhost" {
		return addr, nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return addr, nil
	}
	return "", fmt.Errorf("pprof addr %q must use a loopback host (127.0.0.1, ::1, localhost)", addr)
}

// startPprofServer serves net/http/pprof on its own mux until ctx ends. It is
// diagnostic only, so a failure is logged and never stops the daemon.
func startPprofServer(ctx context.Context, addr string, logf func(string, ...any)) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logf("pprof disabled: %v", err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	logf("pprof listening on %s", ln.Addr())
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logf("pprof stopped: %v", err)
		}
	}()
}
