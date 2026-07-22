package main

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/leolin310148/tmact/internal/tmux"
)

func TestIncrementalCaptureAppend(t *testing.T) {
	info := tmux.CapturePaneInfo{PaneID: "%7", HistorySize: 20}
	cursor := mustCaptureCursor(t, info, 120, false, "one\ntwo\n")
	delta, err := incrementalCapture(cursor, tmux.CapturePaneInfo{PaneID: "%7", HistorySize: 21}, 120, false, "one\ntwo\nthree\n")
	if err != nil {
		t.Fatal(err)
	}
	if delta.Text != "three\n" || delta.FullSnapshot || delta.Reset || delta.ResetReason != "" {
		t.Fatalf("delta = %#v", delta)
	}
}

func TestIncrementalCaptureAppendAfterCaptureWindowShifts(t *testing.T) {
	info := tmux.CapturePaneInfo{PaneID: "%7", HistorySize: 20}
	cursor := mustCaptureCursor(t, info, 120, false, "dropped\none\ntwo\n")
	delta, err := incrementalCapture(cursor, tmux.CapturePaneInfo{PaneID: "%7", HistorySize: 21}, 120, false, "one\ntwo\nthree\n")
	if err != nil {
		t.Fatal(err)
	}
	if delta.Text != "three\n" || delta.FullSnapshot || delta.Reset {
		t.Fatalf("delta = %#v", delta)
	}
}

func TestIncrementalCaptureUnchanged(t *testing.T) {
	info := tmux.CapturePaneInfo{PaneID: "%7", HistorySize: 20}
	cursor := mustCaptureCursor(t, info, 120, false, "one\ntwo\n")
	delta, err := incrementalCapture(cursor, info, 120, false, "one\ntwo\n")
	if err != nil {
		t.Fatal(err)
	}
	if delta.Text != "" || delta.FullSnapshot || delta.Reset {
		t.Fatalf("delta = %#v", delta)
	}
}

func TestIncrementalCaptureRewrittenScreenResets(t *testing.T) {
	info := tmux.CapturePaneInfo{PaneID: "%7", HistorySize: 20}
	cursor := mustCaptureCursor(t, info, 120, false, "prompt\ndraft\n")
	delta, err := incrementalCapture(cursor, info, 120, false, "menu\ndraft\n")
	if err != nil {
		t.Fatal(err)
	}
	assertCaptureReset(t, delta, "menu\ndraft\n", "cursor_unreconciled")
}

func TestIncrementalCaptureHistoryRolloverResets(t *testing.T) {
	cursor := mustCaptureCursor(t, tmux.CapturePaneInfo{PaneID: "%7", HistorySize: 200}, 120, false, "old\n")
	delta, err := incrementalCapture(cursor, tmux.CapturePaneInfo{PaneID: "%7", HistorySize: 150}, 120, false, "fresh\n")
	if err != nil {
		t.Fatal(err)
	}
	assertCaptureReset(t, delta, "fresh\n", "history_rolled")
}

func TestIncrementalCaptureRejectsInvalidCursor(t *testing.T) {
	_, err := incrementalCapture("not-a-cursor", tmux.CapturePaneInfo{PaneID: "%7"}, 120, false, "fresh\n")
	if err == nil || !strings.Contains(err.Error(), "invalid capture cursor") {
		t.Fatalf("err = %v", err)
	}
}

func TestIncrementalCaptureRejectsVersionMismatch(t *testing.T) {
	cursor := base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"p":"%7","n":120,"h":0,"r":[]}`))
	_, err := incrementalCapture(cursor, tmux.CapturePaneInfo{PaneID: "%7"}, 120, false, "fresh\n")
	if err == nil || !strings.Contains(err.Error(), "unsupported capture cursor version 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestCaptureCursorIsBoundedAndContainsOnlyFingerprints(t *testing.T) {
	text := strings.Repeat("secret pane row\n", captureCursorMaxRows+50)
	cursor := mustCaptureCursor(t, tmux.CapturePaneInfo{PaneID: "%7", HistorySize: 500}, 1000, false, text)
	if len(cursor) > captureCursorMaxEncoded {
		t.Fatalf("cursor length = %d", len(cursor))
	}
	if strings.Contains(cursor, "secret") {
		t.Fatalf("cursor contains pane content: %q", cursor)
	}
	payload, err := decodeCaptureCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload.RowHashes) != captureCursorMaxRows {
		t.Fatalf("row fingerprints = %d", len(payload.RowHashes))
	}
}

func TestIncrementalCaptureOptionMismatchResets(t *testing.T) {
	info := tmux.CapturePaneInfo{PaneID: "%7", HistorySize: 20}
	cursor := mustCaptureCursor(t, info, 120, false, "one\n")
	delta, err := incrementalCapture(cursor, info, 80, false, "one\n")
	if err != nil {
		t.Fatal(err)
	}
	assertCaptureReset(t, delta, "one\n", "cursor_mismatch")
}

func mustCaptureCursor(t *testing.T, info tmux.CapturePaneInfo, lines int, nonEmpty bool, text string) string {
	t.Helper()
	cursor, err := newCaptureCursor(info, lines, nonEmpty, text)
	if err != nil {
		t.Fatal(err)
	}
	return cursor
}

func assertCaptureReset(t *testing.T, delta captureDelta, text, reason string) {
	t.Helper()
	if delta.Text != text || !delta.FullSnapshot || !delta.Reset || delta.ResetReason != reason {
		t.Fatalf("delta = %#v", delta)
	}
}
