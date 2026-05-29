// InputBar — React port of the #input-bar shell from index.html + app.js
// wireInput. It composes three regions, in the original DOM order:
//   1. #key-area (the KeyBar unit: #key-bar + #key-toggle) — rendered as a slot
//      so the KeyBar component (separate unit, buildKeyBar/syncKeyBar) owns its
//      own listeners + pointerdown noBlur.
//   2. #mode-indicator (the ModeIndicator unit: #mode-text + #input-error).
//   3. .input-row: #draft-wrap (Draft unit) + #record-btn + #send-btn.
//
// Spec §6 items 27–37 (draft/direct behavior live in the Draft/DirectInput
// units + App) and item 83 (ModeIndicator visibility). This file owns ONLY the
// shell layout + the record/send buttons (the original wired their listeners in
// wireInput).
//
// BUTTON pointerdown audit (ARCHITECTURE §6, app.js wireInput lines 709–712):
//   In wireInput, only #draft-clear, #send-btn, #record-btn, #clear-pane-btn,
//   and #selection-btn get `pointerdown` preventDefault — NOT #upload-btn.
//   Here in the input-row that means #send-btn and #record-btn use
//   onPointerDownNoBlur (#draft-clear is handled inside the Draft unit;
//   #clear-pane-btn/#selection-btn live in ContentWrap, not here).
//
// PARITY MODEL:
//   #record-btn and #send-btn were plain DOM nodes the original mutated by
//   reference: selectPane set `#send-btn.disabled = false`; voice's
//   syncRecordButton toggled `#record-btn.busy`, `.disabled`, and `.title`. To
//   preserve that exact imperative timing, App owns refs to both buttons
//   (recordBtnRef / sendBtnRef) and mutates them in those callbacks; React must
//   not control their disabled state. They start `disabled` exactly as the
//   index.html markup did. Click handlers come from props:
//     onRecord ← `() => startRecording({ confirmOnStop: false })` (useVoice).
//     onSend   ← `sendDraft` (App).
//
//   The .input-bar `.direct` class (renderMode toggled it on #input-bar) is
//   driven by App via the `direct` prop, matching
//     $("input-bar").classList.toggle("direct", direct).

import type { ReactNode, Ref } from "react";
import { onPointerDownNoBlur } from "../lib/dom";

interface InputBarProps {
  /** The KeyBar unit (#key-area / #key-bar / #key-toggle). Rendered first, as
   *  in index.html. KeyBar owns its own listeners + pointerdown handling. */
  keyBar: ReactNode;
  /** The ModeIndicator unit (#mode-indicator). */
  modeIndicator: ReactNode;
  /** The Draft unit (#draft-wrap / #draft / #draft-clear). */
  draft: ReactNode;
  /** renderMode's `$("input-bar").classList.toggle("direct", direct)`. */
  direct: boolean;
  /** Ref to #record-btn (App's syncRecordButton mutates class/disabled/title). */
  recordBtnRef: Ref<HTMLButtonElement>;
  /** Ref to #send-btn (selectPane sets `.disabled`). */
  sendBtnRef: Ref<HTMLButtonElement>;
  /** #record-btn click → startRecording({ confirmOnStop: false }) (useVoice). */
  onRecord: () => void;
  /** #send-btn click → sendDraft() (App). */
  onSend: () => void;
}

export default function InputBar({
  keyBar,
  modeIndicator,
  draft,
  direct,
  recordBtnRef,
  sendBtnRef,
  onRecord,
  onSend,
}: InputBarProps) {
  return (
    <div className={direct ? "input-bar direct" : "input-bar"} id="input-bar">
      {keyBar}
      {modeIndicator}
      <div className="input-row">
        {draft}
        <button
          id="record-btn"
          className="icon-btn"
          type="button"
          ref={recordBtnRef}
          disabled
          title="record voice"
          aria-label="record voice"
          onPointerDown={onPointerDownNoBlur}
          onClick={onRecord}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <rect x="9" y="2" width="6" height="12" rx="3" />
            <path d="M5 10v1a7 7 0 0 0 14 0v-1" />
            <line x1="12" y1="19" x2="12" y2="22" />
          </svg>
        </button>
        <button
          id="send-btn"
          className="icon-btn"
          type="button"
          ref={sendBtnRef}
          disabled
          title="send"
          aria-label="send"
          onPointerDown={onPointerDownNoBlur}
          onClick={onSend}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M22 2 11 13" />
            <path d="M22 2 15 22l-4-9-9-4Z" />
          </svg>
        </button>
      </div>
    </div>
  );
}
