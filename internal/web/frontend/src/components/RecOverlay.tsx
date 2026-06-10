// RecOverlay — the voice-recording overlay (#rec-overlay), a 1:1 port of the
// static markup in static/index.html (lines 81–94).
//
// PARITY MODEL (ARCHITECTURE.md §3, §7):
//   This component is purely presentational — it renders the SAME DOM (ids,
//   classes, attributes, SVG) the original hand-wrote in index.html, so the
//   verbatim CSS in app.css (.rec-overlay / .rec-card / .rec-dot / state classes
//   transcribing|confirming|hotkey-recording) selects on it unchanged. ALL
//   dynamic behavior — overlay state classes, the `hidden` toggle, label/timer
//   text, the rec-stop pinning + rec-card positioning math — is driven
//   IMPERATIVELY by useVoice (matching how voice.js mutated the hand-written
//   nodes via getElementById). The component never reconciles those; it just
//   provides the stable nodes and the static initial text ("Recording…" / "0:00").
//
//   The original wired #rec-stop / #rec-send / #rec-cancel click handlers in
//   app.js's `wireInput` (NOT in voice.js), and gave NONE of them a `pointerdown`
//   preventDefault (only draft-clear/send-btn/record-btn/clear-pane-btn/
//   selection-btn got that — see ARCHITECTURE.md §6). So these three buttons use
//   plain onClick and intentionally OMIT onPointerDownNoBlur.

export interface RecOverlayProps {
  /** app.js `$("rec-stop").onclick` → `stopRecording` (button recording → straight to transcription). */
  onStop: () => void;
  /** app.js `$("rec-send").onclick` → `finishRecordingConfirm(true)` (confirm flow: send). */
  onSend: () => void;
  /** app.js `$("rec-cancel").onclick` → `cancelRecording`. */
  onCancel: () => void;
}

export function RecOverlay({ onStop, onSend, onCancel }: RecOverlayProps) {
  return (
    // `hidden` is set imperatively by useVoice (show/hideRecOverlay); it starts
    // hidden, matching index.html's `<div class="rec-overlay" id="rec-overlay" hidden>`.
    <div className="rec-overlay" id="rec-overlay" hidden>
      <div className="rec-card">
        <div className="rec-dot"></div>
        <div className="rec-info">
          <div className="rec-label" id="rec-label">
            Recording…
          </div>
          <div className="rec-timer" id="rec-timer">
            0:00
          </div>
        </div>
        <canvas
          className="rec-waveform"
          id="rec-waveform"
          width="220"
          height="46"
          aria-label="recording waveform"
        />
        <button className="rec-send" id="rec-send" type="button" onClick={onSend}>
          Send
        </button>
        <button
          className="rec-cancel"
          id="rec-cancel"
          type="button"
          onClick={onCancel}
        >
          Cancel
        </button>
      </div>
      <button
        className="rec-stop"
        id="rec-stop"
        type="button"
        title="stop & transcribe"
        aria-label="stop and transcribe"
        onClick={onStop}
      >
        <svg viewBox="0 0 24 24" aria-hidden="true">
          <rect x="6" y="6" width="12" height="12" rx="2" fill="currentColor" />
        </svg>
      </button>
    </div>
  );
}
