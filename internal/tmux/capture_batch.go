package tmux

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os/exec"
	"strconv"
	"strings"
)

// CaptureSpec is one pane capture inside a CapturePanesBatch call. Escapes and
// JoinWrapped mirror CapturePaneANSI (-e) and CapturePane (-J).
type CaptureSpec struct {
	PaneID      string
	Lines       int
	Escapes     bool
	JoinWrapped bool
}

// CapturePanesBatch captures many panes with as few tmux invocations as
// possible, returning output keyed by spec index. Output for each spec is
// byte-identical to the matching single capturePaneContext call.
//
// A poller that forks one tmux client per pane per tick spends most of its
// CPU in fork/exec, so the captures are chained into one command list with a
// display-message marker after each. tmux aborts the rest of a list when one
// command fails (a pane closed since list-panes), so the failed spec is left
// out of the result for the caller's single-capture fallback to report, and
// the remaining specs are retried in a new batch.
func CapturePanesBatch(specs []CaptureSpec) map[int]string {
	return capturePanesBatch(specs, runCaptureBatch)
}

func capturePanesBatch(specs []CaptureSpec, run func(args []string) string) map[int]string {
	results := make(map[int]string, len(specs))
	start := 0
	for start < len(specs) {
		nonce := batchNonce()
		output := run(captureBatchArgs(specs[start:], nonce))
		got := parseCaptureBatch(output, nonce, len(specs)-start)
		for i, text := range got {
			results[start+i] = text
		}
		if len(got) == 0 {
			// Nothing came back (tmux unreachable, or the very first pane
			// failed): stop rather than fork once per remaining spec; the
			// caller's single captures handle what is missing.
			break
		}
		// got stops at the first failed capture; skip it and retry the rest.
		start += len(got) + 1
	}
	return results
}

func captureBatchArgs(specs []CaptureSpec, nonce string) []string {
	var args []string
	for i, spec := range specs {
		if i > 0 {
			args = append(args, ";")
		}
		lines := spec.Lines
		if lines <= 0 {
			lines = 120
		}
		args = append(args, "capture-pane", "-t", spec.PaneID, "-p", "-S", "-"+strconv.Itoa(lines))
		if spec.Escapes {
			args = append(args, "-e")
		}
		if spec.JoinWrapped {
			args = append(args, "-J")
		}
		args = append(args, ";", "display-message", "-p", batchMarker(nonce, i))
	}
	return args
}

// parseCaptureBatch splits batch output on the per-capture markers and returns
// the captures that completed, in order, up to the first missing marker.
func parseCaptureBatch(output, nonce string, n int) []string {
	var got []string
	pos := 0
	for i := 0; i < n; i++ {
		marker := batchMarker(nonce, i) + "\n"
		idx := -1
		if strings.HasPrefix(output[pos:], marker) {
			idx = pos
		} else if j := strings.Index(output[pos:], "\n"+marker); j >= 0 {
			idx = pos + j + 1
		}
		if idx < 0 {
			break
		}
		got = append(got, output[pos:idx])
		pos = idx + len(marker)
	}
	return got
}

func batchMarker(nonce string, i int) string {
	return "tmact-capture-" + nonce + "-" + strconv.Itoa(i)
}

func batchNonce() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// runCaptureBatch runs the chained command list and returns whatever stdout
// tmux produced; a non-zero exit only means some capture failed, which the
// marker parse already accounts for.
func runCaptureBatch(args []string) string {
	cmd := exec.Command("tmux", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	_ = cmd.Run()
	return stdout.String()
}
