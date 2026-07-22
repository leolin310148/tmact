package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/leolin310148/tmact/internal/tmux"
)

const (
	captureCursorVersion     = 1
	captureCursorMaxRows     = 128
	captureCursorMaxEncoded  = 8192
	captureCursorDigestBytes = 12
)

type captureCursorPayload struct {
	Version        int      `json:"v"`
	PaneID         string   `json:"p"`
	RequestedLines int      `json:"n"`
	NonEmpty       bool     `json:"e,omitempty"`
	HistorySize    int      `json:"h"`
	RowHashes      []string `json:"r"`
}

type captureDelta struct {
	Text         string
	FullSnapshot bool
	Reset        bool
	ResetReason  string
}

func newCaptureCursor(info tmux.CapturePaneInfo, requestedLines int, nonEmpty bool, text string) (string, error) {
	rows := splitCaptureRows(text)
	if len(rows) > captureCursorMaxRows {
		rows = rows[len(rows)-captureCursorMaxRows:]
	}
	hashes := make([]string, len(rows))
	for i, row := range rows {
		hashes[i] = captureRowHash(row)
	}
	payload := captureCursorPayload{
		Version:        captureCursorVersion,
		PaneID:         info.PaneID,
		RequestedLines: requestedLines,
		NonEmpty:       nonEmpty,
		HistorySize:    info.HistorySize,
		RowHashes:      hashes,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode capture cursor: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) > captureCursorMaxEncoded {
		return "", errors.New("encoded capture cursor exceeds size limit")
	}
	return encoded, nil
}

func incrementalCapture(after string, info tmux.CapturePaneInfo, requestedLines int, nonEmpty bool, text string) (captureDelta, error) {
	previous, err := decodeCaptureCursor(after)
	if err != nil {
		return captureDelta{}, err
	}
	if previous.PaneID != info.PaneID || previous.RequestedLines != requestedLines || previous.NonEmpty != nonEmpty {
		return resetCapture(text, "cursor_mismatch"), nil
	}
	if info.HistorySize < previous.HistorySize {
		return resetCapture(text, "history_rolled"), nil
	}

	rows := splitCaptureRows(text)
	hashes := make([]string, len(rows))
	for i, row := range rows {
		hashes[i] = captureRowHash(row)
	}
	if len(previous.RowHashes) == 0 {
		return captureDelta{Text: text}, nil
	}

	maxOverlap := min(len(previous.RowHashes), len(hashes))
	for overlap := maxOverlap; overlap > 0; overlap-- {
		previousSuffix := previous.RowHashes[len(previous.RowHashes)-overlap:]
		matchStart := -1
		matchCount := 0
		for i := 0; i+overlap <= len(hashes); i++ {
			if equalCaptureHashes(hashes[i:i+overlap], previousSuffix) {
				matchStart = i
				matchCount++
			}
		}
		if matchCount == 0 {
			continue
		}
		if matchCount > 1 {
			return resetCapture(text, "cursor_ambiguous"), nil
		}
		if overlap < len(previous.RowHashes) && info.HistorySize == previous.HistorySize {
			return resetCapture(text, "cursor_unreconciled"), nil
		}
		return captureDelta{Text: strings.Join(rows[matchStart+overlap:], "")}, nil
	}
	return resetCapture(text, "cursor_unreconciled"), nil
}

func resetCapture(text, reason string) captureDelta {
	return captureDelta{Text: text, FullSnapshot: true, Reset: true, ResetReason: reason}
}

func decodeCaptureCursor(encoded string) (captureCursorPayload, error) {
	if encoded == "" {
		return captureCursorPayload{}, errors.New("capture cursor cannot be empty")
	}
	if len(encoded) > captureCursorMaxEncoded {
		return captureCursorPayload{}, errors.New("invalid capture cursor: exceeds size limit")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return captureCursorPayload{}, fmt.Errorf("invalid capture cursor: %w", err)
	}
	var version struct {
		Version int `json:"v"`
	}
	if err := json.Unmarshal(raw, &version); err != nil {
		return captureCursorPayload{}, fmt.Errorf("invalid capture cursor: %w", err)
	}
	if version.Version != captureCursorVersion {
		return captureCursorPayload{}, fmt.Errorf("unsupported capture cursor version %d", version.Version)
	}
	var payload captureCursorPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return captureCursorPayload{}, fmt.Errorf("invalid capture cursor: %w", err)
	}
	if payload.PaneID == "" || payload.RequestedLines <= 0 || payload.HistorySize < 0 || len(payload.RowHashes) > captureCursorMaxRows {
		return captureCursorPayload{}, errors.New("invalid capture cursor: invalid metadata")
	}
	for _, hash := range payload.RowHashes {
		decoded, err := base64.RawURLEncoding.DecodeString(hash)
		if err != nil || len(decoded) != captureCursorDigestBytes {
			return captureCursorPayload{}, errors.New("invalid capture cursor: invalid row fingerprint")
		}
	}
	return payload, nil
}

func splitCaptureRows(text string) []string {
	if text == "" {
		return nil
	}
	rows := make([]string, 0, strings.Count(text, "\n")+1)
	for len(text) > 0 {
		newline := strings.IndexByte(text, '\n')
		if newline < 0 {
			rows = append(rows, text)
			break
		}
		rows = append(rows, text[:newline+1])
		text = text[newline+1:]
	}
	return rows
}

func captureRowHash(row string) string {
	sum := sha256.Sum256([]byte(row))
	return base64.RawURLEncoding.EncodeToString(sum[:captureCursorDigestBytes])
}

func equalCaptureHashes(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
