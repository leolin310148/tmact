// ConnStatus centralizes connection health for the status area. Pane WebSocket
// lifecycle text comes from App; snapshot freshness is checked here so the old
// separate stale dot does not create a second, competing connection indicator.

import { useEffect, useState } from "react";
import { useAppState } from "../store/AppStateContext";
import type { ConnState } from "../ws/usePaneStream";

const DEFAULT_STALE_MS = 10000;

interface ConnStatusProps {
  /** Lifecycle state of the selected pane's WebSocket stream. */
  paneState: ConnState;
}

export function ConnStatus({ paneState }: ConnStatusProps) {
  const { state } = useAppState();

  // Re-evaluate freshness even when no snapshot arrives; otherwise a stalled
  // feed would never flip from fresh to stale on its own.
  const [, tick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, []);

  const snap = state.snapshot;
  const hasSnapshot = !!(snap && snap.ts);
  const staleAfterMs =
    snap && Number.isFinite(snap.stale_after_ms) && snap.stale_after_ms > 0
      ? snap.stale_after_ms
      : DEFAULT_STALE_MS;
  const stale = !(
    hasSnapshot &&
    Date.now() - new Date(snap.ts).getTime() <= staleAfterMs
  );
  let message = "";
  let kind = "";
  if (paneState === "reconnecting") {
    message = "pane stream reconnecting...";
    kind = "reconnecting";
  } else if (paneState === "connecting") {
    message = "pane stream connecting...";
    kind = "connecting";
  } else if (stale && hasSnapshot) {
    message = "status updates interrupted - retrying...";
    kind = "stale";
  } else if (stale) {
    message = "status updates connecting...";
    kind = "connecting";
  }

  return (
    <div
      className={[
        "conn-status",
        message !== "" ? "show" : "",
        kind ? "conn-status-" + kind : "",
      ].filter(Boolean).join(" ")}
      id="conn-status"
      role={message ? "status" : undefined}
      aria-live="polite"
      aria-atomic="true"
    >
      {message ? (
        <>
          <span className="conn-status-dot" aria-hidden="true" />
          <span>{message}</span>
        </>
      ) : null}
    </div>
  );
}
