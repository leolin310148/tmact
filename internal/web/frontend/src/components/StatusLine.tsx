// StatusLine — the chip list (#chips), ported 1:1 from app.js renderStatusline.
//
// Parity notes (MIGRATION_SPEC §6 items 7,8,9; ARCHITECTURE §8/§10):
//   - One chip per pane in snapshot.panes, sorted
//     peer → sessionLabel → window_index → pane_index.
//   - Duplicate (baseLabel + session) keys append ":" + window_index.
//   - state.paneOrder is FROZEN to the rendered pane_id order so the Option+key
//     hotkey indices map to the same chips left-to-right (app.js mutated
//     state.paneOrder inside renderStatusline; we do the same, by reference).
//   - Empty pane set renders <span class="empty">No tmux panes.</span>.
//   - Chip click → callbacks.selectPane(pane_id).
//
// The original re-ran renderStatusline imperatively after a mutation; here the
// component re-renders on each bump() (it reads the live state object).

import { useAppState } from "../store/AppStateContext";
import { PANE_HOTKEYS } from "../lib/keymap";
import type { PaneStatus } from "../types/server";
import { Chip } from "./Chip";

// PANE_HOTKEYS is the single source of truth in lib/keymap (it also drives
// HOTKEY_INDEX for the keydown handler). The chip badge shows these labels.

export const RUNTIME_ICON: Record<string, string> = {
  claude: "cc",
  codex: "cx",
  copilot: "cp",
  gemini: "g",
};

// ---- pure pane helpers (verbatim from app.js) ----

export function paneStateClass(p: PaneStatus): string {
  if (p.stale) return "stale";
  if (p.asking) return "asking";
  if (p.running) return "running";
  return "idle";
}

export function paneStateLabel(p: PaneStatus): string {
  if (p.stale) return "stale";
  if (p.asking) return "asking";
  if (p.running) return "working";
  if (p.idle) return "idle";
  if (!p.state || p.state === "unknown") return "—";
  return p.state;
}

export function paneRuntime(p: PaneStatus): string {
  return (p.runtime || "").toLowerCase();
}

export function panePeer(p: PaneStatus | null | undefined): string {
  return p && p.peer ? String(p.peer) : "";
}

export function sessionLabel(p: PaneStatus): string {
  const peer = panePeer(p);
  const session = p && p.session ? String(p.session) : "";
  if (peer && session.startsWith(peer + "@")) return session.slice(peer.length + 1);
  return session;
}

export function StatusLine() {
  const { state, callbacks } = useAppState();
  const snap = state.snapshot;
  const panes: PaneStatus[] = snap && snap.panes ? Object.values(snap.panes) : [];

  if (panes.length === 0) {
    // Freeze the (empty) order exactly as app.js would on a paneless snapshot:
    // app.js returns early WITHOUT touching state.paneOrder, so leave it alone
    // here too — match the original's behavior precisely.
    return (
      <div className="chips" id="chips">
        <span className="empty">No tmux panes.</span>
      </div>
    );
  }

  // Sort: peer → sessionLabel → window_index → pane_index (verbatim).
  panes.sort((a, b) =>
    panePeer(a).localeCompare(panePeer(b)) ||
    sessionLabel(a).localeCompare(sessionLabel(b)) ||
    a.window_index - b.window_index ||
    a.pane_index - b.pane_index);

  // Count occurrences of each peer\0baseLabel so we can disambiguate dupes.
  const perSession: Record<string, number> = {};
  for (const p of panes) {
    const k = panePeer(p) + "\0" + sessionLabel(p);
    perSession[k] = (perSession[k] || 0) + 1;
  }

  // Freeze the rendered order so Option+key hotkeys map to the same chips, in
  // the same left-to-right sequence. Mutate state.paneOrder by reference,
  // exactly like app.js (no bump — App reads it through state).
  state.paneOrder = panes.map((p) => p.pane_id ?? "");

  return (
    <div className="chips" id="chips">
      {panes.map((p, i) => {
        const peer = panePeer(p);
        const baseLabel = sessionLabel(p);
        const labelKey = peer + "\0" + baseLabel;
        const label = (perSession[labelKey] ?? 0) > 1
          ? baseLabel + ":" + p.window_index
          : baseLabel;
        const key = PANE_HOTKEYS[i];
        const paneID = p.pane_id ?? "";
        return (
          <Chip
            // pane_id is stable within a tmux server; index is the fallback for
            // the rare empty-id case. Keep keys stable so React reuses chips.
            key={paneID || "pane-" + i}
            pane={p}
            label={label}
            hotkey={key}
            selected={p.pane_id === state.selected}
            onSelect={() => callbacks.selectPane(paneID)}
          />
        );
      })}
    </div>
  );
}
