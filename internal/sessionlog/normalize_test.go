package sessionlog

import (
	"encoding/json"
	"testing"
	"time"
)

func TestExtractResultMetadataMalformedBracepayloadTerminates(t *testing.T) {
	// Looks like a JSON object but is not valid JSON: the map unmarshal
	// fails, the string unmarshal fails, and before the fix the fallback
	// text was identical to the input, so the trailing "{" check recursed
	// forever and overflowed the stack.
	payload := json.RawMessage(`{not valid json, "exit_code": 3`)
	var record Record
	extractResultMetadata(payload, &record)
	// The plain-text regex fallback still applies to the malformed bytes.
	if record.ExitCode == nil || *record.ExitCode != 3 {
		t.Fatalf("expected exit code 3 via text fallback, got %v", record.ExitCode)
	}
}

func TestExtractResultMetadataUnwrapsStringEncodedObject(t *testing.T) {
	// A JSON string whose content is itself a JSON object must still be
	// unwrapped one layer and parsed.
	payload := json.RawMessage(`"{\"exit_code\": 7, \"duration_ms\": 1500}"`)
	var record Record
	extractResultMetadata(payload, &record)
	if record.ExitCode == nil || *record.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %v", record.ExitCode)
	}
	if record.Duration == nil || *record.Duration != 1500*time.Millisecond {
		t.Fatalf("expected duration 1500ms, got %v", record.Duration)
	}
}

func TestExtractResultMetadataFallbackTextStillMatchesExitCode(t *testing.T) {
	payload := json.RawMessage(`Process exited with code 42`)
	var record Record
	extractResultMetadata(payload, &record)
	if record.ExitCode == nil || *record.ExitCode != 42 {
		t.Fatalf("expected exit code 42 from plain-text fallback, got %v", record.ExitCode)
	}
}
