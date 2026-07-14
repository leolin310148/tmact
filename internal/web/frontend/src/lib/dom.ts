// Pure DOM/UI helpers ported 1:1 from static/js/dom.js.
//
// React uses refs/JSX instead of the original `$` (getElementById) and `h`
// (createElement) helpers, so those are intentionally NOT ported here. Only the
// pure helpers the React port still needs are kept, plus one shared UI helper
// (`onPointerDownNoBlur`) used by buttons to keep the mobile soft keyboard up.

// isMobile mirrors dom.js exactly: phone-width media query.
export const isMobile = (): boolean =>
  window.matchMedia("(max-width: 760px)").matches;

// escapeHTML escapes only & < > (verbatim from dom.js — quotes are NOT escaped).
export function escapeHTML(s: string): string {
  return s.replace(/[&<>]/g, (c) => (c === "&" ? "&amp;" : c === "<" ? "&lt;" : "&gt;"));
}

export function escapeAttribute(s: string): string {
  return s.replace(/[&<>"']/g, (c) => {
    switch (c) {
      case "&":
        return "&amp;";
      case "<":
        return "&lt;";
      case ">":
        return "&gt;";
      case '"':
        return "&quot;";
      default:
        return "&#39;";
    }
  });
}

// clamp mirrors dom.js: when max < min it returns min, otherwise clamps n into
// [min, max].
export function clamp(n: number, min: number, max: number): number {
  if (max < min) return min;
  return Math.max(min, Math.min(max, n));
}

// onPointerDownNoBlur is attached to buttons' onPointerDown so that tapping them
// does not blur the focused input (which would dismiss the mobile soft
// keyboard). The original wired this as
//   btn.addEventListener("pointerdown", (e) => e.preventDefault());
// on every action button (spec §6 item 30).
export function onPointerDownNoBlur(e: { preventDefault: () => void }): void {
  e.preventDefault();
}

function menuItems(menu: ParentNode): HTMLElement[] {
  return Array.from(menu.querySelectorAll<HTMLElement>('[role="menuitem"]'));
}

export function focusMenuEdge(menu: ParentNode | null, edge: "first" | "last"): void {
  if (!menu) return;
  const items = menuItems(menu);
  items[edge === "last" ? items.length - 1 : 0]?.focus();
}

export function moveMenuFocus(menu: ParentNode, key: string): boolean {
  if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(key)) return false;
  const items = menuItems(menu);
  if (items.length === 0) return true;
  const current = items.indexOf(document.activeElement as HTMLElement);
  let next = 0;
  if (key === "End") next = items.length - 1;
  else if (key === "Home") next = 0;
  else if (key === "ArrowDown") next = (current + 1) % items.length;
  else next = current < 0 ? items.length - 1 : (current - 1 + items.length) % items.length;
  items[next]?.focus();
  return true;
}
