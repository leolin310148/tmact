// Snapshot delivery hook — a faithful 1:1 port of static/app.js's snapshot
// lifecycle (the `refreshSnapshot` / `applySnapshot` / `startSnapshotStream` /
// `startPolling` / `restoreSelection` / `pruneCache` / `checkStale` block) plus
// `api.js subscribeSnapshot`.
//
// PARITY MODEL (read ARCHITECTURE.md §0/§7/§9 and MIGRATION_SPEC §2.3/§2.4 +
// §6 items 1–6 before touching this file):
//
//   Snapshot delivery prefers SSE push from /api/snapshot/stream. On stream
//   error (network blip, proxy idle timeout, or an older statusd build) it
//   falls back to a periodic GET with NO dead window: the original's SSE
//   onError handler nulls `snapshotSSE` and calls `startPolling()` synchronously
//   — there is no gap where neither path is running. We mirror that exactly.
//
//   The original kept `snapshotTimer`, `snapshotSSE`, and `selectionRestored`
//   as MODULE-SCOPED MUTABLE state (`let`). To preserve that exact mutation
//   timing (and to avoid stale closures) we hold each in a `useRef` and mutate
//   it by reference, IDENTICALLY to the original. There is NO React state in
//   this hook — re-renders are driven by `bump()` from the store, called exactly
//   where the original re-ran its imperative render functions
//   (renderStatusline / renderMode / checkStale write to the DOM in the
//   original; in the port `bump()` re-renders the chips / mode / ConnStatus
//   components, which read live `state`).
//
//   `state.snapshot` / `state.selected` / `state.paneOrder` / `state.drafts`
//   are mutated by reference (never cloned), exactly like state.js. `paneCache`
//   is App-owned (ARCHITECTURE §7) and passed in as a ref so `pruneCache` can
//   drop dead panes while keeping the selected one.

import { useCallback, useEffect, useRef, type RefObject } from "react";
import { fetchSnapshot, subscribeSnapshot } from "../api/client";
import { logFrontend } from "../lib/frontendLog";
import { useAppState } from "../store/AppStateContext";
import type { PaneStatus, Snapshot } from "../types/server";

// Polling keeps the original cadence. Snapshot freshness prefers the daemon's
// advertised threshold and falls back for older servers that omit it.
const POLL_MS = 1000;
const DEFAULT_STALE_MS = 10000;
// Grace window before a `hidden` tab is torn down. A hidden→visible round-trip
// shorter than this (mobile scroll / address-bar / brief app switch) is treated
// as no interruption at all, so the pane WS is never needlessly reconnected.
const VIS_HIDE_GRACE_MS = 600;

// localStorage key for the last-selected pane — verbatim from app.js
// (`SELECTED_KEY`). Reads tolerate malformed/absent values exactly as the
// original (try/catch → null fallback).
const SELECTED_KEY = "tmact.selectedPane";

/**
 * Shape of the persisted selection blob (`localStorage["tmact.selectedPane"]`).
 * Written by App's `rememberSelection` as
 * `JSON.stringify({ pane: <id>, session: <name> })`. Both fields are read
 * defensively here (the stored value is untrusted JSON).
 */
interface SavedSelection {
  pane?: unknown;
  session?: unknown;
}

/**
 * Injected dependencies — the App-owned bits the original `app.js` snapshot
 * block reached for directly. Mirrors the original call sites exactly:
 *
 *   - applySnapshot → renderStatusline / renderMode / restoreSelection /
 *     syncQuickDock / pruneCache / checkStale. The render functions become a
 *     single `bump()` (the chips/mode components re-read live state); the two
 *     non-render side effects (`syncQuickDock`, `restoreSelection`→`selectPane`)
 *     are injected here.
 *   - pruneCache → mutates the App-owned `paneCache` ref (keeps selected).
 *   - visibilitychange → stop/start SSE+poll (owned here) and close/reopen the
 *     pane WS (owned by App: `closeWS` / `openWS`).
 *
 * `paneCache` is a ref (App owns the object; ARCHITECTURE §7) so pruning mutates
 * the same `Record<string,string[]>` the WS patch path writes to.
 */
export interface SnapshotStreamDeps {
  /**
   * App's `paneCache` ref: a `Record<string, string[]>` keyed by pane id.
   * `pruneCache` deletes entries for panes that no longer exist (keeping the
   * selected pane), mutating this object in place.
   */
  paneCache: RefObject<Record<string, string[]>>;
  /**
   * app.js `selectPane(paneID)`. Called ONCE by `restoreSelection` after the
   * first snapshot lands, with the matched pane's `pane_id`.
   */
  selectPane: (paneID: string) => void;
  /** Clears App's selection and pane stream after an authoritative local
   * snapshot confirms that the selected tmux pane no longer exists. */
  clearSelection: (paneID: string) => void;
  /**
   * quick.js `syncQuickDock()`. `applySnapshot` calls it after every snapshot
   * (the FAB's `.ready` state tracks the selection / pane set).
   */
  syncQuickDock: () => void;
  /**
   * app.js `renderMode()`. `applySnapshot` calls it on every snapshot
   * (old app.js:264) — NOT just on selection/focus events. renderMode has
   * imperative side effects that must apply at boot: it sets the responsive
   * #draft placeholder (mobile vs desktop) and the "Select a pane to enable
   * input" #mode-text when nothing is selected. Without this, a fresh load with
   * no restorable selection (selectPane → renderMode never fires) leaves the
   * mobile placeholder showing the desktop ⌘/Ctrl hint and hides #mode-indicator.
   */
  renderMode: () => void;
  /**
   * app.js `closeWS()` → `paneStream.close()`. Called when the tab goes hidden.
   */
  closeWS: () => void;
  /**
   * app.js `openWS(paneID)`. Called on tab-visible to reopen the WS for the
   * currently-selected pane (seeds from cache + reconnects). Only invoked when
   * `state.selected` is set, matching the original.
   */
  openWS: (paneID: string) => void;
}

/**
 * What App needs to drive the chips, connection summary, and the WS reopen. The hook
 * owns the SSE/poll resources and the snapshot apply path; App calls
 * `refreshSnapshot` once on startup and starts the stream, exactly like the
 * original's bottom block.
 */
export interface SnapshotStream {
  /**
   * app.js `refreshSnapshot()`. Fetch a snapshot (ETag-cached via api/client),
   * apply it on success; on failure keep the last snapshot and re-evaluate
   * staleness (the stale dot surfaces the lost connection). Never throws.
   */
  refreshSnapshot: () => Promise<void>;
  /**
   * app.js `startSnapshotStream()`. Open the SSE subscription (no-op if already
   * open). On a pushed snapshot: stop polling + apply. On stream error: null the
   * SSE handle and start polling synchronously (no dead window).
   */
  startSnapshotStream: () => void;
  /** app.js `stopSnapshotStream()`. Close + null the SSE subscription. */
  stopSnapshotStream: () => void;
  /**
   * app.js `checkStale()` as a pure predicate: `true` when no fresh snapshot has
   * landed within the snapshot's advertised stale threshold (the daemon
   * stalled or the connection dropped).
   * `Invalid Date` (missing/garbage `ts`) is treated as stale and never throws.
   * ConnStatus calls this same predicate logic each render (and on its own
   * 1 s timer) so stale snapshot delivery is surfaced in the unified
   * connection strip.
   */
  checkStale: () => boolean;
}

export function useSnapshotStream(deps: SnapshotStreamDeps): SnapshotStream {
  const { state, bump } = useAppState();

  // Latest injected deps, read through a ref so the long-lived callbacks below
  // (registered once, e.g. on the SSE subscription / visibility listener) always
  // see App's current implementations without being re-created. App may pass
  // freshly-closed callbacks each render; this keeps them current with no churn.
  const depsRef = useRef<SnapshotStreamDeps>(deps);
  depsRef.current = deps;

  // Module-scoped mutable state from app.js, held as refs (ARCHITECTURE §0):
  //   snapshotTimer  — setInterval handle for the 1000 ms poll, or null.
  //   snapshotSSE    — the SSE close() fn returned by subscribeSnapshot, or null.
  //   selectionRestored — guards restoreSelection to fire exactly once.
  const snapshotTimer = useRef<ReturnType<typeof setInterval> | null>(null);
  const snapshotSSE = useRef<(() => void) | null>(null);
  const selectionRestored = useRef(false);

  // checkStale — port of app.js `checkStale()` as a pure predicate.
  // `fresh` is true only when a snapshot exists, has a `ts`, and that timestamp
  // is within STALE_MS of now. `new Date(undefined/garbage).getTime()` is NaN,
  // and `Date.now() - NaN <= STALE_MS` is `false`, so a missing/garbage `ts`
  // falls through to "not fresh" → stale, never throwing (spec §6 item 3).
  const checkStale = useCallback((): boolean => {
    const snap = state.snapshot;
    const staleAfterMs =
      snap && Number.isFinite(snap.stale_after_ms) && snap.stale_after_ms > 0
        ? snap.stale_after_ms
        : DEFAULT_STALE_MS;
    const fresh =
      !!snap &&
      !!snap.ts &&
      Date.now() - new Date(snap.ts).getTime() <= staleAfterMs;
    return !fresh;
  }, [state]);

  // pruneCache — port of app.js `pruneCache(snap)`. Drop cached buffers for
  // panes that no longer exist so the cache tracks the live pane set; the
  // selected pane is kept even if a snapshot momentarily omits it (e.g. a
  // transient peer fetch failure). Mutates App's paneCache object in place.
  const pruneCache = useCallback(
    (snap: Snapshot | null): void => {
      if (!snap || !snap.panes) return;
      const live = new Set<string>();
      for (const p of Object.values(snap.panes)) {
        if (p.pane_id) live.add(p.pane_id);
      }
      const cache = depsRef.current.paneCache.current;
      for (const id in cache) {
        if (id !== state.selected && !live.has(id)) delete cache[id];
      }
    },
    [state],
  );

  // restoreSelection — port of app.js `restoreSelection()`. Runs exactly once,
  // after the first snapshot lands, to re-select the user's last pane: by exact
  // pane id first, then by session name. Guarded by `selectionRestored` so it
  // never fires again (spec §6 item 4).
  const restoreSelection = useCallback((): void => {
    if (selectionRestored.current) return;
    selectionRestored.current = true;
    if (state.selected) return;
    let saved: SavedSelection | null = null;
    try {
      saved = JSON.parse(
        localStorage.getItem(SELECTED_KEY) || "null",
      ) as SavedSelection | null;
    } catch (e) {
      /* malformed/absent — leave saved null, exactly as the original */
    }
    if (!saved) return;
    const panes: PaneStatus[] =
      state.snapshot && state.snapshot.panes
        ? Object.values(state.snapshot.panes)
        : [];
    const savedPane = saved.pane;
    const savedSession = saved.session;
    let target = panes.find((p) => p.pane_id === savedPane);
    if (!target && savedSession) {
      target = panes.find((p) => p.session === savedSession);
    }
    if (target && target.pane_id) depsRef.current.selectPane(target.pane_id);
  }, [state]);

  // applySnapshot — port of app.js `applySnapshot(snap)`. Mutate state by
  // reference, then re-render: the original re-ran renderStatusline (→ bump(),
  // which re-renders the chips/mode components, which read live state and freeze
  // `state.paneOrder` inside StatusLine's render — see note) and then renderMode.
  // renderMode is injected (depsRef.current.renderMode) because, unlike
  // renderStatusline, its imperative side effects (the responsive #draft
  // placeholder + the "Select a pane to enable input" #mode-text) are NOT a pure
  // re-render — bump() alone would not apply them. Calling it here mirrors the
  // original's per-snapshot renderMode and fixes the boot case (fresh load, no
  // restorable selection → selectPane/renderMode otherwise never fire).
  //
  // ORDER MATTERS and mirrors the original exactly:
  //   1. state.snapshot = snap
  //   2. renderStatusline(snap)  → bump() (StatusLine freezes state.paneOrder)
  //   3. renderMode()            → depsRef.current.renderMode() (placeholder + mode-text)
  //   4. restoreSelection()      → may call selectPane (which itself re-renders)
  //   5. syncQuickDock()
  //   6. pruneCache(snap)
  //   7. checkStale()            → ConnStatus recomputes on the bump()
  //
  // The paneOrder freeze (app.js renderStatusline: `state.paneOrder =
  // panes.map(p => p.pane_id)`) lives in the StatusLine component's render in
  // the port (ARCHITECTURE §10), driven by this bump(). restoreSelection runs
  // AFTER that bump, so on the very first snapshot the order is already frozen
  // before selectPane fires — identical to the original sequencing.
  const applySnapshot = useCallback(
    (snap: Snapshot): void => {
      state.snapshot = snap;
      const selected = state.selected;
      // A local snapshot is authoritative for local pane ids. Remote panes can
      // disappear temporarily when a peer fetch fails, so retain those until a
      // dedicated peer-liveness signal is available.
      if (
        selected?.startsWith("%") &&
        !Object.values(snap.panes || {}).some((pane) => pane.pane_id === selected)
      ) {
        depsRef.current.clearSelection(selected);
      }
      bump(); // renderStatusline(snap)
      depsRef.current.renderMode(); // renderMode() — old app.js:264 (placeholder + mode-text)
      restoreSelection();
      depsRef.current.syncQuickDock();
      pruneCache(snap);
      bump(); // checkStale() — re-render ConnStatus from the (possibly) new ts
    },
    [state, bump, restoreSelection, pruneCache],
  );

  // refreshSnapshot — port of app.js `refreshSnapshot()`. On success apply; on
  // failure keep the last snapshot and re-evaluate staleness via a bump (the
  // original called checkStale(); here ConnStatus recomputes on the bump). Never
  // throws — every error is swallowed exactly like the original try/catch.
  const refreshSnapshot = useCallback(async (): Promise<void> => {
    try {
      applySnapshot(await fetchSnapshot());
    } catch (e) {
      // Keep the last snapshot; the stale dot surfaces the lost connection.
      bump(); // checkStale()
    }
  }, [applySnapshot, bump]);

  // startPolling / stopPolling — port of app.js. The interval polls and
  // re-evaluates staleness every POLL_MS. Idempotent start (guarded on a null
  // handle) so repeated calls don't stack intervals.
  const startPolling = useCallback((): void => {
    if (snapshotTimer.current === null) {
      logFrontend("warn", "snapshot_stream", "fallback polling");
      snapshotTimer.current = setInterval(() => {
        void refreshSnapshot();
        bump(); // checkStale()
      }, POLL_MS);
    }
  }, [refreshSnapshot, bump]);

  const stopPolling = useCallback((): void => {
    if (snapshotTimer.current !== null) clearInterval(snapshotTimer.current);
    snapshotTimer.current = null;
  }, []);

  // startSnapshotStream — port of app.js. Open the SSE subscription (no-op if
  // already open). On a pushed snapshot: stop polling, then apply. On stream
  // error: null the handle and start polling SYNCHRONOUSLY inside onError — this
  // is the "no dead window" requirement (spec §6 item 1): there is never a gap
  // where neither SSE nor polling is delivering snapshots.
  const startSnapshotStream = useCallback((): void => {
    if (snapshotSSE.current) return;
    snapshotSSE.current = subscribeSnapshot(
      (snap) => {
        stopPolling();
        applySnapshot(snap);
      },
      () => {
        snapshotSSE.current = null;
        startPolling();
      },
    );
  }, [stopPolling, applySnapshot, startPolling]);

  // stopSnapshotStream — port of app.js. Close + null the SSE handle.
  const stopSnapshotStream = useCallback((): void => {
    if (snapshotSSE.current) {
      snapshotSSE.current();
      snapshotSSE.current = null;
    }
  }, []);

  // Visibility lifecycle — port of app.js's `visibilitychange` listener, with a
  // debounce/idempotence layer (intentional deviation from app.js).
  //   hidden:  after VIS_HIDE_GRACE_MS, stop polling, close SSE, close pane WS.
  //   visible: if a hide was still pending, just cancel it (nothing was torn
  //            down → no reopen); otherwise, only if we genuinely tore down,
  //            refresh the snapshot, restart SSE, reopen the pane WS.
  // Why: app.js closed+reopened on EVERY transition. On phones a transient blip
  // (scroll, address-bar show/hide, brief app switch) fires hidden→visible in
  // quick succession, and the unconditional reopen forced a full WS reconnect
  // that replays the whole pane snapshot — a churn that compounds the render
  // load. The grace window absorbs blips and the torn-down guard makes a
  // redundant "visible" a no-op, so a live connection is left untouched.
  // Registered ONCE on mount; the handler reads live refs/state so it never goes
  // stale. Cleanup removes the listener, clears the pending hide, and tears down
  // SSE/poll so the hook leaves nothing running.
  useEffect(() => {
    let hideTimer: ReturnType<typeof setTimeout> | null = null;
    let tornDown = false;
    const onVisibility = () => {
      if (document.hidden) {
        if (hideTimer !== null) return; // teardown already scheduled
        hideTimer = setTimeout(() => {
          hideTimer = null;
          tornDown = true;
          stopPolling();
          stopSnapshotStream();
          depsRef.current.closeWS();
        }, VIS_HIDE_GRACE_MS);
      } else {
        if (hideTimer !== null) {
          // Came back within the grace window — nothing was torn down, so leave
          // the live SSE/poll/WS exactly as they are (this is the churn fix).
          clearTimeout(hideTimer);
          hideTimer = null;
          return;
        }
        if (!tornDown) return; // already live; a redundant "visible" is a no-op
        tornDown = false;
        void refreshSnapshot();
        startSnapshotStream();
        if (state.selected) depsRef.current.openWS(state.selected);
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      if (hideTimer !== null) clearTimeout(hideTimer);
      stopPolling();
      stopSnapshotStream();
    };
  }, [
    state,
    stopPolling,
    stopSnapshotStream,
    refreshSnapshot,
    startSnapshotStream,
  ]);

  return { refreshSnapshot, startSnapshotStream, stopSnapshotStream, checkStale };
}
