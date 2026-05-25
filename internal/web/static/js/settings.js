// Settings: gear-button modal — font size, running-row effect, server-side
// STT config, and build info. State lives in localStorage and on the server;
// no shared in-memory state with the rest of the app.

import { $ } from "./dom.js";
import { loadSTTConfig, loadVersion, saveSTTConfig } from "./api.js";

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
export function loadClientSettings() {
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
  const buildEl = $("build-time");
  const hashEl = $("asset-hash");
  buildEl.textContent = "loading…";
  hashEl.textContent = "loading…";
  try {
    const { res, data } = await loadVersion();
    if (!res.ok) throw new Error(data.error || ("HTTP " + res.status));
    buildEl.textContent = data.build_time || "unavailable";
    hashEl.textContent = data.asset_hash || "unavailable";
  } catch (e) {
    buildEl.textContent = "unavailable";
    hashEl.textContent = "unavailable";
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

export function wireSettings() {
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
