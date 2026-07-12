package agentusage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	codexDefaultBaseURL = "https://chatgpt.com/backend-api"
	codexBackendPath    = "/wham/usage"
	codexAltPath        = "/api/codex/usage"
)

// codexAuthFile mirrors ~/.codex/auth.json.
type codexAuthFile struct {
	Tokens struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
}

// codexUsageResponse mirrors the /wham/usage payload.
type codexUsageResponse struct {
	PlanType  string `json:"plan_type"`
	RateLimit struct {
		PrimaryWindow   *codexWindow `json:"primary_window"`
		SecondaryWindow *codexWindow `json:"secondary_window"`
	} `json:"rate_limit"`
	Credits *codexCredits `json:"credits"`
}

type codexWindow struct {
	UsedPercent        *float64 `json:"used_percent"`
	ResetAt            *int64   `json:"reset_at"` // epoch seconds
	LimitWindowSeconds int      `json:"limit_window_seconds"`
}

type codexCredits struct {
	HasCredits bool      `json:"has_credits"`
	Unlimited  bool      `json:"unlimited"`
	Balance    flexFloat `json:"balance"`
}

// fetchCodex reads ~/.codex/auth.json and queries the usage endpoint.
func fetchCodex(ctx context.Context) ProviderUsage {
	out := ProviderUsage{Provider: "codex"}

	home := codexHome()
	if home == "" {
		out.Error = "cannot resolve Codex home directory"
		return out
	}
	data, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if err != nil {
		if os.IsNotExist(err) {
			out.Error = "not logged in (no ~/.codex/auth.json)"
		} else {
			out.Error = fmt.Sprintf("read auth.json: %v", err)
		}
		return out
	}
	var auth codexAuthFile
	if err := json.Unmarshal(data, &auth); err != nil {
		out.Error = fmt.Sprintf("parse auth.json: %v", err)
		return out
	}
	if auth.Tokens.AccessToken == "" {
		out.Error = "auth.json missing access token"
		return out
	}

	// Identity / plan come from the id_token JWT when present.
	if claims := jwtClaims(auth.Tokens.IDToken); claims != nil {
		out.Account = jwtString(claims, "email")
		out.Plan = jwtString(claims, "chatgpt_plan_type")
	}
	if out.Account == "" {
		out.Account = auth.Tokens.AccountID
	}

	resp, err := codexUsageRequest(ctx, auth.Tokens.AccessToken, auth.Tokens.AccountID, home)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	applyCodexUsage(&out, resp, time.Now())
	return out
}

func codexUsageRequest(ctx context.Context, accessToken, accountID, home string) (*codexUsageResponse, error) {
	base := codexBaseURL(home)
	path := codexAltPath
	if strings.Contains(base, "/backend-api") {
		path = codexBackendPath
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "tmact")
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("usage request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		var parsed codexUsageResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode usage: %w", err)
		}
		return &parsed, nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("unauthorized (run `codex` to re-authenticate)")
	default:
		return nil, fmt.Errorf("usage HTTP %d: %s", resp.StatusCode, snippet(body))
	}
}

// codexBaseURL resolves the API base, honoring chatgpt_base_url in
// $CODEX_HOME/config.toml, and normalizing chatgpt.com hosts onto /backend-api.
func codexBaseURL(home string) string {
	base := codexDefaultBaseURL
	if custom := codexConfigBaseURL(home); custom != "" {
		base = custom
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return codexDefaultBaseURL
	}
	if (strings.HasPrefix(base, "https://chatgpt.com") ||
		strings.HasPrefix(base, "https://chat.openai.com")) &&
		!strings.Contains(base, "/backend-api") {
		base += "/backend-api"
	}
	return base
}

// codexConfigBaseURL extracts chatgpt_base_url from config.toml without a TOML
// dependency (the value is a simple key = "value" line).
func codexConfigBaseURL(home string) string {
	data, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) != "chatgpt_base_url" {
			continue
		}
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		return strings.TrimSpace(val)
	}
	return ""
}

func applyCodexUsage(out *ProviderUsage, resp *codexUsageResponse, now time.Time) {
	if out.Plan == "" {
		out.Plan = resp.PlanType
	}
	add := func(fallbackName string, w *codexWindow) {
		if w == nil || w.UsedPercent == nil {
			return
		}
		rw := RateWindow{
			Name:          codexWindowName(fallbackName, w.LimitWindowSeconds),
			UsedPercent:   *w.UsedPercent,
			WindowMinutes: w.LimitWindowSeconds / 60,
		}
		if w.ResetAt != nil {
			t := time.Unix(*w.ResetAt, 0).UTC()
			rw.ResetsAt = &t
		}
		rw.Pace = computePace(rw.UsedPercent, rw.WindowMinutes, rw.ResetsAt, now)
		out.Windows = append(out.Windows, rw)
	}
	add("session", resp.RateLimit.PrimaryWindow)
	add("weekly", resp.RateLimit.SecondaryWindow)

	if c := resp.Credits; c != nil && c.HasCredits {
		cost := &CostWindow{Enabled: true, Unlimited: c.Unlimited}
		if c.Balance.Value != nil {
			cost.Used = *c.Balance.Value
		}
		out.Cost = cost
	}
}

// codexWindowName derives the semantic window name from its duration instead
// of assuming the API's primary window is always the five-hour session window.
// OpenAI can temporarily omit the five-hour limit and return the seven-day
// window as primary; retaining positional names would then label weekly quota
// as session quota. Unknown durations keep the API-position fallback so future
// shapes remain visible rather than being discarded.
func codexWindowName(fallback string, windowSeconds int) string {
	switch windowSeconds {
	case 5 * 60 * 60:
		return "session"
	case 7 * 24 * 60 * 60:
		return "weekly"
	default:
		return fallback
	}
}
