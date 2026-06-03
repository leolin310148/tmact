// Draft — React port of the draft-mode textarea (#draft-wrap / #draft /
// #draft-clear) from app.js's wireInput + draft-mode helpers.
//
// Spec §6 items 27 (autoGrowDraft), 28 (Ctrl/Cmd+Enter send, empty-Enter →
// direct mode), 29 (IME guard), 30 (draft-clear pointerdown noBlur), 31 (image
// paste → placeInDraft), 32 (responsive placeholder).
//
// PARITY MODEL:
//   The original kept #draft as a plain DOM node that App, voice, and upload
//   all mutated by reference: App set `.value`/`.disabled` in selectPane,
//   cleared it in clearDraft/sendDraft, sized it in autoGrowDraft; voice and
//   upload mutated its value then called syncDraft. To preserve that EXACT
//   imperative timing, App owns the textarea ref (`draftRef`) and the
//   wrap ref (`draftWrapRef`) and passes them in here — this component does NOT
//   own #draft's value as React state (the original never did) and React must
//   never control the textarea's value.
//
//   App's callbacks own the behavior:
//     - syncDraft()  → toggles #draft-wrap.has-text + autoGrowDraft (App, via
//       refs; needs useLayoutEffect/synchronous scrollHeight per ARCHITECTURE §4).
//     - clearDraft() → wired to #draft-clear click (App).
//     - sendDraft()  → Ctrl/Cmd+Enter (App, via onDraftKeyDown).
//   This component wires the DOM listeners the original attached in wireInput
//   and forwards them to the App-supplied handlers, preserving the original
//   pointerdown-preventDefault on #draft-clear (and ONLY #draft-clear here —
//   send/record are owned by InputBar; see ARCHITECTURE §6).

import type { ClipboardEvent, FocusEvent, FormEvent, KeyboardEvent, Ref } from "react";
import { onPointerDownNoBlur } from "../lib/dom";

interface DraftProps {
  /** Ref to the #draft textarea; owned by App (mutated by selectPane /
   *  clearDraft / sendDraft / syncDraft / voice / upload). */
  draftRef: Ref<HTMLTextAreaElement>;
  /** Ref to the #draft-wrap element; App's syncDraft toggles `.has-text` on it. */
  draftWrapRef: Ref<HTMLDivElement>;
  /** wireInput's `draft.addEventListener("input", …)`:
   *  `if (state.selected) state.drafts[selected] = draft.value; syncDraft();` */
  onDraftInput: (e: FormEvent<HTMLTextAreaElement>) => void;
  /** wireInput's `draft.addEventListener("keydown", …)`: Ctrl/Cmd+Enter →
   *  sendDraft; empty-Enter (non-composing, non-selection) → direct mode +
   *  sendDirect({t:"key",k:"Enter"}). preventDefault handled inside the handler. */
  onDraftKeyDown: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
  /** wireInput's focus path is owned by App so mobile viewport compensation can
   *  keep the pane pinned while the soft keyboard animates in. */
  onDraftFocus: (e: FocusEvent<HTMLTextAreaElement>) => void;
  /** wireInput's `draft.addEventListener("paste", …)`: image → upload +
   *  placeInDraft (preventDefault); plain text falls through to the textarea. */
  onDraftPaste: (e: ClipboardEvent<HTMLTextAreaElement>) => void;
  /** #draft-clear click → clearDraft() (App). */
  onClearDraft: () => void;
}

export default function Draft({
  draftRef,
  draftWrapRef,
  onDraftInput,
  onDraftKeyDown,
  onDraftFocus,
  onDraftPaste,
  onClearDraft,
}: DraftProps) {
  return (
    <div className="draft-wrap" id="draft-wrap" ref={draftWrapRef}>
      {/* placeholder is the desktop default from index.html; App's renderMode
          overwrites it imperatively (mobile vs desktop) via the draftRef, so
          this static value matches the original's initial markup exactly. */}
      <textarea
        id="draft"
        ref={draftRef}
        rows={1}
        placeholder="Type a prompt — ⌘/Ctrl+Enter to send"
        disabled
        onInput={onDraftInput}
        onKeyDown={onDraftKeyDown}
        onFocus={onDraftFocus}
        onPaste={onDraftPaste}
      />
      <button
        id="draft-clear"
        type="button"
        tabIndex={-1}
        title="clear input"
        aria-label="clear input"
        onPointerDown={onPointerDownNoBlur}
        onClick={onClearDraft}
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" aria-hidden="true">
          <path d="M18 6 6 18" />
          <path d="m6 6 12 12" />
        </svg>
      </button>
    </div>
  );
}
