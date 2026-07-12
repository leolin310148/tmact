package panestate

import "testing"

func TestClassifyANSIDistinguishesSuggestionsAndDrafts(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		ansi string
		want string
	}{
		{
			name: "claude suggestion",
			raw:  "I am working on stale output\n❯ source ~/.zsh_aliases\n⏵⏵ auto mode on (shift+tab to cycle) · ← for agents\n",
			ansi: "I am working on stale output\n\x1b[39m❯ \x1b[2msource ~/.zsh_aliases\x1b[0m\n⏵⏵ auto mode on (shift+tab to cycle) · ← for agents\n",
			want: StateWaitingInput,
		},
		{
			name: "claude draft",
			raw:  "❯ source ~/.zsh_aliases\n",
			ansi: "\x1b[38;5;239m\x1b[48;5;237m❯ \x1b[38;5;231msource ~/.zsh_aliases\x1b[0m\n",
			want: StateDraftInput,
		},
		{
			name: "codex suggestion",
			raw:  "› Write tests for @filename\n~/w/ndt/mxcp · main · Context 30% used\n",
			ansi: "\x1b[0;1m›\x1b[0m \x1b[2mWrite tests for @filename\x1b[0m\n~/w/ndt/mxcp · main · Context 30% used\n",
			want: StateWaitingInput,
		},
		{
			name: "codex draft",
			raw:  "› Write tests for store.go\n",
			ansi: "\x1b[0;1m›\x1b[0m \x1b[38;2;205;214;244mWrite tests for store.go\x1b[0m\n",
			want: StateDraftInput,
		},
		{
			name: "empty input",
			raw:  "old working output\n❯\n",
			ansi: "old working output\n\x1b[39m❯ \x1b[0m\n",
			want: StateWaitingInput,
		},
		{
			name: "working with empty steering input",
			raw:  "✽ Choreographing… esc to interrupt\n❯\n",
			ansi: "✽ Choreographing… esc to interrupt\n\x1b[39m❯ \x1b[0m\n",
			want: StateWorking,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyANSI(tt.raw, tt.ansi)
			if got.State != tt.want {
				t.Fatalf("state=%q want=%q signals=%v", got.State, tt.want, got.Signals)
			}
		})
	}
}

func TestClassifyANSIDoesNotOverridePermissionPrompt(t *testing.T) {
	raw := "Allow this command?\n❯ 2. No\n"
	ansi := "Allow this command?\n\x1b[1m❯\x1b[0m \x1b[38;5;231m2. No\x1b[0m\n"
	got := ClassifyANSI(raw, ansi)
	if got.State != StateWaitingPermission || !got.Asking {
		t.Fatalf("result=%#v", got)
	}
}
