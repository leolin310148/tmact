// Package logstats builds privacy-safe aggregates and a plain-file incremental
// index over normalized Claude and Codex session logs.
package logstats

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/leolin310148/tmact/internal/sessionlog"
)

const (
	CacheSchemaVersion   = 1
	DefaultCacheName     = "log-index.json"
	tailFingerprintBytes = 4096
)

type Options struct {
	Since     time.Time
	CachePath string
}

type Bucket struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type ScanError struct {
	Stage    string              `json:"stage"`
	Provider sessionlog.Provider `json:"provider,omitempty"`
	Path     string              `json:"path,omitempty"`
	Error    string              `json:"error"`
}

type IndexSummary struct {
	Status   string `json:"status"`
	Entries  int    `json:"entries"`
	Hits     int    `json:"hits"`
	Misses   int    `json:"misses"`
	Appended int    `json:"appended"`
	Rebuilt  int    `json:"rebuilt"`
	Removed  int    `json:"removed"`
}

type Report struct {
	Since       string       `json:"since,omitempty"`
	Records     int          `json:"records"`
	Providers   []Bucket     `json:"providers"`
	Tools       []Bucket     `json:"tools"`
	Commands    []Bucket     `json:"commands"`
	Subcommands []Bucket     `json:"subcommands"`
	Index       IndexSummary `json:"index"`
	Errors      []ScanError  `json:"errors,omitempty"`
}

type FileHealth struct {
	Discovered int `json:"discovered"`
	Indexed    int `json:"indexed"`
	CacheHits  int `json:"cache_hits"`
	Parsed     int `json:"parsed"`
	Failed     int `json:"failed"`
}

type RecordHealth struct {
	Lines     int `json:"lines"`
	Records   int `json:"records"`
	Known     int `json:"known"`
	Unknown   int `json:"unknown"`
	Malformed int `json:"malformed"`
	Oversized int `json:"oversized"`
	Skipped   int `json:"skipped"`
}

type ProviderCoverage struct {
	Provider sessionlog.Provider `json:"provider"`
	Files    int                 `json:"files"`
	RecordHealth
}

type CacheHealth struct {
	Path    string `json:"path"`
	Healthy bool   `json:"healthy"`
	IndexSummary
}

type DoctorReport struct {
	Files          FileHealth         `json:"files"`
	Records        RecordHealth       `json:"records"`
	SchemaCoverage []ProviderCoverage `json:"schema_coverage"`
	Cache          CacheHealth        `json:"cache"`
	Errors         []ScanError        `json:"errors,omitempty"`
}

type safeRecord struct {
	Timestamp  string              `json:"timestamp,omitempty"`
	Provider   sessionlog.Provider `json:"provider"`
	Kind       sessionlog.Kind     `json:"kind"`
	Tool       string              `json:"tool,omitempty"`
	Command    string              `json:"command,omitempty"`
	Subcommand string              `json:"subcommand,omitempty"`
}

type cachedSource struct {
	Provider        sessionlog.Provider `json:"provider"`
	Size            int64               `json:"size"`
	ModTimeUnixNS   int64               `json:"mtime_unix_ns"`
	ParserVersion   int                 `json:"parser_version"`
	TailBytes       int                 `json:"tail_bytes"`
	TailSHA256      string              `json:"tail_sha256"`
	EndsWithNewline bool                `json:"ends_with_newline"`
	Stats           sessionlog.Stats    `json:"stats"`
	Records         []safeRecord        `json:"records"`
}

type cacheFile struct {
	SchemaVersion int                     `json:"schema_version"`
	ParserVersion int                     `json:"parser_version"`
	Sources       map[string]cachedSource `json:"sources"`
}

type scanResult struct {
	records  []safeRecord
	coverage map[sessionlog.Provider]ProviderCoverage
	files    FileHealth
	index    IndexSummary
	errors   []ScanError
}

func Stats(options Options) (Report, error) {
	result, _, err := scan(options)
	if err != nil {
		return Report{}, err
	}
	providerCounts := map[string]int{}
	toolCounts := map[string]int{}
	commandCounts := map[string]int{}
	subcommandCounts := map[string]int{}
	records := 0
	for _, record := range result.records {
		if !options.Since.IsZero() {
			when, err := time.Parse(time.RFC3339Nano, record.Timestamp)
			if err != nil || when.Before(options.Since) {
				continue
			}
		}
		records++
		providerCounts[string(record.Provider)]++
		if record.Tool != "" {
			toolCounts[record.Tool]++
		}
		if record.Command != "" {
			commandCounts[record.Command]++
		}
		if record.Subcommand != "" {
			subcommandCounts[record.Subcommand]++
		}
	}
	report := Report{
		Records:     records,
		Providers:   buckets(providerCounts),
		Tools:       buckets(toolCounts),
		Commands:    buckets(commandCounts),
		Subcommands: buckets(subcommandCounts),
		Index:       result.index,
		Errors:      result.errors,
	}
	if !options.Since.IsZero() {
		report.Since = options.Since.Format(time.RFC3339Nano)
	}
	return report, nil
}

func Doctor(options Options) (DoctorReport, error) {
	options.Since = time.Time{}
	result, cachePath, err := scan(options)
	if err != nil {
		return DoctorReport{}, err
	}
	providers := []sessionlog.Provider{sessionlog.ProviderClaude, sessionlog.ProviderCodex}
	coverage := make([]ProviderCoverage, 0, len(providers))
	var total RecordHealth
	for _, provider := range providers {
		item := result.coverage[provider]
		coverage = append(coverage, item)
		total.Lines += item.Lines
		total.Records += item.Records
		total.Known += item.Known
		total.Unknown += item.Unknown
		total.Malformed += item.Malformed
		total.Oversized += item.Oversized
		total.Skipped += item.Skipped
	}
	return DoctorReport{
		Files:          result.files,
		Records:        total,
		SchemaCoverage: coverage,
		Cache: CacheHealth{
			Path:         cachePath,
			Healthy:      true,
			IndexSummary: result.index,
		},
		Errors: result.errors,
	}, nil
}

func scan(options Options) (scanResult, string, error) {
	cachePath := options.CachePath
	if cachePath == "" {
		var err error
		cachePath, err = DefaultCachePath()
		if err != nil {
			return scanResult{}, "", err
		}
	}
	cache, loadStatus := loadCache(cachePath)
	result := scanResult{
		coverage: map[sessionlog.Provider]ProviderCoverage{
			sessionlog.ProviderClaude: {Provider: sessionlog.ProviderClaude},
			sessionlog.ProviderCodex:  {Provider: sessionlog.ProviderCodex},
		},
		index: IndexSummary{Status: loadStatus},
	}
	active := make(map[string]bool)
	changed := loadStatus != "healthy"
	for _, provider := range []sessionlog.Provider{sessionlog.ProviderClaude, sessionlog.ProviderCodex} {
		discovery, err := sessionlog.Discover(provider)
		if err != nil {
			result.errors = append(result.errors, ScanError{Stage: "discovery", Provider: provider, Error: err.Error()})
			continue
		}
		for _, discoveryErr := range discovery.Errors {
			message := "unknown discovery error"
			if discoveryErr.Err != nil {
				message = discoveryErr.Err.Error()
			}
			result.errors = append(result.errors, ScanError{Stage: "discovery", Provider: provider, Path: discoveryErr.Path, Error: message})
		}
		result.files.Discovered += len(discovery.Sources)
		for _, source := range discovery.Sources {
			active[source.Path] = true
			info, err := os.Stat(source.Path)
			if err != nil {
				result.files.Failed++
				result.errors = append(result.errors, ScanError{Stage: "stat", Provider: provider, Path: source.Path, Error: err.Error()})
				if _, exists := cache.Sources[source.Path]; exists {
					delete(cache.Sources, source.Path)
					changed = true
				}
				continue
			}
			cached, hit := cache.Sources[source.Path]
			if hit && cacheMatches(cached, source, info) {
				result.files.CacheHits++
				result.index.Hits++
				addSource(&result, cached)
				continue
			}
			result.index.Misses++
			result.files.Parsed++
			entry, appended, streamErr := indexSource(source, info, cached, hit)
			addSource(&result, entry)
			if streamErr != nil {
				result.files.Failed++
				result.errors = append(result.errors, ScanError{Stage: "stream", Provider: provider, Path: source.Path, Error: streamErr.Error()})
				delete(cache.Sources, source.Path)
				changed = true
				continue
			}
			cache.Sources[source.Path] = entry
			if appended {
				result.index.Appended++
			} else {
				result.index.Rebuilt++
			}
			changed = true
		}
	}
	for path := range cache.Sources {
		if !active[path] {
			delete(cache.Sources, path)
			result.index.Removed++
			changed = true
		}
	}
	result.index.Entries = len(cache.Sources)
	result.files.Indexed = len(cache.Sources)
	if changed {
		if err := writeCache(cachePath, cache); err != nil {
			return scanResult{}, cachePath, fmt.Errorf("write log index: %w", err)
		}
		switch loadStatus {
		case "missing", "corrupt", "stale":
			result.index.Status = "rebuilt_" + loadStatus
		default:
			result.index.Status = "updated"
		}
	}
	return result, cachePath, nil
}

func indexSource(source sessionlog.Source, info fs.FileInfo, cached cachedSource, hasCache bool) (cachedSource, bool, error) {
	if hasCache && appendCandidate(cached, source, info) {
		matches, err := cachedTailMatches(source.Path, cached)
		if err != nil {
			return cachedSource{Provider: source.Provider}, false, err
		}
		if matches {
			delta, err := parseSourceFrom(source, cached.Size)
			if err != nil {
				return cachedSource{Provider: source.Provider}, false, err
			}
			delta.Stats = addStats(cached.Stats, delta.Stats)
			delta.Records = append(append([]safeRecord(nil), cached.Records...), delta.Records...)
			return delta, true, nil
		}
	}
	entry, err := parseSourceFrom(source, 0)
	return entry, false, err
}

func appendCandidate(cached cachedSource, source sessionlog.Source, info fs.FileInfo) bool {
	return cached.Provider == source.Provider && cached.ParserVersion == sessionlog.ParserVersion &&
		cached.Size >= 0 && info.Size() > cached.Size && cached.EndsWithNewline
}

func parseSourceFrom(source sessionlog.Source, offset int64) (cachedSource, error) {
	entry := cachedSource{Provider: source.Provider, ParserVersion: sessionlog.ParserVersion}
	file, err := os.Open(source.Path)
	if err != nil {
		return entry, err
	}
	defer file.Close()
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return entry, err
	}
	stats, err := sessionlog.StreamReader(file, source, func(record sessionlog.Record) error {
		verb, subcommand := sessionlog.SafeCommandSummary(record.Command)
		item := safeRecord{Provider: record.Provider, Kind: record.Kind, Tool: record.Tool, Command: verb, Subcommand: subcommand}
		if !record.Timestamp.IsZero() {
			item.Timestamp = record.Timestamp.Format(time.RFC3339Nano)
		}
		entry.Records = append(entry.Records, item)
		return nil
	})
	entry.Stats = stats
	if err != nil {
		return entry, err
	}
	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return entry, err
	}
	info, err := file.Stat()
	if err != nil {
		return entry, err
	}
	if position != info.Size() {
		return entry, fmt.Errorf("source changed while indexing")
	}
	entry.Size = info.Size()
	entry.ModTimeUnixNS = info.ModTime().UnixNano()
	entry.TailSHA256, entry.TailBytes, entry.EndsWithNewline, err = fileTailFingerprint(file, entry.Size)
	if err != nil {
		return entry, err
	}
	return entry, nil
}

func cachedTailMatches(path string, cached cachedSource) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	digest, count, endsWithNewline, err := fileTailFingerprint(file, cached.Size)
	if err != nil {
		return false, err
	}
	return digest == cached.TailSHA256 && count == cached.TailBytes && endsWithNewline == cached.EndsWithNewline, nil
}

func fileTailFingerprint(file *os.File, size int64) (string, int, bool, error) {
	if size < 0 {
		return "", 0, false, fmt.Errorf("negative source size")
	}
	count := int64(tailFingerprintBytes)
	if size < count {
		count = size
	}
	data := make([]byte, int(count))
	if count > 0 {
		if _, err := file.ReadAt(data, size-count); err != nil {
			return "", 0, false, err
		}
	}
	digest := sha256.Sum256(data)
	endsWithNewline := len(data) > 0 && data[len(data)-1] == '\n'
	return fmt.Sprintf("%x", digest), len(data), endsWithNewline, nil
}

func addStats(left, right sessionlog.Stats) sessionlog.Stats {
	return sessionlog.Stats{
		Lines:     left.Lines + right.Lines,
		Records:   left.Records + right.Records,
		Malformed: left.Malformed + right.Malformed,
		Unknown:   left.Unknown + right.Unknown,
		Oversized: left.Oversized + right.Oversized,
	}
}

func addSource(result *scanResult, source cachedSource) {
	result.records = append(result.records, source.Records...)
	coverage := result.coverage[source.Provider]
	coverage.Provider = source.Provider
	coverage.Files++
	coverage.Lines += source.Stats.Lines
	coverage.Records += source.Stats.Records
	coverage.Unknown += source.Stats.Unknown
	coverage.Known += source.Stats.Records - source.Stats.Unknown
	coverage.Malformed += source.Stats.Malformed
	coverage.Oversized += source.Stats.Oversized
	coverage.Skipped += source.Stats.Malformed + source.Stats.Oversized
	result.coverage[source.Provider] = coverage
}

func cacheMatches(cached cachedSource, source sessionlog.Source, info fs.FileInfo) bool {
	return cached.Provider == source.Provider && cached.Size == info.Size() &&
		cached.ModTimeUnixNS == info.ModTime().UnixNano() && cached.ParserVersion == sessionlog.ParserVersion
}

func emptyCache() cacheFile {
	return cacheFile{SchemaVersion: CacheSchemaVersion, ParserVersion: sessionlog.ParserVersion, Sources: map[string]cachedSource{}}
}

func loadCache(path string) (cacheFile, string) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyCache(), "missing"
	}
	if err != nil {
		return emptyCache(), "corrupt"
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var cache cacheFile
	if err := decoder.Decode(&cache); err != nil {
		return emptyCache(), "corrupt"
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return emptyCache(), "corrupt"
	}
	if cache.SchemaVersion != CacheSchemaVersion || cache.ParserVersion != sessionlog.ParserVersion {
		return emptyCache(), "stale"
	}
	if !validCache(cache) {
		return emptyCache(), "corrupt"
	}
	return cache, "healthy"
}

func validCache(cache cacheFile) bool {
	if cache.Sources == nil {
		return false
	}
	for path, source := range cache.Sources {
		if path == "" || source.Size < 0 || source.ParserVersion != sessionlog.ParserVersion ||
			(source.Provider != sessionlog.ProviderClaude && source.Provider != sessionlog.ProviderCodex) ||
			source.Stats.Lines < 0 || source.Stats.Records < 0 || source.Stats.Unknown < 0 ||
			source.Stats.Malformed < 0 || source.Stats.Oversized < 0 || source.Stats.Unknown > source.Stats.Records ||
			len(source.Records) != source.Stats.Records || source.TailBytes < 0 ||
			int64(source.TailBytes) > source.Size || len(source.TailSHA256) != sha256.Size*2 {
			return false
		}
		for _, record := range source.Records {
			if record.Provider != source.Provider {
				return false
			}
		}
	}
	return true
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else {
		return err
	}
}

func writeCache(path string, cache cacheFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".log-index-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cache); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func DefaultCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for log index: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("resolve home directory for log index: empty home directory")
	}
	return filepath.Join(home, ".tmact", DefaultCacheName), nil
}

func buckets(counts map[string]int) []Bucket {
	items := make([]Bucket, 0, len(counts))
	for name, count := range counts {
		items = append(items, Bucket{Name: name, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Name < items[j].Name
		}
		return items[i].Count > items[j].Count
	})
	return items
}
