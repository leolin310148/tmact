package main

import (
	"flag"
	"fmt"
	"os"
)

func runCommands(args []string) error {
	if wantsHelp(args) {
		fmt.Print(`Usage:
  tmact commands [--json]

Print the command catalog. Use --json when another program or LLM needs a
machine-readable list of commands, flags, examples, and safety notes.
`)
		return nil
	}
	fs := flag.NewFlagSet("commands", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *jsonOutput {
		return printJSON(commandManifest())
	}
	printCommandTable(commandManifest().Commands)
	return nil
}
