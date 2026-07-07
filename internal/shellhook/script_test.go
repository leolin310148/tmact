package shellhook

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestInitScriptSupportedShells(t *testing.T) {
	for _, shell := range Shells {
		t.Run(shell, func(t *testing.T) {
			script, err := InitScript(shell)
			if err != nil {
				t.Fatalf("InitScript(%q): %v", shell, err)
			}
			for _, want := range []string{
				"TMUX_PANE",
				"tmact hook emit --quiet --type preexec",
				"tmact hook emit --quiet --type precmd",
				"TMACT_HOOK_CMD_ID",
				"--exit-code",
			} {
				if !strings.Contains(script, want) {
					t.Errorf("%s script missing %q", shell, want)
				}
			}
			// Every emit must be silenced and never block the prompt.
			for _, line := range strings.Split(script, "\n") {
				if !strings.Contains(line, "tmact hook emit") {
					continue
				}
				if !strings.Contains(line, ">/dev/null 2>&1") {
					t.Errorf("%s emit line not silenced: %s", shell, line)
				}
				if !strings.Contains(line, "&") {
					t.Errorf("%s emit line not backgrounded: %s", shell, line)
				}
			}
		})
	}
}

func TestInitScriptBashGuardsAgainstPromptCommandFirings(t *testing.T) {
	script, err := InitScript("bash")
	if err != nil {
		t.Fatalf("InitScript(bash): %v", err)
	}
	// An empty Enter reruns PROMPT_COMMAND, firing the DEBUG trap for our
	// own precmd function and any other PROMPT_COMMAND entries; both must be
	// skipped or every prompt emits a spurious preexec/precmd pair.
	for _, want := range []string{
		`[[ ${BASH_COMMAND} == _tmact_hook_* ]] && return 0`,
		`_tmact_hook_is_prompt_command "$BASH_COMMAND" && return 0`,
		`for tmact_pc in "${PROMPT_COMMAND[@]}"; do`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("bash script missing guard %q", want)
		}
	}
}

func TestInitScriptBashComposesExistingIntegrations(t *testing.T) {
	script, err := InitScript("bash")
	if err != nil {
		t.Fatalf("InitScript(bash): %v", err)
	}
	// It must read the existing DEBUG trap and compose rather than clobber,
	// and branch on PROMPT_COMMAND's type so an array (bash 5.1+) is not
	// flattened into a string.
	for _, want := range []string{
		"trap -p DEBUG",
		"; _tmact_hook_preexec\" DEBUG",
		"declare -p PROMPT_COMMAND",
		"PROMPT_COMMAND+=('_tmact_hook_precmd')",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("bash script missing composition %q", want)
		}
	}
}

func TestInitScriptBashPromptCommandGuardChecksArrayElements(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	script, err := InitScript("bash")
	if err != nil {
		t.Fatalf("InitScript(bash): %v", err)
	}

	fakeBin := t.TempDir()
	writeFakeTmact(t, fakeBin)
	driver := `
export TMUX_PANE=%test
export PATH="` + fakeBin + `:$PATH"
PROMPT_COMMAND=('echo tmact-orig-a' 'echo tmact-orig-b')
eval "$TMACT_HOOK_SCRIPT"
_tmact_hook_is_prompt_command 'echo tmact-orig-b'; printf 'second=%s\n' "$?"
_tmact_hook_is_prompt_command '_tmact_hook_precmd'; printf 'precmd=%s\n' "$?"
_tmact_hook_is_prompt_command 'echo not-present'; printf 'miss=%s\n' "$?"
`
	cmd := exec.Command(bashPath, "--norc", "--noprofile", "-c", driver)
	cmd.Env = append(os.Environ(), "TMACT_HOOK_SCRIPT="+script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bash driver failed: %v\n%s", err, out)
	}
	got := string(out)
	for _, want := range []string{"second=0", "precmd=0", "miss=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want %q", got, want)
		}
	}
}

// TestInitScriptBashComposesAtRuntime actually sources the generated hook in
// a real bash and asserts a pre-existing DEBUG trap and PROMPT_COMMAND both
// survive alongside the tmact hooks.
func TestInitScriptBashComposesAtRuntime(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	script, err := InitScript("bash")
	if err != nil {
		t.Fatalf("InitScript(bash): %v", err)
	}

	// A fake tmact on PATH so `command -v tmact` passes without emitting.
	fakeBin := t.TempDir()
	writeFakeTmact(t, fakeBin)
	out := filepath.Join(t.TempDir(), "inspect.txt")

	// The pre-existing DEBUG trap sets a marker instead of echoing so it does
	// not pollute stdout when it fires between commands.
	driver := `
export TMUX_PANE=%test
export PATH="` + fakeBin + `:$PATH"
trap 'TMACT_PREV_DEBUG_RAN=1' DEBUG
PROMPT_COMMAND='echo tmact-orig-pc'
eval "$TMACT_HOOK_SCRIPT"
{ trap -p DEBUG; declare -p PROMPT_COMMAND; } > "` + out + `"
`
	cmd := exec.Command(bashPath, "--norc", "--noprofile", "-c", driver)
	cmd.Env = append(os.Environ(), "TMACT_HOOK_SCRIPT="+script)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash driver failed: %v\n%s", err, b)
	}
	got := readFile(t, out)

	// Existing DEBUG trap preserved and tmact preexec composed onto it.
	if !strings.Contains(got, "TMACT_PREV_DEBUG_RAN=1") {
		t.Errorf("existing DEBUG trap lost: %s", got)
	}
	if !strings.Contains(got, "_tmact_hook_preexec") {
		t.Errorf("tmact preexec not installed in DEBUG trap: %s", got)
	}
	// Existing PROMPT_COMMAND entry preserved and precmd appended.
	if !strings.Contains(got, "tmact-orig-pc") {
		t.Errorf("existing PROMPT_COMMAND lost: %s", got)
	}
	if !strings.Contains(got, "_tmact_hook_precmd") {
		t.Errorf("tmact precmd not appended to PROMPT_COMMAND: %s", got)
	}
}

// TestInitScriptBashKeepsArrayPromptCommand checks that an array-style
// PROMPT_COMMAND (bash 5.1+) is appended to, not coerced into a string.
func TestInitScriptBashKeepsArrayPromptCommand(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	if !bashSupportsArrayPromptCommand(t, bashPath) {
		t.Skip("bash too old for array PROMPT_COMMAND")
	}
	script, err := InitScript("bash")
	if err != nil {
		t.Fatalf("InitScript(bash): %v", err)
	}

	fakeBin := t.TempDir()
	writeFakeTmact(t, fakeBin)
	out := filepath.Join(t.TempDir(), "inspect.txt")

	driver := `
export TMUX_PANE=%test
export PATH="` + fakeBin + `:$PATH"
PROMPT_COMMAND=('echo tmact-orig-a' 'echo tmact-orig-b')
eval "$TMACT_HOOK_SCRIPT"
declare -p PROMPT_COMMAND > "` + out + `"
`
	cmd := exec.Command(bashPath, "--norc", "--noprofile", "-c", driver)
	cmd.Env = append(os.Environ(), "TMACT_HOOK_SCRIPT="+script)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash driver failed: %v\n%s", err, b)
	}
	got := readFile(t, out)

	// It must still be an array holding the original entries plus precmd.
	if !strings.Contains(got, "declare -a") {
		t.Errorf("PROMPT_COMMAND coerced out of array form: %s", got)
	}
	for _, want := range []string{"tmact-orig-a", "tmact-orig-b", "_tmact_hook_precmd"} {
		if !strings.Contains(got, want) {
			t.Errorf("array PROMPT_COMMAND missing %q: %s", want, got)
		}
	}
}

func writeFakeTmact(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "tmact")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake tmact: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func bashSupportsArrayPromptCommand(t *testing.T, bashPath string) bool {
	t.Helper()
	out, err := exec.Command(bashPath, "-c", "printf '%s.%s' \"${BASH_VERSINFO[0]}\" \"${BASH_VERSINFO[1]}\"").Output()
	if err != nil {
		return false
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), ".", 2)
	if len(parts) != 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major > 5 || (major == 5 && minor >= 1)
}

func TestInitScriptUnknownShell(t *testing.T) {
	if _, err := InitScript("powershell"); err == nil {
		t.Fatal("InitScript accepted unsupported shell")
	}
}
