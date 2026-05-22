async function jsonResponse(url, options) {
  const res = await fetch(url, options);
  let data = {};
  try { data = await res.json(); } catch (e) {}
  return { res, data };
}

export async function fetchSnapshot() {
  const res = await fetch("api/snapshot", { cache: "no-store" });
  if (!res.ok) throw new Error("HTTP " + res.status);
  return res.json();
}

export function transcribeAudio(form) {
  return jsonResponse("/api/transcribe", { method: "POST", body: form });
}

export function uploadClipboardImage(form) {
  return jsonResponse("/api/paste-image", { method: "POST", body: form });
}

export function uploadPaneFiles(form) {
  return jsonResponse("/api/upload-file", { method: "POST", body: form });
}

export function loadSTTConfig() {
  return jsonResponse("/api/settings/stt", { cache: "no-store" });
}

export function saveSTTConfig(payload) {
  return jsonResponse("/api/settings/stt", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}
