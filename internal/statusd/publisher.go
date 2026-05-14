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

type TmuxOptionCache struct {
	values     map[string]map[string]string
	sessionIDs map[string]string
}

func NewTmuxOptionCache() *TmuxOptionCache {
	return &TmuxOptionCache{
		values:     map[string]map[string]string{},
		sessionIDs: map[string]string{},
	}
}

func PublishTmuxOptions(cfg Config, snapshot Snapshot, caches ...*TmuxOptionCache) error {
	cfg = cfg.withDefaults()
	var cache *TmuxOptionCache
	if len(caches) > 0 {
		cache = caches[0]
	}
	if cache != nil && cache.values == nil {
		cache.values = map[string]map[string]string{}
	}
	if cache != nil && cache.sessionIDs == nil {
		cache.sessionIDs = map[string]string{}
	}

	var errs []error
	seen := map[string]bool{}
	for _, session := range snapshot.Sessions {
		seen[session.Session] = true
		if cache != nil {
			cacheSessionID(cache, session)
		}
		if err := setChangedSessionOption(cfg, cache, session.Session, "@ai-tag", session.Tag); err != nil {
			errs = append(errs, err)
		}
		if err := setChangedSessionOption(cfg, cache, session.Session, "@ai-running", RunningGlyph(session.Running)); err != nil {
			errs = append(errs, err)
		}
		if err := setChangedSessionOption(cfg, cache, session.Session, "@ai-asking", AskingGlyph(session.Asking)); err != nil {
			errs = append(errs, err)
		}
		if err := setChangedSessionOption(cfg, cache, session.Session, "@row-bucket", strconv.Itoa(session.RowBucket)); err != nil {
			errs = append(errs, err)
		}
	}
	if cache != nil {
		for session := range cache.values {
			if !seen[session] {
				delete(cache.values, session)
				delete(cache.sessionIDs, session)
			}
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("publish tmux options: %v", errs)
}

func cacheSessionID(cache *TmuxOptionCache, session SessionStatus) {
	if session.SessionID == "" {
		return
	}
	if cache.sessionIDs[session.Session] != "" && cache.sessionIDs[session.Session] != session.SessionID {
		delete(cache.values, session.Session)
	}
	cache.sessionIDs[session.Session] = session.SessionID
}

func setChangedSessionOption(cfg Config, cache *TmuxOptionCache, session string, key string, value string) error {
	if cache != nil {
		if values := cache.values[session]; values != nil {
			cached, ok := values[key]
			if ok && cached == value {
				return nil
			}
		}
	}
	if err := cfg.SetSessionOption(session, key, value); err != nil {
		return err
	}
	if cache != nil {
		if cache.values[session] == nil {
			cache.values[session] = map[string]string{}
		}
		cache.values[session][key] = value
	}
	return nil
}
