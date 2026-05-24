package statusd

import (
	"fmt"
	"strconv"
)

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
