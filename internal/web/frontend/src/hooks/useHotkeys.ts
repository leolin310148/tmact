// Desktop pane-switch + clear-pane hotkeys — a faithful 1:1 port of app.js's
// wireHotkeys (lines 881–904). Spec §6 items 38–40.
//
// BEHAVIOR (must match app.js exactly):
//   - Clear pane: Cmd+K (mac, metaKey && !ctrl && !alt && key==="k") OR
//     Ctrl+L (ctrlKey && !meta && !alt && key==="l") → preventDefault +
//     stopPropagation + clearPaneOutput(). Gated on a pane being selected AND
//     the settings overlay being closed.
//   - Pane switch: plain Option (altKey, no Ctrl/Cmd) + a layout-independent
//     KeyboardEvent.code in HOTKEY_INDEX → selectPane(state.paneOrder[idx]).
//     Skipped entirely on mobile (isMobile()), when settings is open, or when
//     idx is out of range (idx >= state.paneOrder.length).
//
// LISTENER PHASE (load-bearing — ARCHITECTURE.md §9 step 5, spec §6 item 40):
//   The keydown listener is registered in the CAPTURE phase
//   (`document.addEventListener("keydown", handler, true)`) so it wins over the
//   direct-mode relay on #direct-input (which is a bubble-phase listener). In
//   direct mode every keystroke is forwarded to the pane; without capturing
//   first, Option+1 would be sent to the pane instead of switching panes.
//   This hook must be mounted (its effect run) before DirectInput's keydown is
//   wired — App mounts it earlier in the tree per the wiring order.
//
// STATE READS (app.js read module-scoped `state`):
//   `state.selected` and `state.paneOrder` are read from the live, mutated-by-
//   reference store object (useAppState().state), so the capture handler always
//   sees the current values without re-registering — identical to app.js reading
//   the module-level `state`. The settings-overlay-open check is provided by the
//   injected `settingsOpen()` predicate (true === overlay open === app.js's
//   `!$("settings-overlay").hidden`).

import { useEffect } from "react";
import { isMobile } from "../lib/dom";
import { HOTKEY_INDEX } from "../lib/keymap";
import { useAppState } from "../store/AppStateContext";

export interface UseHotkeysDeps {
  /**
   * app.js `selectPane(paneID)`. Selects the Nth statusline chip. (App passes
   * `callbacks.selectPane`.)
   */
  selectPane: (paneID: string) => void;
  /**
   * app.js `clearPaneOutput()` — `wsSend({t:"clear"})` (else error). App owns
   * this (it is not in the AppCallbacks contract); App passes its local impl.
   */
  clearPaneOutput: () => void;
  /**
   * Predicate: is the settings overlay currently OPEN? Mirrors app.js's
   * `!$("settings-overlay").hidden`. The hotkeys are suppressed while it is open
   * (an open settings panel may legitimately want Option for text input, and the
   * clear-pane chord must not fire over a modal).
   */
  settingsOpen: () => boolean;
}

export function useHotkeys({
  selectPane,
  clearPaneOutput,
  settingsOpen,
}: UseHotkeysDeps): void {
  const { state } = useAppState();

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      // Clear-pane chord: only when a pane is selected and settings is closed.
      // app.js gated on `state.selected && $("settings-overlay").hidden`.
      if (state.selected && !settingsOpen()) {
        const k = e.key.toLowerCase();
        const clearPane =
          (e.metaKey && !e.ctrlKey && !e.altKey && k === "k") ||
          (e.ctrlKey && !e.metaKey && !e.altKey && k === "l");
        if (clearPane) {
          e.preventDefault();
          e.stopPropagation();
          clearPaneOutput();
          return;
        }
      }
      // macOS-only chord: plain Option, no Ctrl/Cmd. Mobile has no Option key,
      // and an open settings panel may legitimately want Option for text input.
      if (!e.altKey || e.ctrlKey || e.metaKey || isMobile()) return;
      if (settingsOpen()) return;
      const idx = HOTKEY_INDEX[e.code];
      if (idx === undefined || idx >= state.paneOrder.length) return;
      e.preventDefault();
      e.stopPropagation();
      const target = state.paneOrder[idx];
      // noUncheckedIndexedAccess: guarded by the idx < length check above, but
      // narrow the possibly-undefined access explicitly.
      if (target !== undefined) selectPane(target);
    };

    // Capture phase — see header comment (must precede #direct-input keydown).
    document.addEventListener("keydown", onKeyDown, true);
    return () => document.removeEventListener("keydown", onKeyDown, true);
  }, [selectPane, clearPaneOutput, settingsOpen, state]);
}
