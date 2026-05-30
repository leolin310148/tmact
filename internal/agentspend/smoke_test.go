package agentspend

import (
	"os"
	"testing"
	"time"
)

// TestSmokeRealData prints the computed spend against the developer's real
// local session logs. Skipped unless AGENTSPEND_SMOKE=1 so it never runs in CI
// or touches a home dir during a normal `go test ./...`.
func TestSmokeRealData(t *testing.T) {
	if os.Getenv("AGENTSPEND_SMOKE") != "1" {
		t.Skip("set AGENTSPEND_SMOKE=1 to run against real session logs")
	}
	res := Compute(time.Now())
	for prov, s := range res {
		t.Logf("%-7s week=$%.2f month=$%.2f", prov, s.WeekUSD, s.MonthUSD)
	}
}
