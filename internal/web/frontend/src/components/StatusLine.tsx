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
import { MoreChip } from "./MoreChip";

// PANE_HOTKEYS is the single source of truth in lib/keymap (it also drives
// HOTKEY_INDEX for the keydown handler). The chip badge shows these labels.

export const RUNTIME_ICON: Record<string, string> = {
  claude: "cc",
  codex: "cx",
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

export interface PaneListItem {
  pane: PaneStatus;
  label: string;
}

export function sortedPaneList(panes: PaneStatus[]): PaneStatus[] {
  return panes.slice().sort((a, b) =>
    panePeer(a).localeCompare(panePeer(b)) ||
    sessionLabel(a).localeCompare(sessionLabel(b)) ||
    a.window_index - b.window_index ||
    a.pane_index - b.pane_index);
}

// shouldPinPane decides which panes stay as always-visible chips vs. collapse
// into the "more" overflow popover. A pane is pinned when it carries an agent
// runtime, is the currently-selected pane (so the selection never vanishes into
// the overflow), or needs the user's attention (asking / has a prompt menu).
// Everything else — idle, agent-less panes — collapses.
export function shouldPinPane(pane: PaneStatus, selectedId: string | null): boolean {
  if (RUNTIME_ICON[paneRuntime(pane)]) return true;
  if (selectedId && pane.pane_id === selectedId) return true;
  if (pane.asking || pane.prompt) return true;
  return false;
}

export interface PaneSplit {
  visible: PaneListItem[];
  overflow: PaneListItem[];
}

// splitPaneItems partitions the (already sorted/labeled) pane list into the
// chips shown inline and the ones tucked behind the "more" chip, preserving the
// input order within each group.
export function splitPaneItems(items: PaneListItem[], selectedId: string | null): PaneSplit {
  const visible: PaneListItem[] = [];
  const overflow: PaneListItem[] = [];
  for (const item of items) {
    if (shouldPinPane(item.pane, selectedId)) visible.push(item);
    else overflow.push(item);
  }
  return { visible, overflow };
}

export function paneListItems(panes: PaneStatus[]): PaneListItem[] {
  const sorted = sortedPaneList(panes);
  const perSession: Record<string, number> = {};
  for (const p of sorted) {
    const k = panePeer(p) + "\0" + sessionLabel(p);
    perSession[k] = (perSession[k] || 0) + 1;
  }

  return sorted.map((p) => {
    const peer = panePeer(p);
    const baseLabel = sessionLabel(p);
    const labelKey = peer + "\0" + baseLabel;
    const label = (perSession[labelKey] ?? 0) > 1
      ? baseLabel + ":" + p.window_index
      : baseLabel;
    return { pane: p, label };
  });
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
        {/* History stays reachable even with nothing running — the last
            closed session would otherwise be unrecoverable from the UI. */}
        <MoreChip items={[]} onSelect={callbacks.selectPane} />
      </div>
    );
  }

  const items = paneListItems(panes);
  const { visible, overflow } = splitPaneItems(items, state.selected);

  // Freeze the rendered order so Option+key hotkeys map to the same chips, in
  // the same left-to-right sequence. Only the inline (visible) chips get a slot;
  // overflow panes are reachable by clicking through the "more" popover, not by
  // hotkey, so the visible chips keep stable hotkey indices. Mutate
  // state.paneOrder by reference, exactly like app.js (no bump — App reads it
  // through state).
  state.paneOrder = visible.map(({ pane }) => pane.pane_id ?? "");

  return (
    <div className="chips" id="chips">
      {visible.map(({ pane: p, label }, i) => {
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
      {/* Always rendered: with no hidden panes it still opens the
          recently-closed history. */}
      <MoreChip items={overflow} onSelect={callbacks.selectPane} />
    </div>
  );
}
