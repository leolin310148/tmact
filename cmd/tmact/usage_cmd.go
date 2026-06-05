package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/agentspend"
	"github.com/leolin310148/tmact/internal/agentusage"
	"github.com/leolin310148/tmact/internal/statusd"
	"github.com/leolin310148/tmact/internal/web"
)

func runUsage(args []string) error {
	if len(args) > 0 && args[0] == "serve" {
		return runUsageServe(args[1:])
	}
	if wantsHelp(args) {
		fmt.Printf(`Usage:
  tmact usage [--json] [--provider NAME]
  tmact usage serve [--cost-only] [--web-addr ADDR]

Fetch quota / rate-limit usage for the AI coding agents tmact drives, reading
each agent's local OAuth credentials and querying the provider's usage endpoint.
Read-only: tmact never refreshes or rewrites the agents' credentials.

Supported providers: %s

Flags:
  --provider NAME   Limit to one provider (repeatable). Default: all.
  --json            Emit machine-readable JSON.
`, strings.Join(agentusage.Providers(), ", "))
		return nil
	}

	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOutput := fs.Bool("json", false, "print JSON output")
	var providers repeatedStrings
	fs.Var(&providers, "provider", "limit to a provider (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	snapshot := agentusage.Fetch(ctx, providers...)
	// Fetch returns quota only; attach locally-computed token spend for display
	// (the web server refreshes the two on separate cadences — see agent_usage.go).
	spend := agentspend.Compute(time.Now())
	for i := range snapshot.Providers {
		if sp, ok := spend[snapshot.Providers[i].Provider]; ok {
			snapshot.Providers[i].Spend = &agentusage.SpendWindow{WeekUSD: sp.WeekUSD, MonthUSD: sp.MonthUSD}
		}
	}

	if *jsonOutput {
		return printJSON(snapshot)
	}
	printUsageTable(snapshot)
	return nil
}

func runUsageServe(args []string) error {
	if wantsHelp(args) {
		fmt.Printf(`Usage:
  tmact usage serve [--cost-only] [--web-addr ADDR]

Serve /api/agent-usage without starting statusd or touching tmux. This is useful
on machines that should contribute Claude/Codex token spend but do not have tmux,
such as Windows hosts.

Flags:
  --web-addr ADDR       TCP address to listen on. Default: %s.
  --cost-only           Disable quota fetches and serve token-spend only.
  --agent-usage         Enable quota/rate-limit fetches. Default: true unless --cost-only.
  --agent-cost          Enable token-spend computation. Default: true.
  --usage-interval D    Quota refresh interval. Default: web server default.
  --spend-interval D    Token-spend refresh interval. Default: web server default.
`, statusd.DefaultWebAddr)
		return nil
	}

	fs := flag.NewFlagSet("usage serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	webAddr := fs.String("web-addr", statusd.DefaultWebAddr, "serve /api/agent-usage on this address")
	costOnly := fs.Bool("cost-only", false, "serve token-spend only; do not fetch quota/rate-limit usage")
	agentUsage := fs.Bool("agent-usage", true, "fetch quota/rate-limit usage")
	agentCost := fs.Bool("agent-cost", true, "compute token-spend cost")
	usageInterval := fs.Duration("usage-interval", 0, "quota/rate-limit refresh interval")
	spendInterval := fs.Duration("spend-interval", 0, "token-spend refresh interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	usageEnabled := *agentUsage
	spendEnabled := *agentCost
	if *costOnly {
		usageEnabled = false
		spendEnabled = true
	}
	if !usageEnabled && !spendEnabled {
		return fmt.Errorf("at least one of --agent-usage or --agent-cost must be enabled")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	server := &web.Server{
		Addr:          *webAddr,
		BuildTime:     buildVersionInfo().Time,
		UsageEnabled:  usageEnabled,
		SpendEnabled:  spendEnabled,
		UsageInterval: *usageInterval,
		SpendInterval: *spendInterval,
	}
	fmt.Fprintf(os.Stderr, "usage server listening on %s\n", *webAddr)
	if *costOnly {
		fmt.Fprintln(os.Stderr, "usage server mode: cost-only")
	}
	return server.Serve(ctx)
}

func printUsageTable(snapshot agentusage.Snapshot) {
	for _, p := range snapshot.Providers {
		header := p.Provider
		if p.Plan != "" {
			header += fmt.Sprintf(" (%s)", p.Plan)
		}
		if p.Account != "" {
			header += " " + p.Account
		}
		fmt.Println(header)
		if p.Error != "" {
			fmt.Printf("  ! %s\n", p.Error)
			continue
		}
		if len(p.Windows) == 0 && p.Cost == nil && p.Spend == nil {
			fmt.Println("  (no usage data)")
			continue
		}
		for _, w := range p.Windows {
			line := fmt.Sprintf("  %-14s %5.1f%% used", w.Name, w.UsedPercent)
			if pace := formatPace(w.Pace); pace != "" {
				line += "  " + pace
			}
			if w.ResetsAt != nil {
				line += fmt.Sprintf("  resets %s", formatReset(*w.ResetsAt))
			}
			fmt.Println(line)
		}
		if c := p.Cost; c != nil && c.Enabled {
			switch {
			case c.Unlimited:
				fmt.Println("  extra usage    unlimited credits")
			case c.Limit > 0:
				fmt.Printf("  extra usage    $%.2f / $%.2f\n", c.Used, c.Limit)
			default:
				fmt.Printf("  extra usage    $%.2f\n", c.Used)
			}
		}
		if s := p.Spend; s != nil {
			fmt.Printf("  token spend    $%.2f wk · $%.2f mo (API-rate equiv)\n", s.WeekUSD, s.MonthUSD)
		}
	}
}

// formatPace renders the leading/lagging assessment as a short tag, e.g.
// "[ahead 12%, empty in 5h]" or "[reserve 8%]" or "[on pace]".
func formatPace(p *agentusage.Pace) string {
	if p == nil {
		return ""
	}
	delta := int(p.DeltaPercent + 0.5) // round
	var label string
	switch {
	case p.DeltaPercent > 2:
		label = fmt.Sprintf("ahead %d%%", delta)
	case p.DeltaPercent < -2:
		label = fmt.Sprintf("reserve %d%%", -delta)
	default:
		label = "on pace"
	}
	if !p.LastsUntilReset && p.ETASeconds != nil {
		label += ", empty in " + formatDuration(time.Duration(*p.ETASeconds)*time.Second)
	}
	return "[" + label + "]"
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}

func formatReset(t time.Time) string {
	d := time.Until(t)
	if d <= 0 {
		return "now"
	}
	if d < time.Hour {
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("in %dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("in %dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}
