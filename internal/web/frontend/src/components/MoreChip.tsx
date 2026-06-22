// MoreChip — the "more" overflow chip at the tail of the statusline. It hides
// idle, agent-less panes behind a click-to-open popover list so a long session
// list stays scannable (the inline chips are the ones that matter: agents,
// the selection, and panes asking for input — see StatusLine.shouldPinPane).
//
// The popover is a plain click-triggered list of Chips reused 1:1 from the
// statusline. Selecting a row switches to that pane and closes the popover;
// clicking outside or pressing Escape also closes it.

import { useEffect, useRef, useState } from "react";
import { onPointerDownNoBlur } from "../lib/dom";
import type { PaneListItem } from "./StatusLine";
import { Chip } from "./Chip";

interface MoreChipProps {
  /** The collapsed (agent-less, idle) panes to list in the popover. */
  items: PaneListItem[];
  onSelect: (paneID: string) => void;
}

export function MoreChip({ items, onSelect }: MoreChipProps) {
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement | null>(null);

  // Close on outside pointerdown (capture so it fires before the row click)
  // and on Escape, but only while open.
  useEffect(() => {
    if (!open) return;
    const onDocPointerDown = (e: Event) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", onDocPointerDown, true);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onDocPointerDown, true);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  // If the overflow set empties (the last hidden pane gained focus/an agent),
  // collapse so an empty popover never lingers.
  useEffect(() => {
    if (items.length === 0) setOpen(false);
  }, [items.length]);

  const count = items.length;

  return (
    <div className="more-chip-wrap" ref={wrapRef}>
      <button
        type="button"
        className={"chip more-chip" + (open ? " open" : "")}
        title={count + " more pane" + (count === 1 ? "" : "s")}
        aria-haspopup="menu"
        aria-expanded={open}
        onPointerDown={onPointerDownNoBlur}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="chip-label">more</span>
        <span className="more-count">{count}</span>
      </button>
      {open ? (
        <div className="chip-overflow-pop" role="menu">
          {items.map(({ pane, label }, i) => (
            <Chip
              key={pane.pane_id || "overflow-" + i}
              pane={pane}
              label={label}
              hotkey={undefined}
              selected={false}
              onSelect={() => {
                onSelect(pane.pane_id ?? "");
                setOpen(false);
              }}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}
