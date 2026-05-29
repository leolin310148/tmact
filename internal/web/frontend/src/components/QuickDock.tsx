// QuickDock — phone-only quick-input FAB, its pop-up menu, and the backdrop.
// 1:1 behavioral port of the FAB/menu/backdrop portion of static/js/quick.js
// (`renderQuickMenu`, `openQuickMenu`/`closeQuickMenu`, `wireQuick`'s FAB +
// backdrop wiring) and the markup in static/index.html.
//
// DOM (verbatim ids/classes — app.css selects on them):
//   <div class="qb-backdrop" id="qb-backdrop">           ← rendered by App layout, but
//   <div class="qb-dock" id="qb-dock">                      we render both here so the
//     <div class="qb-menu" id="qb-menu">…buttons…</div>     dock + its backdrop stay
//     <button class="qb-fab" id="qb-fab">…icons…</button>   together (the .open /
//   </div>                                                  .ready classes are toggled
//                                                           imperatively by useQuick).
//
// CLASS-OWNERSHIP RULE (avoids React vs. imperative clobbering):
//   The `.open` and `.ready` classes on `#qb-dock` / `#qb-backdrop` are toggled
//   IMPERATIVELY by useQuick (`openQuickMenu`/`closeQuickMenu`/`syncQuickDock`)
//   exactly as the original did with `classList.add/remove`. This component
//   therefore renders these two elements with their BASE class only and never
//   sets `.open`/`.ready` via React — otherwise a React re-render would clobber
//   the imperative writes. Only the menu CONTENTS are rendered reactively here
//   (the original rebuilt them in `renderQuickMenu`, keyed off `menuVersion`).

import { onPointerDownNoBlur } from "../lib/dom";
import type { UseQuickReturn } from "../hooks/useQuick";

/**
 * Props. App passes the live `useQuick(...)` return value (the same object it
 * uses to wire `loadQuickConfig`/`wireQuick`/`syncQuickDock`/`closeQuickMenu`),
 * so the FAB/menu/backdrop read the reactive surface (`menuVersion`,
 * `applicableQuick`, `toggleQuickMenu`, `onQuickButtonClick`, `closeQuickMenu`).
 */
export interface QuickDockProps {
  quick: UseQuickReturn;
}

export function QuickDock({ quick }: QuickDockProps) {
  const { menuVersion, applicableQuick, toggleQuickMenu, onQuickButtonClick, closeQuickMenu } =
    quick;

  // Recompute the menu items whenever menuVersion changes (the original called
  // renderQuickMenu). Reading it here keeps the dependency explicit.
  void menuVersion;
  const items = applicableQuick();

  // Backdrop click closes only when the click landed on the backdrop itself
  // (e.target === backdrop), verbatim from spec §6 item 53. The original bound
  // the listener directly on #qb-backdrop (so any click fired); in React the
  // backdrop is the only target, but we guard target === currentTarget to match
  // the documented contract.
  const onBackdropClick = (e: { target: EventTarget | null; currentTarget: EventTarget | null }) => {
    if (e.target === e.currentTarget) closeQuickMenu();
  };

  return (
    <>
      <div
        className="qb-backdrop"
        id="qb-backdrop"
        onPointerDown={onPointerDownNoBlur}
        onClick={onBackdropClick}
      ></div>
      <div className="qb-dock" id="qb-dock">
        <div className="qb-menu" id="qb-menu">
          {items.length === 0 ? (
            <div className="qb-empty">
              No quick buttons for this pane — add some in Settings.
            </div>
          ) : (
            items.map((it, i) => (
              <button
                // index key: the menu is fully rebuilt per render (like the
                // original textContent="" + rebuild), so index is stable enough.
                key={i}
                type="button"
                title={it.text}
                onPointerDown={onPointerDownNoBlur}
                onClick={() => onQuickButtonClick(it)}
              >
                {it.label || it.text}
              </button>
            ))
          )}
        </div>
        <button
          className="qb-fab"
          id="qb-fab"
          type="button"
          title="quick input"
          aria-label="quick input"
          onPointerDown={onPointerDownNoBlur}
          onClick={toggleQuickMenu}
        >
          <svg className="qb-ic-open" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <path d="M13 2 3 14h7l-1 8 10-12h-7z" />
          </svg>
          <svg
            className="qb-ic-close"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2.5"
            strokeLinecap="round"
            aria-hidden="true"
          >
            <path d="M18 6 6 18" />
            <path d="m6 6 12 12" />
          </svg>
        </button>
      </div>
    </>
  );
}
