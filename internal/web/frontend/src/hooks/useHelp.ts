// useHelp — React port of static/js/help.js (the `?` coachmark overlay).
//
// help.js was entirely imperative: it built DOM nodes with `h(...)`, measured
// them (getBoundingClientRect / offsetWidth / offsetHeight), and positioned the
// rings/tips/banner with a scoring search. The port keeps that imperative
// measurement-then-position pass (it is inherently a layout-read job), but moves
// the open/close lifecycle, the window-resize reflow, and the Escape handler
// into this hook (the same listeners wireHelp() registered). The DOM nodes are
// rendered by
// <HelpOverlay/>; this hook computes their final `left`/`top` against the live
// target rects, exactly as placeCoachmarks() did.
//
// Parity notes (spec §6 items 78-79):
//   - toggle via #help-btn (pointerdown preventDefault — done in HelpOverlay);
//     close on Escape and on overlay backdrop click; body.help-open class.
//   - tips filtered by skip() (mobile/desktop/disabled/missing element);
//     rings collected before tips; banner is the first `placed` rect.
//   - scoreCoachmark weights: |Δtop|·1 + |Δleft|·0.35, card overlap ·120,
//     ring overlap ·18 (large rings >16% of viewport area are ignored),
//     8 px ring insets, gap 10 / sideGap 12.
//   - reposition on window resize; visualViewport-aware (offset + size).

import { useCallback, useEffect, useRef, useState } from "react";
import { clamp, isMobile } from "../lib/dom";
import { useAppState } from "../store/AppStateContext";

// ---- geometry primitives (verbatim from help.js) ----

export interface Rect {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
}

function overlapArea(a: Rect, b: Rect): number {
  const w = Math.min(a.right, b.right) - Math.max(a.left, b.left);
  const h = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
  return w > 0 && h > 0 ? w * h : 0;
}

function rectAt(left: number, top: number, width: number, height: number): Rect {
  return { left, top, right: left + width, bottom: top + height, width, height };
}

function coachmarkViewport(): Rect {
  const vv = window.visualViewport;
  const left = vv ? vv.offsetLeft : 0;
  const top = vv ? vv.offsetTop : 0;
  const width = vv ? vv.width : window.innerWidth;
  const height = vv ? vv.height : window.innerHeight;
  return {
    left: left + 8,
    top: top + 8,
    right: left + width - 8,
    bottom: top + height - 8,
    width,
    height,
  };
}

function scoreCoachmark(
  rect: Rect,
  preferred: Rect,
  placed: Rect[],
  rings: Rect[],
  vp: Rect,
): number {
  let score =
    Math.abs(rect.top - preferred.top) + Math.abs(rect.left - preferred.left) * 0.35;
  for (const p of placed) score += overlapArea(rect, p) * 120;
  const largeRingArea = vp.width * vp.height * 0.16;
  for (const r of rings) {
    if (r.width * r.height > largeRingArea) continue;
    score += overlapArea(rect, r) * 18;
  }
  return score;
}

interface Pos {
  left: number;
  top: number;
}

function preferredCoachmarkPosition(
  place: string,
  targetRect: Rect,
  cw: number,
  ch: number,
  vp: Rect,
): Pos {
  const gap = 10;
  const sideGap = 12;
  const centeredLeft = targetRect.left + targetRect.width / 2 - cw / 2;
  switch (place) {
    case "inside-top-left":
      return {
        left: targetRect.left + sideGap,
        top: targetRect.top + sideGap,
      };
    case "above-left":
      return {
        left: targetRect.left,
        top: targetRect.top - ch - gap,
      };
    case "above-right":
      return {
        left: targetRect.right - cw,
        top: targetRect.top - ch - gap,
      };
    case "left":
      return {
        left: targetRect.left - cw - sideGap,
        top: targetRect.top + targetRect.height / 2 - ch / 2,
      };
    case "right":
      return {
        left: targetRect.right + sideGap,
        top: targetRect.top + targetRect.height / 2 - ch / 2,
      };
    default: {
      const nearBottom = targetRect.bottom > vp.top + vp.height * 0.58;
      return {
        left: centeredLeft,
        top: nearBottom ? targetRect.top - ch - gap : targetRect.bottom + gap,
      };
    }
  }
}

// placeCoachmarkCard measures the (already-rendered) card element, runs the
// candidate search, writes the winning left/top onto card.style, and pushes the
// chosen rect into `placed` — exactly as help.js did.
function placeCoachmarkCard(
  card: HTMLElement,
  tip: HelpTip,
  targetRect: Rect,
  placed: Rect[],
  rings: Rect[],
  vp: Rect,
): void {
  const cw = card.offsetWidth;
  const ch = card.offsetHeight;
  const gap = 10;
  const sideGap = 12;
  const centeredLeft = targetRect.left + targetRect.width / 2 - cw / 2;
  const rightSideTarget = isMobile() && targetRect.left > vp.left + vp.width * 0.55;
  const defaultPlace = rightSideTarget ? "left" : "";
  const preferredPos = preferredCoachmarkPosition(
    tip.place || defaultPlace,
    targetRect,
    cw,
    ch,
    vp,
  );
  const preferredTop = preferredPos.top;
  const preferredLeft = preferredPos.left;
  const xCandidates = [
    preferredLeft,
    targetRect.left - cw - sideGap,
    targetRect.right + sideGap,
    centeredLeft,
    targetRect.left,
    targetRect.right - cw,
  ];
  const ySeeds = [
    preferredTop,
    targetRect.bottom + gap,
    targetRect.top - ch - gap,
    targetRect.top + sideGap,
    targetRect.top + targetRect.height / 2 - ch / 2,
  ];
  const candidates: Rect[] = [];
  const add = (left: number, top: number): void => {
    const clampedLeft = clamp(left, vp.left, vp.right - cw);
    const clampedTop = clamp(top, vp.top, vp.bottom - ch);
    candidates.push(rectAt(clampedLeft, clampedTop, cw, ch));
  };

  for (const x of xCandidates) {
    for (const y of ySeeds) add(x, y);
    for (let y = vp.top; y <= vp.bottom - ch; y += 12) add(x, y);
  }

  const preferred = rectAt(
    clamp(preferredLeft, vp.left, vp.right - cw),
    clamp(preferredTop, vp.top, vp.bottom - ch),
    cw,
    ch,
  );
  let best = preferred;
  let bestScore = scoreCoachmark(preferred, preferred, placed, rings, vp);
  for (const c of candidates) {
    const score = scoreCoachmark(c, preferred, placed, rings, vp);
    if (score < bestScore) {
      best = c;
      bestScore = score;
    }
  }
  card.style.left = best.left + "px";
  card.style.top = best.top + "px";
  placed.push(best);
}

// ---- tip catalog ----

export interface HelpTip {
  // The original keyed each tip by a target() thunk; the port carries the
  // target element id and resolves it via getElementById, so HelpOverlay can
  // both filter (skip) and measure (rect) without re-deriving thunks.
  targetId: string;
  key: string;
  desc: string;
  tone: string;
  place?: string;
  skip?: () => boolean;
}

// A resolved tip ready for ring/card emission: the target's measured rect plus
// the original tip metadata.
export interface PlacedTip {
  tip: HelpTip;
  rect: Rect;
}

// helpTips mirrors help.js helpTips() exactly, including skip() predicates.
// `state` is read live (mutated in place by the store) for the .selected gates.
function buildHelpTips(state: { selected: string | null }): HelpTip[] {
  return [
    {
      targetId: "chips",
      key: "Option+1…0, q…p",
      desc: "Switch panes (hardware keyboard)",
      tone: "pane",
      place: "above-left",
      skip: () => isMobile() || !chipsHaveChildren(),
    },
    {
      targetId: "draft",
      key: "⌘ / Ctrl + Enter",
      desc: "Send the typed prompt to the selected pane",
      tone: "send",
      place: "above-left",
      skip: () => isMobile(),
    },
    {
      targetId: "send-btn",
      key: "Tap Send",
      desc: "Send the typed prompt to the selected pane",
      tone: "send",
      skip: () => !isMobile() || !state.selected,
    },
    {
      targetId: "record-btn",
      key: "Option+V, then V/C",
      desc: "Record voice, then send or cancel",
      tone: "voice",
      place: "above-right",
      skip: () => isMobile(),
    },
    {
      targetId: "clear-pane-btn",
      key: "⌘K / Ctrl+L",
      desc: "Clear the pane and tmux scrollback",
      tone: "clear",
      place: "above-right",
      skip: () => !state.selected,
    },
    {
      targetId: "content",
      key: "Click pane",
      desc: "Direct mode — your keystrokes go straight to tmux",
      tone: "direct",
      place: "inside-top-left",
      skip: () => isMobile(),
    },
    {
      targetId: "qb-fab",
      key: "Tap ⚡",
      desc: "Quick prompts — configurable in Settings",
      tone: "quick",
    },
    {
      targetId: "upload-btn",
      key: "Tap upload",
      desc: "Upload a file and paste its server path",
      tone: "upload",
      place: "left",
      skip: () => !state.selected,
    },
    {
      targetId: "selection-btn",
      key: "Tap select",
      desc: "Toggle pane selection mode",
      tone: "selection",
      place: "left",
      skip: () => !state.selected,
    },
    {
      targetId: "gear-btn",
      key: "Gear",
      desc: "Settings — quick buttons, voice model, font size",
      tone: "settings",
      place: "left",
    },
  ];
}

// chipsHavechildren mirrors help.js's `$("chips").children.length`. #chips
// always exists in the React tree, so a missing element falls back to 0.
function chipsHaveChildren(): boolean {
  const el = document.getElementById("chips");
  return !!el && el.children.length > 0;
}

// resolveTips runs the help.js placeCoachmarks() pre-pass: drop skipped tips,
// drop tips whose target is missing or collapsed (0×0 rect), then sort by top.
// Returns the resolved (tip, rect) list in draw order.
function resolveTips(state: { selected: string | null }): PlacedTip[] {
  const items: PlacedTip[] = [];
  for (const tip of buildHelpTips(state)) {
    if (tip.skip && tip.skip()) continue;
    const el = document.getElementById(tip.targetId);
    if (!el) continue;
    const r = el.getBoundingClientRect();
    // Display:none and detached elements both produce a 0×0 rect.
    if (r.width < 1 || r.height < 1) continue;
    items.push({
      tip,
      rect: rectAt(r.left, r.top, r.width, r.height),
    });
  }
  items.sort((a, b) => a.rect.top - b.rect.top);
  return items;
}

export interface UseHelp {
  open: boolean;
  // The resolved tips for the current open pass (empty when closed). HelpOverlay
  // renders one ring + one tip card per entry, in this order.
  items: PlacedTip[];
  toggle: () => void;
  close: () => void;
  // place runs the imperative measure-then-position pass for the rendered cards.
  // HelpOverlay calls it from a useLayoutEffect after the rings/tips/banner are
  // in the DOM, passing the live element refs. nonce increments on every
  // (re)placement request (open + each window resize) so the effect re-runs.
  nonce: number;
  place: (refs: PlaceRefs) => void;
}

export interface PlaceRefs {
  banner: HTMLElement | null;
  cards: (HTMLElement | null)[]; // one per `items` entry, same order
}

// useHelp owns the open/close lifecycle, the resolved-tip snapshot, and the
// resize + Escape listeners (the same two wireHelp() registered). It does NOT
// own the DOM nodes — HelpOverlay renders them and calls place() to position
// the cards.
export function useHelp(): UseHelp {
  const { state } = useAppState();
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<PlacedTip[]>([]);
  // nonce forces HelpOverlay's placement layout-effect to re-run on open and on
  // each window resize — matching help.js re-running placeCoachmarks() on resize.
  const [nonce, setNonce] = useState(0);
  const openRef = useRef(open);
  openRef.current = open;

  // openHelp: snapshot the resolvable tips (reads live rects + state.selected),
  // add body.help-open, and bump nonce so the card placement runs.
  const doOpen = useCallback(() => {
    document.body.classList.add("help-open");
    setItems(resolveTips(state));
    setOpen(true);
    setNonce((n) => n + 1);
  }, [state]);

  const close = useCallback(() => {
    document.body.classList.remove("help-open");
    setOpen(false);
    setItems([]);
  }, []);

  const toggle = useCallback(() => {
    if (openRef.current) close();
    else doOpen();
  }, [close, doOpen]);

  // place: imperative measure-then-position for the rendered cards. Mirrors
  // placeCoachmarks()'s second loop: banner is the first `placed` rect, rings
  // are gathered from the items' rects (already drawn by HelpOverlay), then each
  // card is positioned in order, mutating `placed`.
  const place = useCallback(
    (refs: PlaceRefs) => {
      const banner = refs.banner;
      if (!banner) return;
      const placed: Rect[] = [];
      const br = banner.getBoundingClientRect();
      placed.push(rectAt(br.left, br.top, br.width, br.height));
      const rings: Rect[] = [];
      for (const item of items) {
        const r = item.rect;
        rings.push(rectAt(r.left - 4, r.top - 4, r.width + 8, r.height + 8));
      }
      const vp = coachmarkViewport();
      for (let i = 0; i < items.length; i++) {
        const item = items[i];
        const card = refs.cards[i];
        if (!item || !card) continue;
        placeCoachmarkCard(card, item.tip, item.rect, placed, rings, vp);
      }
    },
    [items],
  );

  // Escape closes (only while open) — capture-free, bubble phase, as in help.js.
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === "Escape" && document.body.classList.contains("help-open")) close();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [close]);

  // Reposition on window resize while open. help.js wireHelp() added exactly
  // one listener: window "resize" → placeCoachmarks() (when body.help-open),
  // which re-resolves every target rect from scratch. We mirror that — re-snapshot
  // items (fresh rects) and bump nonce to re-run the card placement. (The
  // visualViewport awareness lives in coachmarkViewport(), which reads
  // window.visualViewport for bounds; the original did not subscribe to vv
  // events, so neither do we.)
  useEffect(() => {
    if (!open) return;
    const reflow = (): void => {
      if (!document.body.classList.contains("help-open")) return;
      setItems(resolveTips(state));
      setNonce((n) => n + 1);
    };
    window.addEventListener("resize", reflow);
    return () => {
      window.removeEventListener("resize", reflow);
    };
  }, [open, state]);

  return { open, items, toggle, close, nonce, place };
}
