package main

import (
	"flag"
	"os/exec"

	"github.com/leolin310148/tmact/internal/dispatch"
)

var decideAvailable = func() bool { _, err := exec.LookPath("decide"); return err == nil }

// Explicit models bypass decide. An explicit provider restricts automatic
// routing; absent --auto-model, decide's availability supplies the default.
func resolveAutoModel(fs *flag.FlagSet, requested bool, agent, model string) (bool, string) {
	if model != "" {
		if agent == "" {
			for _, a := range []string{"claude", "codex"} {
				if _, err := dispatch.ValidateModel(a, model); err == nil {
					agent = a
					break
				}
			}
		}
		return false, agent
	}
	explicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "auto-model" {
			explicit = true
		}
	})
	if explicit {
		return requested, agent
	}
	if agent != "" && agent != "claude" && agent != "codex" {
		return false, agent
	}
	return decideAvailable(), agent
}
