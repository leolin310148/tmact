// Package agentspend computes the dollar-equivalent token spend for the local
// AI coding agents tmact drives (currently Claude and Codex). It reads each
// agent's on-disk session logs, sums the token usage, and prices every call
// against a vendored LiteLLM snapshot — the same approach as the `codeburn`
// tool (https://github.com/getagentseal/codeburn, MIT). The result is the
// "what these tokens would cost at API rates" figure, independent of whatever
// flat-rate plan the user is actually billed under.
//
// This file is the pricing core: a Go port of codeburn's models.ts
// (calculateCost + the model-name alias/canonicalization logic). The pricing
// data in litellm-snapshot.json is vendored verbatim from codeburn's
// src/data/litellm-snapshot.json.
package agentspend

import (
	_ "embed"
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed litellm-snapshot.json
var snapshotJSON []byte

// modelCosts is the per-token pricing for one model, in dollars.
type modelCosts struct {
	inputPerToken      float64
	outputPerToken     float64
	cacheWritePerToken float64
	cacheReadPerToken  float64
	webSearchPerReq    float64
	fastMultiplier     float64
}

const (
	webSearchCost = 0.01
	// A 1h cache write costs 1.6x the 5m rate (codeburn:
	// ONE_HOUR_CACHE_WRITE_MULTIPLIER_FROM_FIVE_MINUTE_RATE).
	oneHourCacheWriteMultiplier = 1.6
)

// fastMultipliers mirrors codeburn's FAST_MULTIPLIERS. The "fast" service tier
// is billed at a higher rate; older Opus 4.x fast tiers are priced at 6x.
var fastMultipliers = map[string]float64{
	"claude-opus-4-7": 6,
	"claude-opus-4-6": 6,
}

// manualPricing contains newly released models that are not yet present in the
// vendored LiteLLM snapshot. Rates are dollars per token.
var manualPricing = map[string]modelCosts{
	"claude-opus-4-8": {
		inputPerToken:      5e-6,
		outputPerToken:     25e-6,
		cacheWritePerToken: 6.25e-6,
		cacheReadPerToken:  0.5e-6,
		webSearchPerReq:    webSearchCost,
		fastMultiplier:     2,
	},
	"anthropic.claude-opus-4-8": {
		inputPerToken:      5e-6,
		outputPerToken:     25e-6,
		cacheWritePerToken: 6.25e-6,
		cacheReadPerToken:  0.5e-6,
		webSearchPerReq:    webSearchCost,
		fastMultiplier:     2,
	},
	"claude-fable-5": {
		inputPerToken:      10e-6,
		outputPerToken:     50e-6,
		cacheWritePerToken: 12.5e-6,
		cacheReadPerToken:  1e-6,
		webSearchPerReq:    webSearchCost,
	},
	"anthropic.claude-fable-5": {
		inputPerToken:      10e-6,
		outputPerToken:     50e-6,
		cacheWritePerToken: 12.5e-6,
		cacheReadPerToken:  1e-6,
		webSearchPerReq:    webSearchCost,
	},
}

// builtinAliases maps the many model-name variants emitted by agent logs onto
// canonical snapshot keys. Ported from codeburn's BUILTIN_ALIASES, trimmed to
// the Claude/Codex families tmact scans, plus the fresh entries the vendored
// snapshot predates.
//
// Claude dotted display names are normalized onto the hyphenated snapshot or
// manual-pricing keys before longest-prefix fallback runs.
var builtinAliases = map[string]string{
	"claude-opus-4.8":   "claude-opus-4-8",
	"claude-opus-4.7":   "claude-opus-4-7",
	"claude-opus-4.6":   "claude-opus-4-6",
	"claude-opus-4.5":   "claude-opus-4-5",
	"claude-sonnet-4.6": "claude-sonnet-4-6",
	"claude-sonnet-4.5": "claude-sonnet-4-5",
	"claude-haiku-4.5":  "claude-haiku-4-5",
	// Codex emits these display-name forms in some logs.
	"gpt-5.3-codex":      "gpt-5.3-codex",
	"gpt-5.1-codex-high": "gpt-5.3-codex",
}

var (
	pricing      map[string]modelCosts
	pricingKeys  []string // sorted longest-first for prefix matching
	pricingOnce  sync.Once
	atSuffixRe   = regexp.MustCompile(`@.*$`)
	dateSuffixRe = regexp.MustCompile(`-\d{8}$`)
	providerPfx  = regexp.MustCompile(`^[^/]+/`)
)

// snapshotEntry is the [input, output, cacheWrite|null, cacheRead|null] tuple
// shape used by the vendored snapshot.
type snapshotEntry [4]*float64

func loadPricing() {
	pricingOnce.Do(func() {
		raw := map[string]snapshotEntry{}
		if err := json.Unmarshal(snapshotJSON, &raw); err != nil {
			// A corrupt embed is a build-time problem; fail soft to an empty
			// table so unknown models simply price at $0 rather than panicking
			// the daemon.
			pricing = map[string]modelCosts{}
			return
		}
		pricing = make(map[string]modelCosts, len(raw))
		for name, e := range raw {
			if e[0] == nil || e[1] == nil {
				continue
			}
			input := *e[0]
			output := *e[1]
			cacheWrite := input * 1.25
			if e[2] != nil {
				cacheWrite = *e[2]
			}
			cacheRead := input * 0.1
			if e[3] != nil {
				cacheRead = *e[3]
			}
			pricing[name] = modelCosts{
				inputPerToken:      input,
				outputPerToken:     output,
				cacheWritePerToken: cacheWrite,
				cacheReadPerToken:  cacheRead,
				webSearchPerReq:    webSearchCost,
				fastMultiplier:     fastMultipliers[name], // 0 → treated as 1 below
			}
		}
		for name, costs := range manualPricing {
			pricing[name] = costs
		}
		pricingKeys = make([]string, 0, len(pricing))
		for k := range pricing {
			pricingKeys = append(pricingKeys, k)
		}
		sort.Slice(pricingKeys, func(i, j int) bool {
			return len(pricingKeys[i]) > len(pricingKeys[j])
		})
	})
}

func resolveAlias(model string) string {
	if v, ok := builtinAliases[model]; ok {
		return v
	}
	return model
}

// canonicalName strips pins, date suffixes, and provider prefixes:
// claude-sonnet-4-6@20250929 → claude-sonnet-4-6,
// claude-sonnet-4-20250514   → claude-sonnet-4,
// anthropic/foo              → foo.
func canonicalName(model string) string {
	s := atSuffixRe.ReplaceAllString(model, "")
	s = dateSuffixRe.ReplaceAllString(s, "")
	s = providerPfx.ReplaceAllString(s, "")
	return s
}

func getModelCosts(model string) (modelCosts, bool) {
	loadPricing()

	// Try with provider prefix preserved (azure/gpt-5.4, openrouter/anthropic/…).
	withPrefix := dateSuffixRe.ReplaceAllString(atSuffixRe.ReplaceAllString(model, ""), "")
	if c, ok := pricing[withPrefix]; ok {
		return c, true
	}

	canonical := resolveAlias(canonicalName(model))
	if c, ok := pricing[canonical]; ok {
		return c, true
	}

	// Longest-first prefix match so gpt-5-mini matches gpt-5-mini, not gpt-5.
	for _, key := range pricingKeys {
		if canonical == key || strings.HasPrefix(canonical, key+"-") {
			return pricing[key], true
		}
	}
	return modelCosts{}, false
}

// safeRate clamps a token count to a sane non-negative finite value. A corrupt
// log emitting a negative or NaN count would otherwise subtract from totals.
func safeCount(n int) float64 {
	f := float64(n)
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0
	}
	return f
}

// calculateCost prices one call. Mirrors codeburn's calculateCost: input +
// output + (5m cache write) + (1h cache write × 1.6) + cache read + web search,
// all scaled by the fast-tier multiplier when speed == "fast". Unknown models
// price at $0 (ok=false).
func calculateCost(
	model string,
	input, output, cacheCreation, cacheRead, webSearch int,
	speed string,
	oneHourCacheCreation int,
) (float64, bool) {
	costs, ok := getModelCosts(model)
	if !ok {
		return 0, false
	}

	multiplier := 1.0
	if speed == "fast" && costs.fastMultiplier > 0 {
		multiplier = costs.fastMultiplier
	}

	oneHour := safeCount(oneHourCacheCreation)
	totalCacheCreation := math.Max(safeCount(cacheCreation), oneHour)
	fiveMinCacheCreation := math.Max(0, totalCacheCreation-oneHour)

	cost := multiplier * (safeCount(input)*costs.inputPerToken +
		safeCount(output)*costs.outputPerToken +
		fiveMinCacheCreation*costs.cacheWritePerToken +
		oneHour*costs.cacheWritePerToken*oneHourCacheWriteMultiplier +
		safeCount(cacheRead)*costs.cacheReadPerToken +
		safeCount(webSearch)*costs.webSearchPerReq)
	return cost, true
}
