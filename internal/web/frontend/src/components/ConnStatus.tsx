// ConnStatus centralizes connection health for the status area. Pane WebSocket
// lifecycle text comes from App; snapshot freshness is checked here so the old
// separate stale dot does not create a second, competing connection indicator.

import { useEffect, useState } from "react";
import { useAppState } from "../store/AppStateContext";

const STALE_MS = 10000;

interface ConnStatusProps {
  /** The current connection-status message; "" hides the strip. */
  text: string;
}

export function ConnStatus({ text }: ConnStatusProps) {
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
  const stale = !(
    hasSnapshot &&
    Date.now() - new Date(snap.ts).getTime() <= STALE_MS
  );
  let message = text;
  let kind = "";
  if (text.includes("reconnecting")) {
    kind = "reconnecting";
  } else if (text.includes("connecting")) {
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
