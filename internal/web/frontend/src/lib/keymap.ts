// Keyboard mapping tables and translation, ported 1:1 from static/app.js.
//
// Covers:
//   - PANE_HOTKEYS / HOTKEY_CODE / HOTKEY_INDEX: Option+<key> pane-switch hotkeys
//     (layout-independent via KeyboardEvent.code) — spec §6 item 38.
//   - KEYMAP + translateKey: direct-mode keystroke translation — spec §6 item 33.

import type { InputMsg } from "../types/server";

// Desktop pane-switch hotkeys: Option+<key> jumps to the Nth chip in the
// statusline. The labels are what the chip badge shows; HOTKEY_CODE maps each
// to its layout-independent KeyboardEvent.code (e.key is unusable here —
// holding Option on macOS rewrites it to an accented character).
export const PANE_HOTKEYS: readonly string[] = [
  "1", "2", "3", "4", "5", "6", "7", "8", "9", "0",
  "q", "w", "e", "r", "t", "y", "u", "i", "o", "p",
];

export const HOTKEY_CODE: Record<string, string> = {
  "1": "Digit1", "2": "Digit2", "3": "Digit3", "4": "Digit4", "5": "Digit5",
  "6": "Digit6", "7": "Digit7", "8": "Digit8", "9": "Digit9", "0": "Digit0",
  "q": "KeyQ", "w": "KeyW", "e": "KeyE", "r": "KeyR", "t": "KeyT",
  "y": "KeyY", "u": "KeyU", "i": "KeyI", "o": "KeyO", "p": "KeyP",
};

// code → chip index, built once so the keydown handler is a single lookup.
export const HOTKEY_INDEX: Record<string, number> = {};
PANE_HOTKEYS.forEach((label, i) => {
  const code = HOTKEY_CODE[label];
  if (code !== undefined) HOTKEY_INDEX[code] = i;
});

/* ---- mode 2: direct keystroke passthrough ---- */

export const KEYMAP: Record<string, string> = {
  Enter: "Enter", Backspace: "BSpace", Tab: "Tab", Escape: "Escape",
  ArrowUp: "Up", ArrowDown: "Down", ArrowLeft: "Left", ArrowRight: "Right",
  Home: "Home", End: "End", PageUp: "PageUp", PageDown: "PageDown", Delete: "Delete",
};

export function translateKey(e: KeyboardEvent): InputMsg | null {
  if (e.metaKey) return null; // leave Cmd shortcuts to the browser
  if (e.ctrlKey) {
    const lk = e.key.toLowerCase();
    if (lk.length === 1 && lk >= "a" && lk <= "z") return { t: "key", k: "C-" + lk };
    const mapped = KEYMAP[e.key];
    if (mapped !== undefined) return { t: "key", k: mapped };
    return null;
  }
  // Shift+Enter inserts a line break instead of submitting: a bare "\n" is
  // pasted (bracketed paste), so the agent's input box takes it as a newline
  // rather than the Return that a plain Enter sends.
  if (e.key === "Enter" && e.shiftKey) return { t: "text", s: "\n" };
  if (e.key === "Tab" && e.shiftKey) return { t: "key", k: "BTab" };
  const mapped = KEYMAP[e.key];
  if (mapped !== undefined) return { t: "key", k: mapped };
  if (e.key.length === 1) return { t: "text", s: e.key };
  return null;
}

// Desktop direct mode uses US physical keys so an active IME cannot turn
// terminal commands into composition text. Mobile keeps its soft-keyboard path.
export function translateDirectKey(e: KeyboardEvent, physical: boolean): InputMsg | null {
  if (e.metaKey) return null;
  if (e.ctrlKey && /^Key[A-Z]$/.test(e.code)) {
    return { t: "key", k: "C-" + e.code.slice(3).toLowerCase() };
  }
  if (!physical) {
    if (e.isComposing || e.keyCode === 229) return null;
    return translateKey(e);
  }
  let key = e.key;
  if (/^Key[A-Z]$/.test(e.code)) {
    const upper = e.shiftKey !== e.getModifierState("CapsLock");
    key = upper ? e.code.slice(3) : e.code.slice(3).toLowerCase();
  } else if (/^Digit[0-9]$/.test(e.code)) {
    const digit = e.code.slice(5);
    key = e.shiftKey ? ")!@#$%^&*("[Number(digit)]! : digit;
  } else {
    const punctuation: Record<string, [string, string]> = {
      Backquote: ["`", "~"], Minus: ["-", "_"], Equal: ["=", "+"],
      BracketLeft: ["[", "{"], BracketRight: ["]", "}"],
      Backslash: ["\\", "|"], Semicolon: [";", ":"], Quote: ["'", '\"'],
      Comma: [",", "<"], Period: [".", ">"], Slash: ["/", "?"], Space: [" ", " "],
    };
    const pair = punctuation[e.code];
    if (pair) key = pair[e.shiftKey ? 1 : 0];
    else if (KEYMAP[e.code]) key = e.code;
    else if (e.isComposing || e.keyCode === 229) return null;
  }
  return translateKey(new KeyboardEvent("keydown", {
    key, ctrlKey: e.ctrlKey, shiftKey: e.shiftKey,
  }));
}
