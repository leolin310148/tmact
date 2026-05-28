// Pane WebSocket with exponential reconnect backoff.
//
// callbacks.onPatch(from, lines, question) — patch from the server
// callbacks.onError(msg)                   — server-side error text
// callbacks.onQuestion(q)                  — cleared (q=null) on close
// callbacks.onStatus(state)                — "connecting" | "open" | "reconnecting" | "closed"
//
// Backoff starts at 1s, doubles to 30s; resets to 1s after a connection has
// been open for STABLE_MS. document.visibilitychange is handled by the caller
// (it manages whether to reconnect at all).
const BACKOFF_MIN_MS = 1000;
const BACKOFF_MAX_MS = 30000;
const STABLE_MS = 5000;

export function createPaneStream(callbacks) {
  let ws = null;
  let wsRetry = null;
  let backoff = BACKOFF_MIN_MS;
  let stableTimer = null;
  let currentPane = null;

  const status = (s) => callbacks.onStatus && callbacks.onStatus(s);

  const cancelRetry = () => {
    if (wsRetry) { clearTimeout(wsRetry); wsRetry = null; }
  };
  const cancelStable = () => {
    if (stableTimer) { clearTimeout(stableTimer); stableTimer = null; }
  };

  const close = () => {
    cancelRetry();
    cancelStable();
    currentPane = null;
    backoff = BACKOFF_MIN_MS;
    if (ws) {
      const old = ws;
      ws = null;
      try { old.close(); } catch (e) {}
    }
    callbacks.onQuestion(null);
    status("closed");
  };

  const open = (paneID) => {
    if (paneID !== currentPane) {
      // Switching panes: reset backoff so a new selection retries quickly.
      backoff = BACKOFF_MIN_MS;
    }
    currentPane = paneID;
    cancelRetry();
    cancelStable();
    if (ws) {
      const old = ws;
      ws = null;
      try { old.close(); } catch (e) {}
    }

    const proto = location.protocol === "https:" ? "wss" : "ws";
    status("connecting");
    const sock = new WebSocket(`${proto}://${location.host}/ws/pane?pane=${encodeURIComponent(paneID)}`);
    ws = sock;
    sock.onopen = () => {
      status("open");
      // Treat the connection as stable once it has stayed up for STABLE_MS;
      // a flaky network that reconnects every few seconds keeps escalating
      // the backoff so the browser does not hammer the server.
      stableTimer = setTimeout(() => { backoff = BACKOFF_MIN_MS; }, STABLE_MS);
    };
    sock.onmessage = (ev) => {
      let m;
      try { m = JSON.parse(ev.data); } catch (e) { return; }
      if (m.t === "patch") callbacks.onPatch(m.from | 0, Array.isArray(m.lines) ? m.lines : [], m.q || null);
      else if (m.t === "error") callbacks.onError(m.s);
    };
    sock.onclose = () => {
      if (ws !== sock) return;
      ws = null;
      cancelStable();
      if (currentPane !== paneID || document.hidden) return;
      const delay = backoff;
      backoff = Math.min(backoff * 2, BACKOFF_MAX_MS);
      status("reconnecting");
      wsRetry = setTimeout(() => {
        wsRetry = null;
        if (callbacks.getSelectedPane() === paneID && !document.hidden) open(paneID);
      }, delay);
    };
    sock.onerror = () => {};
  };

  const send = (obj) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(obj));
      return true;
    }
    return false;
  };

  return { close, open, send };
}
