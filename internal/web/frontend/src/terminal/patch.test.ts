import { describe, expect, it } from "vitest";

import { patchPaneHTML, type PaneDOMState } from "./patch";
import { markPreviewablePaths, render } from "./render";

const mark = (root: Node) => markPreviewablePaths(root, "/repo", "peer-a");

function fullRender(text: string): string {
  const pre = document.createElement("pre");
  pre.innerHTML = render(text);
  mark(pre);
  return pre.innerHTML;
}

const FRAME_A = [
  "\x1b[1mtmact\x1b[0m session",
  "see ./shot.png and docs/README.md for details",
  "link: https://example.com/a/b",
  "\x1b[38;5;214m✻ Thinking… (3s)\x1b[0m",
  "┌───┬───┐",
  "│ a │ b │",
  "└───┴───┘",
  "\x1b[2m> \x1b[0mprompt",
].join("\n");
const FRAME_B = FRAME_A.replace("(3s)", "(4s)").replace("✻", "✶");

describe("patchPaneHTML", () => {
  it("produces the same DOM as a full innerHTML render", () => {
    const pre = document.createElement("pre");
    let st: PaneDOMState | null = null;
    for (const text of [FRAME_A, FRAME_B, FRAME_A + "\nmore ./x.jpg", "", FRAME_B]) {
      st = patchPaneHTML(pre, render(text), st, mark);
      expect(pre.innerHTML).toBe(fullRender(text));
    }
  });

  it("keeps unchanged leading and trailing nodes in place", () => {
    const pre = document.createElement("pre");
    const st = patchPaneHTML(pre, render(FRAME_A), null, mark);
    const first = pre.firstChild;
    const last = pre.lastChild;
    const imgPath = pre.querySelector(".image-path");
    patchPaneHTML(pre, render(FRAME_B), st, mark);
    expect(pre.firstChild).toBe(first);
    expect(pre.lastChild).toBe(last);
    expect(pre.querySelector(".image-path")).toBe(imgPath);
  });

  it("reuses shifted nodes when the line window scrolls", () => {
    const lines = Array.from({ length: 40 }, (_, i) =>
      i % 3 === 0 ? `\x1b[3${i % 7}mrow ${i}\x1b[0m plain ${i}` : `row ${i} see ./img${i}.png`,
    );
    const pre = document.createElement("pre");
    let st = patchPaneHTML(pre, render(lines.join("\n")), null, mark);
    const kept = Array.from(pre.querySelectorAll(".image-path")).find(
      (el) => el.textContent === "./img20.png",
    );
    expect(kept).toBeTruthy();
    for (let step = 0; step < 5; step++) {
      lines.shift();
      lines.push(`new ${step} \x1b[1m✻ working\x1b[0m`);
      st = patchPaneHTML(pre, render(lines.join("\n")), st, mark);
      expect(pre.innerHTML).toBe(fullRender(lines.join("\n")));
    }
    const after = Array.from(pre.querySelectorAll(".image-path")).find(
      (el) => el.textContent === "./img20.png",
    );
    expect(after).toBe(kept);
  });

  it("rebuilds fully when pre was changed behind its back", () => {
    const pre = document.createElement("pre");
    const st = patchPaneHTML(pre, render(FRAME_A), null, mark);
    pre.innerHTML = '<span class="empty">Loading…</span>';
    patchPaneHTML(pre, render(FRAME_B), st, mark);
    expect(pre.innerHTML).toBe(fullRender(FRAME_B));
  });

  it("replaces a placeholder on first commit", () => {
    const pre = document.createElement("pre");
    pre.innerHTML = '<span class="empty">No pane selected.</span>';
    patchPaneHTML(pre, render(FRAME_A), null, mark);
    expect(pre.innerHTML).toBe(fullRender(FRAME_A));
  });
});
