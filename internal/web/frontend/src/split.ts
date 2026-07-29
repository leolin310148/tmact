// Desktop split-view shell — mounted by main.tsx when the URL carries
// ?view=split (and no ?slot). Same "/" document as the normal app, so the PWA
// service worker, manifest scope, and installed standalone mode all keep
// working; the shell is reached from Settings → "Open split view" and left via
// the split menu's Exit entry (both plain same-origin navigations).
//
// The shell embeds the normal app in 1–3 side-by-side iframes, each with
// ?slot=N so its pane selection persists independently (lib/slot.ts). Each
// iframe is a fully independent app instance (own WS, own snapshot stream,
// own input bar); the shell only manages columns and shows which slot owns
// keyboard focus.
//
// Slots beyond the chosen column count are REMOVED from the DOM, not hidden —
// a display:none iframe would keep its WebSocket + capture loop alive on the
// server for no visible benefit.

import { createElement } from "react";
import { createRoot } from "react-dom/client";
import UsagePanel from "./components/UsagePanel";

const COLS_KEY = "tmact.split.cols";
const MAX_COLS = 3;

// Whether the user was last in split view. Set on shell mount, cleared by the
// exit button, read by main.tsx so relaunching the PWA at start_url "/"
// restores the mode without pressing "Open split view" again.
export const SPLIT_ACTIVE_KEY = "tmact.split.active";

export function wasSplitActive(): boolean {
  try {
    return localStorage.getItem(SPLIT_ACTIVE_KEY) === "1";
  } catch {
    return false;
  }
}

const SHELL_CSS = `
  html, body { height: 100%; }
  body {
    margin: 0; background: var(--bg, #0e1116); color: var(--fg, #c9d1d9);
    font-family: var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 12px; display: flex; flex-direction: column; overflow: hidden;
  }
  #root { display: contents; }
  /* The bar is a transparent overlay, not a flow row — the grid gets the full
     viewport height. pointer-events pass through everywhere except the actual
     controls, so the iframes' top edge stays clickable. */
  #split-bar {
    position: absolute; inset: 0 0 auto 0; z-index: 30;
    display: flex; flex-direction: column; align-items: flex-end; gap: 4px;
    padding: 4px 10px; background: transparent;
    pointer-events: none;
  }
  /* One compact control: the split-mode menu button + its dropdown. */
  #split-menu-wrap { position: relative; flex: none; }
  #split-menu-btn {
    font: inherit; color: var(--fg-dim, #8b949e);
    background: rgba(14,17,22,0.82);
    border: 1px solid var(--border, #2a313c); border-radius: 4px;
    padding: 1px 8px; cursor: pointer;
    pointer-events: auto;
  }
  #split-menu-btn.open { color: var(--fg, #c9d1d9); border-color: var(--accent, #4493f8); }
  #split-menu {
    position: absolute; top: calc(100% + 4px); right: 0; min-width: 140px;
    display: none; flex-direction: column;
    background: var(--panel, #161b22);
    border: 1px solid var(--border, #2a313c); border-radius: 6px;
    box-shadow: 0 4px 14px rgba(0,0,0,0.5);
    padding: 4px; pointer-events: auto;
  }
  #split-menu.open { display: flex; }
  #split-menu button {
    font: inherit; color: var(--fg, #c9d1d9); background: transparent;
    border: 0; border-radius: 4px; padding: 5px 8px; cursor: pointer;
    text-align: left;
  }
  #split-menu button:hover { background: var(--panel-2, #1c2330); }
  #split-menu button.sel { color: var(--accent, #4493f8); }
  #split-menu .split-menu-sep {
    height: 1px; margin: 4px 2px; background: var(--border, #2a313c);
  }
  #split-usage { flex: none; }
  /* The shared usage panel keeps its app.css floating-bubble chrome
     (semi-transparent, pointer-events none); only anchor it into the bar's
     flex flow instead of absolute top-right. flex:none + max-width:none undo
     flex-item squeezing — app.css's max-width:80% resolves against this
     content-sized host and used to wrap the grid cells. */
  #split-bar .usage-panel { position: static; flex: none; max-width: none; }
  #split-grid {
    flex: 1; display: grid; min-height: 0;
    grid-template-columns: repeat(var(--cols, 2), 1fr); gap: 1px;
    background: var(--border, #2a313c);
  }
  .slot { position: relative; min-width: 0; min-height: 0; background: var(--bg, #0e1116); }
  .slot iframe { display: block; width: 100%; height: 100%; border: 0; }
  /* Active-slot indicator: an inset top edge in the accent color. Deliberately
     NOT a frame around the whole column — a frame reads as "this pane list and
     input bar are highlighted too", when the thing that owns the keyboard is the
     terminal inside. The app draws its own ring around just the pane text when
     direct mode is on; this edge only marks the column. The overlay must not
     intercept clicks headed for the iframe. */
  .slot::after {
    content: ""; position: absolute; inset: 0 0 auto 0; height: 2px;
    background: transparent; pointer-events: none;
  }
  .slot.active::after { background: var(--accent, #4493f8); }
`;

export function initSplitShell(rootEl: HTMLElement): void {
  document.title = "tmact split";
  try {
    localStorage.setItem(SPLIT_ACTIVE_KEY, "1");
  } catch {
    /* private mode — split just won't survive a relaunch */
  }

  const style = document.createElement("style");
  style.textContent = SHELL_CSS;
  document.head.appendChild(style);

  rootEl.innerHTML = `
    <header id="split-bar">
      <div id="split-usage"></div>
      <div id="split-menu-wrap">
        <button id="split-menu-btn" type="button" aria-haspopup="menu"
                aria-expanded="false" title="split view">⊞ 2</button>
        <div id="split-menu" role="menu" aria-label="split view">
          <button data-cols="1" type="button" role="menuitemradio">1 column</button>
          <button data-cols="2" type="button" role="menuitemradio">2 columns</button>
          <button data-cols="3" type="button" role="menuitemradio">3 columns</button>
          <div class="split-menu-sep" aria-hidden="true"></div>
          <button id="split-exit" type="button" role="menuitem">Exit split view</button>
        </div>
      </div>
    </header>
    <main id="split-grid"></main>
  `;

  const grid = rootEl.querySelector<HTMLElement>("#split-grid");
  if (!grid) return;

  // One shared agent-usage panel for the whole split view (slots suppress
  // theirs) — a small React island; the rest of the shell stays plain DOM.
  const usageHost = rootEl.querySelector<HTMLElement>("#split-usage");
  if (usageHost) createRoot(usageHost).render(createElement(UsagePanel));

  const menuWrap = rootEl.querySelector<HTMLElement>("#split-menu-wrap");
  const menuBtn = rootEl.querySelector<HTMLButtonElement>("#split-menu-btn");
  const menu = rootEl.querySelector<HTMLElement>("#split-menu");
  const colButtons = Array.from(
    rootEl.querySelectorAll<HTMLButtonElement>("#split-menu button[data-cols]"),
  );

  const setMenuOpen = (open: boolean): void => {
    menu?.classList.toggle("open", open);
    menuBtn?.classList.toggle("open", open);
    menuBtn?.setAttribute("aria-expanded", String(open));
  };
  menuBtn?.addEventListener("click", () => {
    setMenuOpen(!menu?.classList.contains("open"));
  });
  // Outside click closes the menu. Clicks landing INSIDE an iframe never
  // reach this document — the window `blur` handler below covers those.
  document.addEventListener("click", (e) => {
    if (e.target instanceof Node && menuWrap?.contains(e.target)) return;
    setMenuOpen(false);
  });

  const makeSlot = (n: number): HTMLElement => {
    const slot = document.createElement("div");
    slot.className = "slot";
    slot.dataset.slot = String(n);
    const frame = document.createElement("iframe");
    frame.src = `/?slot=${n}`;
    frame.title = `tmact slot ${n}`;
    slot.appendChild(frame);
    return slot;
  };

  const applyCols = (cols: number): void => {
    const want = Math.min(Math.max(cols, 1), MAX_COLS);
    grid.style.setProperty("--cols", String(want));
    const slots = Array.from(grid.children);
    for (let i = slots.length; i < want; i++) grid.appendChild(makeSlot(i + 1));
    for (let i = slots.length - 1; i >= want; i--) slots[i]?.remove();
    for (const b of colButtons) {
      const sel = Number(b.dataset.cols) === want;
      b.classList.toggle("sel", sel);
      b.setAttribute("aria-checked", String(sel));
    }
    if (menuBtn) menuBtn.textContent = `⊞ ${want}`;
    try {
      localStorage.setItem(COLS_KEY, String(want));
    } catch {
      /* private mode — column count just won't persist */
    }
  };

  for (const b of colButtons) {
    b.addEventListener("click", () => {
      applyCols(Number(b.dataset.cols));
      setMenuOpen(false);
    });
  }
  rootEl.querySelector("#split-exit")?.addEventListener("click", () => {
    try {
      localStorage.removeItem(SPLIT_ACTIVE_KEY);
    } catch {
      /* ignore */
    }
    window.location.href = "/";
  });

  // When focus moves into an iframe the parent window fires `blur` and
  // document.activeElement becomes that iframe element — the only portable
  // signal for "which split is the keyboard talking to".
  const markActive = (): void => {
    const el = document.activeElement;
    const active = el instanceof HTMLIFrameElement ? el.closest(".slot") : null;
    if (!active) return; // keep the last highlight when focus leaves the page
    for (const slot of grid.children) {
      slot.classList.toggle("active", slot === active);
    }
  };
  window.addEventListener("blur", () => {
    // Focus moved into an iframe: update the active highlight and close the
    // menu (its outside-click handler can't see clicks inside iframes).
    setMenuOpen(false);
    // activeElement updates after the blur event settles.
    setTimeout(markActive, 0);
  });

  // The blur path above only fires on the FIRST hop out of the shell. Going
  // slot 1 → slot 2 moves focus between two iframes while the parent window is
  // already blurred, so nothing fires here and the highlight would stay stuck on
  // slot 1. Each slot document therefore announces its own focus (main.tsx); we
  // trust the message only by matching its source window against a live iframe,
  // never by a slot index in the payload.
  window.addEventListener("message", (e: MessageEvent) => {
    if (e.origin !== window.location.origin) return;
    if (!e.data || (e.data as { tmact?: string }).tmact !== "slot-focus") return;
    const active = Array.from(grid.children).find(
      (slot) => slot.querySelector("iframe")?.contentWindow === e.source,
    );
    if (!active) return;
    setMenuOpen(false);
    for (const slot of grid.children) {
      slot.classList.toggle("active", slot === active);
    }
  });

  let initial = 2;
  try {
    const saved = Number(localStorage.getItem(COLS_KEY));
    if (saved >= 1 && saved <= MAX_COLS) initial = saved;
  } catch {
    /* ignore */
  }
  applyCols(initial);
}
