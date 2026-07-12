// SettingsDialog — faithful port of the #settings-overlay markup (index.html)
// plus the wireSettings event wiring from static/js/settings.js. The overlay is
// always mounted; useSettings drives `visible` (rendered as the `hidden`
// attribute, matching the original which toggled $("settings-overlay").hidden).
//
// Behavior preserved (spec §6 items 69–73):
//  - open on gear (App wires GearButton → callbacks.openSettings)
//  - close on close-button click, on backdrop mousedown when
//    e.target === overlay, and on Escape only while the overlay is not hidden
//  - STT + version reloaded every open (useSettings.openSettings)
//  - font slider/±1 buttons → applyPaneFont (clamp 9–22, --pane-font on <html>)
//  - running-effect select → applyRunningEffect (data-running-effect on <html>,
//    preview animates 4 agent icons via CSS)
//  - STT save button → saveSTTSettings (disabled during save)
//  - loaded frontend build / build-time / asset-hash filled by loadVersionInfo
//
// The form inputs are UNCONTROLLED (refs into the always-mounted DOM), exactly
// like the original $()-driven imperative form: useSettings blanks then
// async-fills #stt-model/#stt-endpoint/#stt-key and reads the font readout back
// from getComputedStyle.
//
// #qb-editor is the QuickEditor mount point from the quick unit; it is passed in
// as the `quickEditor` node so this dialog owns the slot exactly where
// index.html placed it (inside the settings card, between the Quick-buttons
// note and the Server section).

import { useEffect, useLayoutEffect } from "react";
import type { ReactNode, MouseEvent as ReactMouseEvent } from "react";
import type { UseSettingsResult } from "../hooks/useSettings";
import { usePushNotifications } from "../hooks/usePushNotifications";

export interface SettingsDialogProps {
  /** The useSettings handle App created (visibility + refs + handlers). */
  settings: UseSettingsResult;
  /** QuickEditor element rendered into the #qb-editor slot (from useQuick unit). */
  quickEditor?: ReactNode;
}

export default function SettingsDialog({ settings, quickEditor }: SettingsDialogProps) {
  const push = usePushNotifications();
  const {
    visible,
    closeSettings,
    refs,
    onFontInput,
    onFontDec,
    onFontInc,
    onRunningEffectChange,
    onPaneSwitcherLayoutChange,
    onVoiceDeviceChange,
    onRefreshVoiceDevices,
    onSaveSTT,
    voiceDevices,
    selectedVoiceDeviceId,
    voiceDeviceStatus,
    syncFormFromSettings,
  } = settings;
  const selectedVoiceDeviceMissing =
    selectedVoiceDeviceId !== "" &&
    !voiceDevices.some((device) => device.deviceId === selectedVoiceDeviceId);

  // Re-apply persisted slider/readout/select values once the form refs exist
  // (loadClientSettings may have run before this dialog mounted).
  useLayoutEffect(() => {
    syncFormFromSettings();
  }, [syncFormFromSettings]);

  // Escape closes only while not hidden — registered on document like the
  // original wireSettings keydown listener.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape" && visible) closeSettings();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [visible, closeSettings]);

  // A mousedown on the dim backdrop (but not the card) closes the panel.
  const onBackdropMouseDown = (e: ReactMouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget) closeSettings();
  };

  return (
    <div
      className="settings-overlay"
      id="settings-overlay"
      hidden={!visible}
      onMouseDown={onBackdropMouseDown}
    >
      <div className="settings-card" role="dialog" aria-modal="true" aria-label="Settings">
        <div className="settings-head">
          <span>Settings</span>
          <button
            className="settings-close"
            id="settings-close"
            type="button"
            title="close"
            aria-label="close settings"
            onClick={closeSettings}
          >
            <svg
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.5"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M18 6 6 18" />
              <path d="m6 6 12 12" />
            </svg>
          </button>
        </div>
        <div className="settings-body">
          <div className="settings-section">Client · this browser</div>
          <div className="settings-field">
            <span>Notifications</span>
            <div className="notification-row">
              <span
                className={
                  push.state === "subscribed"
                    ? "settings-status ok"
                    : push.state === "error" ||
                        push.state === "blocked" ||
                        push.state === "unsupported" ||
                        push.state === "not-configured"
                      ? "settings-status err"
                      : "settings-status"
                }
                id="notification-status"
              >
                {push.message}
              </span>
              {push.state === "subscribed" ? (
                <button
                  id="notification-disable"
                  type="button"
                  onClick={() => void push.disable()}
                >
                  Disable
                </button>
              ) : (
                <button
                  id="notification-enable"
                  type="button"
                  disabled={
                    push.state === "busy" ||
                    push.state === "checking" ||
                    push.state === "unsupported" ||
                    push.state === "blocked" ||
                    push.state === "not-configured"
                  }
                  onClick={() => void push.enable()}
                >
                  Enable Notifications
                </button>
              )}
            </div>
          </div>
          <div className="settings-field">
            <span>Panel font size</span>
            <div className="font-row">
              <button id="font-dec" type="button" aria-label="smaller font" onClick={onFontDec}>
                A−
              </button>
              <input
                type="range"
                id="font-range"
                min="9"
                max="22"
                step="1"
                aria-label="panel font size"
                ref={(el) => {
                  refs.current.fontRange = el;
                }}
                onInput={(e) => onFontInput(e.currentTarget.value)}
              />
              <button id="font-inc" type="button" aria-label="larger font" onClick={onFontInc}>
                A+
              </button>
              <span
                className="font-val"
                id="font-val"
                ref={(el) => {
                  refs.current.fontVal = el;
                }}
              >
                13px
              </span>
            </div>
          </div>
          <label className="settings-field" htmlFor="running-effect">
            <span>Running effect</span>
            <select
              id="running-effect"
              aria-label="running effect"
              ref={(el) => {
                refs.current.runningEffect = el;
              }}
              onChange={(e) => onRunningEffectChange(e.currentTarget.value)}
            >
              <option value="shine">Shine</option>
              <option value="pulse">Pulse</option>
              <option value="rainbow">Rainbow</option>
              <option value="scan">Scan</option>
              <option value="none">None</option>
            </select>
            <div
              className="effect-preview"
              id="running-effect-preview"
              aria-label="running effect preview"
            >
              <span className="agent-icon runtime-claude running">cc</span>
              <span className="agent-icon runtime-codex running">cx</span>
              <span className="agent-icon runtime-gemini running">g</span>
            </div>
          </label>
          <label className="settings-field" htmlFor="pane-switcher-layout">
            <span>Pane switcher layout</span>
            <select
              id="pane-switcher-layout"
              aria-label="pane switcher layout"
              ref={(el) => {
                refs.current.paneSwitcherLayout = el;
              }}
              onChange={(e) => onPaneSwitcherLayoutChange(e.currentTarget.value)}
            >
              <option value="bottom">Panel list (chips)</option>
              <option value="office">Office layout</option>
            </select>
          </label>
          <label className="settings-field" htmlFor="voice-device">
            <span>Microphone</span>
            <select
              id="voice-device"
              aria-label="microphone"
              value={selectedVoiceDeviceId}
              ref={(el) => {
                refs.current.voiceDevice = el;
              }}
              onChange={(e) => onVoiceDeviceChange(e.currentTarget.value)}
            >
              <option value="">System default</option>
              {selectedVoiceDeviceMissing && (
                <option value={selectedVoiceDeviceId}>Saved microphone unavailable</option>
              )}
              {voiceDevices.map((device, index) => (
                <option key={device.deviceId || `audioinput-${index}`} value={device.deviceId}>
                  {device.label || `Microphone ${index + 1}`}
                </option>
              ))}
            </select>
          </label>
          <div className="settings-actions voice-device-actions">
            <span
              className="settings-status"
              id="voice-device-status"
              ref={(el) => {
                refs.current.voiceDeviceStatus = el;
              }}
            >
              {voiceDeviceStatus}
            </span>
            <button id="voice-device-refresh" type="button" onClick={onRefreshVoiceDevices}>
              Refresh microphones
            </button>
          </div>

          <div className="settings-section">Quick buttons</div>
          <div className="settings-note">
            Phone-only — tap the bolt at a pane's bottom-right corner. Each button types its text
            and presses Enter. The group matching the pane's runtime is shown alongside Common.
          </div>
          <div id="qb-editor">{quickEditor}</div>

          <div className="settings-section">Server · statusd host</div>
          <label className="settings-field" htmlFor="stt-model">
            <span>Voice transcription — model</span>
            <input
              type="text"
              id="stt-model"
              spellCheck={false}
              autoCapitalize="off"
              autoComplete="off"
              placeholder="gpt-4o-transcribe"
              ref={(el) => {
                refs.current.sttModel = el;
              }}
            />
          </label>
          <label className="settings-field" htmlFor="stt-endpoint">
            <span>Endpoint</span>
            <input
              type="text"
              id="stt-endpoint"
              spellCheck={false}
              autoCapitalize="off"
              autoComplete="off"
              placeholder="https://api.openai.com/v1/audio/transcriptions"
              ref={(el) => {
                refs.current.sttEndpoint = el;
              }}
            />
          </label>
          <label className="settings-field" htmlFor="stt-key">
            <span>API key</span>
            <input
              type="password"
              id="stt-key"
              spellCheck={false}
              autoCapitalize="off"
              autoComplete="off"
              ref={(el) => {
                refs.current.sttKey = el;
              }}
            />
          </label>
          <div
            className="settings-note"
            id="stt-note"
            ref={(el) => {
              refs.current.sttNote = el;
            }}
          >
            Leave the key blank to keep the current one.
          </div>
          <div className="settings-actions">
            <span
              className="settings-status"
              id="stt-status"
              ref={(el) => {
                refs.current.sttStatus = el;
              }}
            ></span>
            <button
              id="stt-save"
              type="button"
              ref={(el) => {
                refs.current.sttSave = el;
              }}
              onClick={onSaveSTT}
            >
              Save server config
            </button>
          </div>

          <div className="settings-version">
            <span>Loaded Frontend Build</span>
            <span
              id="frontend-build"
              ref={(el) => {
                refs.current.frontendBuild = el;
              }}
            >
              unavailable
            </span>
          </div>
          <div className="settings-version">
            <span>Build Time</span>
            <span
              id="build-time"
              ref={(el) => {
                refs.current.buildTime = el;
              }}
            >
              unavailable
            </span>
          </div>
          <div className="settings-version">
            <span>Asset Hash</span>
            <span
              id="asset-hash"
              ref={(el) => {
                refs.current.assetHash = el;
              }}
            >
              unavailable
            </span>
          </div>
        </div>
      </div>
    </div>
  );
}
