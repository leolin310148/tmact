package agentusage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	claudeUsageURL = "https://api.anthropic.com/api/oauth/usage"
	// claudeOAuthBeta is required by the OAuth usage endpoint.
	claudeOAuthBeta = "oauth-2025-04-20"
	// claudeUserAgent mirrors the Claude Code client identity the endpoint
	// expects. Anthropic does not gate on the exact version, so a stable
	// fallback is fine.
	claudeUserAgent = "claude-code/2.1.0"
)

// claudeCredentials is the shape stored under "claudeAiOauth" in Claude Code's
// credential store (file or macOS keychain).
type claudeCredentials struct {
	AccessToken      string `json:"accessToken"`
	ExpiresAt        int64  `json:"expiresAt"` // epoch milliseconds
	SubscriptionType string `json:"subscriptionType"`
}

type claudeCredentialsFile struct {
	ClaudeAiOauth claudeCredentials `json:"claudeAiOauth"`
}

// claudeUsageResponse mirrors the /api/oauth/usage payload. Each window is
// optional; absent windows stay nil.
type claudeUsageResponse struct {
	FiveHour       *claudeWindow            `json:"five_hour"`
	SevenDay       *claudeWindow            `json:"seven_day"`
	SevenDayOpus   *claudeWindow            `json:"seven_day_opus"`
	SevenDaySonnet *claudeWindow            `json:"seven_day_sonnet"`
	Limits         []claudeLimit            `json:"limits"`
	ExtraUsage     *claudeExtraUsed         `json:"extra_usage"`
	SevenDayModels map[string]*claudeWindow `json:"-"`
}

type claudeWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

type claudeExtraUsed struct {
	IsEnabled    *bool    `json:"is_enabled"`
	MonthlyLimit *float64 `json:"monthly_limit"` // cents
	UsedCredits  *float64 `json:"used_credits"`  // cents
	Currency     string   `json:"currency"`
}

type claudeLimit struct {
	Group    string            `json:"group"`
	Kind     string            `json:"kind"`
	Percent  *float64          `json:"percent"`
	ResetsAt string            `json:"resets_at"`
	Scope    *claudeLimitScope `json:"scope"`
}

type claudeLimitScope struct {
	Model *claudeLimitModel `json:"model"`
}

type claudeLimitModel struct {
	ID          *string `json:"id"`
	DisplayName string  `json:"display_name"`
}

func (r *claudeUsageResponse) UnmarshalJSON(data []byte) error {
	type known claudeUsageResponse
	if err := json.Unmarshal(data, (*known)(r)); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, value := range raw {
		if !strings.HasPrefix(key, "seven_day_") || isKnownClaudeSevenDayWindow(key) {
			continue
		}
		var window claudeWindow
		if err := json.Unmarshal(value, &window); err != nil {
			return fmt.Errorf("decode %s: %w", key, err)
		}
		if window.Utilization == nil && window.ResetsAt == "" {
			continue
		}
		if r.SevenDayModels == nil {
			r.SevenDayModels = make(map[string]*claudeWindow)
		}
		r.SevenDayModels[strings.TrimPrefix(key, "seven_day_")] = &window
	}
	return nil
}

func isKnownClaudeSevenDayWindow(key string) bool {
	switch key {
	case "seven_day", "seven_day_opus", "seven_day_sonnet":
		return true
	default:
		return false
	}
}

// fetchClaude reads Claude Code's OAuth token and queries the usage endpoint.
func fetchClaude(ctx context.Context) ProviderUsage {
	out := ProviderUsage{Provider: "claude"}

	raw, err := claudeCredentialsJSON()
	if err != nil {
		out.Error = fmt.Sprintf("read credentials: %v", err)
		return out
	}
	if raw == "" {
		out.Error = "not logged in (no Claude Code credentials found)"
		return out
	}

	var file claudeCredentialsFile
	if err := json.Unmarshal([]byte(raw), &file); err != nil {
		out.Error = fmt.Sprintf("parse credentials: %v", err)
		return out
	}
	creds := file.ClaudeAiOauth
	if creds.AccessToken == "" {
		out.Error = "credentials missing access token"
		return out
	}
	out.Plan = creds.SubscriptionType
	if creds.ExpiresAt > 0 && time.UnixMilli(creds.ExpiresAt).Before(time.Now()) {
		out.Error = "access token expired (run `claude` to re-authenticate)"
		return out
	}

	resp, err := claudeUsageRequest(ctx, creds.AccessToken)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	applyClaudeUsage(&out, resp, time.Now())
	return out
}

func claudeUsageRequest(ctx context.Context, accessToken string) (*claudeUsageResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, claudeUsageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", claudeOAuthBeta)
	req.Header.Set("User-Agent", claudeUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("usage request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusOK:
		var parsed claudeUsageResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode usage: %w", err)
		}
		return &parsed, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("unauthorized (run `claude` to re-authenticate)")
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("rate limited by Anthropic; retry in a few minutes")
	default:
		return nil, fmt.Errorf("usage HTTP %d: %s", resp.StatusCode, snippet(body))
	}
}

func applyClaudeUsage(out *ProviderUsage, resp *claudeUsageResponse, now time.Time) {
	seen := map[string]bool{}
	add := func(name string, w *claudeWindow, windowMinutes int) {
		if w == nil || w.Utilization == nil {
			return
		}
		if seen[name] {
			return
		}
		seen[name] = true
		rw := RateWindow{Name: name, UsedPercent: *w.Utilization, WindowMinutes: windowMinutes}
		if t := parseISOTime(w.ResetsAt); t != nil {
			rw.ResetsAt = t
		}
		rw.Pace = computePace(rw.UsedPercent, rw.WindowMinutes, rw.ResetsAt, now)
		out.Windows = append(out.Windows, rw)
	}

	for _, limit := range resp.Limits {
		name, minutes, ok := claudeLimitWindow(limit)
		if !ok || limit.Percent == nil {
			continue
		}
		add(name, &claudeWindow{Utilization: limit.Percent, ResetsAt: limit.ResetsAt}, minutes)
	}

	add("session", resp.FiveHour, 300)
	add("weekly", resp.SevenDay, 10080)
	add("weekly_opus", resp.SevenDayOpus, 10080)
	add("weekly_sonnet", resp.SevenDaySonnet, 10080)
	if len(resp.SevenDayModels) > 0 {
		names := make([]string, 0, len(resp.SevenDayModels))
		for name := range resp.SevenDayModels {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			add("weekly_"+name, resp.SevenDayModels[name], 10080)
		}
	}

	if e := resp.ExtraUsage; e != nil && e.IsEnabled != nil && *e.IsEnabled {
		cost := &CostWindow{Enabled: true, Currency: e.Currency}
		if e.UsedCredits != nil {
			cost.Used = *e.UsedCredits / 100 // cents -> dollars
		}
		if e.MonthlyLimit != nil {
			cost.Limit = *e.MonthlyLimit / 100
		}
		out.Cost = cost
	}
}

func claudeLimitWindow(limit claudeLimit) (name string, windowMinutes int, ok bool) {
	switch limit.Kind {
	case "session":
		return "session", 300, true
	case "weekly_all":
		return "weekly", 10080, true
	case "weekly_scoped":
		if suffix := stableClaudeLimitSuffix(limit); suffix != "" {
			return "weekly_" + suffix, 10080, true
		}
		return "weekly_scoped", 10080, true
	}

	switch limit.Group {
	case "session":
		return "session", 300, true
	case "weekly":
		return "weekly", 10080, true
	default:
		return "", 0, false
	}
}

func stableClaudeLimitSuffix(limit claudeLimit) string {
	if limit.Scope == nil || limit.Scope.Model == nil {
		return ""
	}
	label := limit.Scope.Model.DisplayName
	if label == "" && limit.Scope.Model.ID != nil {
		label = *limit.Scope.Model.ID
	}
	return stableWindowSuffix(label)
}

func stableWindowSuffix(s string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case b.Len() > 0 && !lastUnderscore:
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.TrimSuffix(b.String(), "_")
}
