package main

import "testing"

func TestVersionStringPrefersStampedBuildTime(t *testing.T) {
	info := versionInfo{
		Version:   "dev",
		Revision:  "8b9e502c35a7aaaa",
		Modified:  true,
		Time:      "2026-09-21T08:38:02Z",
		BuildTime: "2026-10-06T09:00:00Z",
		GoVersion: "go1.26.0",
	}
	want := "tmact dev (8b9e502c35a7-dirty) built 2026-10-06T09:00:00Z (commit 2026-09-21T08:38:02Z) with go1.26.0"
	if got := info.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got := info.BuiltAt(); got != "2026-10-06T09:00:00Z" {
		t.Fatalf("BuiltAt() = %q", got)
	}
}

func TestVersionStringFallsBackToCommitTime(t *testing.T) {
	info := versionInfo{Version: "dev", Revision: "8b9e502c35a7", Time: "2026-09-21T08:38:02Z", GoVersion: "go1.26.0"}
	want := "tmact dev (8b9e502c35a7) built 2026-09-21T08:38:02Z with go1.26.0"
	if got := info.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got := info.BuiltAt(); got != "2026-09-21T08:38:02Z" {
		t.Fatalf("BuiltAt() = %q", got)
	}
}
