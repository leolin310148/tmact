package sessionlog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discover resolves the provider's configured session roots and returns the
// JSONL sources beneath them. Results are absolute, unique, and retain stable
// provider order (including CLAUDE_CONFIG_DIRS precedence).
func Discover(provider Provider) (Discovery, error) {
	switch provider {
	case ProviderClaude:
		return discoverClaude(), nil
	case ProviderCodex:
		return discoverCodex(), nil
	default:
		return Discovery{}, fmt.Errorf("unsupported session-log provider %q", provider)
	}
}

func claudeConfigDirs() []string {
	if multi := os.Getenv("CLAUDE_CONFIG_DIRS"); multi != "" {
		var dirs []string
		for _, dir := range strings.Split(multi, string(os.PathListSeparator)) {
			if dir = expandHome(strings.TrimSpace(dir)); dir != "" {
				dirs = append(dirs, absolute(dir))
			}
		}
		if dirs = unique(dirs); len(dirs) > 0 {
			return dirs
		}
	}
	if single := os.Getenv("CLAUDE_CONFIG_DIR"); single != "" {
		return []string{expandHome(single)}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return []string{filepath.Join(home, ".claude")}
	}
	return nil
}

func codexSessionsDir() string {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return filepath.Join(dir, "sessions")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".codex", "sessions")
	}
	return ""
}

func discoverClaude() Discovery {
	var result Discovery
	seen := map[string]bool{}
	for _, configDir := range claudeConfigDirs() {
		projects := filepath.Join(configDir, "projects")
		entries, err := os.ReadDir(projects)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				result.Errors = append(result.Errors, DiscoveryError{Path: projects, Err: err})
			}
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			matches, err := filepath.Glob(filepath.Join(projects, entry.Name(), "*.jsonl"))
			if err != nil {
				result.Errors = append(result.Errors, DiscoveryError{Path: projects, Err: err})
				continue
			}
			for _, path := range matches {
				addSource(&result, seen, ProviderClaude, path)
			}
		}
	}
	return result
}

func discoverCodex() Discovery {
	var result Discovery
	base := codexSessionsDir()
	if base == "" {
		return result
	}
	seen := map[string]bool{}
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			result.Errors = append(result.Errors, DiscoveryError{Path: path, Err: err})
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".jsonl") {
			addSource(&result, seen, ProviderCodex, path)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		result.Errors = append(result.Errors, DiscoveryError{Path: base, Err: err})
	}
	sortSources(result.Sources)
	return result
}

func addSource(result *Discovery, seen map[string]bool, provider Provider, path string) {
	path = absolute(path)
	if seen[path] {
		return
	}
	seen[path] = true
	result.Sources = append(result.Sources, Source{Provider: provider, Path: path})
}

func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func absolute(path string) string {
	abs, err := filepath.Abs(path)
	if err == nil {
		return abs
	}
	return path
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := values[:0]
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func sortSources(sources []Source) {
	sort.Slice(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })
}
