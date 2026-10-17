package statusd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Pane change memory survives daemon restarts so last_changed_at (the web
// UI's "recently working" signal) isn't wiped by every deploy. Only the
// per-pane screen hash and last change time are kept; snapshots themselves
// stay in memory.
const (
	DefaultPaneMemoryName = "statusd-pane-memory.json"
	// DefaultPaneMemorySaveInterval bounds how much change history a crash
	// can lose; a clean shutdown saves immediately.
	DefaultPaneMemorySaveInterval = 30 * time.Second
	paneMemoryFileVersion         = 1
)

// DefaultPaneMemoryPath returns ~/.tmact/statusd-pane-memory.json, or "" when
// the home dir is unknown (which disables persistence).
func DefaultPaneMemoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, DefaultFileConfigDir, DefaultPaneMemoryName)
}

type paneMemoryFile struct {
	Version int                        `json:"version"`
	Panes   map[string]paneMemoryEntry `json:"panes"`
}

type paneMemoryEntry struct {
	Hash          string    `json:"hash"`
	LastChangedAt time.Time `json:"last_changed_at"`
}

// LoadPaneMemory seeds mem from path. Restored entries only carry history:
// the first scan still multi-samples, and a pane whose screen no longer
// matches its saved hash is stamped changed without being reported running.
// A missing file is not an error.
func LoadPaneMemory(path string, mem *Memory) error {
	if path == "" || mem == nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var file paneMemoryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	if file.Version != paneMemoryFileVersion {
		return fmt.Errorf("%s: unsupported version %d", path, file.Version)
	}
	for paneID, entry := range file.Panes {
		if paneID == "" || entry.Hash == "" {
			continue
		}
		mem.panes[paneID] = paneMemory{
			Hash:          entry.Hash,
			LastChangedAt: entry.LastChangedAt,
			restored:      true,
		}
	}
	return nil
}

// encodePaneMemory serializes the change history worth keeping: panes that
// have a last change time (the rest carry nothing the next run can use).
func encodePaneMemory(mem *Memory) ([]byte, error) {
	file := paneMemoryFile{Version: paneMemoryFileVersion, Panes: map[string]paneMemoryEntry{}}
	for paneID, entry := range mem.panes {
		if entry.Hash == "" || entry.LastChangedAt.IsZero() {
			continue
		}
		file.Panes[paneID] = paneMemoryEntry{Hash: entry.Hash, LastChangedAt: entry.LastChangedAt.UTC()}
	}
	return json.MarshalIndent(file, "", "  ")
}

// SavePaneMemory writes mem to path atomically.
func SavePaneMemory(path string, mem *Memory) error {
	if path == "" || mem == nil {
		return nil
	}
	data, err := encodePaneMemory(mem)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeAtomicFile(path, data, 0o600)
}
