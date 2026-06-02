// useInputHistory — a simple, frontend-only history of sent draft messages.
//
// It keeps the last MAX_HISTORY messages typed into the #draft box (persisted
// to localStorage so they survive reloads) and lets the draft recall them with
// ArrowUp / ArrowDown, shell-style.
//
// Scope is deliberately small (the original app.js had no history at all):
//   - record(msg)        — push a just-sent message (called from sendDraft).
//   - recallPrev(current) — older entry (ArrowUp); stashes the live draft first.
//   - recallNext(current) — newer entry (ArrowDown); restores the stash past the
//                           newest entry.
//   - navigating()       — true while the draft is showing a recalled entry, so
//                           App can keep ArrowUp/Down navigating regardless of
//                           caret position once navigation has started.
//   - reset()            — leave navigation mode (called when the user types).
// recall* return the string to put in the draft, or null when there is nothing
// to change (already at the oldest / already live).

import { useCallback, useRef } from "react";

const HISTORY_KEY = "tmact.inputHistory";
const MAX_HISTORY = 20;

function load(): string[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((s): s is string => typeof s === "string").slice(-MAX_HISTORY);
  } catch {
    return []; // bad JSON / quota / private mode — start empty
  }
}

function save(items: string[]): void {
  try {
    localStorage.setItem(HISTORY_KEY, JSON.stringify(items));
  } catch {
    /* ignore — quota / private mode */
  }
}

export interface InputHistory {
  record: (msg: string) => void;
  recallPrev: (current: string) => string | null;
  recallNext: (current: string) => string | null;
  navigating: () => boolean;
  reset: () => void;
}

export function useInputHistory(): InputHistory {
  // Chronological, oldest → newest. Persisted, capped at MAX_HISTORY.
  const itemsRef = useRef<string[]>(load());
  // cursor === items.length means "live draft" (not browsing history).
  const cursorRef = useRef<number>(itemsRef.current.length);
  // The in-progress text saved when history browsing starts, restored on
  // ArrowDown past the newest entry.
  const stashRef = useRef<string>("");

  const reset = useCallback(() => {
    cursorRef.current = itemsRef.current.length;
  }, []);

  const record = useCallback((msg: string) => {
    const trimmed = msg.trim();
    if (!trimmed) return;
    const items = itemsRef.current;
    // Skip consecutive duplicates so spamming the same prompt keeps history useful.
    if (items.length && items[items.length - 1] === trimmed) {
      reset();
      return;
    }
    items.push(trimmed);
    if (items.length > MAX_HISTORY) items.splice(0, items.length - MAX_HISTORY);
    save(items);
    reset();
  }, [reset]);

  const recallPrev = useCallback((current: string): string | null => {
    const items = itemsRef.current;
    if (cursorRef.current >= items.length) stashRef.current = current; // entering history
    if (cursorRef.current <= 0) return null; // already at the oldest
    cursorRef.current -= 1;
    return items[cursorRef.current] ?? null;
  }, []);

  const recallNext = useCallback((_current: string): string | null => {
    const items = itemsRef.current;
    if (cursorRef.current >= items.length) return null; // already live
    cursorRef.current += 1;
    if (cursorRef.current >= items.length) return stashRef.current; // back to live
    return items[cursorRef.current] ?? null;
  }, []);

  const navigating = useCallback(() => cursorRef.current < itemsRef.current.length, []);

  return { record, recallPrev, recallNext, navigating, reset };
}
