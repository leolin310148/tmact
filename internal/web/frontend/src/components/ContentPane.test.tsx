// ContentPane unit coverage. The component owns load-bearing IMPERATIVE behavior
// (ARCHITECTURE §3/§7): React renders a CHILDLESS <pre id="content"> and a
// useLayoutEffect writes pre.innerHTML = render(text), runs markImagePaths, then
// restores scroll. These assertions pin the parts that DON'T need a layout engine:
//   1. the mount placeholder gate (settledRef) paints index.html's placeholder
//      before App's first setContent, and never reconciles it as React children;
//   2. a later text change overwrites innerHTML with the PURE renderer's output —
//      including the trailing-blank trim (end-to-end through render());
//   3. markImagePaths runs AFTER the write, wrapping an image path in a
//      span.image-path with data-path/data-cwd.
//
// NOT covered here: the atBottom stick-to-bottom auto-scroll (ContentPane.tsx
// useLayoutEffect). jsdom has no layout engine — scrollHeight/scrollTop/
// clientHeight are all 0 — so "short pane => no scroll / long pane sticks to
// bottom" is verifiable only in a real browser (borz E2E / docs/smoke-test.md).

import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import ContentPane from "./ContentPane";

afterEach(cleanup);

type Props = Parameters<typeof ContentPane>[0];

function mount(overrides: Partial<Props> = {}) {
  const props: Props = {
    text: "",
    cwd: null,
    peer: null,
    markdown: false,
    selectionMode: false,
    onPreviewImage: vi.fn(),
    onRefocusDirect: vi.fn(),
    onBlurDirect: vi.fn(),
    ...overrides,
  };
  const r = render(<ContentPane {...props} />);
  const pre = document.getElementById("content") as HTMLPreElement;
  return {
    pre,
    rerender: (p: Partial<Props>) => r.rerender(<ContentPane {...props} {...p} />),
  };
}

describe("ContentPane", () => {
  it("paints index.html's placeholder on mount, before any setContent", () => {
    // Even with a non-empty `text`, the first layout-effect run is the mount
    // gate (settledRef) — it writes the placeholder and returns, exactly as
    // app.js left the static index.html placeholder until the first setContent.
    const { pre } = mount({ text: "should not render on first run" });
    expect(pre.innerHTML).toBe('<span class="empty">No pane selected.</span>');
  });

  it("overwrites innerHTML with the trimmed render() output on a content change", () => {
    const { pre, rerender } = mount({ text: "" });
    rerender({ text: "prompt\n❯\n\n\n" }); // trailing blank rows (tmux padding)
    expect(pre.textContent).toBe("prompt\n❯"); // trimmed end-to-end via render()
    expect(pre.querySelector(".empty")).toBeNull(); // placeholder is gone
  });

  it("marks an image path as a previewable span after the render write", () => {
    const { pre, rerender } = mount({ text: "" });
    rerender({ text: "/abs/shot.png", cwd: "/work", peer: "" });
    const span = pre.querySelector(".image-path") as HTMLElement | null;
    expect(span).not.toBeNull();
    expect(span?.dataset.path).toBe("/abs/shot.png");
    expect(span?.dataset.cwd).toBe("/work");
  });

  it("re-renders pane output as a table when markdown flips on", () => {
    // Mount paints the placeholder (settledRef); first real setContent is a rerender.
    const { pre, rerender } = mount({ text: "" });
    rerender({ text: "a | b\nc | d", markdown: false });
    // Raw view: pipes stay as plain text, no table.
    expect(pre.querySelector("table")).toBeNull();
    expect(pre.textContent).toContain("a | b");
    // Toggling markdown on re-runs render() with { markdown: true }.
    rerender({ text: "a | b\nc | d", markdown: true });
    const table = pre.querySelector("table.tui-table");
    expect(table).not.toBeNull();
    expect(pre.querySelector("td")?.textContent).toBe("a");
  });
});
