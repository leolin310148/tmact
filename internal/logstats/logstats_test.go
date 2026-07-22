package logstats

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leolin310148/tmact/internal/sessionlog"
)

func TestStatsAggregatesSafeFieldsAndReusesChangedSourcesIncrementally(t *testing.T) {
	root, cachePath, claudePath, _ := installFixtures(t)
	since := time.Date(2026, 7, 21, 10, 30, 0, 0, time.UTC)

	first, err := Stats(Options{Since: since, CachePath: cachePath})
	if err != nil {
		t.Fatal(err)
	}
	if first.Records != 2 || bucketCount(first.Providers, "claude") != 1 || bucketCount(first.Providers, "codex") != 1 {
		t.Fatalf("provider aggregates = %#v", first)
	}
	if bucketCount(first.Tools, "shell") != 1 || bucketCount(first.Commands, "go") != 1 || bucketCount(first.Subcommands, "test") != 1 {
		t.Fatalf("command aggregates = %#v", first)
	}
	if first.Index.Status != "rebuilt_missing" || first.Index.Misses != 2 || first.Index.Rebuilt != 2 {
		t.Fatalf("first index = %#v", first.Index)
	}
	cacheData, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"topsecret", "partial-secret", "topsecret partial-secret", "codexsecret", "codex-partial", "codexsecret codex-partial", "private-argument", "private prompt", "tool output"} {
		if strings.Contains(string(cacheData), private) {
			t.Fatalf("cache leaked %q: %s", private, cacheData)
		}
	}
	if info, err := os.Stat(cachePath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("cache mode info=%v err=%v", info, err)
	}

	second, err := Stats(Options{CachePath: cachePath})
	if err != nil {
		t.Fatal(err)
	}
	if second.Index.Status != "healthy" || second.Index.Hits != 2 || second.Index.Misses != 0 {
		t.Fatalf("second index = %#v", second.Index)
	}

	file, err := os.OpenFile(claudePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := file.WriteString(`{"type":"assistant","timestamp":"2026-07-21T12:00:00Z","sessionId":"claude-redacted","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"npm test private-argument"}}]}}` + "\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("append err=%v close=%v", writeErr, closeErr)
	}
	third, err := Stats(Options{CachePath: cachePath})
	if err != nil {
		t.Fatal(err)
	}
	if third.Index.Status != "updated" || third.Index.Hits != 1 || third.Index.Misses != 1 || third.Index.Appended != 1 || third.Index.Rebuilt != 0 {
		t.Fatalf("third index = %#v", third.Index)
	}
	if bucketCount(third.Commands, "npm") != 1 || bucketCount(third.Subcommands, "test") != 2 {
		t.Fatalf("updated aggregates = %#v", third)
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private-argument") {
		t.Fatalf("updated cache leaked full arguments: %s", data)
	}
	_ = root
}

func TestDoctorRebuildsCorruptAndStaleCachesAndReportsCoverage(t *testing.T) {
	_, cachePath, _, _ := installFixtures(t)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	doctor, err := Doctor(Options{CachePath: cachePath})
	if err != nil {
		t.Fatal(err)
	}
	if !doctor.Cache.Healthy || doctor.Cache.Status != "rebuilt_corrupt" || doctor.Files.Discovered != 2 || doctor.Files.Indexed != 2 {
		t.Fatalf("doctor = %#v", doctor)
	}
	if doctor.Records.Malformed != 1 || doctor.Records.Skipped != 1 || doctor.Records.Unknown != 1 || doctor.Records.Known != 3 {
		t.Fatalf("record health = %#v", doctor.Records)
	}
	if len(doctor.SchemaCoverage) != 2 || doctor.SchemaCoverage[0].Provider != "claude" || doctor.SchemaCoverage[1].Provider != "codex" {
		t.Fatalf("coverage = %#v", doctor.SchemaCoverage)
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if sessionlog.ParserVersion <= 1 {
		t.Fatalf("parser version = %d, want a bump past the vulnerable version", sessionlog.ParserVersion)
	}
	currentVersion := fmt.Sprintf(`"parser_version": %d`, sessionlog.ParserVersion)
	stale := strings.ReplaceAll(string(data), currentVersion, `"parser_version": 1`)
	if stale == string(data) {
		t.Fatal("cache did not contain parser version")
	}
	if err := os.WriteFile(cachePath, []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := Stats(Options{CachePath: cachePath})
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Index.Status != "rebuilt_stale" || rebuilt.Index.Rebuilt != 2 {
		t.Fatalf("stale rebuild = %#v", rebuilt.Index)
	}
}

func TestStatsRemovesMissingSourceFromIndex(t *testing.T) {
	_, cachePath, _, codexPath := installFixtures(t)
	if _, err := Stats(Options{CachePath: cachePath}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(codexPath); err != nil {
		t.Fatal(err)
	}
	report, err := Stats(Options{CachePath: cachePath})
	if err != nil {
		t.Fatal(err)
	}
	if report.Index.Status != "updated" || report.Index.Removed != 1 || report.Index.Entries != 1 || bucketCount(report.Providers, "codex") != 0 {
		t.Fatalf("index after removal = %#v", report)
	}
}

func TestStatsRebuildsGrowingSourceWhenCachedTailWasRewritten(t *testing.T) {
	_, cachePath, claudePath, _ := installFixtures(t)
	if _, err := Stats(Options{CachePath: cachePath}); err != nil {
		t.Fatal(err)
	}
	replacement := strings.Repeat(`{"type":"system","timestamp":"2026-07-22T00:00:00Z"}`+"\n", 8)
	if err := os.WriteFile(claudePath, []byte(replacement), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Stats(Options{CachePath: cachePath})
	if err != nil {
		t.Fatal(err)
	}
	if report.Index.Appended != 0 || report.Index.Rebuilt != 1 || bucketCount(report.Commands, "git") != 0 {
		t.Fatalf("rewritten source was not rebuilt: %#v", report)
	}
}

func installFixtures(t *testing.T) (root, cachePath, claudePath, codexPath string) {
	t.Helper()
	root = t.TempDir()
	claudeRoot := filepath.Join(root, "claude")
	codexRoot := filepath.Join(root, "codex")
	claudePath = filepath.Join(claudeRoot, "projects", "fixture", "session.jsonl")
	codexPath = filepath.Join(codexRoot, "sessions", "2026", "07", "session.jsonl")
	cachePath = filepath.Join(root, ".tmact", DefaultCacheName)
	writeFixture(t, claudePath, strings.Join([]string{
		`{"type":"assistant","timestamp":"2026-07-21T09:00:00Z","sessionId":"claude-redacted","message":{"role":"assistant","content":[{"type":"text","text":"private prompt"},{"type":"tool_use","name":"Bash","input":{"command":"TOKEN='topsecret partial-secret' git status --short private-argument"}}]}}`,
		`{malformed}`,
		`{"type":"future-event","timestamp":"2026-07-21T11:00:00Z","sessionId":"claude-redacted"}`,
	}, "\n")+"\n")
	writeFixture(t, codexPath, strings.Join([]string{
		`{"type":"session_meta","timestamp":"2026-07-21T10:00:00Z","payload":{"id":"codex-redacted","cwd":"/workspace/redacted"}}`,
		`{"type":"response_item","timestamp":"2026-07-21T11:30:00Z","payload":{"type":"local_shell_call","action":{"command":["env","TOKEN=codexsecret codex-partial","go","test","./...","tool output"]}}}`,
	}, "\n")+"\n")
	t.Setenv("CLAUDE_CONFIG_DIRS", claudeRoot)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", codexRoot)
	return root, cachePath, claudePath, codexPath
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func bucketCount(items []Bucket, name string) int {
	for _, item := range items {
		if item.Name == name {
			return item.Count
		}
	}
	return 0
}
