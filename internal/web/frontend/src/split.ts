// Desktop split-view shell — mounted by main.tsx when the URL carries
// ?view=split (and no ?slot). Same "/" document as the normal app, so the PWA
// service worker, manifest scope, and installed standalone mode all keep
// working; the shell is reached from Settings → "Open split view" and left via
// the ✕ button (both plain same-origin navigations).
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
  #split-bar {
    display: flex; align-items: center; gap: 12px;
    padding: 4px 10px; background: var(--panel, #161b22);
    border-bottom: 1px solid var(--border, #2a313c); flex: none;
  }
  #split-title { color: var(--fg-dim, #8b949e); }
  #split-cols { display: flex; gap: 4px; }
  #split-bar button {
    font: inherit; color: var(--fg-dim, #8b949e); background: transparent;
    border: 1px solid var(--border, #2a313c); border-radius: 4px;
    padding: 1px 8px; cursor: pointer;
  }
  #split-cols button.sel { color: var(--fg, #c9d1d9); border-color: var(--accent, #4493f8); }
  #split-exit { margin-left: auto; }
  #split-grid {
    flex: 1; display: grid; min-height: 0;
    grid-template-columns: repeat(var(--cols, 2), 1fr); gap: 1px;
    background: var(--border, #2a313c);
  }
  .slot { position: relative; min-width: 0; min-height: 0; background: var(--bg, #0e1116); }
  .slot iframe { display: block; width: 100%; height: 100%; border: 0; }
  /* Active-slot indicator: an inset top edge in the accent color. The overlay
     must not intercept clicks headed for the iframe. */
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
      <span id="split-title">tmact split</span>
      <div id="split-cols" role="group" aria-label="columns">
        <button data-cols="1" type="button">1</button>
        <button data-cols="2" type="button">2</button>
        <button data-cols="3" type="button">3</button>
      </div>
      <button id="split-exit" type="button" aria-label="exit split view">✕</button>
    </header>
    <main id="split-grid"></main>
  `;

  const grid = rootEl.querySelector<HTMLElement>("#split-grid");
  if (!grid) return;
  const colButtons = Array.from(
    rootEl.querySelectorAll<HTMLButtonElement>("#split-cols button"),
  );

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
      b.classList.toggle("sel", Number(b.dataset.cols) === want);
    }
    try {
      localStorage.setItem(COLS_KEY, String(want));
    } catch {
      /* private mode — column count just won't persist */
    }
  };

  for (const b of colButtons) {
    b.addEventListener("click", () => applyCols(Number(b.dataset.cols)));
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
    // activeElement updates after the blur event settles.
    setTimeout(markActive, 0);
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
