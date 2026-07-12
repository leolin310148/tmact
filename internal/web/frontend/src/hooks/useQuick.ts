// Quick-input buttons (phone-only FAB) — 1:1 behavioral port of
// static/js/quick.js's `createQuick({...})` factory.
//
// One bottom-right FAB whose menu sends pre-canned commands ("/compact",
// "/clear", custom) into the selected pane. Config lives in localStorage and is
// edited from the settings panel.
//
// PARITY MODEL (ARCHITECTURE.md §1, §3):
//   The original kept `quickConfig` as a module-scoped object mutated BY
//   REFERENCE (editor rows bind directly to entries: `item.label = …`). We hold
//   it in a `useRef` and mutate it in place, IDENTICALLY to the original — never
//   useState, never cloned. The original re-ran imperative DOM rebuilds
//   (`renderQuickMenu` / `renderQuickEditor`) after a mutation; here those are
//   React renders driven by a version bump (`menuVersion` / `editorVersion`),
//   which QuickDock and QuickEditor subscribe to.
//
//   `syncQuickDock` and `closeQuickMenu`/`openQuickMenu` toggle classes /
//   `disabled` on elements that live across several components (`#qb-dock`,
//   `#upload-btn`, `#selection-btn`, `#clear-pane-btn`, `#content-wrap`,
//   `#qb-backdrop`). The original did this with `$()` (getElementById); we keep
//   the SAME imperative DOM writes (via getElementById) so the cross-component
//   coordination matches byte-for-behavior and we don't depend on components we
//   don't own. The menu/editor CONTENTS are rendered reactively by the
//   components instead of being rebuilt here.

import { useCallback, useRef, useState } from "react";
import type { InputMsg, PaneStatus } from "../types/server";
import { useAppState } from "../store/AppStateContext";

// localStorage key — verbatim from quick.js.
const QUICK_KEY = "tmact.quickButtons";

// Groups the FAB can show: "common" appears for every pane; the rest are
// matched to the pane's detected runtime. Keep in step with QB_LABEL,
// RUNTIME_GROUP, and QB_DEFAULT.
export const QB_GROUPS = ["common", "claude", "codex", "shell"] as const;
export type QBGroup = (typeof QB_GROUPS)[number];

export const QB_LABEL: Record<QBGroup, string> = {
  common: "Common · every pane",
  claude: "Claude panes",
  codex: "Codex panes",
  shell: "Shell panes",
};

// Snapshot runtime → quick-button group. A runtime with no group (gemini,
// tmact and unknown runtimes fall back to the Common group alone.
const RUNTIME_GROUP: Record<string, QBGroup | undefined> = {
  claude: "claude",
  codex: "codex",
  shell: "shell",
};

/** One quick-button entry — `{label, text}`, verbatim from quick.js. */
export interface QuickEntry {
  label: string;
  text: string;
}

/** Per-group button lists; all four groups are always present. */
export type QuickConfig = Record<QBGroup, QuickEntry[]>;

// Seeded on first run; the user edits these from the settings panel. Verbatim
// from quick.js QB_DEFAULT.
const QB_DEFAULT: QuickConfig = {
  common: [],
  claude: [
    { label: "/compact", text: "/compact" },
    { label: "/clear", text: "/clear" },
  ],
  codex: [
    { label: "/compact", text: "/compact" },
    { label: "/clear", text: "/clear" },
  ],
  shell: [],
};

/**
 * Injected-deps contract — mirrors `createQuick({...})` in quick.js
 * (ARCHITECTURE.md §3). App provides these (mostly from
 * `useAppState().callbacks`); the hook never reaches back into App by import.
 */
export interface UseQuickDeps {
  /** `wsSend(obj)` — `true` when the socket was OPEN and the frame went out. */
  wsSend: (msg: InputMsg) => boolean;
  /** `showInputError(msg)` — transient error in the input-bar indicator slot. */
  showInputError: (msg: string) => void;
  /** `findPane(id)` — look up a pane in the current snapshot, or null. */
  findPane: (paneID: string | null) => PaneStatus | null;
  /** `syncSelectionButton()` — reconcile the `#selection-btn` classes/disabled. */
  syncSelectionButton: () => void;
}

/**
 * The object QuickDock/QuickEditor consume. Mirrors what `createQuick`
 * returned (`loadQuickConfig` / `wireQuick` / `syncQuickDock` / `closeQuickMenu`)
 * PLUS the reactive surface the React components need to render the menu/editor
 * contents (which the original rebuilt imperatively).
 */
export interface UseQuickReturn {
  // ---- the four functions App wires (parity with createQuick's return) ----
  loadQuickConfig: () => void;
  wireQuick: () => void;
  syncQuickDock: () => void;
  closeQuickMenu: () => void;

  // ---- reactive surface for the components (renderQuickMenu / editor) ----
  /** Whether the FAB menu is open (drives `#qb-dock.open` + `#qb-backdrop.open`). */
  isOpen: boolean;
  /** Toggle the menu (wired to `#qb-fab` click). */
  toggleQuickMenu: () => void;
  /** Buttons to show for the selected pane (common + runtime group, text!==""). */
  applicableQuick: () => QuickEntry[];
  /** Click a quick button: send + close menu, or keep open + showInputError. */
  onQuickButtonClick: (entry: QuickEntry) => void;
  /** Live config (mutated by reference); QuickEditor edits its entries in place. */
  quickConfig: QuickConfig;
  /** Persist `quickConfig` to localStorage (verbatim guard). */
  saveQuickConfig: () => void;
  /** Push a fresh `{label:"",text:""}` row into a group (editor "+ Add"). */
  addQuickRow: (group: QBGroup) => void;
  /** Remove an entry (by reference) from a group (editor "✕"). */
  deleteQuickRow: (group: QBGroup, item: QuickEntry) => void;
  /** Re-render the menu (call after a config keystroke — like `renderQuickMenu`). */
  bumpMenu: () => void;
  /** Re-render the editor (call after add/delete — like `renderQuickEditor`). */
  bumpEditor: () => void;
  /** Monotonic; changes whenever the menu must re-render. */
  menuVersion: number;
  /** Monotonic; changes whenever the editor must re-render. */
  editorVersion: number;
}

export function useQuick(deps: UseQuickDeps): UseQuickReturn {
  const { wsSend, showInputError, findPane, syncSelectionButton } = deps;
  // `state` and `upload` are mutable-by-reference objects (see store); reading
  // them once is fine — they are mutated in place, so closures stay current.
  const { state, upload } = useAppState();

  // quickConfig held by reference (NEVER useState) so editor rows bind straight
  // to entries, exactly like the original. Seeded empty so `applicableQuick`
  // is safe before loadQuickConfig runs (the original assumed loadQuickConfig
  // ran first synchronously — App does the same).
  const quickConfigRef = useRef<QuickConfig>({
    common: [],
    claude: [],
    codex: [],
    shell: [],
  });

  // Menu-open flag. The module-scoped truth in the original was the
  // `#qb-dock.open` class; here we hold it as React state so the component's
  // class binding re-renders, AND mirror it to the DOM in open/close (matching
  // the original's imperative class toggles, which other listeners read). The
  // `openRef` mirrors `isOpen` for the synchronous read in `toggleQuickMenu`
  // (the original read `#qb-dock.contains("open")` synchronously).
  const [isOpen, setIsOpen] = useState(false);
  const openRef = useRef(false);

  // Re-render triggers for the menu and editor contents (the original called
  // renderQuickMenu / renderQuickEditor after mutations).
  const [menuVersion, setMenuVersion] = useState(0);
  const [editorVersion, setEditorVersion] = useState(0);

  const bumpMenu = useCallback(() => setMenuVersion((v) => v + 1), []);
  const bumpEditor = useCallback(() => setEditorVersion((v) => v + 1), []);

  const saveQuickConfig = useCallback(() => {
    try {
      localStorage.setItem(QUICK_KEY, JSON.stringify(quickConfigRef.current));
    } catch (e) {
      // ignore (quota / private mode) — verbatim from quick.js
    }
  }, []);

  // loadQuickConfig reads the per-group button lists from localStorage, seeding
  // the defaults on first run. Every entry is normalised to {label, text} and
  // every group is present, so later code can index quickConfig freely.
  const loadQuickConfig = useCallback(() => {
    let saved: unknown = null;
    try {
      saved = JSON.parse(localStorage.getItem(QUICK_KEY) || "null");
    } catch (e) {
      // ignore malformed JSON — verbatim from quick.js
    }
    if (!saved || typeof saved !== "object") {
      quickConfigRef.current = JSON.parse(JSON.stringify(QB_DEFAULT)) as QuickConfig;
      saveQuickConfig();
      return;
    }
    const savedObj = saved as Record<string, unknown>;
    const next: QuickConfig = { common: [], claude: [], codex: [], shell: [] };
    for (const g of QB_GROUPS) {
      const raw = savedObj[g];
      const list = Array.isArray(raw) ? raw : [];
      next[g] = list.map((x) => {
        const entry = (x ?? {}) as { label?: unknown; text?: unknown };
        return {
          label: typeof entry.label === "string" ? entry.label : "",
          text: typeof entry.text === "string" ? entry.text : "",
        };
      });
    }
    quickConfigRef.current = next;
  }, [saveQuickConfig]);

  // applicableQuick returns the buttons to show for the selected pane: the
  // Common group plus the group matching the pane's runtime. Entries with no
  // text are dropped — they are half-finished editor rows.
  const applicableQuick = useCallback((): QuickEntry[] => {
    const cfg = quickConfigRef.current;
    const out: QuickEntry[] = [];
    for (const it of cfg.common || []) if (it.text) out.push(it);
    const pane = findPane(state.selected);
    const g = pane ? RUNTIME_GROUP[pane.runtime] : undefined;
    if (g) for (const it of cfg[g] || []) if (it.text) out.push(it);
    return out;
  }, [findPane, state]);

  // ---- menu open/close (imperative DOM class toggles, verbatim) ----

  const closeQuickMenu = useCallback(() => {
    openRef.current = false;
    setIsOpen(false);
    const dock = document.getElementById("qb-dock");
    const backdrop = document.getElementById("qb-backdrop");
    if (dock) dock.classList.remove("open");
    if (backdrop) backdrop.classList.remove("open");
  }, []);

  const openQuickMenu = useCallback(() => {
    if (!state.selected) return;
    openRef.current = true;
    setIsOpen(true);
    // renderQuickMenu equivalent: trigger the component's menu render.
    bumpMenu();
    const dock = document.getElementById("qb-dock");
    const backdrop = document.getElementById("qb-backdrop");
    if (dock) dock.classList.add("open");
    if (backdrop) backdrop.classList.add("open");
  }, [state, bumpMenu]);

  const toggleQuickMenu = useCallback(() => {
    // The original keyed off `#qb-dock.contains("open")`; openRef mirrors it.
    if (openRef.current) closeQuickMenu();
    else openQuickMenu();
  }, [closeQuickMenu, openQuickMenu]);

  // Quick button click → "send" message; success closes the menu, failure
  // surfaces the standard error and keeps the menu open.
  const onQuickButtonClick = useCallback(
    (entry: QuickEntry) => {
      if (!wsSend({ t: "send", s: entry.text })) {
        showInputError("not connected — try again");
        return;
      }
      closeQuickMenu();
    },
    [wsSend, showInputError, closeQuickMenu],
  );

  // syncQuickDock reveals the FAB once a pane is selected and hides it (closing
  // any open menu) when none is. Imperative DOM writes via getElementById,
  // verbatim from quick.js (the original used `$()`); the upload/selection/
  // clear-pane buttons and content-wrap live in other components but are owned
  // here exactly as the original did.
  const syncQuickDock = useCallback(() => {
    const dock = document.getElementById("qb-dock");
    const uploadBtn = document.getElementById("upload-btn") as HTMLButtonElement | null;
    const selectionBtn = document.getElementById("selection-btn") as HTMLButtonElement | null;
    const clearPaneBtn = document.getElementById("clear-pane-btn") as HTMLButtonElement | null;
    const wrap = document.getElementById("content-wrap");
    if (state.selected) {
      dock?.classList.add("ready");
      uploadBtn?.classList.add("ready");
      selectionBtn?.classList.add("ready");
      clearPaneBtn?.classList.add("ready");
      wrap?.classList.add("upload-ready");
      if (uploadBtn) uploadBtn.disabled = upload.busy;
      if (selectionBtn) selectionBtn.disabled = false;
      if (clearPaneBtn) clearPaneBtn.disabled = false;
    } else {
      dock?.classList.remove("ready");
      uploadBtn?.classList.remove("ready");
      selectionBtn?.classList.remove("ready");
      clearPaneBtn?.classList.remove("ready");
      wrap?.classList.remove("upload-ready");
      if (uploadBtn) uploadBtn.disabled = true;
      if (selectionBtn) selectionBtn.disabled = true;
      if (clearPaneBtn) clearPaneBtn.disabled = true;
      closeQuickMenu();
    }
    syncSelectionButton();
  }, [state, upload, closeQuickMenu, syncSelectionButton]);

  // ---- editor mutations (live save + re-render, verbatim) ----

  const addQuickRow = useCallback(
    (group: QBGroup) => {
      quickConfigRef.current[group].push({ label: "", text: "" });
      saveQuickConfig();
      bumpEditor();
    },
    [saveQuickConfig, bumpEditor],
  );

  const deleteQuickRow = useCallback(
    (group: QBGroup, item: QuickEntry) => {
      quickConfigRef.current[group] = quickConfigRef.current[group].filter(
        (x) => x !== item,
      );
      saveQuickConfig();
      bumpEditor();
      bumpMenu();
    },
    [saveQuickConfig, bumpEditor, bumpMenu],
  );

  // wireQuick wires the FAB/backdrop/Escape behavior. In the React port the FAB
  // click and backdrop click are bound on the JSX elements (QuickDock); here we
  // only register the document-level Escape listener (matching quick.js's
  // `document.addEventListener("keydown", …)`) and trigger the initial editor
  // render. Returns nothing (parity with createQuick).
  const escHandlerRef = useRef<((e: KeyboardEvent) => void) | null>(null);
  const wireQuick = useCallback(() => {
    // Remove any prior handler (idempotent if App re-invokes wireQuick).
    if (escHandlerRef.current) {
      document.removeEventListener("keydown", escHandlerRef.current);
    }
    const onKeydown = (e: KeyboardEvent) => {
      const dock = document.getElementById("qb-dock");
      if (e.key === "Escape" && dock && dock.classList.contains("open")) {
        closeQuickMenu();
      }
    };
    escHandlerRef.current = onKeydown;
    document.addEventListener("keydown", onKeydown);
    // renderQuickEditor equivalent: trigger the editor's first render.
    bumpEditor();
  }, [closeQuickMenu, bumpEditor]);

  return {
    loadQuickConfig,
    wireQuick,
    syncQuickDock,
    closeQuickMenu,
    isOpen,
    toggleQuickMenu,
    applicableQuick,
    onQuickButtonClick,
    quickConfig: quickConfigRef.current,
    saveQuickConfig,
    addQuickRow,
    deleteQuickRow,
    bumpMenu,
    bumpEditor,
    menuVersion,
    editorVersion,
  };
}
