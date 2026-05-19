package panestatus

import "testing"

func TestClassifyProcessRuntimeDetectsClaude(t *testing.T) {
	detected := ClassifyProcessRuntime([]Process{
		{PID: 123, PPID: 100, Command: "claude", Args: "claude"},
	})

	if detected.Runtime != RuntimeClaude {
		t.Fatalf("runtime = %q", detected.Runtime)
	}
	if detected.Confidence != ConfidenceHigh {
		t.Fatalf("confidence = %q", detected.Confidence)
	}
	if len(detected.Signals) != 1 || detected.Signals[0] != "child_process" {
		t.Fatalf("signals = %#v", detected.Signals)
	}
}

func TestClassifyProcessRuntimeDetectsCommandPath(t *testing.T) {
	detected := ClassifyProcessRuntime([]Process{
		{PID: 123, PPID: 100, Command: "/tmp/tmact-fixture/bin/claude", Args: "/tmp/tmact-fixture/bin/claude"},
	})

	if detected.Runtime != RuntimeClaude {
		t.Fatalf("runtime = %q", detected.Runtime)
	}
}

func TestClassifyProcessRuntimeDetectsGeminiNodeArgs(t *testing.T) {
	detected := ClassifyProcessRuntime([]Process{
		{PID: 123, PPID: 100, Command: "node", Args: "node /tmp/tmact-fixture/bin/gemini"},
	})

	if detected.Runtime != RuntimeGemini {
		t.Fatalf("runtime = %q", detected.Runtime)
	}
}

func TestParseProcesses(t *testing.T) {
	processes := parseProcesses("22247 14018 claude claude\n")
	if len(processes) != 1 {
		t.Fatalf("processes len = %d", len(processes))
	}
	process := processes[0]
	if process.PID != 22247 || process.PPID != 14018 {
		t.Fatalf("pid/ppid = %d/%d", process.PID, process.PPID)
	}
	if process.Command != "claude" || process.Args != "claude" {
		t.Fatalf("command/args = %q/%q", process.Command, process.Args)
	}
}
