package panestatus

import (
	"sort"
	"sync"
)

// processTable is one `ps -A` snapshot shared by every pane in an inspect
// cycle. Walking each pane's process tree with pgrep/ps costs a few forks per
// pane; with dozens of shell panes that was a steady fork storm, so the cycle
// reads the whole table at most once, lazily, and walks it in memory.
type processTable struct {
	once     sync.Once
	load     func() ([]Process, error)
	byPID    map[int]Process
	children map[int][]Process
	err      error
}

func newProcessTable(load func() ([]Process, error)) *processTable {
	return &processTable{load: load}
}

// newProcessTableRuntime returns a per-cycle process runtime detector backed
// by a single `ps -A`. If that read fails it falls back to the per-pane walk.
func newProcessTableRuntime() processRuntimeFunc {
	table := newProcessTable(listAllProcesses)
	return func(pid int) RuntimeDetection {
		processes, err := table.tree(pid, 3)
		if err != nil {
			return DetectChildProcessRuntime(pid)
		}
		return ClassifyProcessRuntime(processes)
	}
}

func listAllProcesses() ([]Process, error) {
	output, err := commandOutput("ps", "-Ao", "pid=,ppid=,comm=,command=")
	if err != nil {
		return nil, err
	}
	return parseProcesses(output), nil
}

func (t *processTable) ensure() error {
	t.once.Do(func() {
		processes, err := t.load()
		if err != nil {
			t.err = err
			return
		}
		t.byPID = make(map[int]Process, len(processes))
		t.children = map[int][]Process{}
		for _, p := range processes {
			t.byPID[p.PID] = p
			t.children[p.PPID] = append(t.children[p.PPID], p)
		}
	})
	return t.err
}

// tree mirrors childProcessTree: the parent itself, then up to maxDepth
// levels of descendants in PID order, stopping once 64 processes are found.
func (t *processTable) tree(parentPID, maxDepth int) ([]Process, error) {
	if parentPID <= 0 || maxDepth <= 0 {
		return nil, nil
	}
	if err := t.ensure(); err != nil {
		return nil, err
	}
	var result []Process
	if p, ok := t.byPID[parentPID]; ok {
		result = append(result, p)
	}
	frontier := []int{parentPID}
	seen := map[int]bool{parentPID: true}
	for depth := 0; depth < maxDepth && len(frontier) > 0 && len(result) < 64; depth++ {
		var children []Process
		for _, pid := range frontier {
			children = append(children, t.children[pid]...)
		}
		if len(children) == 0 {
			return result, nil
		}
		sort.Slice(children, func(a, b int) bool { return children[a].PID < children[b].PID })
		var next []int
		for _, child := range children {
			if seen[child.PID] {
				continue
			}
			seen[child.PID] = true
			result = append(result, child)
			next = append(next, child.PID)
		}
		frontier = next
	}
	return result, nil
}
