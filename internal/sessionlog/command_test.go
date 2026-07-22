package sessionlog

import "testing"

func TestSafeCommandSummaryParsesAssignmentsAndEnvConservatively(t *testing.T) {
	tests := []struct {
		name    string
		command string
		verb    string
		sub     string
	}{
		{name: "single quoted value", command: `SECRET='alpha beta' git status`, verb: "git", sub: "status"},
		{name: "double quoted value", command: `SECRET="alpha beta" git status`, verb: "git", sub: "status"},
		{name: "escaped space", command: `SECRET=alpha\ beta git status`, verb: "git", sub: "status"},
		{name: "multiple assignments", command: `FIRST=alpha SECOND='beta gamma' go test ./...`, verb: "go", sub: "test"},
		{name: "quoted command words", command: `"git" "status"`, verb: "git", sub: "status"},
		{name: "env assignment", command: `env SECRET='alpha beta' git status`, verb: "git", sub: "status"},
		{name: "env quoted assignment argument", command: `env 'SECRET=alpha beta' git status`, verb: "git", sub: "status"},
		{name: "env ignore option", command: `env -i SECRET='alpha beta' git status`, verb: "git", sub: "status"},
		{name: "env unset and chdir options", command: `env --unset OLD --chdir=/tmp SECRET='alpha beta' /usr/bin/git status`, verb: "git", sub: "status"},
		{name: "env verbose and alternate path options", command: `env -v -P /bin SECRET='alpha beta' git status`, verb: "git", sub: "status"},
		{name: "env attached unset option", command: `env -uOLD SECRET='alpha beta' git status`, verb: "git", sub: "status"},
		{name: "env option terminator", command: `env -i -- SECRET='alpha beta' git status`, verb: "git", sub: "status"},
		{name: "rtk and env wrappers", command: `rtk proxy env -i SECRET='alpha beta' git status`, verb: "git", sub: "status"},
		{name: "argv word boundaries", command: joinCommandWords([]string{"env", "SECRET=alpha beta", "git", "status"}), verb: "git", sub: "status"},
		{name: "unbalanced single quote", command: `SECRET='alpha beta git status`},
		{name: "unbalanced double quote", command: `SECRET="alpha beta git status`},
		{name: "command substitution", command: `SECRET=$(printf 'alpha beta') git status`},
		{name: "parameter expansion braces", command: `SECRET=${VALUE:-alpha beta} git status`},
		{name: "dynamic executable", command: `$COMMAND status`},
		{name: "quoted literal executable", command: `'"git"' status`},
		{name: "unsupported env option", command: `env --unknown-option SECRET='alpha beta' git status`},
		{name: "env split string is ambiguous", command: `env --split-string 'git status'`},
		{name: "shell operator is ambiguous", command: `SECRET='alpha beta' git status | tee output`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verb, sub := SafeCommandSummary(test.command)
			if verb != test.verb || sub != test.sub {
				t.Fatalf("SafeCommandSummary(%q) = %q %q, want %q %q", test.command, verb, sub, test.verb, test.sub)
			}
		})
	}
}
