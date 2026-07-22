package agentspend

import "github.com/leolin310148/tmact/internal/sessionlog"

// codexScanner prices normalized Codex token_count records. The cumulative
// baseline remains here because it is spend-specific rather than log parsing.
type codexScanner struct{}

func (codexScanner) provider() string { return string(sessionlog.ProviderCodex) }

func (codexScanner) discover() []string {
	discovery, err := sessionlog.Discover(sessionlog.ProviderCodex)
	if err != nil {
		return nil
	}
	files := make([]string, 0, len(discovery.Sources))
	for _, source := range discovery.Sources {
		files = append(files, source.Path)
	}
	return files
}

func (codexScanner) parseFile(path string) []pricedRow {
	havePrevCumulative := false
	prevCumulative := 0
	var prevInput, prevCached, prevOutput int
	var rows []pricedRow

	_, _ = sessionlog.Stream(sessionlog.Source{Provider: sessionlog.ProviderCodex, Path: path}, func(record sessionlog.Record) error {
		if record.Kind != sessionlog.KindUsage || record.TotalUsage == nil {
			return nil
		}
		total := record.TotalUsage
		cumulative := total.TotalTokens
		if havePrevCumulative && cumulative == prevCumulative {
			return nil
		}

		var input, cached, output int
		if last := record.Usage; last != nil {
			input, cached, output = last.InputTokens, last.CachedInputTokens, last.OutputTokens
		} else if cumulative > 0 {
			input = total.InputTokens - prevInput
			cached = total.CachedInputTokens - prevCached
			output = total.OutputTokens - prevOutput
		}

		prevInput, prevCached, prevOutput = total.InputTokens, total.CachedInputTokens, total.OutputTokens
		havePrevCumulative = true
		prevCumulative = cumulative
		if input+cached+output == 0 || record.Timestamp.IsZero() {
			return nil
		}

		model := record.Model
		if model == "" {
			model = "gpt-5"
		}
		cost, ok := calculateCodexTokenCost(model, input, cached, output)
		if !ok {
			return nil
		}
		rows = append(rows, pricedRow{
			ts:    record.Timestamp,
			cost:  cost,
			dedup: "codex:" + record.SessionID + ":" + itoa(cumulative),
		})
		return nil
	})
	return rows
}

const codexLongContextThreshold = 272_000

// calculateCodexTokenCost normalizes the Responses API usage shape before
// pricing. Cached tokens are included in input_tokens, while reasoning tokens
// are already included in output_tokens and must not be added a second time.
// Current 1.05M-context GPT families charge 2x input and 1.5x output when the
// request input exceeds 272K tokens.
func calculateCodexTokenCost(model string, input, cached, output int) (float64, bool) {
	uncachedInput := input - cached
	if uncachedInput < 0 {
		uncachedInput = 0
	}
	inputCost, ok := calculateCost(model, uncachedInput, 0, 0, cached, 0, "standard", 0)
	if !ok {
		return 0, false
	}
	outputCost, _ := calculateCost(model, 0, output, 0, 0, 0, "standard", 0)
	if input > codexLongContextThreshold && codexHasLongContextSurcharge(model) {
		return 2*inputCost + 1.5*outputCost, true
	}
	return inputCost + outputCost, true
}

func codexHasLongContextSurcharge(model string) bool {
	switch resolveAlias(canonicalName(model)) {
	case "gpt-5.4", "gpt-5.4-pro", "gpt-5.5", "gpt-5.5-pro",
		"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna":
		return true
	default:
		return false
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
