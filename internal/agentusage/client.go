package agentusage

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// flexFloat decodes a JSON number that some endpoints occasionally send as a
// quoted string (e.g. Codex credits.balance). A null or empty value leaves it
// nil so callers can distinguish "absent" from "zero".
type flexFloat struct {
	Value *float64
}

func (f *flexFloat) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return err
		}
		f.Value = &v
		return nil
	}
	var v float64
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	f.Value = &v
	return nil
}

// httpClient is shared across providers. The usage endpoints are external and
// occasionally slow, so we use a generous-but-bounded timeout.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// parseISOTime parses an RFC3339 / ISO-8601 timestamp, returning nil on empty
// or unparseable input.
func parseISOTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return &t
	}
	return nil
}

// snippet trims an error body to a short single line for inclusion in messages.
func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// jwtClaims pulls a few well-known claims out of an unverified JWT payload.
// We only read the token for display fields (email / plan); we never trust it
// for auth decisions, so signature verification is intentionally skipped.
func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil
	}
	return claims
}

// jwtString returns a string-valued claim, descending one level into nested
// objects (e.g. the "https://api.openai.com/auth" namespace) when needed.
func jwtString(claims map[string]any, key string) string {
	if claims == nil {
		return ""
	}
	if v, ok := claims[key].(string); ok && v != "" {
		return v
	}
	for _, v := range claims {
		if nested, ok := v.(map[string]any); ok {
			if s, ok := nested[key].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
