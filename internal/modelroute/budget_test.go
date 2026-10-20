package modelroute

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRequestBudgetIncludesAllAndPerQuestionContext(t *testing.T) {
	in := Input{Task: "task", Files: []TaskFile{{Path: "large.md", Status: "attached", Content: strings.Repeat("x", 35*1024)}}}
	questions := map[string]any{}
	for i := 0; i < 10; i++ {
		questions[string(rune('a'+i))] = map[string]any{"type": "choice", "instructions": strings.Repeat("x", 6*1024), "criteria": map[string]string{"a": "a", "b": "b"}}
	}
	before := measureRequest(in, questions)
	if before.RequestBytes <= MaxRequestBytes || before.QuestionBytes > MaxQuestionBytes {
		t.Fatalf("bad fixture: %+v", before)
	}
	fitted, b, err := FitRequest(in, questions)
	if err != nil {
		t.Fatal(err)
	}
	if !b.fits() || !fitted.Files[0].Truncated || fitted.Task != in.Task || len(fitted.Files[0].Content) >= len(in.Files[0].Content) {
		t.Fatalf("budget=%+v files=%+v", b, fileMetadata(fitted.Files))
	}
	if len(in.Files[0].Content) != 35*1024 {
		t.Fatal("mutated caller evidence")
	}
}

func TestBudgetFairUTF8ExcerptsAndOmissionMetadata(t *testing.T) {
	in := Input{Task: "keep this task"}
	for i := 0; i < 7; i++ {
		text := strings.Repeat("中文🌍", 10000)
		in.Files = append(in.Files, TaskFile{Path: string(rune('a'+i)) + ".md", Status: "attached", Content: text, SHA256: contentHash(text), SourceBytes: int64(len(text))})
	}
	q, _ := decisionQuestions(Input{Candidates: []Candidate{{Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium"}}})
	v, b, err := FitRequest(in, q)
	if err != nil {
		t.Fatal(err)
	}
	if !b.fits() || b.TruncatedFiles != 7 {
		t.Fatalf("budget=%+v", b)
	}
	for _, f := range v.Files {
		if !utf8.ValidString(f.Content) || f.Content == "" || f.IncludedBytes != len(f.Content) || f.OriginalBytes <= f.IncludedBytes || f.Reason != "request_budget" || f.SHA256 != contentHash(f.Content) || f.SourceSHA256 == "" {
			t.Fatalf("bad excerpt=%+v", fileMetadata([]TaskFile{f}))
		}
	}
	omitted := clipFile(v.Files[0], 0, "context_retry")
	if !omitted.Omitted || !omitted.Truncated || omitted.Status != "omitted" || omitted.OriginalBytes == 0 || omitted.IncludedBytes != 0 || omitted.SourceSHA256 != v.Files[0].SourceSHA256 {
		t.Fatal(omitted)
	}
}

func setupBudgetDecide(t *testing.T, mode string) (Input, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TMACT_ROUTING_LOG", filepath.Join(dir, "routing.jsonl"))
	t.Setenv("DECIDE_CLI_CONFIG", "none")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BUDGET_TEST_DIR", dir)
	t.Setenv("BUDGET_TEST_MODE", mode)
	script := `#!/bin/sh
count=0
[ ! -f "$BUDGET_TEST_DIR/calls" ] || count=$(cat "$BUDGET_TEST_DIR/calls")
count=$((count+1))
printf '%s' "$count" > "$BUDGET_TEST_DIR/calls"
cat > "$BUDGET_TEST_DIR/input-$count.json"
if [ "$BUDGET_TEST_MODE" = "always-large" ] || { [ "$BUDGET_TEST_MODE" = "large-once" ] && [ "$count" -eq 1 ]; }; then
 printf '%s\n' 'decide: TypeSafe API HTTP 400: {"detail":{"error_type":"max_tokens_exceeded"},"private":"UPSTREAM_SECRET_NEVER_LOG"}' >&2
 exit 2
fi
if [ "$BUDGET_TEST_MODE" = "auth" ]; then
 printf '%s\n' 'decide: TypeSafe API HTTP 401: {"private":"UPSTREAM_SECRET_NEVER_LOG"}' >&2
 exit 1
fi
if [ "$BUDGET_TEST_MODE" = "unavailable" ]; then
 printf '%s\n' 'decide: TypeSafe API HTTP 503: {"private":"UPSTREAM_SECRET_NEVER_LOG"}' >&2
 exit 1
fi
printf '%s\n' '{"model":"test-decide","private":"PRIVATE_RESPONSE_DATA","answers":{"route":{"choice":"profile_0"}},"confidences":{"route":0.1}}'
`
	os.WriteFile(filepath.Join(dir, "decide"), []byte(script), 0700)
	text := strings.Repeat("PRIVATE_FILE_DATA ", 300)
	in := Input{Task: "PRIVATE_TASK_DATA", Policy: Policy, Files: []TaskFile{{Path: "task.md", Status: "attached", Content: text, SHA256: contentHash(text)}}, Candidates: []Candidate{{Agent: "codex", Model: "gpt-6.1-sol", Effort: "medium"}}, Trace: Trace{DispatchID: NewDispatchID()}}
	return in, dir
}

func TestContextRetryIsBoundedAndAuditedWithoutUpstreamLeak(t *testing.T) {
	for _, tt := range []struct {
		mode  string
		calls int
		code  string
	}{{"large-once", 2, ""}, {"always-large", 2, "context_too_large"}, {"auth", 1, "auth"}, {"unavailable", 1, "unavailable"}} {
		t.Run(tt.mode, func(t *testing.T) {
			in, dir := setupBudgetDecide(t, tt.mode)
			s, err := Choose(context.Background(), in)
			if ErrorCode(err) != tt.code || (err == nil) != (tt.code == "") {
				t.Fatalf("selection=%+v err=%v", s, err)
			}
			calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
			if string(calls) != string(rune('0'+tt.calls)) {
				t.Fatalf("calls=%s", calls)
			}
			entries, err := ReadLog(in.Trace.DispatchID, 10, true)
			if err != nil || len(entries) != 1 || len(entries[0].Attempts) != tt.calls {
				t.Fatalf("audit=%+v err=%v", entries, err)
			}
			e := entries[0]
			if tt.calls == 2 {
				first, second := e.Attempts[0], e.Attempts[1]
				if first.ErrorCode != "context_too_large" || len(second.Input.Files[0].Content) >= len(first.Input.Files[0].Content) || second.Input.Files[0].Reason != "context_retry" {
					t.Fatal("retry did not shrink and record context")
				}
			}
			if s != nil && (s.Attempts != 2 || s.Budget == nil || s.Files[0].OriginalBytes <= s.Files[0].IncludedBytes) {
				t.Fatal(s)
			}
			raw, _ := os.ReadFile(filepath.Join(dir, "routing.jsonl"))
			if strings.Contains(string(raw), "UPSTREAM_SECRET_NEVER_LOG") {
				t.Fatal("provider error leaked")
			}
			redacted, err := ReadLog(in.Trace.DispatchID, 10, false)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(redacted)
			if strings.Contains(string(b), "PRIVATE_TASK_DATA") || strings.Contains(string(b), "PRIVATE_FILE_DATA") || strings.Contains(string(b), "PRIVATE_RESPONSE_DATA") {
				t.Fatal("attempt input/response leaked by default")
			}
		})
	}
}

func TestOversizedTaskNeverInvokesDecideAndNoAttachmentRetry(t *testing.T) {
	in, dir := setupBudgetDecide(t, "always-large")
	in.Task = strings.Repeat("x", MaxQuestionBytes*2)
	_, err := Choose(context.Background(), in)
	if ErrorCode(err) != "context_too_large" {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "calls")); !os.IsNotExist(err) {
		t.Fatal("oversized task invoked decide")
	}
	in.Task = "task"
	in.Files = nil
	_, err = Choose(context.Background(), in)
	if ErrorCode(err) != "context_too_large" {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if string(calls) != "1" {
		t.Fatal("retried without changing attachments")
	}
}

func TestDecideFailureClassification(t *testing.T) {
	for _, tt := range []struct{ text, code string }{{`HTTP 400: {"detail":{"error_type":"max_tokens_exceeded"}}`, "context_too_large"}, {`HTTP 400: {"detail":{"text":"max_tokens_exceeded"}}`, "invalid_request"}, {`HTTP 403: secret`, "auth"}, {"找不到 API key", "auth"}, {"context deadline exceeded", "timeout"}, {"HTTP 429: secret", "unavailable"}, {"unknown secret", "unavailable"}} {
		e := classifyDecideFailure(tt.text)
		if e.Code != tt.code || strings.Contains(e.Error(), "secret") || !e.NoAgentLaunched {
			t.Fatalf("classified=%+v", e)
		}
	}
}

func TestCancellationIsTypedAndDoesNotRetry(t *testing.T) {
	in, dir := setupBudgetDecide(t, "large-once")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Choose(ctx, in)
	if ErrorCode(err) != "cancelled" {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "calls")); !os.IsNotExist(err) {
		t.Fatal("cancelled request launched child")
	}
}
