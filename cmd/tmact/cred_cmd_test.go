package main

import (
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/credreq"
	"github.com/leolin310148/tmact/internal/credstore"
	"github.com/leolin310148/tmact/internal/web"
)

// startCredDaemon serves the statusd cred routes on a unix socket and points
// the CLI's credential store at the same directory.
func startCredDaemon(t *testing.T) (*credreq.Registry, string) {
	t.Helper()
	store := credstore.Store{Root: filepath.Join(t.TempDir(), ".cred")}
	old := defaultCredStore
	defaultCredStore = func() (credstore.Store, error) { return store, nil }
	t.Cleanup(func() { defaultCredStore = old })

	dir, err := os.MkdirTemp("/tmp", "tmact-cred-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	registry := credreq.NewRegistry(store)
	srv := &http.Server{Handler: (&web.Server{CredRequests: registry}).Handler()}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return registry, socket
}

func awaitPending(t *testing.T, registry *credreq.Registry) credreq.Request {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if pending := registry.Pending(); len(pending) == 1 {
			return pending[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no pending request appeared")
	return credreq.Request{}
}

type credRunResult struct {
	out string
	err error
}

func startCredCommand(t *testing.T, args ...string) <-chan credRunResult {
	t.Helper()
	done := make(chan credRunResult, 1)
	go func() {
		out, err := captureRun(t, args...)
		done <- credRunResult{out, err}
	}()
	return done
}

func TestCredRequestPrintsOnlyThePath(t *testing.T) {
	registry, socket := startCredDaemon(t)
	done := startCredCommand(t, "cred", "request", "github-pat", "--reason", "push tag", "--socket-path", socket)
	req := awaitPending(t, registry)
	if req.Name != "github-pat" || req.Reason != "push tag" || req.Host == "" || req.CWD == "" {
		t.Fatalf("request = %+v", req)
	}
	if _, err := registry.Submit(req.ID, []byte("marker-secret-cli")); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.err != nil {
		t.Fatal(got.err)
	}
	path, _ := registry.Store.Path("github-pat")
	if got.out != path+"\n" || strings.Contains(got.out, "marker-secret-cli") {
		t.Fatalf("output = %q, want only %q", got.out, path)
	}

	// Stored now: a second request returns the path without asking.
	out, err := captureRun(t, "cred", "request", "github-pat", "--socket-path", socket)
	if err != nil || out != path+"\n" || len(registry.Pending()) != 0 {
		t.Fatalf("stored request = %q, %v, pending %v", out, err, registry.Pending())
	}

	// --refresh asks again and replaces the stored value.
	done = startCredCommand(t, "cred", "request", "github-pat", "--refresh", "--socket-path", socket)
	req = awaitPending(t, registry)
	if _, err := registry.Submit(req.ID, []byte("rotated")); err != nil {
		t.Fatal(err)
	}
	if got := <-done; got.err != nil || got.out != path+"\n" {
		t.Fatalf("refresh = %q, %v", got.out, got.err)
	}
	if value, err := registry.Store.Read("github-pat"); err != nil || string(value) != "rotated" {
		t.Fatalf("refreshed value = %q, %v", value, err)
	}
}

func TestCredRequestExitCodes(t *testing.T) {
	registry, socket := startCredDaemon(t)
	done := startCredCommand(t, "cred", "request", "x", "--json", "--socket-path", socket)
	req := awaitPending(t, registry)
	if _, err := registry.Decline(req.ID); err != nil {
		t.Fatal(err)
	}
	got := <-done
	var exit exitCodeError
	if !errors.As(got.err, &exit) || exit.code != credExitDeclined || !exit.reported || !strings.Contains(got.out, `"state": "declined"`) {
		t.Fatalf("declined = %q, %#v", got.out, got.err)
	}

	_, err := captureRun(t, "cred", "request", "y", "--timeout", "1s", "--socket-path", socket)
	if !errors.As(err, &exit) || exit.code != credExitExpired || exit.reported {
		t.Fatalf("expired err = %#v", err)
	}
}

func TestCredRequestReportsMissingDaemon(t *testing.T) {
	startCredDaemon(t)
	_, err := captureRun(t, "cred", "request", "x", "--socket-path", "/tmp/tmact-no-such.sock")
	if !errors.Is(err, credreq.ErrDaemonUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestCredRunInjectsEnvironment(t *testing.T) {
	registry, _ := startCredDaemon(t)
	if _, err := registry.Store.Write("npm-token", []byte("tok-123")); err != nil {
		t.Fatal(err)
	}
	var gotPath string
	var gotArgs, gotEnv []string
	oldExec, oldLook := credExec, credLookPath
	credExec = func(path string, args, env []string) error {
		gotPath, gotArgs, gotEnv = path, args, env
		return nil
	}
	credLookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
	t.Cleanup(func() { credExec, credLookPath = oldExec, oldLook })

	out, err := captureRun(t, "cred", "run", "--env", "NPM_TOKEN=npm-token", "--", "npm", "publish")
	if err != nil || out != "" {
		t.Fatalf("run = %q, %v", out, err)
	}
	if gotPath != "/usr/bin/npm" || strings.Join(gotArgs, " ") != "npm publish" {
		t.Fatalf("exec %q %v", gotPath, gotArgs)
	}
	found := false
	for _, kv := range gotEnv {
		found = found || kv == "NPM_TOKEN=tok-123"
	}
	if !found {
		t.Fatal("NPM_TOKEN not in environment")
	}

	for name, args := range map[string][]string{
		"missing cred":  {"cred", "run", "--env", "T=absent", "--", "true"},
		"bad env name":  {"cred", "run", "--env", "1T=npm-token", "--", "true"},
		"no command":    {"cred", "run", "--env", "T=npm-token"},
		"no env":        {"cred", "run", "--", "true"},
		"bad cred name": {"cred", "run", "--env", "T=../x", "--", "true"},
	} {
		if _, err := captureRun(t, args...); err == nil {
			t.Errorf("%s: succeeded", name)
		}
	}
}

func TestCredPathListRemove(t *testing.T) {
	registry, _ := startCredDaemon(t)
	if _, err := captureRun(t, "cred", "path", "a"); err == nil || !strings.Contains(err.Error(), "cred request a") {
		t.Fatalf("missing path err = %v", err)
	}
	if _, err := registry.Store.Write("a", []byte("secret-a")); err != nil {
		t.Fatal(err)
	}
	out, err := captureRun(t, "cred", "path", "a")
	if want, _ := registry.Store.Path("a"); err != nil || out != want+"\n" {
		t.Fatalf("path = %q, %v", out, err)
	}
	out, err = captureRun(t, "cred", "ls", "--json")
	if err != nil || !strings.Contains(out, `"name": "a"`) || strings.Contains(out, "secret-a") {
		t.Fatalf("ls = %q, %v", out, err)
	}
	if _, err := captureRun(t, "cred", "rm", "a"); err != nil {
		t.Fatal(err)
	}
	if out, _ := captureRun(t, "cred", "ls", "--json"); strings.TrimSpace(out) != "[]" {
		t.Fatalf("ls after rm = %q", out)
	}
	if _, err := captureRun(t, "cred", "path", "--bogus", "a"); err == nil {
		t.Fatal("unknown flag accepted")
	}
}
