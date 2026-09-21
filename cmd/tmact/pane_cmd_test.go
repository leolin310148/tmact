package main

import (
	"strings"
	"testing"
)

// TestPaneCommandsRejectPositionalTargets pins the reported surprise: an
// ignored positional made `tmact inspect SESSION:W.P` look scoped while it
// quietly reported every pane on the machine.
func TestPaneCommandsRejectPositionalTargets(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "inspect", args: []string{"inspect", "wt-sf-demo:2.0"}, want: `inspect does not accept positional arguments: "wt-sf-demo:2.0"; pass --target wt-sf-demo:2.0 to inspect that pane`},
		{name: "detect", args: []string{"detect", "%7"}, want: `detect does not accept positional arguments: "%7"; pass --target %7`},
		{name: "ls", args: []string{"ls", "work"}, want: `ls does not accept positional arguments: "work"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out, err := captureRun(t, test.args...)
			if err == nil {
				t.Fatalf("expected a usage error, output = %q", out)
			}
			if err.Error() != test.want {
				t.Fatalf("err = %q, want %q", err.Error(), test.want)
			}
			if strings.TrimSpace(out) != "" {
				t.Fatalf("refused command still printed output: %q", out)
			}
		})
	}
}
