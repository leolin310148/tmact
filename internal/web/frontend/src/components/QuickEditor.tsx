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

import { useId, useLayoutEffect, useRef, useState } from "react";
import type { QuickEntry, UseQuickReturn } from "../hooks/useQuick";
import { QB_GROUPS, QB_LABEL, type QBGroup } from "../hooks/useQuick";

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
  const editorId = useId();
  const pendingFocusId = useRef<string | null>(null);

  const rowId = (group: QBGroup, index: number) =>
    `${editorId}-${group}-button-${index + 1}`;
  const labelInputId = (group: QBGroup, index: number) =>
    `${rowId(group, index)}-label`;
  const addButtonId = (group: QBGroup) => `${editorId}-${group}-add`;

  useLayoutEffect(() => {
    if (!pendingFocusId.current) return;
    document.getElementById(pendingFocusId.current)?.focus();
    pendingFocusId.current = null;
  }, [editorVersion]);

  // editorVersion changes whenever the original would have called
  // renderQuickEditor (add/delete). It is folded into each row's key below so a
  // structural change remounts all rows with fresh defaultValues.

  return (
    <div id="qb-editor">
      {QB_GROUPS.map((g) => (
        <fieldset className="qb-group" key={g}>
          <legend className="qb-group-head">{QB_LABEL[g]}</legend>
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
                id={rowId(g, i)}
                groupLabel={QB_LABEL[g]}
                index={i}
                item={item}
                saveQuickConfig={saveQuickConfig}
                bumpMenu={bumpMenu}
                onDelete={() => {
                  const remainingLength = quickConfig[g].length - 1;
                  pendingFocusId.current =
                    remainingLength === 0
                      ? addButtonId(g)
                      : labelInputId(g, Math.min(i, remainingLength - 1));
                  deleteQuickRow(g, item);
                }}
              />
            ))}
          </div>
          <button
            id={addButtonId(g)}
            className="qb-add"
            type="button"
            aria-label={`Add button to ${QB_LABEL[g]}`}
            onClick={() => {
              pendingFocusId.current = labelInputId(g, quickConfig[g].length);
              addQuickRow(g);
            }}
          >
            + Add button
          </button>
        </fieldset>
      ))}
    </div>
  );
}

interface QuickRowProps {
  id: string;
  groupLabel: string;
  index: number;
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
function QuickRow({
  id,
  groupLabel,
  index,
  item,
  saveQuickConfig,
  bumpMenu,
  onDelete,
}: QuickRowProps) {
  // Hold the initial values so the uncontrolled inputs seed from the entry once
  // (the original set `label.value = item.label` after creating the element).
  const initial = useRef({ label: item.label, text: item.text });
  const [entryLabel, setEntryLabel] = useState(item.label);
  const rowLabel = `${groupLabel} button ${index + 1}`;
  const removeTarget = entryLabel.trim() || "unnamed button";

  return (
    <div className="qb-row" role="group" aria-label={rowLabel}>
      <input
        id={`${id}-label`}
        className="qb-label"
        type="text"
        aria-label={`${rowLabel} label`}
        placeholder="label"
        spellCheck={false}
        autoCapitalize="off"
        autoComplete="off"
        defaultValue={initial.current.label}
        onInput={(e) => {
          item.label = (e.target as HTMLInputElement).value;
          setEntryLabel(item.label);
          saveQuickConfig();
          bumpMenu();
        }}
      />
      <input
        id={`${id}-text`}
        className="qb-text"
        type="text"
        aria-label={`${rowLabel} text sent to the pane`}
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
        aria-label={`Remove "${removeTarget}" from ${rowLabel}`}
        title={`Remove ${removeTarget}`}
        onClick={onDelete}
      >
        ✕
      </button>
    </div>
  );
}
