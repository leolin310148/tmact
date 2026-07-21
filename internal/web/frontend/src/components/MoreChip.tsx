// MoreChip — the "more" overflow chip at the tail of the statusline. It hides
// idle, agent-less panes behind a click-to-open popover list so a long session
// list stays scannable (the inline chips are the ones that matter: agents,
// the selection, and panes asking for input — see StatusLine.shouldPinPane).
//
// The popover is the shared OverflowMenuContent (also used by the office
// layout's floor-lamp trigger): rich rows with a session-exit button plus the
// recently-closed history. The chip renders even with zero hidden panes so the
// history stays reachable; the count badge only appears when panes are hidden.

import { useId } from "react";
import type { PaneListItem } from "./StatusLine";
import { OverflowMenuContent, useMenuPopover } from "./OverflowMenu";

interface MoreChipProps {
  /** The collapsed (agent-less, idle) panes to list in the popover. */
  items: PaneListItem[];
  onSelect: (paneID: string) => void;
}

export function MoreChip({ items, onSelect }: MoreChipProps) {
  const pop = useMenuPopover(true);
  const buttonID = useId();
  const menuID = useId();

  const count = items.length;
  const label =
    count > 0
      ? "Show " + count + " more pane" + (count === 1 ? "" : "s") + " and recently closed sessions"
      : "Show recently closed sessions";

  return (
    <div className="more-chip-wrap">
      <button
        ref={pop.buttonRef}
        id={buttonID}
        type="button"
        className={"chip more-chip" + (pop.open ? " open" : "")}
        title={label}
        aria-haspopup="menu"
        aria-expanded={pop.open}
        aria-controls={menuID}
        aria-label={label}
        onPointerDown={(e) => e.preventDefault()}
        onClick={pop.onTriggerClick}
        onKeyDown={pop.onTriggerKeyDown}
      >
        <span className="chip-label">more</span>
        {count > 0 ? <span className="more-count">{count}</span> : null}
      </button>
      {pop.open ? (
        <div
          ref={pop.menuRef}
          id={menuID}
          className="chip-overflow-pop ovf-pop"
          role="menu"
          aria-labelledby={buttonID}
          onKeyDown={pop.onMenuKeyDown}
        >
          <OverflowMenuContent items={items} onSelect={onSelect} closeRestoring={pop.closeRestoring} />
        </div>
      ) : null}
    </div>
  );
}
