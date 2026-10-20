package modelroute

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Failure contains only allowlisted, safe diagnostics. Raw upstream text is
// classified in memory and never returned, wrapped, or persisted.
type Failure struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	Phase           string `json:"phase"`
	NoAgentLaunched bool   `json:"no_agent_launched"`
}

func (e *Failure) Error() string {
	return "auto-model: " + e.Code + ": " + e.Message + "; no agent launched"
}
func NewFailure(code string) *Failure {
	message := "decide is unavailable"
	switch code {
	case "context_too_large":
		message = "selection context exceeds the available request budget; shorten task or file excerpts"
	case "auth":
		message = "decide authentication failed; check its configured credentials"
	case "configuration":
		message = "decide configuration is invalid"
	case "timeout":
		message = "decide timed out"
	case "cancelled":
		message = "selection was cancelled"
	case "invalid_request":
		message = "decide rejected the selection request"
	case "invalid_response":
		message = "decide returned an invalid selection"
	case "deferred":
		message = "decide could not select a sufficient model"
	default:
		code = "unavailable"
	}
	return &Failure{Code: code, Message: message, Phase: "routing", NoAgentLaunched: true}
}
func FailureInfo(err error) *Failure {
	var f *Failure
	if errors.As(err, &f) {
		copy := *f
		return &copy
	}
	return nil
}
func ErrorCode(err error) string {
	if f := FailureInfo(err); f != nil {
		return f.Code
	}
	return ""
}

var upstreamHTTP = regexp.MustCompile(`HTTP\s+([0-9]{3})`)

func classifyDecideFailure(stderr string) *Failure {
	if at := strings.LastIndex(stderr, "\ndecide: "); at >= 0 {
		stderr = stderr[at+1:]
	}
	// Parse only the HTTP error envelope, never arbitrary nested task data.
	if m := upstreamHTTP.FindStringSubmatchIndex(stderr); m != nil {
		status, _ := strconv.Atoi(stderr[m[2]:m[3]])
		if status == 401 || status == 403 {
			return NewFailure("auth")
		}
		if status == 400 || status == 413 || status == 422 {
			body := strings.TrimLeft(stderr[m[1]:], ":： \t\r\n")
			var envelope struct {
				ErrorType string `json:"error_type"`
				Detail    struct {
					ErrorType string `json:"error_type"`
				} `json:"detail"`
			}
			if json.Unmarshal([]byte(strings.TrimSpace(body)), &envelope) == nil && (envelope.Detail.ErrorType == "max_tokens_exceeded" || envelope.ErrorType == "max_tokens_exceeded") {
				return NewFailure("context_too_large")
			}
			return NewFailure("invalid_request")
		}
		return NewFailure("unavailable")
	}
	text := strings.ToLower(stderr)
	if strings.Contains(text, "找不到 api key") || strings.Contains(text, "missing api key") || strings.Contains(text, "no api key") {
		return NewFailure("auth")
	}
	if strings.Contains(text, "deadline exceeded") || strings.Contains(text, "timed out") {
		return NewFailure("timeout")
	}
	return NewFailure("unavailable")
}
