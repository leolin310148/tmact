// DirectInput — React port of the invisible direct-mode passthrough overlay
// (#direct-input) from app.js's wireInput direct-mode block.
//
// Spec §6 items 33 (translateKey), 34 (sendDirect Ctrl folding), 35 (soft-kbd
// input relay), 36 (compositionend), 37 (direct paste image/text).
//
// PLACEMENT: this textarea lives inside #content-wrap (next to pre#content in
// index.html), NOT inside #input-bar. App composes it there; this file only
// renders the element + wires its listeners. App owns the ref so selectPane can
// focus it, renderMode reads `document.activeElement === #direct-input`, and
// toggleSelectionMode / the empty-Enter path can blur/focus it.
//
// BUBBLE-PHASE keydown (ARCHITECTURE §9 step 5): the original attached
//   direct.addEventListener("keydown", …)   // bubble (no capture flag)
// deliberately, so the capture-phase hotkey + voice-suppression listeners
// (Option+key, Cmd-K/Ctrl-L, Option+V) always run FIRST and can preventDefault/
// stopPropagation before this relay sees the event. React's onKeyDown is the
// bubble phase, so it is registered correctly by default — do NOT switch it to a
// capture listener.
//
// PARITY MODEL:
//   #direct-input was a plain DOM node whose value the original mutated by
//   reference (set "" after relaying soft-keyboard input / compositionend), and
//   whose focus/blur App controlled. So App owns `directRef`; React must never
//   control this textarea's value. All four direct-mode behaviors are App-owned
//   handlers (they need sendDirect/ctrlArmed/state.selectionMode, all of which
//   live in App per ARCHITECTURE §4) and are forwarded here as props:
//     onDirectKeyDown   ← keydown: selectionMode blur; IME guard
//                          (isComposing || keyCode===229); translateKey →
//                          preventDefault + sendDirect.
//     onDirectComposition ← compositionend: sendDirect({t:"text",s:e.data});
//                            direct.value = "".
//     onDirectPaste     ← paste: image → upload + sendDirect(path+" ");
//                          else text relay. preventDefault inside.
//     onDirectInput     ← input: soft-keyboard relay (read value, clear, send).

import type {
  ClipboardEvent,
  CompositionEvent,
  FormEvent,
  KeyboardEvent,
  Ref,
} from "react";

interface DirectInputProps {
  /** Ref to the #direct-input textarea; owned by App (focus/blur + value clear). */
  directRef: Ref<HTMLTextAreaElement>;
  /** Bubble-phase keydown relay (translateKey/sendDirect/IME/selectionMode). */
  onDirectKeyDown: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
  /** compositionend → sendDirect({t:"text",s:e.data}) then clear value. */
  onDirectComposition: (e: CompositionEvent<HTMLTextAreaElement>) => void;
  /** paste → image upload (sendDirect path+" ") or text relay. */
  onDirectPaste: (e: ClipboardEvent<HTMLTextAreaElement>) => void;
  /** input → soft-keyboard relay (read value, clear, sendDirect text). */
  onDirectInput: (e: FormEvent<HTMLTextAreaElement>) => void;
}

export default function DirectInput({
  directRef,
  onDirectKeyDown,
  onDirectComposition,
  onDirectPaste,
  onDirectInput,
}: DirectInputProps) {
  // Attributes verbatim from index.html line 20:
  //   <textarea id="direct-input" spellcheck="false" autocapitalize="off"
  //             autocomplete="off"></textarea>
  return (
    <textarea
      id="direct-input"
      ref={directRef}
      spellCheck={false}
      autoCapitalize="off"
      autoComplete="off"
      onKeyDown={onDirectKeyDown}
      onCompositionEnd={onDirectComposition}
      onPaste={onDirectPaste}
      onInput={onDirectInput}
    />
  );
}
