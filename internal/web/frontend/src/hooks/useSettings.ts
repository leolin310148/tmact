// useSettings — faithful port of static/js/settings.js (module-level, no
// factory). Owns the settings overlay visibility plus the imperative
// form-loading logic for STT config, version info, the panel-font slider, and
// the running-effect select. State lives in localStorage["tmact.settings"] and
// on the server; there is no shared in-memory state with the rest of the app.
//
// Visibility: the original toggled `#settings-overlay.hidden`. Here openSettings
// /closeSettings drive a React `visible` state (the SettingsDialog renders the
// overlay with `hidden={!visible}`), and openSettings additionally reloads STT +
// version every open — exactly like settings.js openSettings().
//
// The DOM-touching helpers (applyPaneFont, applyRunningEffect, setSTTStatus,
// loadSTTSettings, loadVersionInfo, saveSTTSettings) operate on refs into the
// always-mounted overlay markup, preserving the original imperative timing:
// fields are blanked, then async-filled; the font readout is read back from
// getComputedStyle("--pane-font"); status auto-clears after 4 s on success.

import { useCallback, useRef, useState, type MutableRefObject } from "react";
import { clamp } from "../lib/dom";
import { loadSTTConfig, loadVersion, saveSTTConfig } from "../api/client";
import { FRONTEND_BUILD } from "../lib/buildInfo";

const SETTINGS_KEY = "tmact.settings";
const FONT_MIN = 9,
  FONT_MAX = 22,
  FONT_DEFAULT = 13;
const RUNNING_EFFECT_DEFAULT = "shine";
const RUNNING_EFFECTS = ["shine", "pulse", "rainbow", "scan", "none"];
const PANE_SWITCHER_LAYOUT_DEFAULT = "auto";
const PANE_SWITCHER_LAYOUTS = ["auto", "side", "bottom"];
const OFFICE_SCALE_MIN = 60,
  OFFICE_SCALE_MAX = 120,
  OFFICE_SCALE_DEFAULT = 100;

interface ClientSettings {
  paneFont?: number;
  runningEffect?: string;
  paneSwitcherLayout?: string;
  officeScale?: number | "auto";
  voiceInputDeviceId?: string;
  // markdown view toggle (pane output rendered with pipe tables); global,
  // persisted, default off. Owned by App's React state, not the overlay form —
  // it just shares the tmact.settings blob via the read/save helpers below.
  markdownView?: boolean;
}

function readClientSettings(): ClientSettings {
  try {
    return (JSON.parse(localStorage.getItem(SETTINGS_KEY) || "{}") as ClientSettings) || {};
  } catch (e) {
    return {};
  }
}

function saveClientSettings(patch: ClientSettings): void {
  try {
    localStorage.setItem(
      SETTINGS_KEY,
      JSON.stringify(Object.assign(readClientSettings(), patch)),
    );
  } catch (e) {
    /* ignore */
  }
}

// Markdown-view persistence. App seeds its React state from readMarkdownView()
// at startup and calls saveMarkdownView() on each toggle. Kept here so the
// tmact.settings shape stays owned by one module.
export function readMarkdownView(): boolean {
  return readClientSettings().markdownView === true;
}

export function saveMarkdownView(on: boolean): void {
  saveClientSettings({ markdownView: on });
}

export function readVoiceInputDeviceId(): string {
  return (readClientSettings().voiceInputDeviceId || "").trim();
}

function clampFont(px: unknown): number {
  const n = parseInt(px as string, 10);
  if (!Number.isFinite(n)) return FONT_DEFAULT;
  return clamp(n, FONT_MIN, FONT_MAX);
}

function normalizeRunningEffect(effect: string | undefined): string {
  return effect !== undefined && RUNNING_EFFECTS.includes(effect)
    ? effect
    : RUNNING_EFFECT_DEFAULT;
}

function normalizePaneSwitcherLayout(layout: string | undefined): string {
  return layout !== undefined && PANE_SWITCHER_LAYOUTS.includes(layout)
    ? layout
    : PANE_SWITCHER_LAYOUT_DEFAULT;
}

function normalizeOfficeScale(scale: unknown): number | "auto" {
  if (scale === "auto" || scale === undefined || scale === null || scale === "") return "auto";
  const n = parseInt(scale as string, 10);
  if (!Number.isFinite(n)) return "auto";
  return clamp(n, OFFICE_SCALE_MIN, OFFICE_SCALE_MAX);
}

// Refs the SettingsDialog attaches to its form elements, so the imperative
// helpers can touch the live DOM exactly like settings.js did via $().
export interface SettingsRefs {
  fontRange: HTMLInputElement | null;
  fontVal: HTMLElement | null;
  runningEffect: HTMLSelectElement | null;
  paneSwitcherLayout: HTMLSelectElement | null;
  officeScaleRange: HTMLInputElement | null;
  officeScaleVal: HTMLElement | null;
  voiceDevice: HTMLSelectElement | null;
  voiceDeviceStatus: HTMLElement | null;
  sttModel: HTMLInputElement | null;
  sttEndpoint: HTMLInputElement | null;
  sttKey: HTMLInputElement | null;
  sttNote: HTMLElement | null;
  sttStatus: HTMLElement | null;
  sttSave: HTMLButtonElement | null;
  frontendBuild: HTMLElement | null;
  buildTime: HTMLElement | null;
  assetHash: HTMLElement | null;
}

export interface UseSettingsResult {
  /** True when the overlay is shown; SettingsDialog renders hidden={!visible}. */
  visible: boolean;
  /** Apply localStorage settings before first paint (App calls synchronously). */
  loadClientSettings: () => void;
  /** Show overlay + reload STT and version (callbacks.openSettings). */
  openSettings: () => void;
  /** Hide overlay (callbacks.closeSettings; close-btn / backdrop / Escape). */
  closeSettings: () => void;
  /** Mutable refs bag SettingsDialog populates for its form elements. */
  refs: MutableRefObject<SettingsRefs>;
  // Event handlers SettingsDialog wires onto its inputs/buttons (wireSettings):
  onFontInput: (value: string) => void;
  onFontDec: () => void;
  onFontInc: () => void;
  onRunningEffectChange: (value: string) => void;
  onPaneSwitcherLayoutChange: (value: string) => void;
  onOfficeScaleInput: (value: string) => void;
  onOfficeScaleAuto: () => void;
  onVoiceDeviceChange: (value: string) => void;
  onRefreshVoiceDevices: () => void;
  onSaveSTT: () => void;
  voiceDevices: MediaDeviceInfo[];
  selectedVoiceDeviceId: string;
  voiceDeviceStatus: string;
  /** Re-sync slider/readout/select from current localStorage (mount/open). */
  syncFormFromSettings: () => void;
}

export function useSettings(): UseSettingsResult {
  const [visible, setVisible] = useState(false);
  const [voiceDevices, setVoiceDevices] = useState<MediaDeviceInfo[]>([]);
  const [selectedVoiceDeviceId, setSelectedVoiceDeviceId] = useState(() =>
    readVoiceInputDeviceId(),
  );
  const [voiceDeviceStatus, setVoiceDeviceStatus] = useState("");
  const refs = useRef<SettingsRefs>({
    fontRange: null,
    fontVal: null,
    runningEffect: null,
    paneSwitcherLayout: null,
    officeScaleRange: null,
    officeScaleVal: null,
    voiceDevice: null,
    voiceDeviceStatus: null,
    sttModel: null,
    sttEndpoint: null,
    sttKey: null,
    sttNote: null,
    sttStatus: null,
    sttSave: null,
    frontendBuild: null,
    buildTime: null,
    assetHash: null,
  });
  const sttStatusTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // applyPaneFont sets the live --pane-font variable, syncs the slider/readout,
  // and persists the choice. The pane <pre> reads --pane-font, so this takes
  // effect immediately with no reload.
  const applyPaneFont = useCallback((px: unknown) => {
    const v = clampFont(px);
    document.documentElement.style.setProperty("--pane-font", v + "px");
    if (refs.current.fontRange) refs.current.fontRange.value = String(v);
    if (refs.current.fontVal) refs.current.fontVal.textContent = v + "px";
    saveClientSettings({ paneFont: v });
  }, []);

  const applyRunningEffect = useCallback((effect: string | undefined) => {
    const e = normalizeRunningEffect(effect);
    document.documentElement.dataset.runningEffect = e;
    if (refs.current.runningEffect) refs.current.runningEffect.value = e;
    saveClientSettings({ runningEffect: e });
  }, []);

  const applyPaneSwitcherLayout = useCallback((layout: string | undefined) => {
    const v = normalizePaneSwitcherLayout(layout);
    document.documentElement.dataset.paneSwitcherLayout = v;
    if (refs.current.paneSwitcherLayout) refs.current.paneSwitcherLayout.value = v;
    saveClientSettings({ paneSwitcherLayout: v });
  }, []);

  const applyOfficeScale = useCallback((scale: unknown) => {
    const v = normalizeOfficeScale(scale);
    if (v === "auto") {
      document.documentElement.dataset.officeScale = "auto";
      document.documentElement.style.removeProperty("--office-scale");
      if (refs.current.officeScaleRange) refs.current.officeScaleRange.value = String(OFFICE_SCALE_DEFAULT);
      if (refs.current.officeScaleVal) refs.current.officeScaleVal.textContent = "Auto";
      saveClientSettings({ officeScale: "auto" });
      return;
    }
    document.documentElement.dataset.officeScale = "custom";
    document.documentElement.style.setProperty("--office-scale", String(v / 100));
    if (refs.current.officeScaleRange) refs.current.officeScaleRange.value = String(v);
    if (refs.current.officeScaleVal) refs.current.officeScaleVal.textContent = v + "%";
    saveClientSettings({ officeScale: v });
  }, []);

  const applyVoiceDevice = useCallback((deviceId: string | undefined) => {
    const id = (deviceId || "").trim();
    setSelectedVoiceDeviceId(id);
    if (refs.current.voiceDevice) refs.current.voiceDevice.value = id;
    saveClientSettings({ voiceInputDeviceId: id });
  }, []);

  const loadVoiceDevices = useCallback(async (requestPermission = false) => {
    if (
      !navigator.mediaDevices ||
      typeof navigator.mediaDevices.enumerateDevices !== "function"
    ) {
      setVoiceDevices([]);
      setVoiceDeviceStatus("microphone device selection is not supported");
      return;
    }
    let probe: MediaStream | null = null;
    try {
      if (
        requestPermission &&
        typeof navigator.mediaDevices.getUserMedia === "function"
      ) {
        probe = await navigator.mediaDevices.getUserMedia({ audio: true });
      }
      const devices = await navigator.mediaDevices.enumerateDevices();
      const audioInputs = devices.filter((device) => device.kind === "audioinput");
      setVoiceDevices(audioInputs);
      setVoiceDeviceStatus(
        audioInputs.length === 0
          ? "no microphones found"
          : requestPermission
            ? "microphones refreshed"
            : "",
      );
    } catch (e) {
      setVoiceDeviceStatus(
        e instanceof Error && e.name === "NotAllowedError"
          ? "microphone permission denied"
          : "failed to load microphones",
      );
    } finally {
      if (probe) probe.getTracks().forEach((track) => track.stop());
    }
  }, []);

  // loadClientSettings applies the browser-local settings at startup. It runs
  // synchronously before first paint, so saved visual choices take effect before
  // the first snapshot render.
  const loadClientSettings = useCallback(() => {
    const saved = readClientSettings();
    applyPaneFont(saved.paneFont);
    applyRunningEffect(saved.runningEffect);
    applyPaneSwitcherLayout(saved.paneSwitcherLayout);
    applyOfficeScale(saved.officeScale);
    setSelectedVoiceDeviceId((saved.voiceInputDeviceId || "").trim());
  }, [applyPaneFont, applyRunningEffect, applyPaneSwitcherLayout, applyOfficeScale]);

  const currentPaneFont = useCallback((): number => {
    return clampFont(
      parseFloat(
        getComputedStyle(document.documentElement).getPropertyValue("--pane-font"),
      ),
    );
  }, []);

  const setSTTStatus = useCallback((msg: string, kind: string) => {
    const el = refs.current.sttStatus;
    if (el) {
      el.textContent = msg;
      el.className = "settings-status" + (kind ? " " + kind : "");
    }
    if (sttStatusTimer.current) {
      clearTimeout(sttStatusTimer.current);
      sttStatusTimer.current = null;
    }
    if (msg && kind === "ok") {
      sttStatusTimer.current = setTimeout(() => setSTTStatus("", ""), 4000);
    }
  }, []);

  const applySTTNote = useCallback((configured: boolean) => {
    if (refs.current.sttNote) {
      refs.current.sttNote.textContent = configured
        ? "An API key is set on the server — leave the key blank to keep it."
        : "No API key set yet — enter one to enable voice transcription.";
    }
  }, []);

  // loadSTTSettings pulls the server-side STT config into the form. The API key
  // is never returned by the server, so that field always starts blank.
  const loadSTTSettings = useCallback(async () => {
    if (refs.current.sttModel) refs.current.sttModel.value = "";
    if (refs.current.sttEndpoint) refs.current.sttEndpoint.value = "";
    if (refs.current.sttKey) refs.current.sttKey.value = "";
    setSTTStatus("loading…", "");
    try {
      const { res, data } = await loadSTTConfig();
      if (!res.ok) {
        const errData = data as unknown as { error?: string };
        throw new Error(errData.error || "HTTP " + res.status);
      }
      if (refs.current.sttModel) refs.current.sttModel.value = data.model || "";
      if (refs.current.sttEndpoint) refs.current.sttEndpoint.value = data.endpoint || "";
      applySTTNote(!!data.configured);
      setSTTStatus("", "");
    } catch (e) {
      setSTTStatus((e as Error).message || "failed to load", "err");
    }
  }, [setSTTStatus, applySTTNote]);

  const loadVersionInfo = useCallback(async () => {
    const frontendEl = refs.current.frontendBuild;
    const buildEl = refs.current.buildTime;
    const hashEl = refs.current.assetHash;
    if (frontendEl) frontendEl.textContent = FRONTEND_BUILD || "unavailable";
    if (buildEl) buildEl.textContent = "loading…";
    if (hashEl) hashEl.textContent = "loading…";
    try {
      const { res, data } = await loadVersion();
      if (!res.ok) {
        const errData = data as unknown as { error?: string };
        throw new Error(errData.error || "HTTP " + res.status);
      }
      if (buildEl) buildEl.textContent = data.build_time || "unavailable";
      if (hashEl) hashEl.textContent = data.asset_hash || "unavailable";
    } catch (e) {
      if (buildEl) buildEl.textContent = "unavailable";
      if (hashEl) hashEl.textContent = "unavailable";
    }
  }, []);

  const saveSTTSettings = useCallback(async () => {
    const btn = refs.current.sttSave;
    if (btn) btn.disabled = true;
    setSTTStatus("saving…", "");
    try {
      const { res, data } = await saveSTTConfig({
        model: refs.current.sttModel ? refs.current.sttModel.value : "",
        endpoint: refs.current.sttEndpoint ? refs.current.sttEndpoint.value : "",
        api_key: refs.current.sttKey ? refs.current.sttKey.value : "",
      });
      if (!res.ok) {
        const errData = data as unknown as { error?: string };
        throw new Error(errData.error || "HTTP " + res.status);
      }
      if (refs.current.sttModel) refs.current.sttModel.value = data.model || "";
      if (refs.current.sttEndpoint) refs.current.sttEndpoint.value = data.endpoint || "";
      if (refs.current.sttKey) refs.current.sttKey.value = "";
      applySTTNote(!!data.configured);
      setSTTStatus("saved ✓", "ok");
    } catch (e) {
      setSTTStatus((e as Error).message || "save failed", "err");
    } finally {
      if (btn) btn.disabled = false;
    }
  }, [setSTTStatus, applySTTNote]);

  const openSettings = useCallback(() => {
    setVisible(true);
    setSelectedVoiceDeviceId(readVoiceInputDeviceId());
    void loadVoiceDevices(false);
    void loadSTTSettings();
    void loadVersionInfo();
  }, [loadVoiceDevices, loadSTTSettings, loadVersionInfo]);

  const closeSettings = useCallback(() => {
    setVisible(false);
  }, []);

  // wireSettings event handlers (1:1 with settings.js listener bodies):
  const onFontInput = useCallback((value: string) => applyPaneFont(value), [applyPaneFont]);
  const onFontDec = useCallback(
    () => applyPaneFont(currentPaneFont() - 1),
    [applyPaneFont, currentPaneFont],
  );
  const onFontInc = useCallback(
    () => applyPaneFont(currentPaneFont() + 1),
    [applyPaneFont, currentPaneFont],
  );
  const onRunningEffectChange = useCallback(
    (value: string) => applyRunningEffect(value),
    [applyRunningEffect],
  );
  const onPaneSwitcherLayoutChange = useCallback(
    (value: string) => applyPaneSwitcherLayout(value),
    [applyPaneSwitcherLayout],
  );
  const onOfficeScaleInput = useCallback(
    (value: string) => applyOfficeScale(value),
    [applyOfficeScale],
  );
  const onOfficeScaleAuto = useCallback(() => applyOfficeScale("auto"), [applyOfficeScale]);
  const onVoiceDeviceChange = useCallback(
    (value: string) => applyVoiceDevice(value),
    [applyVoiceDevice],
  );
  const onRefreshVoiceDevices = useCallback(
    () => void loadVoiceDevices(true),
    [loadVoiceDevices],
  );
  const onSaveSTT = useCallback(() => void saveSTTSettings(), [saveSTTSettings]);

  // syncFormFromSettings re-applies the persisted slider/readout/select values
  // onto the form elements once they exist (refs attach after first render,
  // whereas loadClientSettings can run before they do).
  const syncFormFromSettings = useCallback(() => {
    const saved = readClientSettings();
    const px = clampFont(saved.paneFont);
    if (refs.current.fontRange) refs.current.fontRange.value = String(px);
    if (refs.current.fontVal) refs.current.fontVal.textContent = px + "px";
    if (refs.current.runningEffect) {
      refs.current.runningEffect.value = normalizeRunningEffect(saved.runningEffect);
    }
    if (refs.current.paneSwitcherLayout) {
      refs.current.paneSwitcherLayout.value = normalizePaneSwitcherLayout(
        saved.paneSwitcherLayout,
      );
    }
    const officeScale = normalizeOfficeScale(saved.officeScale);
    if (refs.current.officeScaleRange) {
      refs.current.officeScaleRange.value =
        officeScale === "auto" ? String(OFFICE_SCALE_DEFAULT) : String(officeScale);
    }
    if (refs.current.officeScaleVal) {
      refs.current.officeScaleVal.textContent =
        officeScale === "auto" ? "Auto" : officeScale + "%";
    }
    const voiceID = (saved.voiceInputDeviceId || "").trim();
    setSelectedVoiceDeviceId(voiceID);
    if (refs.current.voiceDevice) refs.current.voiceDevice.value = voiceID;
  }, []);

  return {
    visible,
    loadClientSettings,
    openSettings,
    closeSettings,
    refs,
    onFontInput,
    onFontDec,
    onFontInc,
    onRunningEffectChange,
    onPaneSwitcherLayoutChange,
    onOfficeScaleInput,
    onOfficeScaleAuto,
    onVoiceDeviceChange,
    onRefreshVoiceDevices,
    onSaveSTT,
    voiceDevices,
    selectedVoiceDeviceId,
    voiceDeviceStatus,
    syncFormFromSettings,
  };
}
