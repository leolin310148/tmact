// Package sessionlog discovers and streams normalized Claude Code and Codex
// CLI session-log records. Content is retained only in memory for explicitly
// privacy-opted-in consumers; this package never persists copied log content.
package sessionlog

import "time"

// Provider identifies a supported on-disk session-log format.
type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
)

// Kind is the normalized purpose of a provider record.
type Kind string

const (
	KindUnknown    Kind = "unknown"
	KindSession    Kind = "session"
	KindContext    Kind = "context"
	KindMessage    Kind = "message"
	KindReasoning  Kind = "reasoning"
	KindToolCall   Kind = "tool_call"
	KindToolResult Kind = "tool_result"
	KindUsage      Kind = "usage"
	KindProgress   Kind = "progress"
	KindSystem     Kind = "system"
	KindQueue      Kind = "queue"
)

// Source is one provider-owned JSONL session log.
type Source struct {
	Provider Provider `json:"provider"`
	Path     string   `json:"path"`
}

// DiscoveryError records a path that could not be inspected. Missing provider
// roots are normal and are not reported as errors.
type DiscoveryError struct {
	Path string `json:"path"`
	Err  error  `json:"-"`
}

// Discovery is the complete result of one provider scan.
type Discovery struct {
	Sources []Source
	Errors  []DiscoveryError
}

// Usage is the normalized token-usage subset needed by local consumers.
type Usage struct {
	InputTokens              int
	CachedInputTokens        int
	OutputTokens             int
	ReasoningOutputTokens    int
	TotalTokens              int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	WebSearchRequests        int
	Ephemeral1hInputTokens   int
	Speed                    string
}

// Record is one normalized provider JSONL record. Context learned from an
// earlier session/context record (session id, cwd, and model) is carried
// forward while a source is streamed.
type Record struct {
	Timestamp time.Time
	// TimestampText preserves the provider spelling for stable legacy dedup.
	TimestampText string
	Provider      Provider
	SessionID     string
	CWD           string
	Role          string
	Kind          Kind
	ProviderEvent string
	Tool          string
	Command       string
	ExitCode      *int
	Duration      *time.Duration
	// Content is normalized message/tool text and may contain private prompts or
	// tool output. Callers must not display it without an explicit user opt-in.
	Content string

	ID         string
	Model      string
	Usage      *Usage
	TotalUsage *Usage
}

// Stats describes parser coverage without treating individual malformed,
// unknown, or oversized records as fatal to the rest of a stream.
type Stats struct {
	Lines     int
	Records   int
	Malformed int
	Unknown   int
	Oversized int
}
