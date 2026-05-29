// ModeIndicator — React port of the #mode-indicator block from app.js
// (renderMode / syncIndicator). Spec §6 item 83 (the syncIndicator visibility
// rule) and items 32/28 (direct-mode signalling) live in App's renderMode;
// this component is the presentational shell that renders the current
// mode-text + input-error strings and collapses itself when both are empty.
//
// PARITY MODEL:
//   app.js owned two separate <span>s inside #mode-indicator: #mode-text and
//   #input-error. renderMode() set #mode-text to "" (direct/selected) or
//   "Select a pane to enable input" (no selection); showInputError /
//   setInputStatus set #input-error. syncIndicator() then toggled the row's
//   display between "flex" (when EITHER span had text) and "none" (both empty).
//
//   In the React port App holds those two strings (mode text + input error) in
//   refs/state and bumps; this component reads them via props and computes its
//   own visibility exactly like syncIndicator. The `.direct` class on
//   #mode-indicator is also applied here (renderMode toggled it on the element)
//   driven by the `direct` prop App computes from
//   state.selected && !state.selectionMode && activeElement === #direct-input.
//
// The original toggled `style.display` imperatively ("flex"/"none"); we keep
// byte-identical behavior by setting the same inline style so app.css's other
// rules (which never set display on this element) are unaffected.

interface ModeIndicatorProps {
  /** Text for #mode-text. "" when a pane is selected (direct/draft), else the
   *  "Select a pane to enable input" hint. App computes this in renderMode. */
  modeText: string;
  /** Text for #input-error (transient error or persistent status note). */
  inputError: string;
  /** Whether direct mode is active — toggles the `.direct` class, exactly as
   *  renderMode did (`ind.classList.toggle("direct", direct)`). */
  direct: boolean;
}

export default function ModeIndicator({ modeText, inputError, direct }: ModeIndicatorProps) {
  // syncIndicator: the row is shown when EITHER span carries text, hidden when
  // both are empty. Mirror the original textContent !== "" comparisons exactly.
  const has = modeText !== "" || inputError !== "";
  return (
    <div
      className={direct ? "mode-indicator direct" : "mode-indicator"}
      id="mode-indicator"
      style={{ display: has ? "flex" : "none" }}
    >
      <span id="mode-text">{modeText}</span>
      <span className="input-error" id="input-error">{inputError}</span>
    </div>
  );
}
