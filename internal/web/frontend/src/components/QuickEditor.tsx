// QuickEditor — the settings-panel editor for the quick-button config.
// 1:1 behavioral port of `renderQuickEditor` / `quickRow` in static/js/quick.js
// and the `<div id="qb-editor">` slot in static/index.html.
//
// One block per group (common / claude / codex / shell), each holding its
// editable rows and an "+ Add button". Each row binds its inputs to a config
// entry BY REFERENCE (the original mutated `item.label`/`item.text` directly,
// then `saveQuickConfig()` + `renderQuickMenu()` per keystroke). We reproduce
// that exactly: the keystroke handler mutates the entry in place, persists, and
// re-renders the live FAB menu (`bumpMenu`) — NO controlled-input clone.
//
// Because each input is bound to a mutable entry and we re-render the editor
// only on add/delete (`editorVersion`), the <input> value comes from the entry
// (`item.label`/`item.text`) and is updated in the entry on every keystroke —
// this is an "uncontrolled-by-mutation" pattern matching the original DOM.

import { useRef } from "react";
import type { QuickEntry, UseQuickReturn } from "../hooks/useQuick";
import { QB_GROUPS, QB_LABEL } from "../hooks/useQuick";

/**
 * Props. App passes the live `useQuick(...)` return value; QuickEditor reads
 * `quickConfig` (mutated by reference), `saveQuickConfig`, `bumpMenu`,
 * `bumpEditor`, `addQuickRow`, `deleteQuickRow`, and `editorVersion`.
 */
export interface QuickEditorProps {
  quick: UseQuickReturn;
}

export function QuickEditor({ quick }: QuickEditorProps) {
  const {
    quickConfig,
    saveQuickConfig,
    bumpMenu,
    editorVersion,
    addQuickRow,
    deleteQuickRow,
  } = quick;

  // editorVersion changes whenever the original would have called
  // renderQuickEditor (add/delete). It is folded into each row's key below so a
  // structural change remounts all rows with fresh defaultValues.

  return (
    <div id="qb-editor">
      {QB_GROUPS.map((g) => (
        <div className="qb-group" key={g}>
          <div className="qb-group-head">{QB_LABEL[g]}</div>
          <div className="qb-rows">
            {quickConfig[g].map((item, i) => (
              <QuickRow
                // Key includes editorVersion so EVERY add/delete remounts all
                // rows with fresh `defaultValue`s seeded from the current
                // entries — exactly like the original `renderQuickEditor`, which
                // did `root.textContent = ""` and rebuilt every input from
                // `item.label`/`item.text`. With a plain index key the
                // uncontrolled inputs would reuse a deleted row's DOM (and its
                // stale typed value); the version-namespaced key avoids that.
                key={editorVersion + "-" + g + "-" + i}
                item={item}
                saveQuickConfig={saveQuickConfig}
                bumpMenu={bumpMenu}
                onDelete={() => deleteQuickRow(g, item)}
              />
            ))}
          </div>
          <button
            className="qb-add"
            type="button"
            onClick={() => addQuickRow(g)}
          >
            + Add button
          </button>
        </div>
      ))}
    </div>
  );
}

interface QuickRowProps {
  item: QuickEntry;
  saveQuickConfig: () => void;
  bumpMenu: () => void;
  onDelete: () => void;
}

// quickRow builds one editable button row, bound by object reference to its
// config entry so edits land straight in quickConfig. The inputs are
// uncontrolled (defaultValue = the entry's value at mount); each keystroke
// mutates the entry in place, persists, and re-renders the live menu — matching
// the original `label.addEventListener("input", …)` handlers exactly.
function QuickRow({ item, saveQuickConfig, bumpMenu, onDelete }: QuickRowProps) {
  // Hold the initial values so the uncontrolled inputs seed from the entry once
  // (the original set `label.value = item.label` after creating the element).
  const initial = useRef({ label: item.label, text: item.text });

  return (
    <div className="qb-row">
      <input
        className="qb-label"
        type="text"
        placeholder="label"
        spellCheck={false}
        autoCapitalize="off"
        autoComplete="off"
        defaultValue={initial.current.label}
        onInput={(e) => {
          item.label = (e.target as HTMLInputElement).value;
          saveQuickConfig();
          bumpMenu();
        }}
      />
      <input
        className="qb-text"
        type="text"
        placeholder="text sent to the pane (Enter is added)"
        spellCheck={false}
        autoCapitalize="off"
        autoComplete="off"
        defaultValue={initial.current.text}
        onInput={(e) => {
          item.text = (e.target as HTMLInputElement).value;
          saveQuickConfig();
          bumpMenu();
        }}
      />
      <button
        className="qb-del"
        type="button"
        title="remove button"
        onClick={onDelete}
      >
        ✕
      </button>
    </div>
  );
}
