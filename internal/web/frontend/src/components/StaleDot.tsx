// StaleDot — the red "no fresh snapshot" dot (#stale-dot), ported 1:1 from
// app.js checkStale (MIGRATION_SPEC §6 item 3).
//
// app.js:
//   const STALE_MS = 10000;
//   function checkStale() {
//     const snap = state.snapshot;
//     const fresh = snap && snap.ts &&
//       (Date.now() - new Date(snap.ts).getTime() <= STALE_MS);
//     $("stale-dot").style.display = fresh ? "none" : "block";
//   }
//
// app.js called checkStale after every applySnapshot AND on the 1000 ms poll
// tick, so the dot lights even when no new snapshot arrives. Here we recompute
// freshness on every render (driven by bump() when snapshots land) AND on a
// 1000 ms self-timer so a stalled feed still flips the dot on its own — matching
// the original's poll-driven re-check cadence.
//
// Edge cases preserved verbatim:
//   - No snapshot / no `ts` → `fresh` falsy → dot shown (display:block).
//   - Invalid `ts` → `getTime()` is NaN → comparison is false → dot shown; no
//     crash (§6 item 3).
// The element keeps its inline display toggle (CSS default is `display: none`),
// exactly like the original imperative `style.display` write.

import { useEffect, useState } from "react";
import { useAppState } from "../store/AppStateContext";

const STALE_MS = 10000;

export function StaleDot() {
  const { state } = useAppState();

  // A self-driven 1 s ticker forces a re-check even with no new snapshot, so the
  // dot can transition fresh → stale on its own (app.js's poll-interval
  // checkStale). We only need it to re-run the freshness computation below.
  const [, tick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, []);

  const snap = state.snapshot;
  const fresh = !!(
    snap &&
    snap.ts &&
    Date.now() - new Date(snap.ts).getTime() <= STALE_MS
  );

  return (
    <span
      className="stale-dot"
      id="stale-dot"
      title="no connection for 10s+"
      style={{ display: fresh ? "none" : "block" }}
    />
  );
}
