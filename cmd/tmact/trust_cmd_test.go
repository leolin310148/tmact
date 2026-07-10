package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/foldertrust"
)

func TestTrustFolderCLIIsDryRunByDefault(t *testing.T) {
	defer stubCLIHooks(t)()
	dir := t.TempDir()
	trustFolderRun = func(_ context.Context, opts foldertrust.Options) (foldertrust.Result, error) {
		if opts.Target != "%7" || opts.Dir != dir || opts.Agent != "codex" || !opts.DryRun || opts.Timeout != 5*time.Second {
			t.Fatalf("opts = %#v", opts)
		}
		return foldertrust.Result{Target: opts.Target, Dir: dir, Agent: opts.Agent, PromptFound: true, DryRun: true, OptionNumber: 1, OptionLabel: "Yes, continue"}, nil
	}
	out, err := captureRun(t, "trust-folder", "--target", "%7", "--dir", dir, "--agent", "CODEX", "--timeout", "5s")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "dry-run trust-folder") || !strings.Contains(out, "would accept option 1") {
		t.Fatalf("output = %s", out)
	}
}

func TestTrustFolderCLIExecuteIsExplicit(t *testing.T) {
	defer stubCLIHooks(t)()
	dir := t.TempDir()
	trustFolderRun = func(_ context.Context, opts foldertrust.Options) (foldertrust.Result, error) {
		if opts.DryRun {
			t.Fatal("--execute must disable dry-run")
		}
		return foldertrust.Result{Target: opts.Target, Dir: dir, Agent: opts.Agent, PromptFound: true, Accepted: true, OptionNumber: 1, OptionLabel: "Trust folder"}, nil
	}
	out, err := captureRun(t, "trust-folder", "--target", "%7", "--dir", dir, "--agent", "claude", "--execute")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "execute trust-folder") || !strings.Contains(out, "accepted option 1") {
		t.Fatalf("output = %s", out)
	}
}
