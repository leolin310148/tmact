// MoreChip — the "more" overflow chip at the tail of the statusline. It hides
// idle, agent-less panes behind a click-to-open popover list so a long session
// list stays scannable (the inline chips are the ones that matter: agents,
// the selection, and panes asking for input — see StatusLine.shouldPinPane).
//
// The popover reuses the statusline Chips with menu-item semantics and complete
// keyboard traversal. Selecting a row switches to that pane and closes the
// popover; clicking outside or pressing Escape also closes it.

import {
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { focusMenuEdge, moveMenuFocus, onPointerDownNoBlur } from "../lib/dom";
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
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const menuRef = useRef<HTMLDivElement | null>(null);
  const pendingFocusRef = useRef<"first" | "last" | null>(null);
  const buttonID = useId();
  const menuID = useId();

  const openFromKeyboard = (edge: "first" | "last") => {
    pendingFocusRef.current = edge;
    setOpen(true);
  };

  useLayoutEffect(() => {
    if (!open || !pendingFocusRef.current || !menuRef.current) return;
    focusMenuEdge(menuRef.current, pendingFocusRef.current);
    pendingFocusRef.current = null;
  }, [open, items.length]);

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
      if (e.key === "Escape") {
        e.preventDefault();
        const restoreTrigger = menuRef.current?.contains(document.activeElement) ?? false;
        pendingFocusRef.current = null;
        setOpen(false);
        if (restoreTrigger) buttonRef.current?.focus();
      }
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

  const onButtonKeyDown = (e: ReactKeyboardEvent<HTMLButtonElement>) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      openFromKeyboard(e.key === "ArrowUp" ? "last" : "first");
      return;
    }
    if (!open && (e.key === "Enter" || e.key === " ")) {
      e.preventDefault();
      openFromKeyboard("first");
    }
  };

  const onMenuKeyDown = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    if (!moveMenuFocus(e.currentTarget, e.key)) return;
    e.preventDefault();
  };

  return (
    <div className="more-chip-wrap" ref={wrapRef}>
      <button
        ref={buttonRef}
        id={buttonID}
        type="button"
        className={"chip more-chip" + (open ? " open" : "")}
        title={count + " more pane" + (count === 1 ? "" : "s")}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={menuID}
        aria-label={"Show " + count + " more pane" + (count === 1 ? "" : "s")}
        onPointerDown={onPointerDownNoBlur}
        onClick={() => {
          pendingFocusRef.current = null;
          setOpen((v) => !v);
        }}
        onKeyDown={onButtonKeyDown}
      >
        <span className="chip-label">more</span>
        <span className="more-count">{count}</span>
      </button>
      {open ? (
        <div
          ref={menuRef}
          id={menuID}
          className="chip-overflow-pop"
          role="menu"
          aria-labelledby={buttonID}
          onKeyDown={onMenuKeyDown}
        >
          {items.map(({ pane, label }, i) => (
            <Chip
              key={pane.pane_id || "overflow-" + i}
              pane={pane}
              label={label}
              hotkey={undefined}
              selected={false}
              menuItem
              onSelect={() => {
                const restoreTrigger =
                  menuRef.current?.contains(document.activeElement) ?? false;
                onSelect(pane.pane_id ?? "");
                setOpen(false);
                if (restoreTrigger) buttonRef.current?.focus();
              }}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}
