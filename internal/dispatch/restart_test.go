package dispatch

import (
	"strings"
	"testing"
	"time"
)

func TestWaitExitInputWaitsForPaintWithoutRepeatingPaste(t *testing.T) {
	now := time.Now()
	reads := 0
	sleeps := 0
	deps := Deps{
		Now:   func() time.Time { return now },
		Sleep: func(d time.Duration) { now = now.Add(d); sleeps++ },
		CapturePane: func(string, int) (string, error) {
			reads++
			if reads < 3 {
				return "Claude Code\n❯\n⏵⏵ auto mode on", nil
			}
			return "Claude Code\n❯ /exit\n", nil
		},
	}
	if err := waitExitInput(Options{}, deps, "%1"); err != nil {
		t.Fatal(err)
	}
	if sleeps != 2 {
		t.Fatalf("sleeps=%d", sleeps)
	}
}

func TestWaitExitInputTimesOutWithoutEnter(t *testing.T) {
	now := time.Now()
	deps := Deps{Now: func() time.Time { return now }, Sleep: func(d time.Duration) { now = now.Add(d) }, CapturePane: func(string, int) (string, error) { return "Claude Code\n❯\n⏵⏵ auto mode on", nil }}
	err := waitExitInput(Options{}, deps, "%1")
	if err == nil || !strings.Contains(err.Error(), "refusing Enter") {
		t.Fatalf("err=%v", err)
	}
}
