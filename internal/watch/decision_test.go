package watch

import (
	"path/filepath"
	"testing"

	"github.com/leolin310148/tmact/internal/prompt"
)

func TestEvaluateDirectoryAccessAllowsPathsUnderAllowlist(t *testing.T) {
	base := t.TempDir()
	detected := directoryAccessPrompt(filepath.Join(base, "packages"))
	rule := RuleConfig{
		AllowPaths: []string{base},
	}

	decision := evaluateDirectoryAccess(rule, detected)
	if !decision.Accept {
		t.Fatalf("accept = false, reason = %s", decision.Reason)
	}
	if decision.Signature == "" {
		t.Fatal("expected signature")
	}
}

func TestEvaluateDirectoryAccessBlocksPathsOutsideAllowlist(t *testing.T) {
	base := t.TempDir()
	other := t.TempDir()
	detected := directoryAccessPrompt(filepath.Join(other, "packages"))
	rule := RuleConfig{
		AllowPaths: []string{base},
	}

	decision := evaluateDirectoryAccess(rule, detected)
	if decision.Accept {
		t.Fatal("accept = true")
	}
	if decision.Reason != "path_not_allowed" {
		t.Fatalf("reason = %q", decision.Reason)
	}
}

func TestEvaluateDirectoryAccessAllowsPathPatterns(t *testing.T) {
	detected := directoryAccessPrompt("/tmp/tmact-sample-home.md", "/tmp/tmact-sample-network.json")
	rule := RuleConfig{
		AllowPathPatterns: []string{"/tmp/tmact-sample-*"},
	}

	decision := evaluateDirectoryAccess(rule, detected)
	if !decision.Accept {
		t.Fatalf("accept = false, reason = %s", decision.Reason)
	}
}

func TestEvaluateDirectoryAccessBlocksNonMatchingPathPatterns(t *testing.T) {
	detected := directoryAccessPrompt("/tmp/tmact-sample-home.md", "/tmp/other.json")
	rule := RuleConfig{
		AllowPathPatterns: []string{"/tmp/tmact-sample-*"},
	}

	decision := evaluateDirectoryAccess(rule, detected)
	if decision.Accept {
		t.Fatal("accept = true")
	}
	if decision.Reason != "path_not_allowed" {
		t.Fatalf("reason = %q", decision.Reason)
	}
}

func TestEvaluateDirectoryAccessRequiresSelectedOption(t *testing.T) {
	detected := directoryAccessPrompt(t.TempDir())
	detected.SelectedOption = nil
	rule := RuleConfig{
		AllowPaths: detected.Paths,
	}

	decision := evaluateDirectoryAccess(rule, detected)
	if decision.Accept {
		t.Fatal("accept = true")
	}
	if decision.Reason != "no_selected_option" {
		t.Fatalf("reason = %q", decision.Reason)
	}
}

func TestEvaluateDirectoryAccessRequiresAffirmativeSelectedOption(t *testing.T) {
	detected := directoryAccessPrompt(t.TempDir())
	detected.SelectedOption = &prompt.Option{
		Number:   3,
		Label:    "No (Esc)",
		Selected: true,
	}
	rule := RuleConfig{
		AllowPaths: detected.Paths,
	}

	decision := evaluateDirectoryAccess(rule, detected)
	if decision.Accept {
		t.Fatal("accept = true")
	}
	if decision.Reason != "selected_option_not_accepting" {
		t.Fatalf("reason = %q", decision.Reason)
	}
}

func directoryAccessPrompt(paths ...string) *prompt.DirectoryAccess {
	return &prompt.DirectoryAccess{
		Title:    "Allow directory access",
		Question: "Do you want to allow this?",
		Paths:    paths,
		SelectedOption: &prompt.Option{
			Number:   2,
			Label:    "Yes, and add these directories to the allowed list",
			Selected: true,
		},
	}
}
