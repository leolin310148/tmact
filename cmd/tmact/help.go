package main

import (
	"fmt"
	"strings"
)

type helpFlag struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

type commandHelp struct {
	Command     string     `json:"command"`
	Summary     string     `json:"summary"`
	Usage       []string   `json:"usage,omitempty"`
	Subcommands []string   `json:"subcommands,omitempty"`
	Flags       []helpFlag `json:"flags,omitempty"`
	Examples    []string   `json:"examples,omitempty"`
	Safety      []string   `json:"safety,omitempty"`
	Notes       []string   `json:"notes,omitempty"`
}

type helpManifest struct {
	Name        string        `json:"name"`
	Summary     string        `json:"summary"`
	GlobalFlags []helpFlag    `json:"global_flags,omitempty"`
	Commands    []commandHelp `json:"commands"`
}

func runHelp(args []string) error {
	jsonOutput := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		filtered = append(filtered, arg)
	}
	if len(filtered) == 0 || wantsHelp(filtered) {
		if jsonOutput {
			return printJSON(commandManifest())
		}
		return usage()
	}
	name := strings.Join(filtered, " ")
	if jsonOutput {
		help, ok := commandHelpFor(name)
		if !ok {
			return fmt.Errorf("unknown help topic %q", name)
		}
		return printJSON(help)
	}
	return printCommandHelp(name)
}

func commandHelpFor(name string) (commandHelp, bool) {
	normalized := strings.Join(strings.Fields(name), " ")
	for _, help := range commandHelpCatalog() {
		if help.Command == normalized {
			return help, true
		}
	}
	return commandHelp{}, false
}

func commandManifest() helpManifest {
	return helpManifest{
		Name:    "tmact",
		Summary: "Local tmux automation CLI for inspecting panes, sending guarded input, and running loop daemons.",
		GlobalFlags: []helpFlag{
			{Name: "-t, --target", Value: "TARGET", Description: "target selector for send; may be a tmux target or a numbered index from tmact ls"},
		},
		Commands: commandHelpCatalog(),
	}
}
