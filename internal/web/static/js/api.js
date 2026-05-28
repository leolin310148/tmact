async function jsonResponse(url, options) {
  const res = await fetch(url, options);
  let data = {};
  try { data = await res.json(); } catch (e) {}
  return { res, data };
}

let snapshotEtag = "";

// fetchSnapshot caches the snapshot ETag so repeated polls return 304 from
// the server until the daemon writes a new file. The cached snapshot is
// returned on 304 so callers always see a value.
let lastSnapshot = null;
export async function fetchSnapshot() {
  const headers = {};
  if (snapshotEtag) headers["If-None-Match"] = snapshotEtag;
  const res = await fetch("api/snapshot", { cache: "no-store", headers });
  if (res.status === 304) {
    if (!lastSnapshot) throw new Error("HTTP 304 without cached snapshot");
    return lastSnapshot;
  }
  if (!res.ok) throw new Error("HTTP " + res.status);
  const tag = res.headers.get("ETag");
  if (tag) snapshotEtag = tag;
  lastSnapshot = await res.json();
  return lastSnapshot;
}

// subscribeSnapshot opens an SSE stream; onSnapshot fires for every push,
// onError when the connection breaks. The caller is expected to fall back to
// fetchSnapshot polling on error. Returns a close() function.
export function subscribeSnapshot(onSnapshot, onError) {
  if (typeof EventSource === "undefined") {
    onError(new Error("EventSource not supported"));
    return () => {};
  }
  const es = new EventSource("/api/snapshot/stream");
  es.addEventListener("snapshot", (ev) => {
    try { onSnapshot(JSON.parse(ev.data)); } catch (e) { /* ignore */ }
  });
  es.onerror = () => {
    es.close();
    onError(new Error("snapshot stream closed"));
  };
  return () => es.close();
}

export function transcribeAudio(form) {
  return jsonResponse("/api/transcribe", { method: "POST", body: form });
}

function peerQuery(peer) {
  return peer ? "?peer=" + encodeURIComponent(peer) : "";
}

export function uploadClipboardImage(form, peer) {
  return jsonResponse("/api/paste-image" + peerQuery(peer), { method: "POST", body: form });
}

export function uploadPaneFiles(form, peer) {
  return jsonResponse("/api/upload-file" + peerQuery(peer), { method: "POST", body: form });
}

export function loadSTTConfig() {
  return jsonResponse("/api/settings/stt", { cache: "no-store" });
}

export function loadVersion() {
  return jsonResponse("/api/version", { cache: "no-store" });
}

export function saveSTTConfig(payload) {
  return jsonResponse("/api/settings/stt", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}
