package agentspend

import (
	"math"
	"testing"
)

func approx(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s: got %.12f want %.12f", label, got, want)
	}
}

func TestOpus5ManualPricing(t *testing.T) {
	cost, ok := calculateCost("claude-opus-5", 1_000_000, 1_000_000, 2_000_000, 1_000_000, 0, "standard", 1_000_000)
	if !ok {
		t.Fatal("claude-opus-5 unpriced")
	}
	// input $5 + output $25 + 5m cache write $6.25 + 1h cache write $10 + read $0.50.
	approx(t, "claude opus 5 line", cost, 46.75)
}

func TestOpus5ProviderPrefixPricing(t *testing.T) {
	cost, ok := calculateCost("anthropic.claude-opus-5-20260724-v1:0", 1_000_000, 0, 0, 0, 0, "standard", 0)
	if !ok {
		t.Fatal("anthropic claude opus 5 unpriced")
	}
	approx(t, "anthropic claude opus 5 input", cost, 5)
}

func TestOpus5FastMultiplier(t *testing.T) {
	standard, _ := calculateCost("claude-opus-5", 1_000, 0, 0, 0, 0, "standard", 0)
	fast, _ := calculateCost("claude-opus-5", 1_000, 0, 0, 0, 0, "fast", 0)
	approx(t, "claude opus 5 fast 2x", fast, standard*2)
}

// Validates the load-bearing manual entry: claude-opus-4-8 is absent from the
// snapshot and must price at $5/$25 per Mtok, NOT the longest-prefix fallback
// claude-opus-4 ($15/$75).
func TestOpus48ManualPricing(t *testing.T) {
	c46, ok := getModelCosts("claude-opus-4-6")
	if !ok {
		t.Fatal("claude-opus-4-6 missing from snapshot")
	}
	c48, ok := getModelCosts("claude-opus-4-8")
	if !ok {
		t.Fatal("claude-opus-4-8 did not resolve")
	}
	if c48.inputPerToken != c46.inputPerToken || c48.outputPerToken != c46.outputPerToken {
		t.Fatalf("opus-4-8 priced as %v, expected opus-4-6 %v", c48, c46)
	}
	if c48.inputPerToken != 5e-6 {
		t.Fatalf("opus-4-8 input rate %.2e, expected 5e-6 (got opus-4 base?)", c48.inputPerToken)
	}
	if c48.fastMultiplier != 2 {
		t.Fatalf("opus-4-8 fast multiplier = %v, want 2", c48.fastMultiplier)
	}
}

func TestOpus48ProviderPrefixPricing(t *testing.T) {
	cost, ok := calculateCost("anthropic.claude-opus-4-8-20260528-v1:0", 1_000_000, 0, 0, 0, 0, "standard", 0)
	if !ok {
		t.Fatal("unpriced")
	}
	approx(t, "anthropic claude opus 4.8 input", cost, 5)
}

// Hand-computed from a real Claude assistant usage record:
// model=claude-opus-4-8, input=6159, cache_creation=4962 (all 1h),
// cache_read=16529, output=193, speed=standard. Priced at opus-4-6 rates
// [input 5e-6, output 2.5e-5, cacheWrite 6.25e-6, cacheRead 5e-7]:
//
//	input  6159  * 5e-6            = 0.0307950
//	output 193   * 2.5e-5          = 0.0048250
//	1h cw  4962  * 6.25e-6 * 1.6   = 0.0496200
//	read   16529 * 5e-7            = 0.0082645
//	                          total = 0.0935045
func TestClaudeKnownLineCost(t *testing.T) {
	cost, ok := calculateCost("claude-opus-4-8", 6159, 193, 4962, 16529, 0, "standard", 4962)
	if !ok {
		t.Fatal("unpriced")
	}
	approx(t, "claude opus-4-8 line", cost, 0.0935045)
}

func TestClaudeFable5Pricing(t *testing.T) {
	cost, ok := calculateCost("claude-fable-5", 1_000_000, 1_000_000, 2_000_000, 1_000_000, 0, "standard", 1_000_000)
	if !ok {
		t.Fatal("unpriced")
	}
	// input $10 + output $50 + 5m cache write $12.50 + 1h cache write $20 + read $1.
	approx(t, "claude fable 5 line", cost, 93.5)
}

func TestClaudeFable5ProviderPrefixPricing(t *testing.T) {
	cost, ok := calculateCost("anthropic.claude-fable-5-20260610-v1:0", 1_000_000, 0, 0, 0, 0, "standard", 0)
	if !ok {
		t.Fatal("unpriced")
	}
	approx(t, "anthropic claude fable 5 input", cost, 10)
}

func TestClaudeFable51Pricing(t *testing.T) {
	cost, ok := calculateCost("claude-fable-5-1", 1_000_000, 1_000_000, 2_000_000, 1_000_000, 0, "standard", 1_000_000)
	if !ok {
		t.Fatal("unpriced")
	}
	// input $10 + output $50 + 5m cache write $12.50 + 1h cache write $20 + read $0.25.
	approx(t, "claude fable 5.1 line", cost, 92.75)
}

// claude-fable-5-1 must resolve to its own entry, not fall through the
// longest-prefix match onto claude-fable-5 (which has a different cache-read rate).
func TestClaudeFable51DoesNotFallBackToFable5(t *testing.T) {
	c51, ok := getModelCosts("claude-fable-5-1")
	if !ok {
		t.Fatal("claude-fable-5-1 did not resolve")
	}
	c5, _ := getModelCosts("claude-fable-5")
	if c51 == c5 {
		t.Fatalf("fable-5-1 priced identically to fable-5 %v; expected distinct cache-read rate", c51)
	}
	if c51.cacheReadPerToken != 0.25e-6 {
		t.Fatalf("fable-5-1 cache read rate %.2e, want 0.25e-6", c51.cacheReadPerToken)
	}
}

func TestClaudeFable51DottedAliasPricing(t *testing.T) {
	cost, ok := calculateCost("claude-fable-5.1", 0, 0, 0, 1_000_000, 0, "standard", 0)
	if !ok {
		t.Fatal("unpriced")
	}
	approx(t, "claude fable 5.1 dotted cache read", cost, 0.25)
}

func TestClaudeFable51ProviderPrefixPricing(t *testing.T) {
	cost, ok := calculateCost("anthropic.claude-fable-5-1-20260901-v1:0", 1_000_000, 0, 0, 0, 0, "standard", 0)
	if !ok {
		t.Fatal("unpriced")
	}
	approx(t, "anthropic claude fable 5.1 input", cost, 10)
}

// Codex normalizes cached into cacheRead. Reasoning is already included in
// output_tokens and must not be added a second time by the log parser.
// gpt-5.3-codex = [input 1.75e-6, output 1.4e-5, cacheWrite nil, cacheRead 1.75e-7].
// normalized uncachedInput=800, output=600, cacheRead=200:
//
//	800*1.75e-6 + 600*1.4e-5 + 200*1.75e-7 = 0.0014 + 0.0084 + 0.000035 = 0.009835
func TestCodexKnownLineCost(t *testing.T) {
	cost, ok := calculateCost("gpt-5.3-codex", 800, 600, 0, 200, 0, "standard", 0)
	if !ok {
		t.Fatal("unpriced")
	}
	approx(t, "codex gpt-5.3 line", cost, 0.009835)
}

func TestCodexTokenCostDoesNotDoubleCountReasoning(t *testing.T) {
	cost, ok := calculateCodexTokenCost("gpt-5.3-codex", 1000, 200, 500)
	if !ok {
		t.Fatal("unpriced")
	}
	approx(t, "codex output includes reasoning", cost, 0.008435)
}

func TestGPT6AstraPricing(t *testing.T) {
	for _, model := range []string{"gpt-6-astra", "openai/gpt-6-astra", "gpt-6-astra@20260907", "gpt-6-astra-20260907"} {
		t.Run(model, func(t *testing.T) {
			for _, speed := range []string{"standard", "fast"} {
				cost, ok := calculateCost(model, 1_000_000, 1_000_000, 1_000_000, 1_000_000, 0, speed, 0)
				if !ok {
					t.Fatal("gpt-6-astra unpriced")
				}
				// Input $10 + output $50 + cache write $12.50 + cache read $1.
				want := 73.5
				if speed == "fast" {
					want = 147
				}
				approx(t, speed, cost, want)
			}
			for _, tc := range []struct {
				input int
				want  float64
			}{
				{271_999, 1.41999},
				{272_000, 1.42},
				// All input (including cached) is 2x; output is 1.5x.
				{272_001, 2.59002},
			} {
				cost, ok := calculateCodexTokenCost(model, tc.input, 200_000, 10_000)
				if !ok {
					t.Fatal("gpt-6-astra unpriced")
				}
				approx(t, "Codex long-context boundary", cost, tc.want)
			}
		})
	}
}

func TestCodexGPT56LongContextPricing(t *testing.T) {
	standard, ok := calculateCodexTokenCost("gpt-5.6-sol", 272_000, 200_000, 10_000)
	if !ok {
		t.Fatal("gpt-5.6-sol unpriced")
	}
	// input: 72K*$5/M + 200K*$0.50/M = $0.46; output: 10K*$30/M = $0.30.
	approx(t, "gpt-5.6 threshold is standard", standard, 0.76)

	long, ok := calculateCodexTokenCost("gpt-5.6-sol", 272_001, 200_000, 10_000)
	if !ok {
		t.Fatal("gpt-5.6-sol unpriced")
	}
	// Above 272K, all input is 2x and output is 1.5x.
	approx(t, "gpt-5.6 long context", long, 2*(72_001*5e-6+200_000*0.5e-6)+1.5*(10_000*30e-6))
}

func TestGPT6SolPricing(t *testing.T) {
	for _, tc := range []struct {
		model     string
		cacheRead float64
	}{
		{"gpt-6-sol", 0.2},
		{"gpt-6.1-sol", 0.1},
	} {
		t.Run(tc.model, func(t *testing.T) {
			cost, ok := calculateCost(tc.model, 1_000_000, 1_000_000, 1_000_000, 1_000_000, 0, "standard", 0)
			if !ok {
				t.Fatal("unpriced")
			}
			// Input $2 + output $10 + cache write $2.50 + cache read.
			approx(t, "standard", cost, 14.5+tc.cacheRead)

			long, ok := calculateCodexTokenCost(tc.model, 272_001, 200_000, 10_000)
			if !ok {
				t.Fatal("unpriced")
			}
			// Above 272K, all input is 2x and output is 1.5x.
			approx(t, "long context", long, 2*(72_001*2e-6+200_000*tc.cacheRead*1e-6)+1.5*(10_000*10e-6))
		})
	}
}

// Claude Code's [1m] context tag and bare family aliases must price as the
// current model, not fall back to an older prefix or $0.
func TestClaudeAliasAndContextTagPricing(t *testing.T) {
	want, _ := calculateCost("claude-opus-5-5", 1000, 1000, 0, 0, 0, "standard", 0)
	got, ok := calculateCost("claude-opus-5-5[1m]", 1000, 1000, 0, 0, 0, "standard", 0)
	if !ok {
		t.Fatal("claude-opus-5-5[1m] unpriced")
	}
	approx(t, "[1m] prices as opus 5.5", got, want)

	for alias, model := range map[string]string{"opus": "claude-opus-5-5", "sonnet": "claude-sonnet-5-5", "haiku": "claude-haiku-5-5"} {
		want, _ := calculateCost(model, 1000, 1000, 0, 0, 0, "standard", 0)
		got, ok := calculateCost(alias, 1000, 1000, 0, 0, 0, "standard", 0)
		if !ok {
			t.Fatalf("%s unpriced", alias)
		}
		approx(t, alias, got, want)
	}
}

// Opus 4.8 fast tier is billed at 2x standard. Older 4.6/4.7 fast tiers remain
// at 6x through the vendored codeburn multiplier table.
func TestFastMultiplier(t *testing.T) {
	std, _ := calculateCost("claude-opus-4-8", 1000, 0, 0, 0, 0, "standard", 0)
	fast, _ := calculateCost("claude-opus-4-8", 1000, 0, 0, 0, 0, "fast", 0)
	approx(t, "opus 4.8 fast 2x", fast, std*2)

	std46, _ := calculateCost("claude-opus-4-6", 1000, 0, 0, 0, 0, "standard", 0)
	fast46, _ := calculateCost("claude-opus-4-6", 1000, 0, 0, 0, 0, "fast", 0)
	approx(t, "opus 4.6 fast 6x", fast46, std46*6)
}

func TestUnknownModelIsFree(t *testing.T) {
	if cost, ok := calculateCost("some-local-llama:7b-q4", 1000, 1000, 0, 0, 0, "standard", 0); ok || cost != 0 {
		t.Fatalf("unknown model should be $0/unpriced, got cost=%v ok=%v", cost, ok)
	}
}

// claude-opus-5-5 must not fall back to the claude-opus-5 prefix ($5/$25).
func TestOpus55ManualPricing(t *testing.T) {
	cost, ok := calculateCost("claude-opus-5-5", 1_000_000, 1_000_000, 0, 1_000_000, 0, "standard", 0)
	if !ok {
		t.Fatal("claude-opus-5-5 unpriced")
	}
	// input $4 + output $20 + read $0.20.
	approx(t, "claude opus 5.5 line", cost, 24.2)
	fast, _ := calculateCost("claude-opus-5-5", 1_000_000, 0, 0, 0, 0, "fast", 0)
	approx(t, "claude opus 5.5 fast", fast, 8)
	dotted, _ := calculateCost("claude-opus-5.5", 1_000_000, 0, 0, 0, 0, "standard", 0)
	approx(t, "claude opus 5.5 dotted alias", dotted, 4)
	bedrock, _ := calculateCost("anthropic.claude-opus-5-5-v1:0", 1_000_000, 0, 0, 0, 0, "standard", 0)
	approx(t, "anthropic claude opus 5.5", bedrock, 4)
}

func TestSonnet5FamilyPricing(t *testing.T) {
	for _, model := range []string{"claude-sonnet-5-5", "claude-sonnet-5"} {
		cost, ok := calculateCost(model, 1_000_000, 1_000_000, 1_000_000, 1_000_000, 0, "standard", 0)
		if !ok {
			t.Fatalf("%s unpriced", model)
		}
		// input $2 + output $10 + 5m cache write $2.50 + read $0.20.
		approx(t, model, cost, 14.7)
	}
}

func TestHaiku55LongContextTier(t *testing.T) {
	short, ok := calculateCost("claude-haiku-5-5", 50_000, 10_000, 0, 50_000, 0, "standard", 0)
	if !ok {
		t.Fatal("claude-haiku-5-5 unpriced")
	}
	// Exactly 100K prompt tokens stays on the base tier:
	// 50K*$0.10 + 10K*$0.50 + 50K*$0.01 per MTok.
	approx(t, "haiku 5.5 at threshold", short, 0.0105)
	long, _ := calculateCost("claude-haiku-5-5", 50_001, 10_000, 0, 50_000, 0, "standard", 0)
	approx(t, "haiku 5.5 above threshold", long, (50_001*0.1e-6+10_000*0.5e-6+50_000*0.01e-6)*5)
}
