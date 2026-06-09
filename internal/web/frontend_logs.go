package web

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"unicode"
)

const (
	frontendLogsMaxBodyBytes = 64 << 10
	frontendLogsMaxEntries   = 100
	frontendLogMaxMessage    = 300
	frontendLogMaxString     = 300
	frontendLogMaxDataBytes  = 2 << 10
)

type frontendLogPayloadIn struct {
	SessionID string               `json:"session_id"`
	SentAt    string               `json:"sent_at"`
	Device    frontendLogDeviceIn  `json:"device"`
	Entries   []frontendLogEntryIn `json:"entries"`
}

type frontendLogDeviceIn struct {
	Platform         string             `json:"platform,omitempty"`
	UserAgentSummary string             `json:"user_agent_summary,omitempty"`
	UserAgentBrands  []frontendLogBrand `json:"user_agent_brands,omitempty"`
	Viewport         frontendLogSize    `json:"viewport,omitempty"`
	Screen           frontendLogSize    `json:"screen,omitempty"`
	DevicePixelRatio float64            `json:"device_pixel_ratio,omitempty"`
	Orientation      string             `json:"orientation,omitempty"`
	VisibilityState  string             `json:"visibility_state,omitempty"`
	Online           bool               `json:"online"`
}

type frontendLogBrand struct {
	Brand   string `json:"brand,omitempty"`
	Version string `json:"version,omitempty"`
}

type frontendLogSize struct {
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

type frontendLogEntryIn struct {
	TS      string          `json:"ts"`
	Level   string          `json:"level"`
	Event   string          `json:"event"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type frontendLogPayloadOut struct {
	SessionID string                `json:"session_id,omitempty"`
	SentAt    string                `json:"sent_at,omitempty"`
	Device    frontendLogDeviceOut  `json:"device"`
	Entries   []frontendLogEntryOut `json:"entries"`
}

type frontendLogDeviceOut struct {
	Platform         string             `json:"platform,omitempty"`
	UserAgentSummary string             `json:"user_agent_summary,omitempty"`
	UserAgentBrands  []frontendLogBrand `json:"user_agent_brands,omitempty"`
	Viewport         frontendLogSize    `json:"viewport,omitempty"`
	Screen           frontendLogSize    `json:"screen,omitempty"`
	DevicePixelRatio float64            `json:"device_pixel_ratio,omitempty"`
	Orientation      string             `json:"orientation,omitempty"`
	VisibilityState  string             `json:"visibility_state,omitempty"`
	Online           bool               `json:"online"`
}

type frontendLogEntryOut struct {
	TS      string         `json:"ts,omitempty"`
	Level   string         `json:"level"`
	Event   string         `json:"event,omitempty"`
	Message string         `json:"message,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

func (s *Server) handleFrontendLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, frontendLogsMaxBodyBytes)
	var in frontendLogPayloadIn
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&in); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: trailing data")
		return
	}

	out := sanitizeFrontendLogPayload(in)
	compact, err := json.Marshal(out)
	if err == nil {
		s.logf("frontend log: %s", compact)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func sanitizeFrontendLogPayload(in frontendLogPayloadIn) frontendLogPayloadOut {
	entries := in.Entries
	if len(entries) > frontendLogsMaxEntries {
		entries = entries[:frontendLogsMaxEntries]
	}
	out := frontendLogPayloadOut{
		SessionID: sanitizeLogString(in.SessionID, 80),
		SentAt:    sanitizeLogString(in.SentAt, 64),
		Device:    sanitizeFrontendLogDevice(in.Device),
		Entries:   make([]frontendLogEntryOut, 0, len(entries)),
	}
	for _, entry := range entries {
		out.Entries = append(out.Entries, frontendLogEntryOut{
			TS:      sanitizeLogString(entry.TS, 64),
			Level:   sanitizeFrontendLogLevel(entry.Level),
			Event:   sanitizeLogString(entry.Event, 80),
			Message: sanitizeLogString(entry.Message, frontendLogMaxMessage),
			Data:    sanitizeFrontendLogData(entry.Data),
		})
	}
	return out
}

func sanitizeFrontendLogDevice(in frontendLogDeviceIn) frontendLogDeviceOut {
	brands := make([]frontendLogBrand, 0, min(len(in.UserAgentBrands), 8))
	for _, b := range in.UserAgentBrands {
		brands = append(brands, frontendLogBrand{
			Brand:   sanitizeLogString(b.Brand, 80),
			Version: sanitizeLogString(b.Version, 40),
		})
		if len(brands) == 8 {
			break
		}
	}
	return frontendLogDeviceOut{
		Platform:         sanitizeLogString(in.Platform, 80),
		UserAgentSummary: sanitizeLogString(in.UserAgentSummary, 160),
		UserAgentBrands:  brands,
		Viewport:         sanitizeLogSize(in.Viewport),
		Screen:           sanitizeLogSize(in.Screen),
		DevicePixelRatio: sanitizeFiniteFloat(in.DevicePixelRatio, 100),
		Orientation:      sanitizeLogString(in.Orientation, 80),
		VisibilityState:  sanitizeLogString(in.VisibilityState, 40),
		Online:           in.Online,
	}
}

func sanitizeLogSize(in frontendLogSize) frontendLogSize {
	return frontendLogSize{
		Width:  clampInt(in.Width, 0, 100000),
		Height: clampInt(in.Height, 0, 100000),
	}
}

func sanitizeFiniteFloat(v float64, max float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}

func sanitizeFrontendLogLevel(level string) string {
	clean := sanitizeLogString(level, 16)
	switch clean {
	case "info", "warn", "error":
		return clean
	default:
		return "info"
	}
}

func sanitizeFrontendLogData(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil
	}
	out := sanitizeJSONObject(obj, 0)
	if len(out) == 0 {
		return out
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	if len(b) <= frontendLogMaxDataBytes {
		return out
	}
	return trimJSONObjectToBytes(out, frontendLogMaxDataBytes)
}

func trimJSONObjectToBytes(in map[string]any, maxBytes int) map[string]any {
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := map[string]any{"_truncated": true}
	for _, k := range keys {
		candidate := make(map[string]any, len(out)+1)
		for existingK, existingV := range out {
			candidate[existingK] = existingV
		}
		candidate[k] = in[k]
		b, err := json.Marshal(candidate)
		if err != nil || len(b) > maxBytes {
			continue
		}
		out = candidate
	}
	return out
}

func sanitizeJSONObject(in map[string]any, depth int) map[string]any {
	if depth >= 4 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, min(len(keys), 40))
	for _, k := range keys {
		if len(out) == 40 {
			break
		}
		cleanKey := sanitizeLogString(k, 80)
		if cleanKey == "" {
			continue
		}
		if v, ok := sanitizeJSONValue(in[k], depth+1); ok {
			out[cleanKey] = v
		}
	}
	return out
}

func sanitizeJSONValue(v any, depth int) (any, bool) {
	switch x := v.(type) {
	case nil:
		return nil, true
	case bool:
		return x, true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return nil, false
		}
		return x, true
	case string:
		return sanitizeLogString(x, frontendLogMaxString), true
	case []any:
		if depth >= 4 {
			return nil, false
		}
		n := min(len(x), 20)
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			if v, ok := sanitizeJSONValue(x[i], depth+1); ok {
				out = append(out, v)
			}
		}
		return out, true
	case map[string]any:
		return sanitizeJSONObject(x, depth), true
	default:
		return nil, false
	}
}

func sanitizeLogString(s string, maxRunes int) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	clean := b.String()
	if maxRunes <= 0 {
		return clean
	}
	count := 0
	for i := range clean {
		if count == maxRunes {
			return clean[:i]
		}
		count++
	}
	return clean
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
