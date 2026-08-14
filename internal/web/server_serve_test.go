package web

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/agentusage"
)

func TestServeKeepsUnixIPCWhileTCPBindRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	socketDir, err := os.MkdirTemp("/tmp", "tmact-web-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	socketPath := filepath.Join(socketDir, "statusd.sock")
	ready := make(chan string, 4)
	var tcpAttempts atomic.Int32
	var usageRefreshes atomic.Int32
	server := &Server{
		Addr:          "100.64.0.1:7890",
		SocketPath:    socketPath,
		UsageEnabled:  true,
		UsageInterval: time.Hour,
		FetchUsage: func(context.Context) agentusage.Snapshot {
			usageRefreshes.Add(1)
			return agentusage.Snapshot{}
		},
		Listen: func(network, address string) (net.Listener, error) {
			if network == "unix" {
				return net.Listen(network, address)
			}
			if tcpAttempts.Add(1) < 3 {
				return nil, &net.OpError{Op: "listen", Net: "tcp", Err: syscall.EADDRNOTAVAIL}
			}
			return net.Listen("tcp", "127.0.0.1:0")
		},
		ListenRetryDelay: func(int) time.Duration { return time.Millisecond },
		OnListenerReady:  func(network, _ string) { ready <- network },
		Logf:             func(string, ...any) {},
	}

	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()
	waitForReady(t, ready, "unix")
	if _, err := os.Stat(socketPath); err != nil {
		t.Fatalf("unix socket not available during TCP retry: %v", err)
	}
	waitForReady(t, ready, "tcp")
	if got := tcpAttempts.Load(); got != 3 {
		t.Fatalf("TCP attempts = %d, want 3", got)
	}
	waitForAtomicAtLeast(t, &usageRefreshes, 1)
	if got := usageRefreshes.Load(); got != 1 {
		t.Fatalf("usage refreshes = %d, want one worker across retries", got)
	}

	cancel()
	if err := waitForServe(t, done); err != nil {
		t.Fatalf("Serve returned %v after cancellation", err)
	}
	if _, err := os.Stat(socketPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unix socket still exists after shutdown: %v", err)
	}
}

func TestServeCancellationInterruptsTCPRetryDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempted := make(chan struct{}, 1)
	server := &Server{
		Addr: "100.64.0.1:7890",
		Listen: func(string, string) (net.Listener, error) {
			attempted <- struct{}{}
			return nil, syscall.EADDRNOTAVAIL
		},
		ListenRetryDelay: func(int) time.Duration { return time.Hour },
		Logf:             func(string, ...any) {},
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()

	select {
	case <-attempted:
	case <-time.After(time.Second):
		t.Fatal("TCP listen was not attempted")
	}
	cancel()
	if err := waitForServe(t, done); err != nil {
		t.Fatalf("Serve returned %v after cancellation", err)
	}
}

func TestServeReturnsPermanentTCPBindError(t *testing.T) {
	server := &Server{
		Addr: "bad-address",
		Listen: func(string, string) (net.Listener, error) {
			return nil, os.ErrPermission
		},
		Logf: func(string, ...any) {},
	}
	err := server.Serve(context.Background())
	if err == nil || !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), "listen tcp") {
		t.Fatalf("Serve error = %v, want permanent TCP bind error", err)
	}
}

func TestServeRebindsAfterTCPListenerStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 3)
	var attempts atomic.Int32
	server := &Server{
		Addr: "127.0.0.1:0",
		Listen: func(string, string) (net.Listener, error) {
			if attempts.Add(1) == 1 {
				return failingListener{}, nil
			}
			return net.Listen("tcp", "127.0.0.1:0")
		},
		ListenRetryDelay: func(int) time.Duration { return time.Millisecond },
		OnListenerReady:  func(network, _ string) { ready <- network },
		Logf:             func(string, ...any) {},
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()

	waitForReady(t, ready, "tcp")
	waitForReady(t, ready, "tcp")
	if got := attempts.Load(); got != 2 {
		t.Fatalf("TCP attempts = %d, want listener recovery on second attempt", got)
	}
	cancel()
	if err := waitForServe(t, done); err != nil {
		t.Fatalf("Serve returned %v after cancellation", err)
	}
}

func TestListenRetryDelayCapsAtThirtySeconds(t *testing.T) {
	server := &Server{}
	wants := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	for attempt, want := range wants {
		if got := server.listenRetryDelay(attempt); got != want {
			t.Fatalf("attempt %d delay = %s, want %s", attempt, got, want)
		}
	}
}

type failingListener struct{}

func (failingListener) Accept() (net.Conn, error) { return nil, errors.New("listener failed") }
func (failingListener) Close() error              { return nil }
func (failingListener) Addr() net.Addr            { return testAddr("tcp") }

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return string(a) }

func waitForReady(t *testing.T, ready <-chan string, want string) {
	t.Helper()
	select {
	case got := <-ready:
		if got != want {
			t.Fatalf("listener ready = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s listener", want)
	}
}

func waitForAtomicAtLeast(t *testing.T, value *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for value.Load() < want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if value.Load() < want {
		t.Fatalf("value = %d, want at least %d", value.Load(), want)
	}
}

func waitForServe(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("Serve did not stop")
		return nil
	}
}
