export function createPaneStream(callbacks) {
  let ws = null;
  let wsRetry = null;

  const close = () => {
    if (wsRetry) { clearTimeout(wsRetry); wsRetry = null; }
    if (ws) {
      const old = ws;
      ws = null;
      try { old.close(); } catch (e) {}
    }
    callbacks.onQuestion(null);
  };

  const open = (paneID) => {
    close();
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const sock = new WebSocket(`${proto}://${location.host}/ws/pane?pane=${encodeURIComponent(paneID)}`);
    ws = sock;
    sock.onmessage = (ev) => {
      let m;
      try { m = JSON.parse(ev.data); } catch (e) { return; }
      if (m.t === "content") callbacks.onContent(m.s, m.q);
      else if (m.t === "error") callbacks.onError(m.s);
    };
    sock.onclose = () => {
      if (ws !== sock) return;
      ws = null;
      if (callbacks.getSelectedPane() === paneID && !document.hidden) {
        wsRetry = setTimeout(() => {
          if (callbacks.getSelectedPane() === paneID) open(paneID);
        }, 1000);
      }
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
