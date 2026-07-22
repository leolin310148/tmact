package sessionlog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStreamClaudeNormalizesToolCallAndResult(t *testing.T) {
	records, stats := readFixture(t, ProviderClaude, "claude.jsonl")
	if stats != (Stats{Lines: 3, Records: 3, Unknown: 1}) {
		t.Fatalf("stats = %#v", stats)
	}
	call := records[0]
	if call.Provider != ProviderClaude || call.SessionID != "claude-session-redacted" || call.CWD != "/workspace/redacted" {
		t.Fatalf("call identity = %#v", call)
	}
	if call.Kind != KindToolCall || call.Role != "assistant" || call.Tool != "Bash" || call.Command != "git status --short" {
		t.Fatalf("call metadata = %#v", call)
	}
	if call.Content != "synthetic response\ngit status --short" {
		t.Fatalf("call content = %q", call.Content)
	}
	if call.TimestampText != "2026-07-20T09:10:11.123Z" || call.Timestamp.IsZero() || call.Model != "claude-sonnet-4-5" {
		t.Fatalf("call time/model = %#v", call)
	}
	if call.Usage == nil || call.Usage.InputTokens != 120 || call.Usage.CacheReadInputTokens != 20 || call.Usage.WebSearchRequests != 1 {
		t.Fatalf("call usage = %#v", call.Usage)
	}

	result := records[1]
	if result.Kind != KindToolResult || result.Role != "user" || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("result metadata = %#v", result)
	}
	if result.Duration == nil || *result.Duration != 250*time.Millisecond {
		t.Fatalf("result duration = %v", result.Duration)
	}
	if result.Content != "synthetic output" {
		t.Fatalf("result content = %q", result.Content)
	}
	if records[2].Kind != KindUnknown {
		t.Fatalf("unknown kind = %q", records[2].Kind)
	}
}

func TestStreamCodexNormalizesCurrentToolShapesAndContext(t *testing.T) {
	records, stats := readFixture(t, ProviderCodex, "codex.jsonl")
	if stats != (Stats{Lines: 10, Records: 10, Unknown: 1}) {
		t.Fatalf("stats = %#v", stats)
	}
	for i, record := range records[1:] {
		if record.SessionID != "codex-session-redacted" || record.CWD != "/workspace/redacted" {
			t.Fatalf("record %d did not inherit context: %#v", i+1, record)
		}
	}
	call := records[2]
	if call.Kind != KindToolCall || call.Tool != "exec_command" || call.Command != "go test ./internal/example" {
		t.Fatalf("function call = %#v", call)
	}
	result := records[3]
	if result.Kind != KindToolResult || result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("function result = %#v", result)
	}
	if result.Duration == nil || *result.Duration != 1250*time.Millisecond {
		t.Fatalf("function result duration = %v", result.Duration)
	}
	if result.Content != "synthetic output" {
		t.Fatalf("function result content = %q", result.Content)
	}
	customCall := records[4]
	if customCall.Kind != KindToolCall || customCall.Tool != "shell" || customCall.Command != "git status --short" {
		t.Fatalf("custom call = %#v", customCall)
	}
	customResult := records[5]
	if customResult.ExitCode == nil || *customResult.ExitCode != 7 {
		t.Fatalf("custom result = %#v", customResult)
	}
	localCall := records[6]
	if localCall.Kind != KindToolCall || localCall.Tool != "shell" || localCall.Command != "go test ./internal/example" {
		t.Fatalf("local shell call = %#v", localCall)
	}
	localResult := records[7]
	if localResult.Kind != KindToolResult || localResult.Tool != "shell" || localResult.ExitCode == nil || *localResult.ExitCode != 0 {
		t.Fatalf("local shell result = %#v", localResult)
	}
	if localResult.Duration == nil || *localResult.Duration != 50*time.Millisecond {
		t.Fatalf("local shell duration = %v", localResult.Duration)
	}
	usage := records[8]
	if usage.Kind != KindUsage || usage.Model != "gpt-5.6-codex" || usage.Usage == nil || usage.TotalUsage == nil {
		t.Fatalf("usage record = %#v", usage)
	}
	if usage.Usage.CachedInputTokens != 25 || usage.TotalUsage.TotalTokens != 140 {
		t.Fatalf("normalized usage = last %#v total %#v", usage.Usage, usage.TotalUsage)
	}
}

func TestStreamSkipsMalformedAndOversizedRecordsThenContinues(t *testing.T) {
	valid := `{"type":"system","timestamp":"2026-07-20T00:00:00Z"}`
	input := "{malformed}\n" + strings.Repeat("x", MaxRecordBytes+1) + "\n" + valid + "\n"
	var records []Record
	stats, err := StreamReader(strings.NewReader(input), Source{Provider: ProviderClaude, Path: "synthetic.jsonl"}, func(record Record) error {
		records = append(records, record)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats != (Stats{Lines: 3, Records: 1, Malformed: 1, Oversized: 1}) {
		t.Fatalf("stats = %#v", stats)
	}
	if len(records) != 1 || records[0].Kind != KindSystem {
		t.Fatalf("records = %#v", records)
	}
}

func TestStreamReturnsVisitorError(t *testing.T) {
	want := errors.New("stop")
	stats, err := StreamReader(strings.NewReader(`{"type":"system"}`+"\n"), Source{Provider: ProviderClaude}, func(Record) error {
		return want
	})
	if !errors.Is(err, want) || stats.Records != 1 {
		t.Fatalf("stats=%#v err=%v", stats, err)
	}
}

func TestStreamRejectsUnknownProvider(t *testing.T) {
	_, err := StreamReader(strings.NewReader(""), Source{Provider: "other"}, func(Record) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("err = %v", err)
	}
}

func TestNonCommandToolInputIsNotNormalizedAsCommand(t *testing.T) {
	input := `{"type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"synthetic patch body"}}` + "\n"
	var got Record
	_, err := StreamReader(strings.NewReader(input), Source{Provider: ProviderCodex}, func(record Record) error {
		got = record
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindToolCall || got.Tool != "apply_patch" || got.Command != "" {
		t.Fatalf("record = %#v", got)
	}
}

func readFixture(t *testing.T, provider Provider, name string) ([]Record, Stats) {
	t.Helper()
	path := filepath.Join("testdata", name)
	var records []Record
	stats, err := Stream(Source{Provider: provider, Path: path}, func(record Record) error {
		records = append(records, record)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return records, stats
}

func TestStreamOpenError(t *testing.T) {
	_, err := Stream(Source{Provider: ProviderClaude, Path: filepath.Join(t.TempDir(), "missing.jsonl")}, func(Record) error { return nil })
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}
