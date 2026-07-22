package sessionlog

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type parseState struct {
	provider  Provider
	sessionID string
	cwd       string
	model     string
}

type envelope struct {
	Type       string          `json:"type"`
	Timestamp  string          `json:"timestamp"`
	SessionID  string          `json:"sessionId"`
	SessionV2  string          `json:"session_id"`
	CWD        string          `json:"cwd"`
	UUID       string          `json:"uuid"`
	Message    json.RawMessage `json:"message"`
	Payload    json.RawMessage `json:"payload"`
	Data       json.RawMessage `json:"data"`
	ToolUse    json.RawMessage `json:"toolUseResult"`
	Duration   json.RawMessage `json:"durationMs"`
	DurationV2 json.RawMessage `json:"duration_ms"`
}

type message struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *claudeUsage    `json:"usage"`
}

type claudeUsage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheCreationTokens int `json:"cache_creation_input_tokens"`
	CacheReadTokens     int `json:"cache_read_input_tokens"`
	ServerToolUse       struct {
		WebSearchRequests int `json:"web_search_requests"`
	} `json:"server_tool_use"`
	CacheCreation struct {
		Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
	Speed string `json:"speed"`
}

type codexUsage struct {
	InputTokens           int `json:"input_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
	TotalTokens           int `json:"total_tokens"`
}

func normalize(line []byte, state *parseState) (Record, bool, error) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Record{}, false, err
	}
	if strings.TrimSpace(env.Type) == "" {
		return Record{}, false, errors.New("session-log record is missing type")
	}
	record := Record{
		Timestamp:     parseTimestamp(env.Timestamp),
		TimestampText: env.Timestamp,
		Provider:      state.provider,
		SessionID:     state.sessionID,
		CWD:           state.cwd,
		Model:         state.model,
		Kind:          KindUnknown,
		ProviderEvent: env.Type,
		ID:            env.UUID,
	}
	if env.SessionID != "" {
		state.sessionID = env.SessionID
		record.SessionID = env.SessionID
	} else if env.SessionV2 != "" {
		state.sessionID = env.SessionV2
		record.SessionID = env.SessionV2
	}
	if env.CWD != "" {
		state.cwd = env.CWD
		record.CWD = env.CWD
	}
	extractDuration(env.Duration, &record)
	extractDuration(env.DurationV2, &record)

	var known bool
	switch state.provider {
	case ProviderClaude:
		known = normalizeClaude(env, &record)
	case ProviderCodex:
		known = normalizeCodex(env, &record, state)
	}
	return record, known, nil
}

func normalizeClaude(env envelope, record *Record) bool {
	var msg message
	if len(env.Message) > 0 {
		_ = json.Unmarshal(env.Message, &msg)
	}
	if msg.ID != "" {
		record.ID = msg.ID
	}
	if msg.Role != "" {
		record.Role = msg.Role
	}
	if msg.Model != "" {
		record.Model = msg.Model
	}
	if msg.Usage != nil {
		record.Usage = &Usage{
			InputTokens:              msg.Usage.InputTokens,
			OutputTokens:             msg.Usage.OutputTokens,
			CacheCreationInputTokens: msg.Usage.CacheCreationTokens,
			CacheReadInputTokens:     msg.Usage.CacheReadTokens,
			WebSearchRequests:        msg.Usage.ServerToolUse.WebSearchRequests,
			Ephemeral1hInputTokens:   msg.Usage.CacheCreation.Ephemeral1hInputTokens,
			Speed:                    msg.Usage.Speed,
		}
	}

	switch env.Type {
	case "assistant", "user":
		record.Kind = KindMessage
		if record.Role == "" {
			record.Role = env.Type
		}
		if kind, tool, command := claudeContentMetadata(msg.Content); kind != "" {
			record.Kind, record.Tool, record.Command = kind, tool, command
		}
		if env.Type == "user" && record.Kind == KindMessage && len(env.ToolUse) > 0 {
			record.Kind = KindToolResult
		}
		extractResultMetadata(env.ToolUse, record)
		return true
	case "system":
		record.Kind = KindSystem
		return true
	case "progress":
		record.Kind = KindProgress
		return true
	case "queue-operation":
		record.Kind = KindQueue
		return true
	case "summary", "file-history-snapshot":
		record.Kind = KindContext
		return true
	default:
		return false
	}
}

func claudeContentMetadata(raw json.RawMessage) (Kind, string, string) {
	var blocks []struct {
		Type    string          `json:"type"`
		Name    string          `json:"name"`
		Input   json.RawMessage `json:"input"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", "", ""
	}
	for _, block := range blocks {
		switch block.Type {
		case "tool_use", "server_tool_use":
			return KindToolCall, block.Name, commandFromArguments(block.Name, block.Input)
		case "tool_result":
			return KindToolResult, block.Name, ""
		}
	}
	return "", "", ""
}

func normalizeCodex(env envelope, record *Record, state *parseState) bool {
	var payload struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		SessionID string          `json:"session_id"`
		CWD       string          `json:"cwd"`
		Role      string          `json:"role"`
		Model     string          `json:"model"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Input     json.RawMessage `json:"input"`
		Output    json.RawMessage `json:"output"`
		ExitCode  *int            `json:"exit_code"`
		Duration  json.RawMessage `json:"duration_ms"`
		WallTime  *float64        `json:"wall_time_seconds"`
		Action    struct {
			Command  []string `json:"command"`
			Commands []string `json:"commands"`
		} `json:"action"`
		Info *struct {
			Model           string      `json:"model"`
			ModelName       string      `json:"model_name"`
			LastTokenUsage  *codexUsage `json:"last_token_usage"`
			TotalTokenUsage *codexUsage `json:"total_token_usage"`
		} `json:"info"`
	}
	if len(env.Payload) > 0 {
		_ = json.Unmarshal(env.Payload, &payload)
	}
	if payload.SessionID == "" {
		payload.SessionID = payload.ID
	}
	if payload.SessionID != "" && env.Type == "session_meta" {
		state.sessionID = payload.SessionID
		record.SessionID = payload.SessionID
	}
	if payload.CWD != "" {
		state.cwd = payload.CWD
		record.CWD = payload.CWD
	}
	if payload.Model != "" {
		state.model = payload.Model
		record.Model = payload.Model
	}
	if payload.Role != "" {
		record.Role = payload.Role
	}
	if payload.ExitCode != nil {
		record.ExitCode = payload.ExitCode
	}
	extractDuration(payload.Duration, record)
	if payload.WallTime != nil {
		duration := time.Duration(*payload.WallTime * float64(time.Second))
		record.Duration = &duration
	}

	switch env.Type {
	case "session_meta":
		record.Kind = KindSession
		return true
	case "turn_context":
		record.Kind = KindContext
		return true
	case "event_msg":
		record.ProviderEvent = env.Type + "/" + payload.Type
		switch payload.Type {
		case "token_count":
			record.Kind = KindUsage
			if payload.Info != nil {
				if payload.Info.Model != "" {
					record.Model = payload.Info.Model
				} else if payload.Info.ModelName != "" {
					record.Model = payload.Info.ModelName
				}
				record.Usage = normalizedCodexUsage(payload.Info.LastTokenUsage)
				record.TotalUsage = normalizedCodexUsage(payload.Info.TotalTokenUsage)
			}
			return true
		case "user_message":
			record.Kind, record.Role = KindMessage, "user"
			return true
		case "agent_message":
			record.Kind, record.Role = KindMessage, "assistant"
			return true
		case "task_started", "task_complete", "turn_aborted", "context_compacted":
			record.Kind = KindProgress
			return true
		default:
			return false
		}
	case "response_item":
		record.ProviderEvent = env.Type + "/" + payload.Type
		switch payload.Type {
		case "message":
			record.Kind = KindMessage
			return true
		case "reasoning":
			record.Kind = KindReasoning
			return true
		case "function_call", "custom_tool_call":
			record.Kind, record.Tool = KindToolCall, payload.Name
			if record.Role == "" {
				record.Role = "assistant"
			}
			if len(payload.Arguments) > 0 {
				record.Command = commandFromArguments(payload.Name, payload.Arguments)
			} else {
				record.Command = commandFromArguments(payload.Name, payload.Input)
			}
			return true
		case "local_shell_call", "shell_call":
			record.Kind, record.Role, record.Tool = KindToolCall, "assistant", "shell"
			if len(payload.Action.Command) > 0 {
				record.Command = strings.Join(payload.Action.Command, " ")
			} else {
				record.Command = strings.Join(payload.Action.Commands, "\n")
			}
			return true
		case "apply_patch_call", "tool_search_call", "web_search_call", "image_generation_call", "mcp_call":
			record.Kind, record.Role = KindToolCall, "assistant"
			record.Tool = responseToolName(payload.Type, payload.Name)
			return true
		case "function_call_output", "custom_tool_call_output", "local_shell_call_output", "shell_call_output",
			"apply_patch_call_output", "tool_search_output", "mcp_call_output":
			record.Kind, record.Tool = KindToolResult, payload.Name
			if record.Role == "" {
				record.Role = "tool"
			}
			if record.Tool == "" {
				record.Tool = responseToolName(payload.Type, "")
			}
			extractResultMetadata(payload.Output, record)
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func responseToolName(eventType, name string) string {
	if name != "" {
		return name
	}
	switch eventType {
	case "local_shell_call", "local_shell_call_output", "shell_call", "shell_call_output":
		return "shell"
	case "apply_patch_call", "apply_patch_call_output":
		return "apply_patch"
	case "tool_search_call", "tool_search_output":
		return "tool_search"
	case "web_search_call":
		return "web_search"
	case "image_generation_call":
		return "image_generation"
	case "mcp_call", "mcp_call_output":
		return "mcp"
	}
	return ""
}

func normalizedCodexUsage(usage *codexUsage) *Usage {
	if usage == nil {
		return nil
	}
	return &Usage{
		InputTokens:           usage.InputTokens,
		CachedInputTokens:     usage.CachedInputTokens,
		OutputTokens:          usage.OutputTokens,
		ReasoningOutputTokens: usage.ReasoningOutputTokens,
		TotalTokens:           usage.TotalTokens,
	}
}

func commandFromArguments(tool string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var encoded string
	if json.Unmarshal(raw, &encoded) == nil {
		trimmed := strings.TrimSpace(encoded)
		if strings.HasPrefix(trimmed, "{") {
			return commandFromArguments(tool, json.RawMessage(trimmed))
		}
		if commandTool(tool) {
			return encoded
		}
		return ""
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return ""
	}
	for _, key := range []string{"cmd", "command"} {
		value, ok := values[key]
		if !ok {
			continue
		}
		if json.Unmarshal(value, &encoded) == nil {
			return encoded
		}
		var parts []string
		if json.Unmarshal(value, &parts) == nil {
			return strings.Join(parts, " ")
		}
	}
	return ""
}

func commandTool(tool string) bool {
	switch strings.ToLower(tool) {
	case "bash", "shell", "shell_command", "exec_command", "terminal":
		return true
	default:
		return false
	}
}

var exitCodePattern = regexp.MustCompile(`(?i)(?:process exited with code|exit[_ ]code["']?\s*[:=])\s*(-?\d+)`)

func extractResultMetadata(raw json.RawMessage, record *Record) {
	if len(raw) == 0 {
		return
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) == nil {
		for _, key := range []string{"exit_code", "exitCode"} {
			if value, ok := object[key]; ok && record.ExitCode == nil {
				var code int
				if json.Unmarshal(value, &code) == nil {
					record.ExitCode = &code
				}
			}
		}
		for _, key := range []string{"duration_ms", "durationMs"} {
			if value, ok := object[key]; ok {
				extractDuration(value, record)
			}
		}
		if value, ok := object["wall_time_seconds"]; ok {
			var seconds float64
			if json.Unmarshal(value, &seconds) == nil {
				duration := time.Duration(seconds * float64(time.Second))
				record.Duration = &duration
			}
		}
		for _, key := range []string{"output", "content", "metadata"} {
			if nested, ok := object[key]; ok {
				extractResultMetadata(nested, record)
			}
		}
		return
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		text = string(raw)
	}
	if record.ExitCode == nil {
		if match := exitCodePattern.FindStringSubmatch(text); len(match) == 2 {
			if code, err := strconv.Atoi(match[1]); err == nil {
				record.ExitCode = &code
			}
		}
	}
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") {
		extractResultMetadata(json.RawMessage(trimmed), record)
	}
}

func extractDuration(raw json.RawMessage, record *Record) {
	if len(raw) == 0 {
		return
	}
	var millis float64
	if json.Unmarshal(raw, &millis) == nil {
		duration := time.Duration(millis * float64(time.Millisecond))
		record.Duration = &duration
	}
}

func parseTimestamp(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func sessionIDFromPath(source Source) string {
	name := strings.TrimSuffix(filepath.Base(source.Path), filepath.Ext(source.Path))
	return name
}
