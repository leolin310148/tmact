package statusd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func WriteSnapshot(path string, snapshot Snapshot) error {
	if path == "" {
		path = DefaultStatePath
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func ReadSnapshot(path string) (Snapshot, error) {
	if path == "" {
		path = DefaultStatePath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func PublishTmuxOptions(cfg Config, snapshot Snapshot) error {
	cfg = cfg.withDefaults()
	var errs []error
	for _, session := range snapshot.Sessions {
		if err := cfg.SetSessionOption(session.Session, "@ai-tag", session.Tag); err != nil {
			errs = append(errs, err)
		}
		if err := cfg.SetSessionOption(session.Session, "@ai-running", RunningGlyph(session.Running)); err != nil {
			errs = append(errs, err)
		}
		if err := cfg.SetSessionOption(session.Session, "@ai-asking", AskingGlyph(session.Asking)); err != nil {
			errs = append(errs, err)
		}
		if err := cfg.SetSessionOption(session.Session, "@row-bucket", strconv.Itoa(session.RowBucket)); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("publish tmux options: %v", errs)
}
