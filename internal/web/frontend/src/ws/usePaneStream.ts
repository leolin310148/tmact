// Pane WebSocket hook — 1:1 behavioral port of static/js/stream.js.
//
// The original `createPaneStream(callbacks)` is a closure factory holding
// module-scoped mutable state (`ws`, `wsRetry`, `backoff`, `stableTimer`,
// `currentPane`) and exposing `{ close, open, send }`. This hook reproduces it
// EXACTLY, with that mutable state held in refs (never useState) so the
// mutation timing — especially the backoff reset/double ordering and the stale
// `ws !== sock` close guard — matches byte-for-behavior.
//
// callbacks.onPatch(from, lines, question) — patch from the server
// callbacks.onError(msg)                   — server-side error text
// callbacks.onQuestion(q)                  — cleared (q=null) on close
// callbacks.onStatus(state)                — "connecting" | "open" | "reconnecting" | "closed"
//
// Backoff starts at 1s, doubles to 30s; resets to 1s after a connection has
// been open for STABLE_MS. document.visibilitychange is handled by the caller
// (it manages whether to reconnect at all).

import { useCallback, useEffect, useRef } from "react";
import { logFrontend } from "../lib/frontendLog";
import type { InputMsg, OutMsg, Question } from "../types/server";

const BACKOFF_MIN_MS = 1000;
const BACKOFF_MAX_MS = 30000;
const STABLE_MS = 5000;

/** Connection-status values surfaced through `onStatus` (verbatim from stream.js). */
export type ConnState = "connecting" | "open" | "reconnecting" | "closed";

/**
 * Injected-deps contract — mirrors the `createPaneStream(callbacks)` argument in
 * stream.js (see ARCHITECTURE.md §3). App provides these; the hook never reaches
 * back into App by import.
 */
export interface PaneStreamCallbacks {
  /** `() => state.selected`. Consulted at reconnect time to skip stale retries. */
  getSelectedPane: () => string | null;
  /** Server patch: replace buffer (from=0) or splice `lines` in at `from`. */
  onPatch: (from: number, lines: string[], question: Question | null) => void;
  /** Question payload; called with `null` on close to clear the option bar. */
  onQuestion: (q: Question | null) => void;
  /** Server-side error text; does NOT close the socket. */
  onError: (msg: string) => void;
  /** Connection lifecycle for the fixed conn-status overlay. */
  onStatus: (s: ConnState) => void;
}

/** The closure object stream.js returns: `{ close, open, send }`. */
export interface PaneStream {
  open: (paneID: string) => void;
  close: () => void;
  send: (obj: InputMsg) => boolean;
}

export function usePaneStream(callbacks: PaneStreamCallbacks): PaneStream {
  // Module-scoped mutable state from stream.js → refs (NEVER useState), so the
  // exact mutation timing (backoff reset-before / double-after, stale-close
  // guard) is preserved and `open`'s recursive setTimeout never sees a stale
  // closure value.
  const wsRef = useRef<WebSocket | null>(null);
  const wsRetryRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backoffRef = useRef<number>(BACKOFF_MIN_MS);
  const stableTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const currentPaneRef = useRef<string | null>(null);

  // Callbacks are read through a ref so the stable `open`/`close`/`send`
  // identities always invoke the latest App-provided implementations without
  // re-creating the stream object (which would orphan in-flight timers/sockets).
  const cbRef = useRef<PaneStreamCallbacks>(callbacks);
  cbRef.current = callbacks;

  const status = useCallback((s: ConnState) => {
    const cb = cbRef.current;
    // Mirror stream.js's `callbacks.onStatus && callbacks.onStatus(s)` guard.
    if (cb.onStatus) cb.onStatus(s);
  }, []);

  const cancelRetry = useCallback(() => {
    if (wsRetryRef.current) {
      clearTimeout(wsRetryRef.current);
      wsRetryRef.current = null;
    }
  }, []);

  const cancelStable = useCallback(() => {
    if (stableTimerRef.current) {
      clearTimeout(stableTimerRef.current);
      stableTimerRef.current = null;
    }
  }, []);

  const close = useCallback(() => {
    cancelRetry();
    cancelStable();
    currentPaneRef.current = null;
    backoffRef.current = BACKOFF_MIN_MS;
    if (wsRef.current) {
      const old = wsRef.current;
      wsRef.current = null;
      try {
        old.close();
      } catch {
        // ignore — matches stream.js `catch (e) {}`
      }
    }
    cbRef.current.onQuestion(null);
    status("closed");
  }, [cancelRetry, cancelStable, status]);

  // `open` is declared via ref so the recursive reconnect (inside onclose's
  // setTimeout) can call the same identity without listing itself as a dep.
  const openRef = useRef<(paneID: string) => void>(() => {});

  const open = useCallback(
    (paneID: string) => {
      if (paneID !== currentPaneRef.current) {
        // Switching panes: reset backoff so a new selection retries quickly.
        backoffRef.current = BACKOFF_MIN_MS;
      }
      currentPaneRef.current = paneID;
      cancelRetry();
      cancelStable();
      if (wsRef.current) {
        const old = wsRef.current;
        wsRef.current = null;
        try {
          old.close();
        } catch {
          // ignore — matches stream.js `catch (e) {}`
        }
      }

      const proto = location.protocol === "https:" ? "wss" : "ws";
      status("connecting");
      logFrontend("info", "pane_ws", "connecting", { pane: paneID });
      const sock = new WebSocket(
        `${proto}://${location.host}/ws/pane?pane=${encodeURIComponent(paneID)}`,
      );
      wsRef.current = sock;
      sock.onopen = () => {
        // A replaced socket may still dispatch queued events. Only the current
        // socket is allowed to change connection state or reset retry backoff.
        if (wsRef.current !== sock || currentPaneRef.current !== paneID) return;
        status("open");
        logFrontend("info", "pane_ws", "open", { pane: paneID });
        // Treat the connection as stable once it has stayed up for STABLE_MS;
        // a flaky network that reconnects every few seconds keeps escalating
        // the backoff so the browser does not hammer the server.
        stableTimerRef.current = setTimeout(() => {
          backoffRef.current = BACKOFF_MIN_MS;
        }, STABLE_MS);
      };
      sock.onmessage = (ev: MessageEvent) => {
        if (wsRef.current !== sock || currentPaneRef.current !== paneID) return;
        let m: OutMsg;
        try {
          m = JSON.parse(ev.data as string) as OutMsg;
        } catch {
          logFrontend("warn", "pane_ws", "message parse failed", { pane: paneID });
          return;
        }
        if (m.t === "patch") {
          // stream.js: `m.from | 0` (coerces undefined→0 too); `(m.from ?? 0)|0`
          // is identical for any number and type-clean under noUncheckedIndexedAccess.
          cbRef.current.onPatch(
            (m.from ?? 0) | 0,
            Array.isArray(m.lines) ? m.lines : [],
            m.q ?? null,
          );
        } else if (m.t === "error") {
          logFrontend("error", "pane_ws", "server error frame", {
            pane: paneID,
            error: m.s,
          });
          cbRef.current.onError(m.s);
        }
      };
      sock.onclose = () => {
        if (wsRef.current !== sock) return;
        wsRef.current = null;
        cancelStable();
        logFrontend("warn", "pane_ws", "closed", { pane: paneID });
        if (currentPaneRef.current !== paneID || document.hidden) return;
        const delay = backoffRef.current;
        backoffRef.current = Math.min(backoffRef.current * 2, BACKOFF_MAX_MS);
        status("reconnecting");
        logFrontend("warn", "pane_ws", "reconnecting", {
          pane: paneID,
          delay_ms: delay,
        });
        wsRetryRef.current = setTimeout(() => {
          wsRetryRef.current = null;
          if (cbRef.current.getSelectedPane() === paneID && !document.hidden) {
            openRef.current(paneID);
          }
        }, delay);
      };
      sock.onerror = () => {
        if (wsRef.current !== sock || currentPaneRef.current !== paneID) return;
        logFrontend("error", "pane_ws", "socket error", { pane: paneID });
      };
    },
    [cancelRetry, cancelStable, status],
  );

  openRef.current = open;

  const send = useCallback((obj: InputMsg): boolean => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(obj));
      return true;
    }
    return false;
  }, []);

  // Tear down any live socket/timers when the hook unmounts. The original
  // module never "unmounted", but App owns this stream for its entire lifetime;
  // this guards against a stray socket surviving a hot reload / teardown without
  // altering the runtime semantics (close() is idempotent and resets state).
  useEffect(() => {
    return () => {
      cancelRetry();
      cancelStable();
      if (wsRef.current) {
        const old = wsRef.current;
        wsRef.current = null;
        try {
          old.close();
        } catch {
          // ignore
        }
      }
    };
  }, [cancelRetry, cancelStable]);

  return { open, close, send };
}
