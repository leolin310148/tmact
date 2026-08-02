// Active-slot highlight in the split shell.
//
// The shell's own `blur` handler only fires on the FIRST hop out of the parent
// document, so slot→slot focus moves are reported by the slot documents over
// postMessage (main.tsx). These tests cover that path: the highlight must
// follow the announcing iframe and must be ignored for anything that is not a
// live slot window.

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { initSplitShell } from "./split";

let root: HTMLElement;
let disposeShell: () => void;

function slots(): HTMLElement[] {
  return Array.from(document.querySelectorAll<HTMLElement>("#split-grid .slot"));
}

/** Nth slot (1-based) plus its iframe window — both asserted present. */
function slotAt(n: number): { el: HTMLElement; win: MessageEventSource } {
  const el = slots()[n - 1];
  if (!el) throw new Error(`slot ${n} missing`);
  const win = el.querySelector("iframe")?.contentWindow;
  if (!win) throw new Error(`slot ${n} has no iframe window`);
  return { el, win };
}

function announceFocus(source: MessageEventSource | null): void {
  window.dispatchEvent(
    new MessageEvent("message", {
      data: { tmact: "slot-focus" },
      origin: window.location.origin,
      source,
    }),
  );
}

beforeEach(() => {
  localStorage.clear();
  root = document.createElement("div");
  root.id = "root";
  document.body.appendChild(root);
  disposeShell = initSplitShell(root);
});

afterEach(() => {
  // Removing the element is not enough: the shell's React island and its
  // document/window listeners outlive it, and React's scheduler then keeps
  // working against a torn-down environment.
  disposeShell();
  root.remove();
  localStorage.clear();
});

describe("split shell active-slot highlight", () => {
  it("moves the highlight to whichever slot announces focus", () => {
    const first = slotAt(1);
    const second = slotAt(2);

    announceFocus(first.win);
    expect(first.el.classList.contains("active")).toBe(true);
    expect(second.el.classList.contains("active")).toBe(false);

    // The regression: without the message path the parent never hears about
    // this hop, so slot 1 kept its top edge lit alongside slot 2.
    announceFocus(second.win);
    expect(first.el.classList.contains("active")).toBe(false);
    expect(second.el.classList.contains("active")).toBe(true);
  });

  it("ignores a focus claim from a window that is not one of the slots", () => {
    const first = slotAt(1);
    const second = slotAt(2);
    announceFocus(first.win);

    announceFocus(window);

    expect(first.el.classList.contains("active")).toBe(true);
    expect(second.el.classList.contains("active")).toBe(false);
  });

  // Regression: the shell used to leak its React island, its document/window
  // listeners, and its stylesheet on every mount, because initSplitShell had
  // no way to undo itself. In tests that left React's scheduler working
  // against a torn-down jsdom, surfacing as a stray "window is not defined".
  it("removes its listeners and stylesheet when disposed", () => {
    const shellStyles = (): HTMLStyleElement[] =>
      Array.from(document.head.querySelectorAll("style")).filter((el) =>
        el.textContent?.includes("#split-grid"),
      );
    const first = slotAt(1);
    announceFocus(first.win);
    expect(first.el.classList.contains("active")).toBe(true);
    expect(shellStyles()).toHaveLength(1);

    disposeShell();

    expect(shellStyles()).toHaveLength(0);
    first.el.classList.remove("active");
    announceFocus(first.win);
    expect(first.el.classList.contains("active")).toBe(false);

    // afterEach disposes again; that must not throw or warn.
    disposeShell();
  });

  it("ignores messages from another origin", () => {
    const first = slotAt(1);
    window.dispatchEvent(
      new MessageEvent("message", {
        data: { tmact: "slot-focus" },
        origin: "https://elsewhere.example",
        source: first.win,
      }),
    );

    expect(first.el.classList.contains("active")).toBe(false);
  });
});
