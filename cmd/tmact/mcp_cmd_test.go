package main

import (
	"strings"
	"testing"
)

func TestIsLoopbackAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:7895": true, "localhost:1": true, "[::1]:7895": true,
		"0.0.0.0:7895": false, ":7895": false, "100.64.0.1:7895": false, "bad": false,
	} {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("%s = %v, want %v", addr, got, want)
		}
	}
}

func TestMCPServeRequiresAccessOrLoopback(t *testing.T) {
	if err := runMCPServe(nil); err == nil || !strings.Contains(err.Error(), "--access-team") {
		t.Fatalf("missing access err = %v", err)
	}
	if err := runMCPServe([]string{"--no-access", "--addr", "0.0.0.0:7895"}); err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("non-loopback err = %v", err)
	}
}
