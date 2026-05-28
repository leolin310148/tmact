import { fetchSnapshot, subscribeSnapshot, transcribeAudio } from "./js/api.js";
import { $, clamp, h, isMobile } from "./js/dom.js";
import { state, upload } from "./js/state.js";
import { createPaneStream } from "./js/stream.js";
import { setContent } from "./js/terminal.js";
import { createVoice } from "./js/voice.js";
import { wireHelp } from "./js/help.js";
import { loadClientSettings, wireSettings } from "./js/settings.js";
import { createQuick } from "./js/quick.js";
import { createUpload } from "./js/upload.js";

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
  if (p.stale) return "stale";
  if (p.asking) return "asking";
  if (p.running) return "running";
  return "idle";
}
function paneStateLabel(p) {
  if (p.stale) return "stale";
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
function panePeer(p) {
  return p && p.peer ? String(p.peer) : "";
}
function sessionLabel(p) {
  const peer = panePeer(p);
  const session = p && p.session ? String(p.session) : "";
  if (peer && session.startsWith(peer + "@")) return session.slice(peer.length + 1);
  return session;
}
function paneIndicator(p) {
  const runtime = paneRuntime(p);
  const icon = RUNTIME_ICON[runtime];
  if (icon) {
    const cls = ["agent-icon", "runtime-" + runtime];
    if (p.stale) cls.push("stale");
    else {
      if (p.running) cls.push("running");
      if (p.asking) cls.push("asking");
    }
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
    panePeer(a).localeCompare(panePeer(b)) ||
    sessionLabel(a).localeCompare(sessionLabel(b)) ||
    a.window_index - b.window_index ||
    a.pane_index - b.pane_index);

  const perSession = {};
  for (const p of panes) {
    const k = panePeer(p) + "\0" + sessionLabel(p);
    perSession[k] = (perSession[k] || 0) + 1;
  }

  // Freeze the rendered order so the Option+key hotkeys map to the same chips
  // the user sees, in the same left-to-right sequence.
  state.paneOrder = panes.map((p) => p.pane_id);

  panes.forEach((p, i) => {
    const peer = panePeer(p);
    const baseLabel = sessionLabel(p);
    const labelKey = peer + "\0" + baseLabel;
    const label = perSession[labelKey] > 1
      ? baseLabel + ":" + p.window_index
      : baseLabel;
    const indicator = paneIndicator(p);
    const key = PANE_HOTKEYS[i];
    const chip = h("div", {
      class: "chip" + (p.pane_id === state.selected ? " sel" : "") + (p.stale ? " stale" : ""),
      title: (peer ? peer + " — " : "") + (p.cwd || p.session) + " — " + paneStateLabel(p)
        + (key ? " — Option+" + key : ""),
    },
      key ? h("span", { class: "chip-key", text: key }) : null,
      peer ? h("span", { class: "peer-badge", text: peer }) : null,
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
    applySnapshot(await fetchSnapshot());
  } catch (e) {
    // Keep the last snapshot; the stale dot surfaces the lost connection.
    checkStale();
  }
}

// Snapshot delivery: prefer SSE push from /api/snapshot/stream; on stream
// error (network blip, idle timeout in some proxies, or older statusd build),
// fall back to a periodic GET. Either way the rendered UI ends up the same.
let snapshotTimer = null;
let snapshotSSE = null;

function applySnapshot(snap) {
  state.snapshot = snap;
  renderStatusline(snap);
  renderMode();
  restoreSelection();
  syncQuickDock();
  checkStale();
}

function startPolling() {
  if (snapshotTimer === null) {
    snapshotTimer = setInterval(() => { refreshSnapshot(); checkStale(); }, POLL_MS);
  }
}
function stopPolling() {
  clearInterval(snapshotTimer);
  snapshotTimer = null;
}

function startSnapshotStream() {
  if (snapshotSSE) return;
  snapshotSSE = subscribeSnapshot(
    (snap) => { stopPolling(); applySnapshot(snap); },
    () => {
      snapshotSSE = null;
      startPolling();
    },
  );
}
function stopSnapshotStream() {
  if (snapshotSSE) { snapshotSSE(); snapshotSSE = null; }
}

/* ---- pane WebSocket (output stream + input relay) ---- */

// paneLines mirrors the server-side lastLines buffer: each patch from the
// WS replaces lines[from:], so the client only renders the joined string.
// Reset on every openWS — a fresh connection always starts with from=0.
let paneLines = [];
const paneStream = createPaneStream({
  getSelectedPane: () => state.selected,
  onPatch: (from, lines, question) => {
    paneLines = paneLines.slice(0, from).concat(lines);
    const p = findPane(state.selected);
    setContent(paneLines.join("\n"), { cwd: p && p.cwd, peer: panePeer(p) });
    renderOptions(question);
  },
  onQuestion: renderOptions,
  onError: showInputError,
  onStatus: (s) => {
    // Surface a reconnecting hint in the mode-indicator line; on reconnect
    // the next "open" clears it. The error timer is independent — its 6s
    // auto-clear can still wipe an inline error message.
    if (s === "connecting") {
      setInputStatus("connecting…");
      if (paneLines.length === 0) setContent("Connecting…");
    } else if (s === "reconnecting") {
      setInputStatus("reconnecting…");
      if (paneLines.length === 0) setContent("Reconnecting…");
    } else if (s === "open") setInputStatus("");
  },
});

// tmux window size is owned by statusd (fixed --pane-cols/--pane-rows) so the
// browser doesn't request a resize on viewport change. The pane <pre> uses
// CSS pre-wrap to fit narrower viewports without re-flowing the tmux grid.

function closeWS() {
  paneStream.close();
}

function openWS(paneID) {
  paneLines = [];
  paneStream.open(paneID);
}

function wsSend(obj) {
  return paneStream.send(obj);
}

function selectPane(paneID) {
  if (!paneID) return;
  if (paneID === state.selected) {
    paneLines = [];
    setContent("Reconnecting…");
    renderOptions(null);
    openWS(paneID);
    return;
  }
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

function clearPaneOutput() {
  if (!state.selected) return;
  if (!wsSend({ t: "clear" })) showInputError("not connected — try again");
}

const _voice = createVoice({ showInputError, syncDraft });
const {
  syncRecordButton,
  positionRecOverlay,
  startRecording,
  stopRecording,
  cancelRecording,
  finishRecordingConfirm,
  wireRecordHotkey,
} = _voice;

const _upload = createUpload({
  setInputStatus,
  showInputError,
  syncDraft,
  wsSend,
  getSelectedPeer: () => {
    const p = findPane(state.selected);
    return panePeer(p);
  },
});
const {
  clipboardImage,
  pasteImage,
  uploadFilesToPane,
  openFileUploadPicker,
  placeInDraft,
} = _upload;

/* ---- image path preview ---- */

let imagePreview = null;
function ensureImagePreview() {
  if (imagePreview) return imagePreview;

  const overlay = h("div", { class: "image-preview", hidden: "" },
    h("div", { class: "image-preview-card" },
      h("button", { class: "image-preview-close", type: "button", title: "close", "aria-label": "close image preview" },
        h("svg", { viewBox: "0 0 24 24", fill: "none", stroke: "currentColor", "stroke-width": "2.5", "stroke-linecap": "round", "aria-hidden": "true" },
          h("path", { d: "M18 6 6 18" }),
          h("path", { d: "m6 6 12 12" }))),
      h("img", { alt: "preview" }),
      h("div", { class: "image-preview-path" })));
  document.body.appendChild(overlay);

  const close = () => {
    overlay.hidden = true;
    overlay.querySelector("img").removeAttribute("src");
  };
  overlay.addEventListener("click", (e) => {
    if (e.target === overlay) close();
  });
  overlay.querySelector(".image-preview-close").addEventListener("click", close);
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !overlay.hidden) close();
  });

  imagePreview = { overlay, close };
  return imagePreview;
}

function previewImagePath(path, cwd, peer) {
  const ui = ensureImagePreview();
  const qs = new URLSearchParams({ path });
  if (cwd) qs.set("cwd", cwd);
  if (peer) qs.set("peer", peer);
  ui.overlay.querySelector("img").src = "/api/image?" + qs.toString();
  ui.overlay.querySelector(".image-preview-path").textContent = path;
  ui.overlay.hidden = false;
}

const IMAGE_LONG_PRESS_MS = 550;
const IMAGE_LONG_PRESS_MOVE = 10;
let imagePress = null;

function clearImagePress() {
  if (imagePress && imagePress.timer) clearTimeout(imagePress.timer);
  imagePress = null;
}

function imageTarget(e) {
  return e.target && e.target.closest ? e.target.closest(".image-path") : null;
}

function openImageTarget(target) {
  previewImagePath(target.dataset.path || target.textContent, target.dataset.cwd || "", target.dataset.peer || "");
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
      return;
    }
    if (!e.isComposing && e.key === "Enter" && !e.shiftKey && state.selected && !draft.value.trim()) {
      e.preventDefault();
      state.selectionMode = false;
      syncSelectionButton();
      $("direct-input").focus();
      renderMode();
      sendDirect({ t: "key", k: "Enter" });
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
  $("clear-pane-btn").addEventListener("pointerdown", (e) => e.preventDefault());
  $("selection-btn").addEventListener("pointerdown", (e) => e.preventDefault());
  $("send-btn").addEventListener("click", sendDraft);
  $("clear-pane-btn").addEventListener("click", clearPaneOutput);
  $("upload-btn").addEventListener("click", openFileUploadPicker);
  $("selection-btn").addEventListener("click", toggleSelectionMode);
  $("file-upload").addEventListener("change", (e) => {
    const files = Array.from(e.target.files || []);
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
  // A plain click on the output focuses the invisible overlay for direct
  // keystroke passthrough; a drag selects pane text (desktop only — coarse
  // pointers keep the toggle button so a tap can't accidentally start a
  // selection). The mouseup handler skips refocusing direct-input when a
  // non-empty selection lives in pre#content, otherwise the focus change
  // would erase the selection before the user can copy it.
  const content = $("content");
  content.addEventListener("mouseup", () => {
    if (state.selectionMode) {
      direct.blur();
      renderMode();
      return;
    }
    // After a shift-drag we want to leave focus alone — refocusing
    // #direct-input would clear the just-made selection in pre#content
    // and the user can't even copy it. The next plain click returns to
    // direct mode via this same handler.
    const sel = window.getSelection();
    if (sel && !sel.isCollapsed && content.contains(sel.anchorNode)) return;
    direct.focus();
  });
  content.addEventListener("click", (e) => {
    if (imagePress && imagePress.opened) {
      e.preventDefault();
      e.stopPropagation();
      clearImagePress();
      return;
    }
    const target = imageTarget(e);
    if (!target || !e.metaKey) return;
    e.preventDefault();
    e.stopPropagation();
    openImageTarget(target);
  });
  content.addEventListener("pointerdown", (e) => {
    const target = imageTarget(e);
    if (!target || e.pointerType === "mouse") return;
    clearImagePress();
    imagePress = {
      target,
      x: e.clientX,
      y: e.clientY,
      opened: false,
      timer: setTimeout(() => {
        if (!imagePress || imagePress.target !== target) return;
        imagePress.opened = true;
        openImageTarget(target);
      }, IMAGE_LONG_PRESS_MS),
    };
  });
  content.addEventListener("pointermove", (e) => {
    if (!imagePress) return;
    const dx = Math.abs(e.clientX - imagePress.x);
    const dy = Math.abs(e.clientY - imagePress.y);
    if (dx > IMAGE_LONG_PRESS_MOVE || dy > IMAGE_LONG_PRESS_MOVE) clearImagePress();
  });
  content.addEventListener("pointerup", () => {
    if (!imagePress || imagePress.opened) return;
    clearImagePress();
  });
  content.addEventListener("pointercancel", clearImagePress);
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

// The soft keyboard shrinks the visual viewport but not the layout viewport.
// Keep the flex shell sized to the visible viewport and cancel Safari's
// document-level focus scroll; pane output has its own scroll container, so
// resizing must not force it to the captured tmux buffer's blank tail.
function fitViewport() {
  const vv = window.visualViewport;
  if (!vv) return;
  document.documentElement.style.setProperty("--tmact-vvh", vv.height + "px");
  window.scrollTo(0, 0);
  positionRecOverlay();
}
function scheduleFitViewport() {
  fitViewport();
  requestAnimationFrame(fitViewport);
  setTimeout(fitViewport, 80);
  setTimeout(fitViewport, 260);
}
if (window.visualViewport) {
  window.visualViewport.addEventListener("resize", scheduleFitViewport);
  window.visualViewport.addEventListener("scroll", fitViewport);
  window.addEventListener("orientationchange", scheduleFitViewport);
  document.addEventListener("focusin", scheduleFitViewport);
  fitViewport();
}


const _quick = createQuick({
  wsSend,
  showInputError,
  findPane,
  syncSelectionButton,
});
const { loadQuickConfig, wireQuick, syncQuickDock, closeQuickMenu } = _quick;


/* ---- desktop pane-switch hotkeys ---- */

// wireHotkeys binds Option+<key> to "select the Nth statusline chip". The
// listener runs in the capture phase so it wins over the direct-mode relay —
// in direct mode every keystroke is forwarded to the pane, and without
// capturing first, Option+1 would be sent to the pane instead of switching.
function wireHotkeys() {
  document.addEventListener("keydown", (e) => {
    if (state.selected && $("settings-overlay").hidden) {
      const k = e.key.toLowerCase();
      const clearPane = (e.metaKey && !e.ctrlKey && !e.altKey && k === "k")
        || (e.ctrlKey && !e.metaKey && !e.altKey && k === "l");
      if (clearPane) {
        e.preventDefault();
        e.stopPropagation();
        clearPaneOutput();
        return;
      }
    }
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
    stopSnapshotStream();
    closeWS();
  } else {
    refreshSnapshot();
    startSnapshotStream();
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
if (!document.hidden) startSnapshotStream();
