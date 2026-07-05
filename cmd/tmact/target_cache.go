package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/leolin310148/tmact/internal/statusd"
)

type targetCache struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Panes       []listPaneRow `json:"panes"`
}

const targetCacheMaxAge = 30 * time.Minute

func resolveTarget(selector string) (string, error) {
	index, err := strconv.Atoi(selector)
	if err != nil {
		return selector, nil
	}
	if index < 0 {
		return "", fmt.Errorf("target index %d is invalid", index)
	}
	cache, err := readTargetCache()
	if err != nil {
		return "", err
	}
	if tmactNow().Sub(cache.GeneratedAt) > targetCacheMaxAge {
		return "", fmt.Errorf("target cache is older than %s; run `tmact ls` again", targetCacheMaxAge)
	}
	if index >= len(cache.Panes) {
		return "", fmt.Errorf("target index %d not found; run `tmact ls` again", index)
	}
	row := cache.Panes[index]
	if peer, _ := statusd.SplitPeerTarget(row.Target); peer != "" {
		return row.Target, nil
	}
	if _, err := listTargetTmuxPanes(row.Target); err != nil {
		return "", fmt.Errorf("cached target %d (%s) is no longer available; run `tmact ls` again: %w", index, row.Target, err)
	}
	return row.Target, nil
}

func targetCachePath() string {
	return filepath.Join(".cache", "tmact-targets.json")
}

func writeTargetCache(cache targetCache) error {
	path := targetCachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func readTargetCache() (targetCache, error) {
	var cache targetCache
	data, err := os.ReadFile(targetCachePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cache, errors.New("target cache not found; run `tmact ls` first")
		}
		return cache, err
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		return cache, fmt.Errorf("read target cache: %w", err)
	}
	return cache, nil
}
