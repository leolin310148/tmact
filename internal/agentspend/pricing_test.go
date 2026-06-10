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

// Validates the load-bearing alias: claude-opus-4-8 is absent from the
// snapshot and must resolve to claude-opus-4-6 pricing ($5/$25 per Mtok), NOT
// the longest-prefix fallback claude-opus-4 ($15/$75).
func TestOpus48AliasPricing(t *testing.T) {
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

// Codex normalizes cached into cacheRead and folds reasoning into output.
// gpt-5.3-codex = [input 1.75e-6, output 1.4e-5, cacheWrite nil, cacheRead 1.75e-7].
// uncachedInput=800, output+reasoning=600, cacheRead=200:
//
//	800*1.75e-6 + 600*1.4e-5 + 200*1.75e-7 = 0.0014 + 0.0084 + 0.000035 = 0.009835
func TestCodexKnownLineCost(t *testing.T) {
	cost, ok := calculateCost("gpt-5.3-codex", 800, 600, 0, 200, 0, "standard", 0)
	if !ok {
		t.Fatal("unpriced")
	}
	approx(t, "codex gpt-5.3 line", cost, 0.009835)
}

// fast tier on opus is billed at 6x standard.
func TestFastMultiplier(t *testing.T) {
	std, _ := calculateCost("claude-opus-4-8", 1000, 0, 0, 0, 0, "standard", 0)
	fast, _ := calculateCost("claude-opus-4-8", 1000, 0, 0, 0, 0, "fast", 0)
	approx(t, "fast 6x", fast, std*6)
}

func TestUnknownModelIsFree(t *testing.T) {
	if cost, ok := calculateCost("some-local-llama:7b-q4", 1000, 1000, 0, 0, 0, "standard", 0); ok || cost != 0 {
		t.Fatalf("unknown model should be $0/unpriced, got cost=%v ok=%v", cost, ok)
	}
}
