package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "workflow.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func minimalConfig(extra, stages string) string {
	return `version: 2
workspace: {root: .}
defaults: {timeout: 5s}
` + extra + `stages:
` + stages
}

func TestLoadStrictValidation(t *testing.T) {
	tests := []struct{ name, extra, stages, want string }{
		{"unknown field", "mystery: true\n", "  - {id: ok, type: command, argv: [/usr/bin/true]}\n", "field mystery"},
		{"duplicate key", "workspace: {root: .}\n", "  - {id: ok, type: command, argv: [/usr/bin/true]}\n", "duplicate YAML key"},
		{"duplicate stage", "", "  - {id: same, type: command, argv: [/usr/bin/true]}\n  - {id: same, type: command, argv: [/usr/bin/true]}\n", "duplicate stage id"},
		{"cycle", "", "  - {id: a, type: command, needs: [b], argv: [/usr/bin/true]}\n  - {id: b, type: command, needs: [a], argv: [/usr/bin/true]}\n", "cycle"},
		{"shell", "", "  - {id: bad, type: command, argv: [sh, -c, echo nope]}\n", "must not use a shell"},
		{"cwd escape", "", "  - {id: bad, type: command, cwd: ../outside, argv: [/usr/bin/true]}\n", "escapes workspace"},
		{"bad template", "", "  - {id: bad, type: command, argv: [/usr/bin/true, '{{ nope(']}\n", "template"},
		{"stage union", "", "  - {id: bad, type: command, argv: [/usr/bin/true], prompt: nope}\n", "different stage type"},
		{"condition field", "", "  - id: gate\n    type: gate\n    condition:\n      stage: {id: gate, mystery: value}\n", "field mystery"},
		{"multiple documents", "", "  - {id: ok, type: command, argv: [/usr/bin/true]}\n---\nversion: 2\n", "exactly one YAML document"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			body := minimalConfig(tc.extra, tc.stages)
			if tc.name == "duplicate key" {
				body = "version: 2\nworkspace: {root: .}\n" + tc.extra + "defaults: {timeout: 5s}\nstages:\n" + tc.stages
			}
			path := writeConfig(t, dir, body)
			_, err := Load(path, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q\n%s", err, tc.want, body)
			}
		})
	}
}

func TestActorUnionAndReferences(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, minimalConfig(`actors:
  worker: {agent: existing, launch: {runtime: codex, session: work}}
`, "  - {id: ask, type: agent, actor: worker, prompt: hi, outcomes: {ok: success}}\n"))
	_, err := Load(path, nil)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("error=%v", err)
	}
}

func TestLoadRejectsCopilotActorRuntime(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, minimalConfig(`actors:
  worker: {launch: {runtime: copilot, session: work}}
`, "  - {id: ask, type: agent, actor: worker, prompt: hi, outcomes: {ok: success}}\n"))
	_, err := Load(path, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported runtime") {
		t.Fatalf("error=%v", err)
	}
}

func TestVariablesAreTypedAndRequired(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, minimalConfig(`variables:
  count: {type: int, required: true, enum: [2, 3]}
`, "  - {id: ok, type: command, argv: [/usr/bin/true]}\n"))
	if _, err := Load(path, nil); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("error=%v", err)
	}
	loaded, err := Load(path, map[string]string{"count": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Variables["count"] != int64(2) {
		t.Fatalf("vars=%#v", loaded.Variables)
	}
}

func TestStringListVariableAndCommandArgvReference(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, minimalConfig(`variables:
  verify: {type: string_list, default: [/usr/bin/true]}
`, "  - {id: verify, type: command, argv_variable: verify}\n"))
	loaded, err := Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := loaded.Variables["verify"].([]string); !ok || strings.Join(got, ",") != "/usr/bin/true" {
		t.Fatalf("default verify=%#v", loaded.Variables["verify"])
	}
	loaded, err = Load(path, map[string]string{"verify": `["/usr/bin/printf","hello world"]`})
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Variables["verify"].([]string); strings.Join(got, "|") != "/usr/bin/printf|hello world" {
		t.Fatalf("override verify=%#v", got)
	}
}

func TestCommandArgvVariableValidation(t *testing.T) {
	tests := []struct {
		name      string
		variables string
		stage     string
		supplied  map[string]string
		want      string
	}{
		{name: "unknown", stage: "  - {id: verify, type: command, argv_variable: missing}\n", want: "unknown variable"},
		{name: "wrong type", variables: "  verify: {type: string, default: /usr/bin/true}\n", stage: "  - {id: verify, type: command, argv_variable: verify}\n", want: "must have type string_list"},
		{name: "missing value", variables: "  verify: {type: string_list}\n", stage: "  - {id: verify, type: command, argv_variable: verify}\n", want: "value is required"},
		{name: "both sources", variables: "  verify: {type: string_list, default: [/usr/bin/true]}\n", stage: "  - {id: verify, type: command, argv: [/usr/bin/true], argv_variable: verify}\n", want: "exactly one"},
		{name: "shell rejected", variables: "  verify: {type: string_list, default: [sh, -c, echo]}\n", stage: "  - {id: verify, type: command, argv_variable: verify}\n", want: "must not use a shell"},
		{name: "bad JSON", variables: "  verify: {type: string_list, required: true}\n", stage: "  - {id: verify, type: command, argv_variable: verify}\n", supplied: map[string]string{"verify": "make test"}, want: "JSON array"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			extra := ""
			if tc.variables != "" {
				extra = "variables:\n" + tc.variables
			}
			path := writeConfig(t, t.TempDir(), minimalConfig(extra, tc.stage))
			_, err := Load(path, tc.supplied)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want=%q", err, tc.want)
			}
		})
	}
}
