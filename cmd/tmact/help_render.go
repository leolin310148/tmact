package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

func printCommandHelp(name string) error {
	help, ok := commandHelpFor(name)
	if !ok {
		return fmt.Errorf("unknown help topic %q", name)
	}
	fmt.Printf("%s\n\n%s\n", help.Command, help.Summary)
	if len(help.Usage) > 0 {
		fmt.Println("\nUsage:")
		for _, usage := range help.Usage {
			fmt.Printf("  %s\n", usage)
		}
	}
	if len(help.Subcommands) > 0 {
		fmt.Println("\nSubcommands:")
		for _, subcommand := range help.Subcommands {
			fmt.Printf("  %s\n", subcommand)
		}
	}
	if len(help.Flags) > 0 {
		fmt.Println("\nFlags:")
		writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, flag := range help.Flags {
			name := flag.Name
			if flag.Value != "" {
				name += " " + flag.Value
			}
			required := ""
			if flag.Required {
				required = " required"
			}
			fmt.Fprintf(writer, "  %s\t%s%s\n", name, flag.Description, required)
		}
		_ = writer.Flush()
	}
	if len(help.Examples) > 0 {
		fmt.Println("\nExamples:")
		for _, example := range help.Examples {
			fmt.Printf("  %s\n", example)
		}
	}
	if len(help.Safety) > 0 {
		fmt.Println("\nSafety:")
		for _, note := range help.Safety {
			fmt.Printf("  %s\n", note)
		}
	}
	if len(help.Notes) > 0 {
		fmt.Println("\nNotes:")
		for _, note := range help.Notes {
			fmt.Printf("  %s\n", note)
		}
	}
	return nil
}

func printCommandTable(commands []commandHelp) {
	writer := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "command\tsummary")
	for _, command := range commands {
		if strings.Contains(command.Command, " ") {
			continue
		}
		fmt.Fprintf(writer, "%s\t%s\n", command.Command, command.Summary)
	}
	_ = writer.Flush()
}
