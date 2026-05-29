// On-screen helper key bar (Termius-style, for mobile) — a faithful 1:1 port of
// app.js's buildKeyBar/syncKeyBar plus the sticky-Ctrl folding state that
// app.js's setCtrl/sendDirect share. Spec §6 items 41–42; ARCHITECTURE.md §6
// (every key-bar button + #key-toggle get pointerdown preventDefault).
//
// Structure mirrors index.html exactly:
//   <div class="key-area" id="key-area">
//     <div class="key-bar" id="key-bar"> … buttons … </div>
//     <button class="key-toggle" id="key-toggle"> ⌃ / ⌄ </button>
//   </div>
// The container ids/classes are kept verbatim so app.css selects on them.
//
// PARITY NOTES
//   - HELPER_KEYS, the Ctrl-sticky folding, and the single-row clipping logic
//     are reproduced byte-for-behavior from app.js (lines 596–660).
//   - `ctrlArmed` was a module-scoped `let` in app.js mutated by setCtrl() and
//     read/cleared by both the key-bar handler and direct-mode sendDirect(). Per
//     ARCHITECTURE.md golden rule #1 it lives in a `useRef` here. App owns the
//     single shared instance and passes it in (so DirectInput's sendDirect and
//     this KeyBar fold the SAME armed flag), exactly like the original shared the
//     module-level variable. The `.armed` class on #ctrl-key is reconciled from
//     the ref via a bump-driven render (KeyBar re-renders on bump and reads
//     ctrlArmedRef.current).
//   - syncKeyBar reads `firstBtn.offsetHeight` / `bar.scrollHeight` (synchronous
//     layout reads) and writes `bar.style.maxHeight` + toggles
//     `.overflowing`/`.expanded` on #key-area. These imperative layout reads run
//     in a useLayoutEffect (so they observe a laid-out DOM) and on window
//     "resize", matching app.js's `window.addEventListener("resize", syncKeyBar)`
//     and its post-build `syncKeyBar()` call.

import { useLayoutEffect, useRef } from "react";
import type { RefObject } from "react";
import { onPointerDownNoBlur } from "../lib/dom";
import type { InputMsg } from "../types/server";

// Helper keys — verbatim from app.js HELPER_KEYS (label + tmux key, or ctrl
// flag for the sticky-Ctrl toggle).
interface HelperKey {
  label: string;
  key?: string;
  ctrl?: boolean;
}
const HELPER_KEYS: readonly HelperKey[] = [
  { label: "Esc", key: "Escape" },
  { label: "^C", key: "C-c" },
  { label: "Tab", key: "Tab" },
  { label: "⇧Tab", key: "BTab" },
  { label: "ctl", ctrl: true },
  { label: "↵", key: "Enter" },
  { label: "↑", key: "Up" },
  { label: "↓", key: "Down" },
  { label: "←", key: "Left" },
  { label: "→", key: "Right" },
  { label: "Home", key: "Home" },
  { label: "End", key: "End" },
  { label: "PgUp", key: "PageUp" },
  { label: "PgDn", key: "PageDown" },
];

export interface KeyBarProps {
  /**
   * app.js `wsSend(obj)`. Returns true when the frame was sent; false → caller
   * surfaces `"not connected — try again"`. (App passes `callbacks.wsSend`.)
   */
  wsSend: (msg: InputMsg) => boolean;
  /**
   * app.js `showInputError(msg)`. (App passes `callbacks.showInputError`.)
   */
  showInputError: (msg: string) => void;
  /**
   * Shared sticky-Ctrl flag (app.js module-scoped `ctrlArmed`). App owns this
   * ref and ALSO passes it to DirectInput's sendDirect so a single armed flag is
   * folded/cleared across both surfaces, exactly like the original shared the
   * module-level variable. KeyBar toggles it (the "ctl" button) and clears it
   * after sending a helper key while armed.
   */
  ctrlArmedRef: RefObject<boolean>;
  /**
   * Re-render trigger (app.js re-ran nothing for the bar specifically, but the
   * `.armed` class on #ctrl-key must reflect the shared ref after a toggle or an
   * external disarm in sendDirect). App passes `bump` so KeyBar repaints the
   * armed class. Mirrors setCtrl()'s `b.classList.toggle("armed", on)`.
   */
  bump: () => void;
}

export default function KeyBar({
  wsSend,
  showInputError,
  ctrlArmedRef,
  bump,
}: KeyBarProps) {
  const areaRef = useRef<HTMLDivElement>(null);
  const barRef = useRef<HTMLDivElement>(null);
  const toggleRef = useRef<HTMLButtonElement>(null);

  // setCtrl mirrors app.js: flip the shared armed flag and re-render so the
  // #ctrl-key button's `.armed` class follows. (app.js toggled the class
  // imperatively; here it is derived from ctrlArmedRef.current in render.)
  const setCtrl = (on: boolean) => {
    ctrlArmedRef.current = on;
    bump();
  };

  // syncKeyBar — clips the helper-key bar to a single row and reveals the expand
  // toggle only when the keys overflow that row. overflow:hidden keeps
  // scrollHeight reporting the full wrapped height even while the bar is clipped.
  // Verbatim port of app.js syncKeyBar (lines 625–636).
  const syncKeyBar = () => {
    const area = areaRef.current;
    const bar = barRef.current;
    const toggle = toggleRef.current;
    if (!area || !bar || !toggle) return;
    const firstBtn = bar.querySelector("button");
    if (!firstBtn) return;
    const rowH = (firstBtn as HTMLElement).offsetHeight;
    const overflows = rowH > 0 && bar.scrollHeight > rowH + 2;
    if (!overflows) area.classList.remove("expanded");
    const expanded = area.classList.contains("expanded");
    area.classList.toggle("overflowing", overflows);
    toggle.textContent = expanded ? "⌃" : "⌄";
    bar.style.maxHeight = overflows && !expanded ? rowH + "px" : "";
  };

  // app.js wired `window.addEventListener("resize", syncKeyBar)` in buildKeyBar
  // and called syncKeyBar() once after appending the buttons. The layout effect
  // runs after the buttons are in the DOM (so offsetHeight/scrollHeight are
  // valid) and registers/cleans up the resize listener. Re-runs whenever the
  // armed class changes (it does not affect layout, but keeps the toggle glyph
  // consistent after a re-render) — harmless and matches the post-mutation sync.
  useLayoutEffect(() => {
    syncKeyBar();
    window.addEventListener("resize", syncKeyBar);
    return () => window.removeEventListener("resize", syncKeyBar);
    // syncKeyBar closes over stable refs only; deps intentionally empty to mirror
    // app.js's one-time wiring + persistent resize listener.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const onToggleClick = () => {
    const area = areaRef.current;
    if (!area) return;
    area.classList.toggle("expanded");
    syncKeyBar();
  };

  const ctrlArmed = ctrlArmedRef.current;

  return (
    <div className="key-area" id="key-area" ref={areaRef}>
      <div className="key-bar" id="key-bar" ref={barRef}>
        {HELPER_KEYS.map((k, i) => {
          const onClick = () => {
            if (k.ctrl) {
              setCtrl(!ctrlArmedRef.current);
              return;
            }
            // k.key is always defined for non-ctrl entries (HELPER_KEYS table).
            if (!wsSend({ t: "key", k: k.key as string })) {
              showInputError("not connected — try again");
            }
            if (ctrlArmedRef.current) setCtrl(false);
          };
          return (
            <button
              key={i}
              {...(k.ctrl ? { id: "ctrl-key" } : {})}
              type="button"
              className={k.ctrl && ctrlArmed ? "armed" : undefined}
              onPointerDown={onPointerDownNoBlur}
              onClick={onClick}
            >
              {k.label}
            </button>
          );
        })}
      </div>
      <button
        className="key-toggle"
        id="key-toggle"
        type="button"
        title="show/hide all keys"
        ref={toggleRef}
        onPointerDown={onPointerDownNoBlur}
        onClick={onToggleClick}
      />
    </div>
  );
}
