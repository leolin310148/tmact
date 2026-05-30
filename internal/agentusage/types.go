// Package agentusage fetches quota / rate-limit usage for the AI coding agents
// tmact drives (currently Claude and Codex), reading each agent's local OAuth
// credentials and querying the provider's usage endpoint.
//
// The data here is read-only: tmact never refreshes or rewrites the agents'
// credential files. An expired or missing credential surfaces as a per-provider
// Error in the snapshot rather than a hard failure, so one broken provider never
// hides the others.
package agentusage

import "time"

// Snapshot is the result of fetching usage for one or more providers.
type Snapshot struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Providers   []ProviderUsage `json:"providers"`
}

// ProviderUsage is one agent's usage. On failure Windows/Cost are nil and Error
// carries a human-readable reason.
type ProviderUsage struct {
	Provider string `json:"provider"` // "claude" | "codex"
	// Account is an email or account identifier when the provider exposes one.
	Account string `json:"account,omitempty"`
	// Plan is the subscription tier (e.g. "max", "pro", "plus", "team").
	Plan string `json:"plan,omitempty"`
	// Windows are the rolling rate-limit windows, ordered most-significant first
	// (session window before weekly windows).
	Windows []RateWindow `json:"windows,omitempty"`
	// Cost is the metered/extra-usage spend window when the plan exposes one.
	Cost *CostWindow `json:"cost,omitempty"`
	// Spend is the dollar-equivalent token spend computed locally from this
	// agent's on-disk session logs (LiteLLM-priced, like codeburn) — what the
	// tokens would cost at API rates, independent of the flat-rate plan. Nil
	// when the provider has no local logs or scanning is unsupported.
	Spend *SpendWindow `json:"spend,omitempty"`
	Error string       `json:"error,omitempty"`
}

// SpendWindow is the locally-computed token spend over the current calendar
// week-to-date and month-to-date, in USD.
type SpendWindow struct {
	WeekUSD  float64 `json:"week_usd"`
	MonthUSD float64 `json:"month_usd"`
}

// RateWindow is a single rolling usage window.
type RateWindow struct {
	// Name is a stable key for the window: "session", "weekly", "weekly_opus",
	// "weekly_sonnet", etc.
	Name string `json:"name"`
	// UsedPercent is 0..100 (may exceed 100 when over an allowance).
	UsedPercent float64 `json:"used_percent"`
	// WindowMinutes is the window length when known (300 = 5h, 10080 = 7d).
	WindowMinutes int `json:"window_minutes,omitempty"`
	// ResetsAt is when the window rolls over, when known.
	ResetsAt *time.Time `json:"resets_at,omitempty"`
	// Pace is the leading/lagging assessment (consumed faster or slower than a
	// linear pace), present only when ResetsAt and WindowMinutes are known.
	Pace *Pace `json:"pace,omitempty"`
}

// CostWindow describes a metered spend allowance (Claude "extra usage",
// Codex credits).
type CostWindow struct {
	Enabled bool `json:"enabled"`
	// Used and Limit are in major currency units (dollars), already converted
	// from any cents-based source field.
	Used     float64 `json:"used"`
	Limit    float64 `json:"limit"`
	Currency string  `json:"currency,omitempty"`
	// Unlimited marks plans with no spend cap.
	Unlimited bool `json:"unlimited,omitempty"`
}
