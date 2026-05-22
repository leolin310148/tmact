// Quick-input buttons (phone-only FAB).
// One bottom-right FAB whose menu sends pre-canned commands ("/compact",
// "/clear", custom) into the selected pane. Config lives in localStorage and
// is edited from the settings panel.

import { $, h } from "./dom.js";
import { state, upload } from "./state.js";

const QUICK_KEY = "tmact.quickButtons";
// Groups the FAB can show: "common" appears for every pane; the rest are
// matched to the pane's detected runtime. Keep in step with QB_LABEL,
// RUNTIME_GROUP, and QB_DEFAULT.
const QB_GROUPS = ["common", "claude", "codex", "shell"];
const QB_LABEL = {
  common: "Common · every pane",
  claude: "Claude panes",
  codex: "Codex panes",
  shell: "Shell panes",
};
// Snapshot runtime → quick-button group. A runtime with no group (gemini,
// copilot, tmact, unknown) falls back to the Common group alone.
const RUNTIME_GROUP = { claude: "claude", codex: "codex", shell: "shell" };
// Seeded on first run; the user edits these from the settings panel.
const QB_DEFAULT = {
  common: [],
  claude: [{ label: "/compact", text: "/compact" }, { label: "/clear", text: "/clear" }],
  codex: [{ label: "/compact", text: "/compact" }, { label: "/clear", text: "/clear" }],
  shell: [],
};

export function createQuick({ wsSend, showInputError, findPane, syncSelectionButton }) {
  let quickConfig = {};

  // loadQuickConfig reads the per-group button lists from localStorage, seeding
  // the defaults on first run. Every entry is normalised to {label, text} and
  // every group is present, so later code can index quickConfig freely.
  function loadQuickConfig() {
    let saved = null;
    try { saved = JSON.parse(localStorage.getItem(QUICK_KEY) || "null"); }
    catch (e) {}
    if (!saved || typeof saved !== "object") {
      quickConfig = JSON.parse(JSON.stringify(QB_DEFAULT));
      saveQuickConfig();
      return;
    }
    quickConfig = {};
    for (const g of QB_GROUPS) {
      const list = Array.isArray(saved[g]) ? saved[g] : [];
      quickConfig[g] = list.map((x) => ({
        label: x && typeof x.label === "string" ? x.label : "",
        text: x && typeof x.text === "string" ? x.text : "",
      }));
    }
  }

  function saveQuickConfig() {
    try { localStorage.setItem(QUICK_KEY, JSON.stringify(quickConfig)); }
    catch (e) {}
  }

  // applicableQuick returns the buttons to show for the selected pane: the
  // Common group plus the group matching the pane's runtime. Entries with no
  // text are dropped — they are half-finished editor rows.
  function applicableQuick() {
    const out = [];
    for (const it of quickConfig.common || []) if (it.text) out.push(it);
    const pane = findPane(state.selected);
    const g = pane && RUNTIME_GROUP[pane.runtime];
    if (g) for (const it of quickConfig[g] || []) if (it.text) out.push(it);
    return out;
  }

  // renderQuickMenu rebuilds the FAB menu from the current pane + config. Each
  // button types its text into the pane and presses Enter (a "send" message).
  function renderQuickMenu() {
    const menu = $("qb-menu");
    menu.textContent = "";
    const items = applicableQuick();
    if (items.length === 0) {
      menu.appendChild(h("div", { class: "qb-empty",
        text: "No quick buttons for this pane — add some in Settings." }));
      return;
    }
    for (const it of items) {
      const btn = h("button", { type: "button", title: it.text, text: it.label || it.text });
      // pointerdown preventDefault keeps the soft keyboard up and focus in place.
      btn.addEventListener("pointerdown", (e) => e.preventDefault());
      btn.addEventListener("click", () => {
        if (!wsSend({ t: "send", s: it.text })) {
          showInputError("not connected — try again");
          return;
        }
        closeQuickMenu();
      });
      menu.appendChild(btn);
    }
  }

  function openQuickMenu() {
    if (!state.selected) return;
    renderQuickMenu();
    $("qb-dock").classList.add("open");
    $("qb-backdrop").classList.add("open");
  }
  function closeQuickMenu() {
    $("qb-dock").classList.remove("open");
    $("qb-backdrop").classList.remove("open");
  }
  function toggleQuickMenu() {
    if ($("qb-dock").classList.contains("open")) closeQuickMenu();
    else openQuickMenu();
  }

  // syncQuickDock reveals the FAB once a pane is selected and hides it (closing
  // any open menu) when none is.
  function syncQuickDock() {
    const dock = $("qb-dock");
    const uploadBtn = $("upload-btn");
    const selectionBtn = $("selection-btn");
    const wrap = $("content-wrap");
    if (state.selected) {
      dock.classList.add("ready");
      uploadBtn.classList.add("ready");
      selectionBtn.classList.add("ready");
      wrap.classList.add("upload-ready");
      uploadBtn.disabled = upload.busy;
      selectionBtn.disabled = false;
    } else {
      dock.classList.remove("ready");
      uploadBtn.classList.remove("ready");
      selectionBtn.classList.remove("ready");
      wrap.classList.remove("upload-ready");
      uploadBtn.disabled = true;
      selectionBtn.disabled = true;
      closeQuickMenu();
    }
    syncSelectionButton();
  }

  // quickRow builds one editable button row, bound by object reference to its
  // config entry so edits land straight in quickConfig.
  function quickRow(group, item) {
    const label = h("input", { class: "qb-label", type: "text", placeholder: "label",
      spellcheck: "false", autocapitalize: "off", autocomplete: "off" });
    label.value = item.label;
    const text = h("input", { class: "qb-text", type: "text",
      placeholder: "text sent to the pane (Enter is added)",
      spellcheck: "false", autocapitalize: "off", autocomplete: "off" });
    text.value = item.text;
    label.addEventListener("input", () => {
      item.label = label.value; saveQuickConfig(); renderQuickMenu();
    });
    text.addEventListener("input", () => {
      item.text = text.value; saveQuickConfig(); renderQuickMenu();
    });
    const del = h("button", { class: "qb-del", type: "button",
      title: "remove button", text: "✕" });
    del.addEventListener("click", () => {
      quickConfig[group] = quickConfig[group].filter((x) => x !== item);
      saveQuickConfig();
      renderQuickEditor();
      renderQuickMenu();
    });
    return h("div", { class: "qb-row" }, label, text, del);
  }

  // renderQuickEditor rebuilds the settings-panel editor: one block per group,
  // each holding its rows and an "add" button.
  function renderQuickEditor() {
    const root = $("qb-editor");
    root.textContent = "";
    for (const g of QB_GROUPS) {
      const rows = h("div", { class: "qb-rows" });
      for (const item of quickConfig[g]) rows.appendChild(quickRow(g, item));
      const add = h("button", { class: "qb-add", type: "button", text: "+ Add button" });
      add.addEventListener("click", () => {
        quickConfig[g].push({ label: "", text: "" });
        saveQuickConfig();
        renderQuickEditor();
      });
      root.appendChild(h("div", { class: "qb-group" },
        h("div", { class: "qb-group-head", text: QB_LABEL[g] }),
        rows, add));
    }
  }

  function wireQuick() {
    // pointerdown preventDefault keeps the soft keyboard up and focus in place.
    $("qb-fab").addEventListener("pointerdown", (e) => e.preventDefault());
    $("qb-fab").addEventListener("click", toggleQuickMenu);
    $("qb-backdrop").addEventListener("click", closeQuickMenu);
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape" && $("qb-dock").classList.contains("open")) closeQuickMenu();
    });
    renderQuickEditor();
  }

  return {
    loadQuickConfig,
    wireQuick,
    syncQuickDock,
    closeQuickMenu,
  };
}
