package panestatus

import (
	"errors"
	"reflect"
	"testing"
)

func TestProcessTableTreeWalksDescendantsInPIDOrder(t *testing.T) {
	loads := 0
	table := newProcessTable(func() ([]Process, error) {
		loads++
		return parseProcesses(`
  1     0 launchd /sbin/launchd
100     1 zsh -zsh
130   100 node node /opt/homebrew/bin/claude
120   100 zsh zsh -c make
121   120 make make test
200     1 zsh -zsh
999   121 go go test ./...
`), nil
	})

	got, err := table.tree(100, 2)
	if err != nil {
		t.Fatal(err)
	}
	var pids []int
	for _, p := range got {
		pids = append(pids, p.PID)
	}
	// Depth 2 stops before 999 (great-grandchild), matching childProcessTree.
	if want := []int{100, 120, 130, 121}; !reflect.DeepEqual(pids, want) {
		t.Fatalf("pids = %v, want %v", pids, want)
	}
	if rt := ClassifyProcessRuntime(got).Runtime; rt != RuntimeClaude {
		t.Fatalf("runtime = %q, want claude", rt)
	}

	if _, err := table.tree(200, 3); err != nil {
		t.Fatal(err)
	}
	if loads != 1 {
		t.Fatalf("ps loads = %d, want 1 per cycle", loads)
	}
}

func TestProcessTableTreeReportsLoadError(t *testing.T) {
	table := newProcessTable(func() ([]Process, error) { return nil, errors.New("ps failed") })
	if _, err := table.tree(100, 3); err == nil {
		t.Fatal("want load error so the caller falls back to the per-pane walk")
	}
}
