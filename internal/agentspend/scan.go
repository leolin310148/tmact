package agentspend

import (
	"os"
	"sync"
	"time"
)

// pricedRow is one billed call after pricing: when it happened, what it would
// cost at API rates, and a key for cross-file deduplication within a provider.
type pricedRow struct {
	ts    time.Time
	cost  float64
	dedup string
}

// scanner discovers a provider's on-disk session files and prices the rows in
// one file. parseFileChecked must be pure with respect to the file's bytes (no shared
// state) so results can be cached by path+mtime+size.
type scanner interface {
	provider() string
	discover() []string
	parseFileChecked(path string) ([]pricedRow, error)
}

// fileCache memoizes parsed+priced rows per file, keyed by path and invalidated
// on mtime/size change. statusd is long-lived and re-scans every few minutes;
// most session files never change after a session ends, so this turns each
// refresh into "re-read only the files touched since last time".
type fileCache struct {
	mu sync.Mutex
	m  map[string]cachedFile
}

type cachedFile struct {
	mtime time.Time
	size  int64
	rows  []pricedRow
}

func newFileCache() *fileCache { return &fileCache{m: map[string]cachedFile{}} }

func (c *fileCache) getChecked(path string, info os.FileInfo, parse func(string) ([]pricedRow, error)) ([]pricedRow, error) {
	c.mu.Lock()
	ent, ok := c.m[path]
	c.mu.Unlock()
	if ok && ent.mtime.Equal(info.ModTime()) && ent.size == info.Size() {
		return ent.rows, nil
	}
	rows, err := parse(path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.m[path] = cachedFile{mtime: info.ModTime(), size: info.Size(), rows: rows}
	c.mu.Unlock()
	return rows, nil
}

// scanRows runs a scanner over its session files, skipping files last modified
// before `earliest` (they cannot contain rows inside the reporting window).
func scanRows(s scanner, earliest time.Time, fc *fileCache) []pricedRow {
	var out []pricedRow
	for _, path := range s.discover() {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		if info.ModTime().Before(earliest) {
			continue
		}
		rows, err := fc.getChecked(path, info, s.parseFileChecked)
		if err == nil {
			out = append(out, rows...)
		}
	}
	return out
}
