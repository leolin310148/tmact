package agentspend

import (
	"github.com/leolin310148/tmact/internal/sessionlog"
)

// claudeScanner prices normalized Claude Code session-log records. Provider
// path resolution and JSONL handling live in sessionlog so other commands can
// consume the same records without duplicating the log formats.
type claudeScanner struct{}

func (claudeScanner) provider() string { return string(sessionlog.ProviderClaude) }

func (claudeScanner) discover() []string {
	discovery, err := sessionlog.Discover(sessionlog.ProviderClaude)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(discovery.Sources))
	for _, source := range discovery.Sources {
		files = append(files, source.Path)
	}
	return files
}

func (claudeScanner) parseFile(path string) []pricedRow {
	var rows []pricedRow
	_, _ = sessionlog.Stream(sessionlog.Source{Provider: sessionlog.ProviderClaude, Path: path}, func(record sessionlog.Record) error {
		usage := record.Usage
		if record.ProviderEvent != "assistant" || usage == nil || record.Timestamp.IsZero() {
			return nil
		}
		model := record.Model
		if model == "" || model == "<synthetic>" {
			return nil
		}
		cost, ok := calculateCost(
			model,
			usage.InputTokens,
			usage.OutputTokens,
			usage.CacheCreationInputTokens,
			usage.CacheReadInputTokens,
			usage.WebSearchRequests,
			usage.Speed,
			usage.Ephemeral1hInputTokens,
		)
		if !ok {
			return nil
		}
		dedup := record.ID
		if dedup == "" {
			dedup = "claude:" + record.TimestampText
		}
		rows = append(rows, pricedRow{ts: record.Timestamp, cost: cost, dedup: dedup})
		return nil
	})
	return rows
}
