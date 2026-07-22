// Package logsearch performs privacy-safe searches over normalized local
// Claude and Codex session logs.
package logsearch

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/leolin310148/tmact/internal/sessionlog"
)

const (
	DefaultLimit    = 100
	MaxContentBytes = 16 * 1024
)

type Options struct {
	Query       string
	Providers   []sessionlog.Provider
	Since       time.Time
	CWD         string
	Kind        sessionlog.Kind
	Limit       int
	ShowContent bool
}

type Match struct {
	Timestamp         string              `json:"timestamp,omitempty"`
	Provider          sessionlog.Provider `json:"provider"`
	SessionID         string              `json:"session_id,omitempty"`
	CWD               string              `json:"cwd,omitempty"`
	Role              string              `json:"role,omitempty"`
	Kind              sessionlog.Kind     `json:"kind"`
	Event             string              `json:"event,omitempty"`
	Tool              string              `json:"tool,omitempty"`
	CommandVerb       string              `json:"command_verb,omitempty"`
	CommandSubcommand string              `json:"command_subcommand,omitempty"`
	ExitCode          *int                `json:"exit_code,omitempty"`
	DurationMS        *float64            `json:"duration_ms,omitempty"`
	Model             string              `json:"model,omitempty"`
	Content           string              `json:"content,omitempty"`
	ContentTruncated  bool                `json:"content_truncated,omitempty"`
}

type CoverageError struct {
	Stage string `json:"stage"`
	Path  string `json:"path"`
	Error string `json:"error"`
}

type Coverage struct {
	Provider  sessionlog.Provider `json:"provider"`
	Sources   int                 `json:"sources"`
	Lines     int                 `json:"lines"`
	Records   int                 `json:"records"`
	Malformed int                 `json:"malformed"`
	Unknown   int                 `json:"unknown"`
	Oversized int                 `json:"oversized"`
	Errors    []CoverageError     `json:"errors,omitempty"`
}

type Report struct {
	Matches  []Match    `json:"matches"`
	Coverage []Coverage `json:"coverage"`
}

type discoverFunc func(sessionlog.Provider) (sessionlog.Discovery, error)
type streamFunc func(sessionlog.Source, func(sessionlog.Record) error) (sessionlog.Stats, error)

func Search(options Options) (Report, error) {
	return search(options, sessionlog.Discover, sessionlog.Stream)
}

func search(options Options, discover discoverFunc, stream streamFunc) (Report, error) {
	if strings.TrimSpace(options.Query) == "" {
		return Report{}, fmt.Errorf("search query cannot be empty")
	}
	if options.Limit <= 0 {
		return Report{}, fmt.Errorf("limit must be greater than zero")
	}
	providers := options.Providers
	if len(providers) == 0 {
		providers = []sessionlog.Provider{sessionlog.ProviderClaude, sessionlog.ProviderCodex}
	}
	if err := validateProviders(providers); err != nil {
		return Report{}, err
	}
	if options.Kind != "" && !ValidKind(options.Kind) {
		return Report{}, fmt.Errorf("unknown log kind %q", options.Kind)
	}

	report := Report{Matches: []Match{}, Coverage: make([]Coverage, 0, len(providers))}
	query := strings.ToLower(options.Query)
	var candidates []candidate
	sequence := 0
	for _, provider := range providers {
		coverage := Coverage{Provider: provider}
		discovery, err := discover(provider)
		if err != nil {
			coverage.Errors = append(coverage.Errors, CoverageError{Stage: "discovery", Error: err.Error()})
			report.Coverage = append(report.Coverage, coverage)
			continue
		}
		for _, discoveryErr := range discovery.Errors {
			message := "unknown discovery error"
			if discoveryErr.Err != nil {
				message = discoveryErr.Err.Error()
			}
			coverage.Errors = append(coverage.Errors, CoverageError{Stage: "discovery", Path: discoveryErr.Path, Error: message})
		}
		coverage.Sources = len(discovery.Sources)
		for _, source := range discovery.Sources {
			stats, streamErr := stream(source, func(record sessionlog.Record) error {
				if !matchesFilters(record, options) || !matchesQuery(record, query) {
					return nil
				}
				sequence++
				item := candidate{match: safeMatch(record, options.ShowContent), timestamp: record.Timestamp, sequence: sequence}
				candidates = keepNewest(candidates, item, options.Limit)
				return nil
			})
			coverage.Lines += stats.Lines
			coverage.Records += stats.Records
			coverage.Malformed += stats.Malformed
			coverage.Unknown += stats.Unknown
			coverage.Oversized += stats.Oversized
			if streamErr != nil {
				coverage.Errors = append(coverage.Errors, CoverageError{Stage: "stream", Path: source.Path, Error: streamErr.Error()})
			}
		}
		report.Coverage = append(report.Coverage, coverage)
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidateNewer(candidates[i], candidates[j]) })
	for _, item := range candidates {
		report.Matches = append(report.Matches, item.match)
	}
	return report, nil
}

func validateProviders(providers []sessionlog.Provider) error {
	seen := map[sessionlog.Provider]bool{}
	for _, provider := range providers {
		if provider != sessionlog.ProviderClaude && provider != sessionlog.ProviderCodex {
			return fmt.Errorf("unknown log provider %q (want claude or codex)", provider)
		}
		if seen[provider] {
			return fmt.Errorf("log provider %q was specified more than once", provider)
		}
		seen[provider] = true
	}
	return nil
}

func ValidKind(kind sessionlog.Kind) bool {
	switch kind {
	case sessionlog.KindUnknown, sessionlog.KindSession, sessionlog.KindContext,
		sessionlog.KindMessage, sessionlog.KindReasoning, sessionlog.KindToolCall,
		sessionlog.KindToolResult, sessionlog.KindUsage, sessionlog.KindProgress,
		sessionlog.KindSystem, sessionlog.KindQueue:
		return true
	default:
		return false
	}
}

func matchesFilters(record sessionlog.Record, options Options) bool {
	if !options.Since.IsZero() && (record.Timestamp.IsZero() || record.Timestamp.Before(options.Since)) {
		return false
	}
	if options.CWD != "" && filepath.Clean(record.CWD) != filepath.Clean(options.CWD) {
		return false
	}
	return options.Kind == "" || record.Kind == options.Kind
}

func matchesQuery(record sessionlog.Record, query string) bool {
	duration := ""
	if record.Duration != nil {
		duration = record.Duration.String()
	}
	exitCode := ""
	if record.ExitCode != nil {
		exitCode = fmt.Sprint(*record.ExitCode)
	}
	values := []string{
		record.TimestampText, string(record.Provider), record.SessionID, record.CWD,
		record.Role, string(record.Kind), record.ProviderEvent, record.Tool,
		record.Command, exitCode, duration, record.Model, record.Content,
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func safeMatch(record sessionlog.Record, showContent bool) Match {
	verb, subcommand := commandSummary(record.Command)
	match := Match{
		Provider:          record.Provider,
		SessionID:         record.SessionID,
		CWD:               record.CWD,
		Role:              record.Role,
		Kind:              record.Kind,
		Event:             record.ProviderEvent,
		Tool:              record.Tool,
		CommandVerb:       verb,
		CommandSubcommand: subcommand,
		ExitCode:          record.ExitCode,
		Model:             record.Model,
	}
	if !record.Timestamp.IsZero() {
		match.Timestamp = record.Timestamp.Format(time.RFC3339Nano)
	}
	if record.Duration != nil {
		millis := float64(*record.Duration) / float64(time.Millisecond)
		match.DurationMS = &millis
	}
	if showContent {
		match.Content, match.ContentTruncated = truncateUTF8(record.Content, MaxContentBytes)
	}
	return match
}

type candidate struct {
	match     Match
	timestamp time.Time
	sequence  int
}

func keepNewest(items []candidate, item candidate, limit int) []candidate {
	if len(items) < limit {
		return append(items, item)
	}
	oldest := 0
	for i := 1; i < len(items); i++ {
		if candidateNewer(items[oldest], items[i]) {
			oldest = i
		}
	}
	if candidateNewer(item, items[oldest]) {
		items[oldest] = item
	}
	return items
}

func candidateNewer(left, right candidate) bool {
	if left.timestamp.Equal(right.timestamp) {
		return left.sequence > right.sequence
	}
	if left.timestamp.IsZero() {
		return false
	}
	if right.timestamp.IsZero() {
		return true
	}
	return left.timestamp.After(right.timestamp)
}

var (
	environmentAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	safeCommandWord       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
)

var subcommandVerbs = map[string]bool{
	"az": true, "aws": true, "bun": true, "cargo": true, "docker": true,
	"docker-compose": true, "gh": true, "git": true, "gcloud": true,
	"go": true, "helm": true, "kubectl": true, "npm": true, "npx": true,
	"pnpm": true, "terraform": true, "tmact": true, "tofu": true, "yarn": true,
}

func commandSummary(command string) (string, string) {
	line := command
	if newline := strings.IndexByte(line, '\n'); newline >= 0 {
		line = line[:newline]
	}
	fields := strings.Fields(line)
	index := 0
	for index < len(fields) && environmentAssignment.MatchString(fields[index]) {
		index++
	}
	if index >= len(fields) {
		return "", ""
	}
	verb := cleanCommandWord(filepath.Base(fields[index]))
	index++
	if verb == "rtk" && index < len(fields) {
		if fields[index] == "proxy" {
			index++
		} else if fields[index] == "gain" {
			return "rtk", "gain"
		}
		if index < len(fields) {
			verb = cleanCommandWord(filepath.Base(fields[index]))
			index++
		}
	}
	if verb == "" || !subcommandVerbs[verb] || index >= len(fields) || strings.HasPrefix(fields[index], "-") {
		return verb, ""
	}
	return verb, cleanCommandWord(fields[index])
}

func cleanCommandWord(value string) string {
	value = strings.Trim(value, "'\"")
	if !safeCommandWord.MatchString(value) {
		return ""
	}
	return value
}

func truncateUTF8(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	cut := limit
	for cut > 0 && value[cut]&0xc0 == 0x80 {
		cut--
	}
	return value[:cut], true
}
