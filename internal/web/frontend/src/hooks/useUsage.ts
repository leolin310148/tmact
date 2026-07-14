// useUsage — 1:1 port of static/js/usage.js wireUsage().
//
// Owns the agent-usage poll (60 s after completion + an immediate first fetch), mirrors
// the original module's lifecycle exactly:
//   - 404 → stop polling permanently AND hide the panel for good (the panel is
//     server-disabled; clearTimeout + never re-poll).
//   - !res.ok (transient) → keep the last render (no state change).
//   - fetch() exceptions → swallowed; the next tick retries.
//   - every successful poll stores the snapshot and triggers a re-render so the
//     panel recomputes the reset countdowns against the current wall clock.
//
// Per ARCHITECTURE.md §3 the hook OWNS the timer (via refs) and the original
// module-scoped mutable `lastSnap` / `timer` / disabled flag live in refs, NOT
// state. The only React state is a `tick` counter used solely to force the
// consuming component to re-render after a poll (the data itself is read from
// the returned `snap`). UsagePanel calls this hook directly; App just mounts
// <UsagePanel/> (see UsagePanel.tsx header for the wiring decision).

import { useEffect, useRef, useState } from "react";
import { loadAgentUsage } from "../api/client";
import type { AgentUsage } from "../types/server";

// USAGE_POLL_MS mirrors usage.js (60 s).
const USAGE_POLL_MS = 60000;

export interface UsageState {
  // Latest successfully-fetched snapshot, or null before the first success.
  // Mirrors the original module-scoped `lastSnap`.
  snap: AgentUsage | null;
  // True once a 404 disabled the panel server-side: the panel stays hidden
  // permanently and polling has stopped (mirrors clearInterval + panel.hidden).
  disabled: boolean;
}

export function useUsage(): UsageState {
  // lastSnap / disabled mirror usage.js module-scoped mutable state → refs.
  const lastSnap = useRef<AgentUsage | null>(null);
  const disabled = useRef(false);
  // tick forces a re-render after each successful poll so the consuming
  // component recomputes fmtCountdown/fmtShort against the current Date.now().
  const [, setTick] = useState(0);

  useEffect(() => {
    // Guard against double-fire under StrictMode's mount/unmount/mount: a single
    // timer + cancellation flag keep exactly one poll loop alive.
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const stop = () => {
      if (timer !== null) {
        clearTimeout(timer);
        timer = null;
      }
    };

    const scheduleNext = () => {
      if (cancelled || disabled.current) return;
      timer = setTimeout(() => {
        timer = null;
        void refresh();
      }, USAGE_POLL_MS);
    };

    async function refresh(): Promise<void> {
      if (cancelled || disabled.current) return;
      try {
        const { res, data } = await loadAgentUsage();
        if (cancelled) return;
        if (res.status === 404) {
          // Panel disabled server-side — stop polling AND hide permanently.
          disabled.current = true;
          stop();
          setTick((t) => t + 1);
          return;
        }
        if (!res.ok) return; // keep last render; transient unavailability
        lastSnap.current = data;
        setTick((t) => t + 1);
      } catch (e) {
        /* keep last render; next tick retries */
      } finally {
        scheduleNext();
      }
    }

    // Schedule each poll only after the previous request settles. This keeps the
    // loop single-flight even when the endpoint takes longer than the cadence.
    void refresh();

    return () => {
      cancelled = true;
      stop();
    };
  }, []);

  return { snap: lastSnap.current, disabled: disabled.current };
}
