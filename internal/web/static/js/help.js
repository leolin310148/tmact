// Help / coachmark overlay.
// The `?` button stacked above the FAB lifts a dimmed overlay that points
// at the main controls with their hotkey, so newcomers don't have to guess.
// Tips with `skip` true (mobile-only feature on desktop, or vice-versa, or a
// disabled control) are dropped before drawing.

import { $, clamp, h, isMobile } from "./dom.js";
import { state } from "./state.js";

function helpTips() {
  return [
    { target: () => $("chips"),
      key: "Option+1…0, q…p",
      desc: "Switch panes (hardware keyboard)",
      tone: "pane",
      place: "above-left",
      skip: () => isMobile() || !$("chips").children.length },
    { target: () => $("draft"),
      key: "⌘ / Ctrl + Enter",
      desc: "Send the typed prompt to the selected pane",
      tone: "send",
      place: "above-left",
      skip: () => isMobile() },
    { target: () => $("send-btn"),
      key: "Tap Send",
      desc: "Send the typed prompt to the selected pane",
      tone: "send",
      skip: () => !isMobile() || !state.selected },
    { target: () => $("record-btn"),
      key: "Option+V, then V/C",
      desc: "Record voice, then send or cancel",
      tone: "voice",
      place: "above-right",
      skip: () => isMobile() },
    { target: () => $("content"),
      key: "Click pane",
      desc: "Direct mode — your keystrokes go straight to tmux",
      tone: "direct",
      place: "inside-top-left",
      skip: () => isMobile() },
    { target: () => $("qb-fab"),
      key: "Tap ⚡",
      desc: "Quick prompts — configurable in Settings",
      tone: "quick" },
    { target: () => $("upload-btn"),
      key: "Tap upload",
      desc: "Upload a file and paste its server path",
      tone: "upload",
      place: "left",
      skip: () => !state.selected },
    { target: () => $("selection-btn"),
      key: "Tap select",
      desc: "Toggle pane selection mode",
      tone: "selection",
      place: "left",
      skip: () => !state.selected },
    { target: () => $("gear-btn"),
      key: "Gear",
      desc: "Settings — quick buttons, voice model, font size",
      tone: "settings",
      place: "left" },
  ];
}

function overlapArea(a, b) {
  const w = Math.min(a.right, b.right) - Math.max(a.left, b.left);
  const h = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
  return w > 0 && h > 0 ? w * h : 0;
}

function rectAt(left, top, width, height) {
  return { left, top, right: left + width, bottom: top + height, width, height };
}

function coachmarkViewport() {
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

function scoreCoachmark(rect, preferred, placed, rings, vp) {
  let score = Math.abs(rect.top - preferred.top) + Math.abs(rect.left - preferred.left) * 0.35;
  for (const p of placed) score += overlapArea(rect, p) * 120;
  const largeRingArea = vp.width * vp.height * 0.16;
  for (const r of rings) {
    if (r.width * r.height > largeRingArea) continue;
    score += overlapArea(rect, r) * 18;
  }
  return score;
}

function preferredCoachmarkPosition(place, targetRect, cw, ch, vp) {
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

function placeCoachmarkCard(card, tip, targetRect, placed, rings, vp) {
  const cw = card.offsetWidth;
  const ch = card.offsetHeight;
  const gap = 10;
  const sideGap = 12;
  const centeredLeft = targetRect.left + targetRect.width / 2 - cw / 2;
  const rightSideTarget = isMobile() && targetRect.left > vp.left + vp.width * 0.55;
  const defaultPlace = rightSideTarget ? "left" : "";
  const preferredPos = preferredCoachmarkPosition(tip.place || defaultPlace, targetRect, cw, ch, vp);
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
  const candidates = [];
  const add = (left, top) => {
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

function placeCoachmarks() {
  const overlay = $("help-overlay");
  for (const el of overlay.querySelectorAll(".help-ring, .help-tip, .help-banner")) el.remove();
  overlay.appendChild(h("div", { class: "help-banner",
    text: "Hotkey hints · tap anywhere or press Esc to close" }));
  const banner = overlay.querySelector(".help-banner");
  const placed = [banner.getBoundingClientRect()];
  const rings = [];
  const items = [];
  for (const tip of helpTips()) {
    if (tip.skip && tip.skip()) continue;
    const el = tip.target();
    if (!el) continue;
    const r = el.getBoundingClientRect();
    // Display:none and detached elements both produce a 0×0 rect.
    if (r.width < 1 || r.height < 1) continue;
    items.push({ tip, rect: r });
  }
  items.sort((a, b) => a.rect.top - b.rect.top);
  for (const item of items) {
    const r = item.rect;
    const tone = item.tip.tone ? " tone-" + item.tip.tone : "";
    const ring = h("div", { class: "help-ring" + tone });
    ring.style.left = (r.left - 4) + "px";
    ring.style.top = (r.top - 4) + "px";
    ring.style.width = (r.width + 8) + "px";
    ring.style.height = (r.height + 8) + "px";
    overlay.appendChild(ring);
    rings.push(rectAt(r.left - 4, r.top - 4, r.width + 8, r.height + 8));
  }
  const vp = coachmarkViewport();
  for (const item of items) {
    const r = item.rect;
    const tip = item.tip;
    const tone = tip.tone ? " tone-" + tip.tone : "";
    const card = h("div", { class: "help-tip" + tone },
      h("span", { class: "help-key", text: tip.key }),
      h("span", { class: "help-desc", text: tip.desc }));
    overlay.appendChild(card);
    placeCoachmarkCard(card, tip, r, placed, rings, vp);
  }
}

function openHelp() {
  document.body.classList.add("help-open");
  placeCoachmarks();
}

function closeHelp() {
  document.body.classList.remove("help-open");
  const overlay = $("help-overlay");
  for (const el of overlay.querySelectorAll(".help-ring, .help-tip, .help-banner")) el.remove();
}

function toggleHelp() {
  if (document.body.classList.contains("help-open")) closeHelp();
  else openHelp();
}

export function wireHelp() {
  // Mirror the qb-fab pattern: pointerdown preventDefault keeps the focused
  // input focused. iOS Safari rearranges its chrome (URL bar, safe-area)
  // whenever focus moves, and fitViewport() then pins body to the new
  // visualViewport.height — the visible safe-area padding under the input
  // bar disappears after a toggle cycle if we let the focus shift.
  $("help-btn").addEventListener("pointerdown", (e) => e.preventDefault());
  $("help-btn").addEventListener("click", (e) => { e.stopPropagation(); toggleHelp(); });
  $("help-overlay").addEventListener("pointerdown", (e) => e.preventDefault());
  $("help-overlay").addEventListener("click", closeHelp);
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && document.body.classList.contains("help-open")) closeHelp();
  });
  window.addEventListener("resize", () => {
    if (document.body.classList.contains("help-open")) placeCoachmarks();
  });
}
