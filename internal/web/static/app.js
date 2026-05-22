import {
  fetchSnapshot,
  loadSTTConfig,
  loadVersion,
  saveSTTConfig,
  transcribeAudio,
  uploadClipboardImage,
  uploadPaneFiles,
} from "./js/api.js";
import { $, clamp, h, isMobile } from "./js/dom.js";
import { state, upload, voice } from "./js/state.js";
import { createPaneStream } from "./js/stream.js";
import { setContent } from "./js/terminal.js";

const POLL_MS = 1000;
const STALE_MS = 10000;

// Desktop pane-switch hotkeys: Option+<key> jumps to the Nth chip in the
// statusline. The labels are what the chip badge shows; HOTKEY_CODE maps each
// to its layout-independent KeyboardEvent.code (e.key is unusable here —
// holding Option on macOS rewrites it to an accented character).
const PANE_HOTKEYS = [
  "1", "2", "3", "4", "5", "6", "7", "8", "9", "0",
  "q", "w", "e", "r", "t", "y", "u", "i", "o", "p",
];
const HOTKEY_CODE = {
  "1": "Digit1", "2": "Digit2", "3": "Digit3", "4": "Digit4", "5": "Digit5",
  "6": "Digit6", "7": "Digit7", "8": "Digit8", "9": "Digit9", "0": "Digit0",
  "q": "KeyQ", "w": "KeyW", "e": "KeyE", "r": "KeyR", "t": "KeyT",
  "y": "KeyY", "u": "KeyU", "i": "KeyI", "o": "KeyO", "p": "KeyP",
};
// code → chip index, built once so the keydown handler is a single lookup.
const HOTKEY_INDEX = {};
PANE_HOTKEYS.forEach((label, i) => { HOTKEY_INDEX[HOTKEY_CODE[label]] = i; });

function paneStateClass(p) {
  if (p.asking) return "asking";
  if (p.running) return "running";
  return "idle";
}
function paneStateLabel(p) {
  if (p.asking) return "asking";
  if (p.running) return "working";
  if (p.idle) return "idle";
  if (!p.state || p.state === "unknown") return "—";
  return p.state;
}
const RUNTIME_ICON = { claude: "cc", codex: "cx", copilot: "cp", gemini: "g" };
function paneRuntime(p) {
  return (p.runtime || "").toLowerCase();
}
function paneIndicator(p) {
  const runtime = paneRuntime(p);
  const icon = RUNTIME_ICON[runtime];
  if (icon) {
    const cls = ["agent-icon", "runtime-" + runtime];
    if (p.running) cls.push("running");
    if (p.asking) cls.push("asking");
    return h("span", {
      class: cls.join(" "),
      title: runtime + " — " + paneStateLabel(p),
      text: icon,
    });
  }

  if (!p.asking) return null;
  const dotCls = paneStateClass(p);
  const dot = h("span", { class: "dot " + dotCls });
  dot.textContent = "?";
  return dot;
}
function findPane(paneID) {
  const snap = state.snapshot;
  if (!snap || !paneID) return null;
  for (const t in snap.panes) if (snap.panes[t].pane_id === paneID) return snap.panes[t];
  return null;
}

/* ---- status line ---- */

function renderStatusline(snap) {
  const chips = $("chips");
  chips.textContent = "";
  const panes = snap && snap.panes ? Object.values(snap.panes) : [];
  if (panes.length === 0) {
    chips.appendChild(h("span", { class: "empty", text: "No tmux panes." }));
    return;
  }
  panes.sort((a, b) =>
    a.session.localeCompare(b.session) ||
    a.window_index - b.window_index ||
    a.pane_index - b.pane_index);

  const perSession = {};
  for (const p of panes) perSession[p.session] = (perSession[p.session] || 0) + 1;

  // Freeze the rendered order so the Option+key hotkeys map to the same chips
  // the user sees, in the same left-to-right sequence.
  state.paneOrder = panes.map((p) => p.pane_id);

  panes.forEach((p, i) => {
    const label = perSession[p.session] > 1
      ? p.session + ":" + p.window_index
      : p.session;
    const indicator = paneIndicator(p);
    const key = PANE_HOTKEYS[i];
    const chip = h("div", {
      class: "chip" + (p.pane_id === state.selected ? " sel" : ""),
      title: (p.cwd || p.session) + " — " + paneStateLabel(p)
        + (key ? " — Option+" + key : ""),
    },
      key ? h("span", { class: "chip-key", text: key }) : null,
      indicator,
      h("span", { text: label }));
    chip.onclick = () => selectPane(p.pane_id);
    chips.appendChild(chip);
  });
}

// checkStale lights the red dot when no fresh snapshot has landed for 10s+ —
// the daemon stalled, or the browser lost its connection to it.
function checkStale() {
  const snap = state.snapshot;
  const fresh = snap && snap.ts &&
    (Date.now() - new Date(snap.ts).getTime() <= STALE_MS);
  $("stale-dot").style.display = fresh ? "none" : "block";
}

// syncIndicator collapses the mode-indicator line whenever it has nothing to
// say. In plain draft mode the textarea placeholder already carries the send
// hint, so the row is just wasted height — it reappears only for a
// no-selection / DIRECT / error state.
function syncIndicator() {
  const has = $("mode-text").textContent !== "" || $("input-error").textContent !== "";
  $("mode-indicator").style.display = has ? "flex" : "none";
}

function renderMode() {
  const text = $("mode-text");
  const wrap = $("content-wrap");
  const ind = $("mode-indicator");
  const mobile = isMobile();
  // Mobile keyboards have no Ctrl/Cmd, so the only send path is the button.
  $("draft").placeholder = mobile
    ? "Type a prompt, then tap Send"
    : "Type a prompt — ⌘/Ctrl+Enter to send";
  const direct = !!state.selected && !state.selectionMode && document.activeElement === $("direct-input");
  $("input-bar").classList.toggle("direct", direct);
  ind.classList.toggle("direct", direct);
  wrap.classList.toggle("direct", direct);
  wrap.classList.toggle("selection-mode", state.selectionMode);
  // Direct mode is signalled by the light-blue panel border alone — no text.
  text.textContent = state.selected ? "" : "Select a pane to enable input";
  syncIndicator();
}

function syncSelectionButton() {
  const btn = $("selection-btn");
  btn.classList.toggle("ready", !!state.selected);
  btn.classList.toggle("active", state.selectionMode);
  btn.disabled = !state.selected;
  btn.setAttribute("aria-pressed", state.selectionMode ? "true" : "false");
  btn.title = state.selectionMode ? "selection mode on" : "selection mode";
}

function toggleSelectionMode() {
  if (!state.selected) return;
  state.selectionMode = !state.selectionMode;
  if (state.selectionMode) $("direct-input").blur();
  syncSelectionButton();
  renderMode();
}

let errorTimer = null;
function showInputError(msg) {
  $("input-error").textContent = msg;
  syncIndicator();
  if (errorTimer) clearTimeout(errorTimer);
  errorTimer = setTimeout(() => { $("input-error").textContent = ""; syncIndicator(); }, 6000);
}

// setInputStatus shows a transient note in the indicator's message slot (e.g.
// "uploading image…"). It cancels any pending error-clear timer so the note is
// not wiped early, and the caller clears it by passing "".
function setInputStatus(msg) {
  if (errorTimer) { clearTimeout(errorTimer); errorTimer = null; }
  $("input-error").textContent = msg;
  syncIndicator();
}

// renderOptions rebuilds the quick-answer bar from a detected menu prompt:
// each numbered option becomes a button that relays its digit into the pane.
// An absent or empty question clears the bar. The bar is CSS-hidden on
// desktop, so this runs unconditionally and the media query gates visibility.
function renderOptions(q) {
  const bar = $("option-bar");
  bar.textContent = "";
  if (!q || !Array.isArray(q.choices)) return;
  for (const c of q.choices) {
    if (typeof c.number !== "number") continue;
    const btn = h("button", { type: "button", title: c.label || ("Option " + c.number) },
      h("span", { class: "opt-num", text: String(c.number) }),
      c.label ? h("span", { class: "opt-label", text: c.label }) : null);
    // pointerdown preventDefault keeps the soft keyboard up and focus in place.
    btn.addEventListener("pointerdown", (e) => e.preventDefault());
    btn.addEventListener("click", () => {
      if (!wsSend({ t: "text", s: String(c.number) })) {
        showInputError("not connected — try again");
      }
    });
    bar.appendChild(btn);
  }
}

/* ---- snapshot polling ---- */

async function refreshSnapshot() {
  try {
    const snap = await fetchSnapshot();
    state.snapshot = snap;
    renderStatusline(snap);
    renderMode();
    restoreSelection();
    syncQuickDock();
  } catch (e) {
    // Keep the last snapshot; the stale dot surfaces the lost connection.
  }
  checkStale();
}

let snapshotTimer = null;
function startPolling() {
  if (snapshotTimer === null) {
    snapshotTimer = setInterval(() => { refreshSnapshot(); checkStale(); }, POLL_MS);
  }
}
function stopPolling() {
  clearInterval(snapshotTimer);
  snapshotTimer = null;
}

/* ---- pane WebSocket (output stream + input relay) ---- */

const paneStream = createPaneStream({
  getSelectedPane: () => state.selected,
  onContent: (text, question) => { setContent(text); renderOptions(question); },
  onQuestion: renderOptions,
  onError: showInputError,
});

function closeWS() {
  paneStream.close();
}

function openWS(paneID) {
  paneStream.open(paneID);
}

function wsSend(obj) {
  return paneStream.send(obj);
}

function selectPane(paneID) {
  if (!paneID || paneID === state.selected) return;
  state.selected = paneID;
  rememberSelection(paneID);
  const draft = $("draft");
  draft.value = state.drafts[paneID] || "";
  draft.disabled = false;
  $("send-btn").disabled = false;
  $("upload-btn").disabled = false;
  syncDraft();
  syncRecordButton();
  syncSelectionButton();
  setContent("Loading…");
  renderStatusline(state.snapshot);
  renderMode();
  closeQuickMenu();
  syncQuickDock();
  const sel = $("chips").querySelector(".chip.sel");
  if (sel) sel.scrollIntoView({ block: "nearest", inline: "nearest" });
  openWS(paneID);
  // On desktop, selecting a pane drops straight into direct mode — focus the
  // overlay so keystrokes pass through without first clicking the output. On
  // mobile this is skipped: it would raise the soft keyboard unprompted, and
  // the draft box is the expected entry point there.
  if (!isMobile() && !state.selectionMode) $("direct-input").focus();
}

const SELECTED_KEY = "tmact.selectedPane";

// rememberSelection persists the chosen pane so a full page refresh can
// restore it. Pane ids are stable within a tmux server; the session name is
// kept as a fallback for when tmux restarts and reassigns ids.
function rememberSelection(paneID) {
  const p = findPane(paneID);
  try {
    localStorage.setItem(SELECTED_KEY,
      JSON.stringify({ pane: paneID, session: p ? p.session : "" }));
  } catch (e) {}
}

let selectionRestored = false;
// restoreSelection runs once, after the first snapshot lands, to re-select the
// pane the user last chose — by exact pane id, then by session name.
function restoreSelection() {
  if (selectionRestored) return;
  selectionRestored = true;
  if (state.selected) return;
  let saved = null;
  try { saved = JSON.parse(localStorage.getItem(SELECTED_KEY) || "null"); }
  catch (e) {}
  if (!saved) return;
  const panes = state.snapshot && state.snapshot.panes
    ? Object.values(state.snapshot.panes) : [];
  let target = panes.find((p) => p.pane_id === saved.pane);
  if (!target && saved.session) target = panes.find((p) => p.session === saved.session);
  if (target) selectPane(target.pane_id);
}

/* ---- mode 1: draft textarea ---- */

// autoGrowDraft sizes the textarea to its content — one line by default,
// taller as lines are added, up to the CSS max-height (then it scrolls).
function autoGrowDraft() {
  const draft = $("draft");
  draft.style.height = "auto";
  const cs = getComputedStyle(draft);
  const max = parseFloat(cs.maxHeight) || 200;
  const border = parseFloat(cs.borderTopWidth) + parseFloat(cs.borderBottomWidth);
  const full = draft.scrollHeight + border;
  draft.style.height = Math.min(full, max) + "px";
  draft.style.overflowY = full > max ? "auto" : "hidden";
}

// syncDraft keeps the "×" clear button and the textarea height in step with
// the draft's current contents.
function syncDraft() {
  const draft = $("draft");
  $("draft-wrap").classList.toggle("has-text", !draft.disabled && draft.value !== "");
  autoGrowDraft();
}

function clearDraft() {
  const draft = $("draft");
  draft.value = "";
  if (state.selected) delete state.drafts[state.selected];
  syncDraft();
  draft.focus();
}

function sendDraft() {
  if (!state.selected) return;
  const draft = $("draft");
  if (!draft.value.trim()) return;
  if (!wsSend({ t: "send", s: draft.value })) {
    showInputError("not connected — try again");
    return;
  }
  draft.value = "";
  delete state.drafts[state.selected];
  syncDraft();
}

/* ---- voice transcription: record locally, transcribe on the statusd server ---- */

function voiceSupported() {
  return !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia && window.MediaRecorder);
}

function preferredAudioType() {
  const types = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4", "audio/wav"];
  for (const t of types) {
    if (MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(t)) return t;
  }
  return "";
}

// The record button only ever starts a recording — the live state and the
// stop control live in the overlay — so it just reflects enabled/busy.
function syncRecordButton() {
  const btn = $("record-btn");
  btn.classList.toggle("busy", voice.busy);
  btn.disabled = voice.busy || !!voice.pendingBlob || !state.selected || !voiceSupported();
  btn.title = voice.busy ? "transcribing…" : voice.pendingBlob ? "confirm recording" : "record voice";
}

function fmtElapsed(ms) {
  const s = Math.floor(ms / 1000);
  return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
}

// positionRecOverlay pins the Stop button right over the mic button (same
// size, same spot) and the info card just above the input bar, so the finger
// that tapped Record only has to tap again to stop.
function positionRecOverlay() {
  const ov = $("rec-overlay");
  if (ov.hidden) return;
  const mic = $("record-btn").getBoundingClientRect();
  const stop = $("rec-stop");
  stop.style.left = mic.left + "px";
  stop.style.top = mic.top + "px";
  stop.style.width = mic.width + "px";
  stop.style.height = mic.height + "px";
  const bar = $("input-bar").getBoundingClientRect();
  ov.querySelector(".rec-card").style.bottom =
    Math.max(8, window.innerHeight - bar.top + 8) + "px";
}

function showRecOverlay() {
  const ov = $("rec-overlay");
  ov.classList.remove("transcribing", "confirming", "hotkey-recording");
  $("rec-label").textContent = "Recording…";
  $("rec-timer").textContent = "0:00";
  ov.hidden = false;
  positionRecOverlay();
  voice.startedAt = Date.now();
  if (voice.timer) clearInterval(voice.timer);
  voice.timer = setInterval(() => {
    $("rec-timer").textContent = fmtElapsed(Date.now() - voice.startedAt);
  }, 250);
}

function showHotkeyRecordingOverlay() {
  const ov = $("rec-overlay");
  ov.classList.remove("transcribing", "confirming");
  ov.classList.add("hotkey-recording");
  $("rec-label").textContent = "Recording…";
  $("rec-timer").textContent = "0:00";
  ov.hidden = false;
  voice.startedAt = Date.now();
  if (voice.timer) clearInterval(voice.timer);
  voice.timer = setInterval(() => {
    $("rec-timer").textContent = fmtElapsed(Date.now() - voice.startedAt);
  }, 250);
}

function recOverlayTranscribing() {
  $("rec-overlay").classList.add("transcribing");
  $("rec-overlay").classList.remove("confirming", "hotkey-recording");
  $("rec-label").textContent = "Transcribing…";
  if (voice.timer) { clearInterval(voice.timer); voice.timer = null; }
}

function hideRecOverlay() {
  $("rec-overlay").hidden = true;
  $("rec-overlay").classList.remove("transcribing", "confirming", "hotkey-recording");
  if (voice.timer) { clearInterval(voice.timer); voice.timer = null; }
}

function showRecordingConfirm(blob) {
  voice.pendingBlob = blob;
  const ov = $("rec-overlay");
  ov.classList.remove("transcribing", "hotkey-recording");
  ov.classList.add("confirming");
  $("rec-label").textContent = "Send recording?";
  $("rec-timer").textContent = "V send · C cancel";
  ov.hidden = false;
  if (voice.timer) { clearInterval(voice.timer); voice.timer = null; }
  syncRecordButton();
}

function finishRecordingConfirm(send) {
  const blob = voice.pendingBlob;
  voice.pendingBlob = null;
  syncRecordButton();
  if (send && blob) {
    uploadRecording(blob);
    return;
  }
  hideRecOverlay();
}

function insertTranscript(text) {
  const draft = $("draft");
  const clean = text.trim();
  if (!clean) {
    showInputError("empty transcript");
    return;
  }
  if (document.activeElement === draft &&
      typeof draft.selectionStart === "number" &&
      typeof draft.selectionEnd === "number") {
    const start = draft.selectionStart, end = draft.selectionEnd;
    draft.value = draft.value.slice(0, start) + clean + draft.value.slice(end);
    const pos = start + clean.length;
    draft.setSelectionRange(pos, pos);
  } else if (!draft.value.trim()) {
    draft.value = clean;
  } else {
    draft.value = draft.value.replace(/\s*$/, "") + "\n" + clean;
  }
  if (state.selected) state.drafts[state.selected] = draft.value;
  draft.disabled = false;
  syncDraft();
  draft.focus();
}

// recordingFilename names the upload after its real container. The server
// re-sniffs the bytes regardless, but a correct name is a useful fallback.
function recordingFilename(type) {
  const t = (type || "").toLowerCase();
  let ext = "webm";
  if (t.includes("webm")) ext = "webm";
  else if (t.includes("ogg") || t.includes("oga")) ext = "ogg";
  else if (t.includes("mp4") || t.includes("m4a") || t.includes("aac")) ext = "m4a";
  else if (t.includes("mpeg") || t.includes("mpga") || t.includes("mp3")) ext = "mp3";
  else if (t.includes("wav")) ext = "wav";
  return "recording." + ext;
}

async function uploadRecording(blob) {
  if (!blob || blob.size === 0) {
    hideRecOverlay();
    showInputError("no audio recorded");
    return;
  }
  voice.busy = true;
  syncRecordButton();
  recOverlayTranscribing();
  try {
    const form = new FormData();
    form.append("audio", blob, recordingFilename(voice.mimeType || blob.type));
    const { res, data } = await transcribeAudio(form);
    if (!res.ok) throw new Error(data.error || ("transcription failed: HTTP " + res.status));
    insertTranscript(data.text || "");
  } catch (e) {
    showInputError(e.message || "transcription failed");
  } finally {
    voice.busy = false;
    hideRecOverlay();
    syncRecordButton();
  }
}

async function startRecording(recordOpts) {
  if (!state.selected || voice.busy || voice.recorder || voice.pendingBlob) return false;
  if (!voiceSupported()) {
    showInputError("microphone recording is not supported in this browser");
    return false;
  }
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const type = preferredAudioType();
    const mediaOpts = type ? { mimeType: type } : undefined;
    const recorder = new MediaRecorder(stream, mediaOpts);
    voice.stream = stream;
    voice.recorder = recorder;
    voice.chunks = [];
    voice.canceled = false;
    voice.confirmOnStop = !!(recordOpts && recordOpts.confirmOnStop === true);
    // Trust the type we asked for: iOS Safari leaves recorder.mimeType empty,
    // which would otherwise mislabel an MP4 recording as webm.
    voice.mimeType = type || recorder.mimeType || "";
    recorder.ondataavailable = (ev) => {
      if (ev.data && ev.data.size > 0) voice.chunks.push(ev.data);
    };
    recorder.onerror = () => {
      hideRecOverlay();
      showInputError("microphone recording failed");
    };
    recorder.onstop = () => {
      stream.getTracks().forEach((track) => track.stop());
      const blob = new Blob(voice.chunks, { type: voice.mimeType || recorder.mimeType || "audio/mp4" });
      const canceled = voice.canceled;
      voice.recorder = null;
      voice.stream = null;
      voice.chunks = [];
      const confirmOnStop = voice.confirmOnStop;
      voice.confirmOnStop = false;
      voice.hotkeyDown = false;
      voice.hotkeyStopPending = false;
      syncRecordButton();
      if (canceled) { hideRecOverlay(); return; }
      if (confirmOnStop) showRecordingConfirm(blob);
      else uploadRecording(blob);
    };
    recorder.start();
    if (voice.confirmOnStop) showHotkeyRecordingOverlay();
    else showRecOverlay();
    syncRecordButton();
    if (voice.hotkeyStopPending) {
      voice.hotkeyStopPending = false;
      stopRecording();
    }
    return true;
  } catch (e) {
    const denied = e && (e.name === "NotAllowedError" || e.name === "PermissionDeniedError");
    showInputError(denied ? "microphone permission denied" : "microphone unavailable");
    if (voice.stream) voice.stream.getTracks().forEach((track) => track.stop());
    voice.stream = null;
    voice.recorder = null;
    voice.confirmOnStop = false;
    voice.hotkeyDown = false;
    voice.hotkeyStopPending = false;
    hideRecOverlay();
    syncRecordButton();
    return false;
  }
}

// Button recordings stop straight into transcription. Hotkey recordings use a
// confirmation step after key release, so an accidental Option+V hold can still
// be canceled before uploading.
function stopRecording() {
  if (voice.recorder && voice.recorder.state === "recording") {
    voice.recorder.stop();
  }
}

function cancelRecording() {
  if (voice.pendingBlob) {
    finishRecordingConfirm(false);
    return;
  }
  if (voice.recorder && voice.recorder.state === "recording") {
    voice.canceled = true;
    voice.recorder.stop();
  } else {
    voice.confirmOnStop = false;
    hideRecOverlay();
  }
}

function isRecordHotkey(e) {
  return e.altKey && !e.ctrlKey && !e.metaKey && e.code === "KeyV";
}

function suppressRecordTextInput(e) {
  voice.suppressInputUntil = Date.now() + 700;
  const draft = $("draft");
  if (document.activeElement === draft) voice.suppressedDraftValue = draft.value;
  e.preventDefault();
  e.stopPropagation();
}

function isTextInputTarget(el) {
  return el === $("draft") || el === $("direct-input");
}

function stopHotkeyRecording() {
  if (!voice.hotkeyDown && !voice.hotkeyStopPending) return;
  voice.hotkeyDown = false;
  voice.suppressInputUntil = Date.now() + 700;
  if (voice.recorder && voice.recorder.state === "recording") {
    voice.hotkeyStopPending = false;
    stopRecording();
  } else {
    voice.hotkeyStopPending = true;
  }
}

function wireRecordHotkey() {
  document.addEventListener("keydown", async (e) => {
    if (isMobile() || $("settings-overlay").hidden === false) return;
    if (voice.pendingBlob && (e.code === "KeyV" || e.code === "KeyC" || e.key === "Escape")) {
      e.preventDefault();
      e.stopPropagation();
      finishRecordingConfirm(e.code === "KeyV");
      return;
    }
    if (!isRecordHotkey(e)) return;
    if (e.repeat || voice.hotkeyDown) {
      suppressRecordTextInput(e);
      return;
    }
    suppressRecordTextInput(e);
    voice.hotkeyDown = true;
    voice.hotkeyStopPending = false;
    const started = await startRecording({ confirmOnStop: true });
    if (!started) {
      voice.hotkeyDown = false;
      voice.hotkeyStopPending = false;
    }
  }, true);

  document.addEventListener("keyup", (e) => {
    if (!voice.hotkeyDown && !voice.hotkeyStopPending) return;
    if (e.code !== "KeyV" && e.code !== "AltLeft" && e.code !== "AltRight") return;
    suppressRecordTextInput(e);
    stopHotkeyRecording();
  }, true);

  document.addEventListener("beforeinput", (e) => {
    if (Date.now() > voice.suppressInputUntil || !isTextInputTarget(e.target)) return;
    e.preventDefault();
    e.stopPropagation();
  }, true);

  document.addEventListener("input", (e) => {
    if (Date.now() > voice.suppressInputUntil || !isTextInputTarget(e.target)) return;
    e.preventDefault();
    e.stopPropagation();
    if (e.target === $("draft")) {
      e.target.value = voice.suppressedDraftValue || "";
      if (state.selected) state.drafts[state.selected] = e.target.value;
      syncDraft();
    } else {
      e.target.value = "";
    }
  }, true);

  window.addEventListener("blur", stopHotkeyRecording);
}

/* ---- image paste: upload to the statusd server, relay the saved path ---- */

// clipboardImage returns the first image File on a paste event's clipboard, or
// null. A screenshot or copied picture arrives as a file item while plain text
// does not, so this also tells an image paste apart from a text paste.
function clipboardImage(e) {
  const items = (e.clipboardData && e.clipboardData.items) || [];
  for (const it of items) {
    if (it.kind === "file" && it.type && it.type.indexOf("image/") === 0) {
      const f = it.getAsFile();
      if (f) return f;
    }
  }
  return null;
}

let imgUploading = false;

// pasteImage uploads a clipboard image to the server, which saves it to a file
// and returns the path; place() then relays that path onward. A terminal pane
// has no channel for raw image bytes, so the path is the handoff — every
// supported agent reads an image when given its path.
async function pasteImage(file, place) {
  if (imgUploading) return; // one upload at a time — ignore a paste mid-flight
  imgUploading = true;
  setInputStatus("uploading image…");
  try {
    const form = new FormData();
    form.append("image", file, file.name || "paste.png");
    const { res, data } = await uploadClipboardImage(form);
    if (!res.ok || !data.path) {
      throw new Error(data.error || ("image upload failed: HTTP " + res.status));
    }
    setInputStatus("");
    place(data.path);
  } catch (e) {
    showInputError(e.message || "image upload failed");
  } finally {
    imgUploading = false;
  }
}

async function uploadFilesToPane(files) {
  files = Array.from(files || []).filter(Boolean);
  if (upload.busy || files.length === 0) return;
  if (!state.selected) {
    showInputError("select a pane first");
    return;
  }

  upload.busy = true;
  $("upload-btn").disabled = true;
  setInputStatus(files.length === 1 ? "uploading file…" : "uploading " + files.length + " files…");
  try {
    const form = new FormData();
    files.forEach((file, i) => form.append("file", file, file.name || ("upload-" + (i + 1))));
    const { res, data } = await uploadPaneFiles(form);
    const paths = Array.isArray(data.paths) ? data.paths : (data.path ? [data.path] : []);
    if (!res.ok || paths.length === 0) {
      throw new Error(data.error || ("file upload failed: HTTP " + res.status));
    }
    setInputStatus("");
    if (!wsSend({ t: "text", s: paths.join(" ") + " " })) {
      showInputError("uploaded, but pane is not connected");
    }
  } catch (e) {
    showInputError(e.message || "file upload failed");
  } finally {
    upload.busy = false;
    $("upload-btn").disabled = !state.selected;
  }
}

function openFileUploadPicker() {
  const input = $("file-upload");
  input.value = "";
  try {
    if (input.showPicker) input.showPicker();
    else input.click();
  } catch (e) {
    try { input.click(); }
    catch (err) { showInputError(err.message || "file picker blocked"); }
  }
}

// placeInDraft inserts text into the draft box at the cursor, or appends it
// when the draft is unfocused — used to drop a pasted image's path in for
// review before the prompt is sent.
function placeInDraft(text) {
  const draft = $("draft");
  if (draft.disabled) return;
  if (document.activeElement === draft &&
      typeof draft.selectionStart === "number") {
    const s = draft.selectionStart, end = draft.selectionEnd;
    draft.value = draft.value.slice(0, s) + text + draft.value.slice(end);
    const pos = s + text.length;
    draft.setSelectionRange(pos, pos);
  } else if (draft.value.trim() === "") {
    draft.value = text;
  } else {
    draft.value = draft.value.replace(/\s*$/, "") + " " + text;
  }
  if (state.selected) state.drafts[state.selected] = draft.value;
  syncDraft();
  draft.focus();
}

/* ---- mode 2: direct keystroke passthrough ---- */

const KEYMAP = {
  Enter: "Enter", Backspace: "BSpace", Tab: "Tab", Escape: "Escape",
  ArrowUp: "Up", ArrowDown: "Down", ArrowLeft: "Left", ArrowRight: "Right",
  Home: "Home", End: "End", PageUp: "PageUp", PageDown: "PageDown", Delete: "Delete",
};

function translateKey(e) {
  if (e.metaKey) return null; // leave Cmd shortcuts to the browser
  if (e.ctrlKey) {
    const lk = e.key.toLowerCase();
    if (lk.length === 1 && lk >= "a" && lk <= "z") return { t: "key", k: "C-" + lk };
    if (KEYMAP[e.key]) return { t: "key", k: KEYMAP[e.key] };
    return null;
  }
  // Shift+Enter inserts a line break instead of submitting: a bare "\n" is
  // pasted (bracketed paste), so the agent's input box takes it as a newline
  // rather than the Return that a plain Enter sends.
  if (e.key === "Enter" && e.shiftKey) return { t: "text", s: "\n" };
  if (e.key === "Tab" && e.shiftKey) return { t: "key", k: "BTab" };
  if (KEYMAP[e.key]) return { t: "key", k: KEYMAP[e.key] };
  if (e.key.length === 1) return { t: "text", s: e.key };
  return null;
}

/* ---- on-screen helper key bar (Termius-style, for mobile) ---- */

const HELPER_KEYS = [
  { label: "Esc", key: "Escape" },
  { label: "^C", key: "C-c" },
  { label: "Tab", key: "Tab" },
  { label: "⇧Tab", key: "BTab" },
  { label: "ctl", ctrl: true },
  { label: "↵", key: "Enter" },
  { label: "↑", key: "Up" },
  { label: "↓", key: "Down" },
  { label: "←", key: "Left" },
  { label: "→", key: "Right" },
  { label: "Home", key: "Home" },
  { label: "End", key: "End" },
  { label: "PgUp", key: "PageUp" },
  { label: "PgDn", key: "PageDown" },
];

// Ctrl is a sticky modifier: arm it, then the next typed letter is sent as
// C-<letter>. It auto-disarms after one keystroke.
let ctrlArmed = false;
function setCtrl(on) {
  ctrlArmed = on;
  const b = $("ctrl-key");
  if (b) b.classList.toggle("armed", on);
}

// syncKeyBar clips the helper-key bar to a single row and reveals the expand
// toggle only when the keys overflow that row. overflow:hidden keeps
// scrollHeight reporting the full wrapped height even while the bar is clipped.
function syncKeyBar() {
  const area = $("key-area"), bar = $("key-bar"), toggle = $("key-toggle");
  const firstBtn = bar.querySelector("button");
  if (!firstBtn) return;
  const rowH = firstBtn.offsetHeight;
  const overflows = rowH > 0 && bar.scrollHeight > rowH + 2;
  if (!overflows) area.classList.remove("expanded");
  const expanded = area.classList.contains("expanded");
  area.classList.toggle("overflowing", overflows);
  toggle.textContent = expanded ? "⌃" : "⌄";
  bar.style.maxHeight = overflows && !expanded ? rowH + "px" : "";
}

function buildKeyBar() {
  const bar = $("key-bar");
  for (const k of HELPER_KEYS) {
    const btn = h("button", { type: "button", text: k.label });
    if (k.ctrl) btn.id = "ctrl-key";
    // pointerdown preventDefault keeps the soft keyboard up and focus in place.
    btn.addEventListener("pointerdown", (e) => e.preventDefault());
    btn.addEventListener("click", () => {
      if (k.ctrl) { setCtrl(!ctrlArmed); return; }
      if (!wsSend({ t: "key", k: k.key })) showInputError("not connected — try again");
      if (ctrlArmed) setCtrl(false);
    });
    bar.appendChild(btn);
  }
  const toggle = $("key-toggle");
  toggle.addEventListener("pointerdown", (e) => e.preventDefault());
  toggle.addEventListener("click", () => {
    $("key-area").classList.toggle("expanded");
    syncKeyBar();
  });
  window.addEventListener("resize", syncKeyBar);
  syncKeyBar();
}

// sendDirect relays one direct-mode message, folding an armed Ctrl modifier
// into a single typed letter (C-<letter>) before it goes out.
function sendDirect(msg) {
  let out = msg;
  if (ctrlArmed && msg.t === "text" && msg.s.length === 1) {
    const c = msg.s.toLowerCase();
    if (c >= "a" && c <= "z") out = { t: "key", k: "C-" + c };
  }
  if (!wsSend(out)) showInputError("not connected — try again");
  if (ctrlArmed) setCtrl(false);
}

function wireInput() {
  const draft = $("draft");
  draft.addEventListener("input", () => {
    if (state.selected) state.drafts[state.selected] = draft.value;
    syncDraft();
  });
  draft.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
      e.preventDefault();
      sendDraft();
    }
  });
  // An image paste uploads the picture and drops its path into the draft; a
  // plain text paste falls through to the browser's default textarea handling.
  draft.addEventListener("paste", (e) => {
    const img = clipboardImage(e);
    if (!img) return;
    e.preventDefault();
    pasteImage(img, placeInDraft);
  });
  // pointerdown preventDefault keeps the soft keyboard up and focus in place.
  const clearBtn = $("draft-clear");
  clearBtn.addEventListener("pointerdown", (e) => e.preventDefault());
  clearBtn.addEventListener("click", clearDraft);
  // pointerdown preventDefault keeps the draft focused (and the soft keyboard
  // up) when Send/Record is tapped.
  $("send-btn").addEventListener("pointerdown", (e) => e.preventDefault());
  $("record-btn").addEventListener("pointerdown", (e) => e.preventDefault());
  $("selection-btn").addEventListener("pointerdown", (e) => e.preventDefault());
  $("send-btn").addEventListener("click", sendDraft);
  $("upload-btn").addEventListener("click", openFileUploadPicker);
  $("selection-btn").addEventListener("click", toggleSelectionMode);
  $("file-upload").addEventListener("change", (e) => {
    const files = e.target.files;
    e.target.value = "";
    uploadFilesToPane(files);
  });
  $("record-btn").addEventListener("click", () => startRecording({ confirmOnStop: false }));
  $("rec-stop").addEventListener("click", stopRecording);
  $("rec-send").addEventListener("click", () => finishRecordingConfirm(true));
  $("rec-cancel").addEventListener("click", cancelRecording);
  syncRecordButton();
  syncDraft();

  buildKeyBar();
  window.addEventListener("resize", () => {
    positionRecOverlay();
    autoGrowDraft();
  });

  const direct = $("direct-input");
  // A plain click/tap on the output focuses the invisible overlay — direct
  // keystroke passthrough on desktop, and on mobile this is what raises the
  // keyboard. But a click-drag is a text selection: focusing the overlay
  // there would collapse the selection and make ⌘/Ctrl+C copy the empty
  // overlay instead of the pane. So only a click that neither moved nor left
  // a selection focuses the overlay; a drag instead blurs it, leaving the
  // selection on the <pre> for the copy.
  const content = $("content");
  let pressX = 0, pressY = 0;
  content.addEventListener("mousedown", (e) => { pressX = e.clientX; pressY = e.clientY; });
  content.addEventListener("mouseup", (e) => {
    if (state.selectionMode) {
      direct.blur();
      renderMode();
      return;
    }
    const moved = Math.abs(e.clientX - pressX) > 4 || Math.abs(e.clientY - pressY) > 4;
    const sel = window.getSelection();
    if (moved || (sel && !sel.isCollapsed && sel.toString() !== "")) {
      direct.blur();
      return;
    }
    direct.focus();
  });
  direct.addEventListener("keydown", (e) => {
    if (state.selectionMode) {
      direct.blur();
      renderMode();
      return;
    }
    if (e.isComposing || e.keyCode === 229) return; // let the IME compose
    const msg = translateKey(e);
    if (!msg) return;
    e.preventDefault();
    sendDirect(msg);
  });
  direct.addEventListener("compositionend", (e) => {
    if (e.data) sendDirect({ t: "text", s: e.data });
    direct.value = "";
  });
  direct.addEventListener("paste", (e) => {
    e.preventDefault();
    const img = clipboardImage(e);
    if (img) {
      // Send the saved path plus a trailing space so the agent's input box
      // keeps it as one token, separate from whatever is typed next.
      pasteImage(img, (path) => sendDirect({ t: "text", s: path + " " }));
      return;
    }
    const t = (e.clipboardData || window.clipboardData).getData("text");
    if (t) sendDirect({ t: "text", s: t });
  });
  // Soft keyboards often insert text with no usable keydown — relay whatever
  // landed in the overlay, then clear it so it stays invisible.
  direct.addEventListener("input", (e) => {
    if (e.isComposing) return;
    const v = direct.value;
    direct.value = "";
    if (v) sendDirect({ t: "text", s: v });
  });

  document.addEventListener("focusin", renderMode);
  document.addEventListener("focusout", () => setTimeout(renderMode, 0));
}

/* ---- mobile: keep the app inside the visual viewport ---- */

// The soft keyboard shrinks the visual viewport but not the layout, so the
// input bar and the bottom of the pane end up hidden behind it. Pin the body
// to the visual viewport; on a resize (keyboard open/close) also re-anchor the
// output to its last line so the live tail stays in view.
function fitViewport() {
  const vv = window.visualViewport;
  if (!vv) return;
  document.body.style.height = vv.height + "px";
  window.scrollTo(0, 0);
  positionRecOverlay();
}
function pinViewport() {
  fitViewport();
  const pre = $("content");
  pre.scrollTop = pre.scrollHeight;
}
if (window.visualViewport) {
  window.visualViewport.addEventListener("resize", pinViewport);
  window.visualViewport.addEventListener("scroll", fitViewport);
  fitViewport();
}

/* ---- settings ---- */

const SETTINGS_KEY = "tmact.settings";
const FONT_MIN = 9, FONT_MAX = 22, FONT_DEFAULT = 13;
const RUNNING_EFFECT_DEFAULT = "shine";
const RUNNING_EFFECTS = ["shine", "pulse", "rainbow", "scan", "none"];

function readClientSettings() {
  try { return JSON.parse(localStorage.getItem(SETTINGS_KEY) || "{}") || {}; }
  catch (e) { return {}; }
}

function saveClientSettings(patch) {
  try {
    localStorage.setItem(SETTINGS_KEY,
      JSON.stringify(Object.assign(readClientSettings(), patch)));
  } catch (e) {}
}

function clampFont(px) {
  px = parseInt(px, 10);
  if (!Number.isFinite(px)) return FONT_DEFAULT;
  return Math.max(FONT_MIN, Math.min(FONT_MAX, px));
}

// applyPaneFont sets the live --pane-font variable, syncs the slider/readout,
// and persists the choice. The pane <pre> reads --pane-font, so this takes
// effect immediately with no reload.
function applyPaneFont(px) {
  px = clampFont(px);
  document.documentElement.style.setProperty("--pane-font", px + "px");
  $("font-range").value = px;
  $("font-val").textContent = px + "px";
  saveClientSettings({ paneFont: px });
}

function normalizeRunningEffect(effect) {
  return RUNNING_EFFECTS.includes(effect) ? effect : RUNNING_EFFECT_DEFAULT;
}

function applyRunningEffect(effect) {
  effect = normalizeRunningEffect(effect);
  document.documentElement.dataset.runningEffect = effect;
  $("running-effect").value = effect;
  saveClientSettings({ runningEffect: effect });
}

// loadClientSettings applies the browser-local settings at startup. It runs
// synchronously before first paint, so saved visual choices take effect before
// the first snapshot render.
function loadClientSettings() {
  const saved = readClientSettings();
  applyPaneFont(saved.paneFont);
  applyRunningEffect(saved.runningEffect);
}

function currentPaneFont() {
  return clampFont(parseFloat(
    getComputedStyle(document.documentElement).getPropertyValue("--pane-font")));
}

let sttStatusTimer = null;
function setSTTStatus(msg, kind) {
  const el = $("stt-status");
  el.textContent = msg;
  el.className = "settings-status" + (kind ? " " + kind : "");
  if (sttStatusTimer) { clearTimeout(sttStatusTimer); sttStatusTimer = null; }
  if (msg && kind === "ok") {
    sttStatusTimer = setTimeout(() => setSTTStatus("", ""), 4000);
  }
}

function applySTTNote(configured) {
  $("stt-note").textContent = configured
    ? "An API key is set on the server — leave the key blank to keep it."
    : "No API key set yet — enter one to enable voice transcription.";
}

// loadSTTSettings pulls the server-side STT config into the form. The API key
// is never returned by the server, so that field always starts blank.
async function loadSTTSettings() {
  $("stt-model").value = "";
  $("stt-endpoint").value = "";
  $("stt-key").value = "";
  setSTTStatus("loading…", "");
  try {
    const { res, data } = await loadSTTConfig();
    if (!res.ok) throw new Error(data.error || ("HTTP " + res.status));
    $("stt-model").value = data.model || "";
    $("stt-endpoint").value = data.endpoint || "";
    applySTTNote(!!data.configured);
    setSTTStatus("", "");
  } catch (e) {
    setSTTStatus(e.message || "failed to load", "err");
  }
}

async function loadVersionInfo() {
  const el = $("build-time");
  el.textContent = "loading…";
  try {
    const { res, data } = await loadVersion();
    if (!res.ok) throw new Error(data.error || ("HTTP " + res.status));
    el.textContent = data.build_time || "unavailable";
  } catch (e) {
    el.textContent = "unavailable";
  }
}

async function saveSTTSettings() {
  const btn = $("stt-save");
  btn.disabled = true;
  setSTTStatus("saving…", "");
  try {
    const { res, data } = await saveSTTConfig({
      model: $("stt-model").value,
      endpoint: $("stt-endpoint").value,
      api_key: $("stt-key").value,
    });
    if (!res.ok) throw new Error(data.error || ("HTTP " + res.status));
    $("stt-model").value = data.model || "";
    $("stt-endpoint").value = data.endpoint || "";
    $("stt-key").value = "";
    applySTTNote(!!data.configured);
    setSTTStatus("saved ✓", "ok");
  } catch (e) {
    setSTTStatus(e.message || "save failed", "err");
  } finally {
    btn.disabled = false;
  }
}

function openSettings() {
  $("settings-overlay").hidden = false;
  loadSTTSettings();
  loadVersionInfo();
}
function closeSettings() {
  $("settings-overlay").hidden = true;
}

function wireSettings() {
  $("gear-btn").addEventListener("click", openSettings);
  $("settings-close").addEventListener("click", closeSettings);
  // A click on the dim backdrop (but not the card) closes the panel.
  $("settings-overlay").addEventListener("mousedown", (e) => {
    if (e.target === $("settings-overlay")) closeSettings();
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !$("settings-overlay").hidden) closeSettings();
  });
  $("font-range").addEventListener("input", (e) => applyPaneFont(e.target.value));
  $("font-dec").addEventListener("click", () => applyPaneFont(currentPaneFont() - 1));
  $("font-inc").addEventListener("click", () => applyPaneFont(currentPaneFont() + 1));
  $("running-effect").addEventListener("change", (e) => applyRunningEffect(e.target.value));
  $("stt-save").addEventListener("click", saveSTTSettings);
}

/* ---- quick-input buttons (phone-only FAB) ---- */

const QUICK_KEY = "tmact.quickButtons";
// Groups the FAB can show: "common" appears for every pane; the rest are
// matched to the pane's detected runtime. Keep in step with QB_LABEL,
// RUNTIME_GROUP, and QB_DEFAULT.
const QB_GROUPS = ["common", "claude", "codex", "shell"];
const QB_LABEL = {
  common: "Common · every pane",
  claude: "Claude panes",
  codex: "Codex panes",
  shell: "Shell panes",
};
// Snapshot runtime → quick-button group. A runtime with no group (gemini,
// copilot, tmact, unknown) falls back to the Common group alone.
const RUNTIME_GROUP = { claude: "claude", codex: "codex", shell: "shell" };
// Seeded on first run; the user edits these from the settings panel.
const QB_DEFAULT = {
  common: [],
  claude: [{ label: "/compact", text: "/compact" }, { label: "/clear", text: "/clear" }],
  codex: [{ label: "/compact", text: "/compact" }, { label: "/clear", text: "/clear" }],
  shell: [],
};

let quickConfig = {};

// loadQuickConfig reads the per-group button lists from localStorage, seeding
// the defaults on first run. Every entry is normalised to {label, text} and
// every group is present, so later code can index quickConfig freely.
function loadQuickConfig() {
  let saved = null;
  try { saved = JSON.parse(localStorage.getItem(QUICK_KEY) || "null"); }
  catch (e) {}
  if (!saved || typeof saved !== "object") {
    quickConfig = JSON.parse(JSON.stringify(QB_DEFAULT));
    saveQuickConfig();
    return;
  }
  quickConfig = {};
  for (const g of QB_GROUPS) {
    const list = Array.isArray(saved[g]) ? saved[g] : [];
    quickConfig[g] = list.map((x) => ({
      label: x && typeof x.label === "string" ? x.label : "",
      text: x && typeof x.text === "string" ? x.text : "",
    }));
  }
}

function saveQuickConfig() {
  try { localStorage.setItem(QUICK_KEY, JSON.stringify(quickConfig)); }
  catch (e) {}
}

// applicableQuick returns the buttons to show for the selected pane: the
// Common group plus the group matching the pane's runtime. Entries with no
// text are dropped — they are half-finished editor rows.
function applicableQuick() {
  const out = [];
  for (const it of quickConfig.common || []) if (it.text) out.push(it);
  const pane = findPane(state.selected);
  const g = pane && RUNTIME_GROUP[pane.runtime];
  if (g) for (const it of quickConfig[g] || []) if (it.text) out.push(it);
  return out;
}

// renderQuickMenu rebuilds the FAB menu from the current pane + config. Each
// button types its text into the pane and presses Enter (a "send" message).
function renderQuickMenu() {
  const menu = $("qb-menu");
  menu.textContent = "";
  const items = applicableQuick();
  if (items.length === 0) {
    menu.appendChild(h("div", { class: "qb-empty",
      text: "No quick buttons for this pane — add some in Settings." }));
    return;
  }
  for (const it of items) {
    const btn = h("button", { type: "button", title: it.text, text: it.label || it.text });
    // pointerdown preventDefault keeps the soft keyboard up and focus in place.
    btn.addEventListener("pointerdown", (e) => e.preventDefault());
    btn.addEventListener("click", () => {
      if (!wsSend({ t: "send", s: it.text })) {
        showInputError("not connected — try again");
        return;
      }
      closeQuickMenu();
    });
    menu.appendChild(btn);
  }
}

function openQuickMenu() {
  if (!state.selected) return;
  renderQuickMenu();
  $("qb-dock").classList.add("open");
  $("qb-backdrop").classList.add("open");
}
function closeQuickMenu() {
  $("qb-dock").classList.remove("open");
  $("qb-backdrop").classList.remove("open");
}
function toggleQuickMenu() {
  if ($("qb-dock").classList.contains("open")) closeQuickMenu();
  else openQuickMenu();
}

// syncQuickDock reveals the FAB once a pane is selected and hides it (closing
// any open menu) when none is.
function syncQuickDock() {
  const dock = $("qb-dock");
  const uploadBtn = $("upload-btn");
  const selectionBtn = $("selection-btn");
  const wrap = $("content-wrap");
  if (state.selected) {
    dock.classList.add("ready");
    uploadBtn.classList.add("ready");
    selectionBtn.classList.add("ready");
    wrap.classList.add("upload-ready");
    uploadBtn.disabled = upload.busy;
    selectionBtn.disabled = false;
  } else {
    dock.classList.remove("ready");
    uploadBtn.classList.remove("ready");
    selectionBtn.classList.remove("ready");
    wrap.classList.remove("upload-ready");
    uploadBtn.disabled = true;
    selectionBtn.disabled = true;
    closeQuickMenu();
  }
  syncSelectionButton();
}

// quickRow builds one editable button row, bound by object reference to its
// config entry so edits land straight in quickConfig.
function quickRow(group, item) {
  const label = h("input", { class: "qb-label", type: "text", placeholder: "label",
    spellcheck: "false", autocapitalize: "off", autocomplete: "off" });
  label.value = item.label;
  const text = h("input", { class: "qb-text", type: "text",
    placeholder: "text sent to the pane (Enter is added)",
    spellcheck: "false", autocapitalize: "off", autocomplete: "off" });
  text.value = item.text;
  label.addEventListener("input", () => {
    item.label = label.value; saveQuickConfig(); renderQuickMenu();
  });
  text.addEventListener("input", () => {
    item.text = text.value; saveQuickConfig(); renderQuickMenu();
  });
  const del = h("button", { class: "qb-del", type: "button",
    title: "remove button", text: "✕" });
  del.addEventListener("click", () => {
    quickConfig[group] = quickConfig[group].filter((x) => x !== item);
    saveQuickConfig();
    renderQuickEditor();
    renderQuickMenu();
  });
  return h("div", { class: "qb-row" }, label, text, del);
}

// renderQuickEditor rebuilds the settings-panel editor: one block per group,
// each holding its rows and an "add" button.
function renderQuickEditor() {
  const root = $("qb-editor");
  root.textContent = "";
  for (const g of QB_GROUPS) {
    const rows = h("div", { class: "qb-rows" });
    for (const item of quickConfig[g]) rows.appendChild(quickRow(g, item));
    const add = h("button", { class: "qb-add", type: "button", text: "+ Add button" });
    add.addEventListener("click", () => {
      quickConfig[g].push({ label: "", text: "" });
      saveQuickConfig();
      renderQuickEditor();
    });
    root.appendChild(h("div", { class: "qb-group" },
      h("div", { class: "qb-group-head", text: QB_LABEL[g] }),
      rows, add));
  }
}

function wireQuick() {
  // pointerdown preventDefault keeps the soft keyboard up and focus in place.
  $("qb-fab").addEventListener("pointerdown", (e) => e.preventDefault());
  $("qb-fab").addEventListener("click", toggleQuickMenu);
  $("qb-backdrop").addEventListener("click", closeQuickMenu);
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && $("qb-dock").classList.contains("open")) closeQuickMenu();
  });
  renderQuickEditor();
}

/* ---- help / coachmark overlay ----
   The `?` button stacked above the FAB lifts a dimmed overlay that points
   at the main controls with their hotkey, so newcomers don't have to guess.
   Tips with `skip` true (mobile-only feature on desktop, or vice-versa, or a
   disabled control) are dropped before drawing. */

function helpTips() {
  return [
    { target: () => $("chips"),
      key: "Option+1…0, q…p",
      desc: "Switch panes (hardware keyboard)",
      tone: "pane",
      place: "above-left",
      skip: () => isMobile() || !$("chips").children.length },
    { target: () => $("draft"),
      key: "⌘ / Ctrl + Enter",
      desc: "Send the typed prompt to the selected pane",
      tone: "send",
      place: "above-left",
      skip: () => isMobile() },
    { target: () => $("send-btn"),
      key: "Tap Send",
      desc: "Send the typed prompt to the selected pane",
      tone: "send",
      skip: () => !isMobile() || !state.selected },
    { target: () => $("record-btn"),
      key: "Option+V, then V/C",
      desc: "Record voice, then send or cancel",
      tone: "voice",
      place: "above-right",
      skip: () => isMobile() },
    { target: () => $("content"),
      key: "Click pane",
      desc: "Direct mode — your keystrokes go straight to tmux",
      tone: "direct",
      place: "inside-top-left",
      skip: () => isMobile() },
    { target: () => $("qb-fab"),
      key: "Tap ⚡",
      desc: "Quick prompts — configurable in Settings",
      tone: "quick" },
    { target: () => $("upload-btn"),
      key: "Tap upload",
      desc: "Upload a file and paste its server path",
      tone: "upload",
      place: "left",
      skip: () => !state.selected },
    { target: () => $("selection-btn"),
      key: "Tap select",
      desc: "Toggle pane selection mode",
      tone: "selection",
      place: "left",
      skip: () => !state.selected },
    { target: () => $("gear-btn"),
      key: "Gear",
      desc: "Settings — quick buttons, voice model, font size",
      tone: "settings",
      place: "left" },
  ];
}

function overlapArea(a, b) {
  const w = Math.min(a.right, b.right) - Math.max(a.left, b.left);
  const h = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
  return w > 0 && h > 0 ? w * h : 0;
}

function rectAt(left, top, width, height) {
  return { left, top, right: left + width, bottom: top + height, width, height };
}

function coachmarkViewport() {
  const vv = window.visualViewport;
  const left = vv ? vv.offsetLeft : 0;
  const top = vv ? vv.offsetTop : 0;
  const width = vv ? vv.width : window.innerWidth;
  const height = vv ? vv.height : window.innerHeight;
  return {
    left: left + 8,
    top: top + 8,
    right: left + width - 8,
    bottom: top + height - 8,
    width,
    height,
  };
}

function scoreCoachmark(rect, preferred, placed, rings, vp) {
  let score = Math.abs(rect.top - preferred.top) + Math.abs(rect.left - preferred.left) * 0.35;
  for (const p of placed) score += overlapArea(rect, p) * 120;
  const largeRingArea = vp.width * vp.height * 0.16;
  for (const r of rings) {
    if (r.width * r.height > largeRingArea) continue;
    score += overlapArea(rect, r) * 18;
  }
  return score;
}

function preferredCoachmarkPosition(place, targetRect, cw, ch, vp) {
  const gap = 10;
  const sideGap = 12;
  const centeredLeft = targetRect.left + targetRect.width / 2 - cw / 2;
  switch (place) {
    case "inside-top-left":
      return {
        left: targetRect.left + sideGap,
        top: targetRect.top + sideGap,
      };
    case "above-left":
      return {
        left: targetRect.left,
        top: targetRect.top - ch - gap,
      };
    case "above-right":
      return {
        left: targetRect.right - cw,
        top: targetRect.top - ch - gap,
      };
    case "left":
      return {
        left: targetRect.left - cw - sideGap,
        top: targetRect.top + targetRect.height / 2 - ch / 2,
      };
    case "right":
      return {
        left: targetRect.right + sideGap,
        top: targetRect.top + targetRect.height / 2 - ch / 2,
      };
    default: {
      const nearBottom = targetRect.bottom > vp.top + vp.height * 0.58;
      return {
        left: centeredLeft,
        top: nearBottom ? targetRect.top - ch - gap : targetRect.bottom + gap,
      };
    }
  }
}

function placeCoachmarkCard(card, tip, targetRect, placed, rings, vp) {
  const cw = card.offsetWidth;
  const ch = card.offsetHeight;
  const gap = 10;
  const sideGap = 12;
  const centeredLeft = targetRect.left + targetRect.width / 2 - cw / 2;
  const rightSideTarget = isMobile() && targetRect.left > vp.left + vp.width * 0.55;
  const defaultPlace = rightSideTarget ? "left" : "";
  const preferredPos = preferredCoachmarkPosition(tip.place || defaultPlace, targetRect, cw, ch, vp);
  const preferredTop = preferredPos.top;
  const preferredLeft = preferredPos.left;
  const xCandidates = [
    preferredLeft,
    targetRect.left - cw - sideGap,
    targetRect.right + sideGap,
    centeredLeft,
    targetRect.left,
    targetRect.right - cw,
  ];
  const ySeeds = [
    preferredTop,
    targetRect.bottom + gap,
    targetRect.top - ch - gap,
    targetRect.top + sideGap,
    targetRect.top + targetRect.height / 2 - ch / 2,
  ];
  const candidates = [];
  const add = (left, top) => {
    const clampedLeft = clamp(left, vp.left, vp.right - cw);
    const clampedTop = clamp(top, vp.top, vp.bottom - ch);
    candidates.push(rectAt(clampedLeft, clampedTop, cw, ch));
  };

  for (const x of xCandidates) {
    for (const y of ySeeds) add(x, y);
    for (let y = vp.top; y <= vp.bottom - ch; y += 12) add(x, y);
  }

  const preferred = rectAt(
    clamp(preferredLeft, vp.left, vp.right - cw),
    clamp(preferredTop, vp.top, vp.bottom - ch),
    cw,
    ch,
  );
  let best = preferred;
  let bestScore = scoreCoachmark(preferred, preferred, placed, rings, vp);
  for (const c of candidates) {
    const score = scoreCoachmark(c, preferred, placed, rings, vp);
    if (score < bestScore) {
      best = c;
      bestScore = score;
    }
  }
  card.style.left = best.left + "px";
  card.style.top = best.top + "px";
  placed.push(best);
}

function placeCoachmarks() {
  const overlay = $("help-overlay");
  for (const el of overlay.querySelectorAll(".help-ring, .help-tip, .help-banner")) el.remove();
  overlay.appendChild(h("div", { class: "help-banner",
    text: "Hotkey hints · tap anywhere or press Esc to close" }));
  const banner = overlay.querySelector(".help-banner");
  const placed = [banner.getBoundingClientRect()];
  const rings = [];
  const items = [];
  for (const tip of helpTips()) {
    if (tip.skip && tip.skip()) continue;
    const el = tip.target();
    if (!el) continue;
    const r = el.getBoundingClientRect();
    // Display:none and detached elements both produce a 0×0 rect.
    if (r.width < 1 || r.height < 1) continue;
    items.push({ tip, rect: r });
  }
  items.sort((a, b) => a.rect.top - b.rect.top);
  for (const item of items) {
    const r = item.rect;
    const tone = item.tip.tone ? " tone-" + item.tip.tone : "";
    const ring = h("div", { class: "help-ring" + tone });
    ring.style.left = (r.left - 4) + "px";
    ring.style.top = (r.top - 4) + "px";
    ring.style.width = (r.width + 8) + "px";
    ring.style.height = (r.height + 8) + "px";
    overlay.appendChild(ring);
    rings.push(rectAt(r.left - 4, r.top - 4, r.width + 8, r.height + 8));
  }
  const vp = coachmarkViewport();
  for (const item of items) {
    const r = item.rect;
    const tip = item.tip;
    const tone = tip.tone ? " tone-" + tip.tone : "";
    const card = h("div", { class: "help-tip" + tone },
      h("span", { class: "help-key", text: tip.key }),
      h("span", { class: "help-desc", text: tip.desc }));
    overlay.appendChild(card);
    placeCoachmarkCard(card, tip, r, placed, rings, vp);
  }
}

function openHelp() {
  document.body.classList.add("help-open");
  placeCoachmarks();
}

function closeHelp() {
  document.body.classList.remove("help-open");
  const overlay = $("help-overlay");
  for (const el of overlay.querySelectorAll(".help-ring, .help-tip, .help-banner")) el.remove();
}

function toggleHelp() {
  if (document.body.classList.contains("help-open")) closeHelp();
  else openHelp();
}

function wireHelp() {
  // Mirror the qb-fab pattern: pointerdown preventDefault keeps the focused
  // input focused. iOS Safari rearranges its chrome (URL bar, safe-area)
  // whenever focus moves, and fitViewport() then pins body to the new
  // visualViewport.height — the visible safe-area padding under the input
  // bar disappears after a toggle cycle if we let the focus shift.
  $("help-btn").addEventListener("pointerdown", (e) => e.preventDefault());
  $("help-btn").addEventListener("click", (e) => { e.stopPropagation(); toggleHelp(); });
  $("help-overlay").addEventListener("pointerdown", (e) => e.preventDefault());
  $("help-overlay").addEventListener("click", closeHelp);
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && document.body.classList.contains("help-open")) closeHelp();
  });
  window.addEventListener("resize", () => {
    if (document.body.classList.contains("help-open")) placeCoachmarks();
  });
}

/* ---- desktop pane-switch hotkeys ---- */

// wireHotkeys binds Option+<key> to "select the Nth statusline chip". The
// listener runs in the capture phase so it wins over the direct-mode relay —
// in direct mode every keystroke is forwarded to the pane, and without
// capturing first, Option+1 would be sent to the pane instead of switching.
function wireHotkeys() {
  document.addEventListener("keydown", (e) => {
    // macOS-only chord: plain Option, no Ctrl/Cmd. Mobile has no Option key,
    // and an open settings panel may legitimately want Option for text input.
    if (!e.altKey || e.ctrlKey || e.metaKey || isMobile()) return;
    if (!$("settings-overlay").hidden) return;
    const idx = HOTKEY_INDEX[e.code];
    if (idx === undefined || idx >= state.paneOrder.length) return;
    e.preventDefault();
    e.stopPropagation();
    selectPane(state.paneOrder[idx]);
  }, true);
}

/* ---- lifecycle ---- */

document.addEventListener("visibilitychange", () => {
  if (document.hidden) {
    stopPolling();
    closeWS();
  } else {
    refreshSnapshot();
    startPolling();
    if (state.selected) openWS(state.selected);
  }
});

loadClientSettings();
loadQuickConfig();
wireInput();
wireSettings();
wireQuick();
wireHelp();
wireRecordHotkey();
wireHotkeys();
if ("serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch(() => {});
  });
}
refreshSnapshot();
if (!document.hidden) startPolling();
