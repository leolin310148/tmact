// HelpOverlay — React port of the help.js coachmark DOM (#help-overlay) plus
// the #help-btn that toggles it. help.js built the banner / rings / tip cards
// imperatively with h(...) and positioned them after measuring; here the nodes
// are JSX (so app.css's .help-* rules apply by class) and the card positions are
// written imperatively by useHelp.place() in a layout effect — the same
// measure-then-position pass the original ran inside placeCoachmarks().
//
// Parity (spec §6 items 78-79):
//   - #help-btn: pointerdown preventDefault (keep focused input focused), click
//     stopPropagation + toggle. #help-overlay: pointerdown preventDefault, click
//     (backdrop) closes. Escape close + body.help-open live in useHelp.
//   - banner first; one .help-ring per resolved tip (drawn at the target rect
//     inset by -4 / +8 px); one .help-tip per resolved tip (positioned by
//     useHelp.place). tone-<name> appended to ring/tip class when present.
//   - rings carry their fixed left/top/width/height inline immediately (no
//     measurement needed); tip cards render position-less, then useHelp.place
//     measures offsetWidth/offsetHeight and writes left/top.

import { useLayoutEffect, useRef } from "react";
import { onPointerDownNoBlur } from "../lib/dom";
import type { UseHelp } from "../hooks/useHelp";

// HelpOverlay receives the live useHelp() return value from App. App owns the
// single useHelp() instance so the same open/items/place identity drives both
// the overlay and any external toggles (e.g. the help-btn lives here).
export interface HelpOverlayProps {
  help: UseHelp;
  /**
   * Whether to render the #help-btn here. Defaults true (self-contained). App
   * passes false because UploadControls already emits #help-btn inside
   * #content-wrap exactly where index.html places it; rendering it here too would
   * duplicate the id and double-wire the toggle. The overlay (#help-overlay) is
   * always rendered.
   */
  renderButton?: boolean;
}

export function HelpOverlay({ help, renderButton = true }: HelpOverlayProps) {
  const { open, items, toggle, close, nonce, place } = help;

  const bannerRef = useRef<HTMLDivElement | null>(null);
  // One card ref per rendered tip, in `items` order — passed to place().
  const cardRefs = useRef<(HTMLDivElement | null)[]>([]);
  cardRefs.current = [];

  // Measure-then-position pass. Runs after the banner/rings/cards are committed
  // to the DOM (useLayoutEffect = synchronous, before paint, like help.js doing
  // it inline after appendChild). Re-runs whenever useHelp bumps `nonce`
  // (open / resize / visualViewport) or the items list changes.
  useLayoutEffect(() => {
    if (!open) return;
    place({ banner: bannerRef.current, cards: cardRefs.current });
    // nonce participates so resize/visualViewport reflows re-trigger placement.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, nonce, items, place]);

  return (
    <>
      {renderButton ? (
        <button
          className="help-btn"
          id="help-btn"
          type="button"
          title="hotkey hints"
          aria-label="hotkey hints"
          onPointerDown={onPointerDownNoBlur}
          onClick={(e) => {
            e.stopPropagation();
            toggle();
          }}
        >
          ?
        </button>
      ) : null}
      <div
        className="help-overlay"
        id="help-overlay"
        onPointerDown={onPointerDownNoBlur}
        onClick={close}
      >
        <div className="help-backdrop" id="help-backdrop" />
        {open && (
          <>
            <div className="help-banner" ref={bannerRef}>
              Hotkey hints · tap anywhere or press Esc to close
            </div>
            {items.map((item, i) => {
              const tone = item.tip.tone ? " tone-" + item.tip.tone : "";
              const r = item.rect;
              return (
                <div
                  key={"ring-" + i}
                  className={"help-ring" + tone}
                  style={{
                    left: r.left - 4 + "px",
                    top: r.top - 4 + "px",
                    width: r.width + 8 + "px",
                    height: r.height + 8 + "px",
                  }}
                />
              );
            })}
            {items.map((item, i) => {
              const tone = item.tip.tone ? " tone-" + item.tip.tone : "";
              return (
                <div
                  key={"tip-" + i}
                  className={"help-tip" + tone}
                  ref={(el) => {
                    cardRefs.current[i] = el;
                  }}
                >
                  <span className="help-key">{item.tip.key}</span>
                  <span className="help-desc">{item.tip.desc}</span>
                </div>
              );
            })}
          </>
        )}
      </div>
    </>
  );
}
