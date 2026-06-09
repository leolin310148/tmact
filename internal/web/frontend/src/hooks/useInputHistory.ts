// useInputHistory — a simple, frontend-only history of sent draft messages.
//
// It keeps the last MAX_HISTORY messages typed into the #draft box per pane
// (persisted to localStorage so they survive reloads) and lets the draft recall
// them with ArrowUp / ArrowDown, shell-style.
//
// Scope is deliberately small (the original app.js had no history at all):
//   - record(pane,msg)         — push a just-sent message (called from sendDraft).
//   - recallPrev(pane,current) — older entry (ArrowUp); stashes the live draft first.
//   - recallNext(pane,current) — newer entry (ArrowDown); restores the stash past
//                                the newest entry.
//   - navigating(pane)         — true while the draft is showing a recalled entry,
//                                so App can keep ArrowUp/Down navigating regardless
//                                of caret position once navigation has started.
//   - reset(pane)              — leave navigation mode (called when the user types).
// recall* return the string to put in the draft, or null when there is nothing
// to change (already at the oldest / already live).

import { useCallback, useRef } from "react";

const HISTORY_KEY = "tmact.inputHistory";
const MAX_HISTORY = 20;

type HistoryByPane = Record<string, string[]>;

interface StoredHistory {
  panes?: unknown;
}

function cleanItems(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((s): s is string => typeof s === "string").slice(-MAX_HISTORY);
}

function load(): HistoryByPane {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    const panes = (parsed as StoredHistory).panes;
    if (!panes || typeof panes !== "object" || Array.isArray(panes)) return {};
    const next: HistoryByPane = {};
    for (const [pane, items] of Object.entries(panes)) {
      const cleaned = cleanItems(items);
      if (cleaned.length) next[pane] = cleaned;
    }
    return next;
  } catch {
    return {}; // bad JSON / quota / private mode — start empty
  }
}

function save(panes: HistoryByPane): void {
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify({ panes }));
  } catch {
    /* ignore — quota / private mode */
  }
}

export interface InputHistory {
  record: (pane: string | null, msg: string) => void;
  recallPrev: (pane: string | null, current: string) => string | null;
  recallNext: (pane: string | null, current: string) => string | null;
  navigating: (pane: string | null) => boolean;
  reset: (pane: string | null) => void;
}

export function useInputHistory(): InputHistory {
  // Chronological, oldest → newest, keyed by pane id. Persisted, capped at MAX_HISTORY.
  const itemsRef = useRef<HistoryByPane>(load());
  // cursor === items.length means "live draft" (not browsing history), per pane.
  const cursorRef = useRef<Record<string, number>>({});
  // The in-progress text saved when history browsing starts, restored on
  // ArrowDown past the newest entry, per pane.
  const stashRef = useRef<Record<string, string>>({});

  const itemsFor = useCallback((pane: string | null): string[] | null => {
    if (!pane) return null;
    return itemsRef.current[pane] || [];
  }, []);

  const reset = useCallback(
    (pane: string | null) => {
      const items = itemsFor(pane);
      if (!pane || !items) return;
      cursorRef.current[pane] = items.length;
    },
    [itemsFor],
  );

  const record = useCallback(
    (pane: string | null, msg: string) => {
      if (!pane) return;
      const trimmed = msg.trim();
      if (!trimmed) return;
      const items = itemsRef.current[pane] || [];
      // Skip consecutive duplicates so spamming the same prompt keeps history useful.
      if (items.length && items[items.length - 1] === trimmed) {
        reset(pane);
        return;
      }
      const next = items.concat(trimmed).slice(-MAX_HISTORY);
      itemsRef.current[pane] = next;
      save(itemsRef.current);
      reset(pane);
    },
    [reset],
  );

  const recallPrev = useCallback(
    (pane: string | null, current: string): string | null => {
      const items = itemsFor(pane);
      if (!pane || !items || !items.length) return null;
      const cursor = cursorRef.current[pane] ?? items.length;
      if (cursor >= items.length) stashRef.current[pane] = current; // entering history
      if (cursor <= 0) return null; // already at the oldest
      const nextCursor = cursor - 1;
      cursorRef.current[pane] = nextCursor;
      return items[nextCursor] ?? null;
    },
    [itemsFor],
  );

  const recallNext = useCallback(
    (pane: string | null, _current: string): string | null => {
      const items = itemsFor(pane);
      if (!pane || !items) return null;
      const cursor = cursorRef.current[pane] ?? items.length;
      if (cursor >= items.length) return null; // already live
      const nextCursor = cursor + 1;
      cursorRef.current[pane] = nextCursor;
      if (nextCursor >= items.length) return stashRef.current[pane] || ""; // back to live
      return items[nextCursor] ?? null;
    },
    [itemsFor],
  );

  const navigating = useCallback(
    (pane: string | null) => {
      const items = itemsFor(pane);
      if (!pane || !items) return false;
      return (cursorRef.current[pane] ?? items.length) < items.length;
    },
    [itemsFor],
  );

  return { record, recallPrev, recallNext, navigating, reset };
}
