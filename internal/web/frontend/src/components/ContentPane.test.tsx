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

import { cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import ContentPane from "./ContentPane";

vi.mock("mermaid", () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn(async (_id: string, source: string) => ({
      svg: `<svg data-testid="pane-mermaid-svg"><text>${source}</text></svg>`,
      bindFunctions: vi.fn(),
    })),
  },
}));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.useRealTimers();
});

type Props = Parameters<typeof ContentPane>[0];

function mount(overrides: Partial<Props> = {}) {
  const props: Props = {
    paneID: null,
    text: "",
    cwd: null,
    peer: null,
    markdown: false,
    selectionMode: false,
    onPreviewImage: vi.fn(),
    onPreviewMarkdown: vi.fn(),
    onScrollTop: vi.fn(),
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

function selectText(node: Node): Selection {
  const range = document.createRange();
  range.selectNodeContents(node);
  const selection = window.getSelection();
  if (!selection) throw new Error("Selection API unavailable");
  selection.removeAllRanges();
  selection.addRange(range);
  return selection;
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

  it("opens image preview on Ctrl+click for Windows and Linux browsers", () => {
    const onPreviewImage = vi.fn();
    const { pre, rerender } = mount({ text: "", onPreviewImage });
    rerender({ text: "/abs/shot.png", cwd: "/work", peer: "peer-a", onPreviewImage });
    const span = pre.querySelector(".image-path") as HTMLElement;

    fireEvent.click(span, { ctrlKey: true });

    expect(onPreviewImage).toHaveBeenCalledWith("/abs/shot.png", "/work", "peer-a");
  });

  it("opens markdown preview on Ctrl+click for Windows and Linux browsers", () => {
    const onPreviewMarkdown = vi.fn();
    const { pre, rerender } = mount({ text: "", onPreviewMarkdown });
    rerender({ text: "/abs/README.md", cwd: "/work", peer: "peer-a", onPreviewMarkdown });
    const span = pre.querySelector(".markdown-path") as HTMLElement;

    fireEvent.click(span, { ctrlKey: true });

    expect(onPreviewMarkdown).toHaveBeenCalledWith("/abs/README.md", "/work", "peer-a");
  });

  it("notifies App when the pane scrolls", () => {
    const onScrollTop = vi.fn();
    const { pre } = mount({ onScrollTop });

    fireEvent.scroll(pre);

    expect(onScrollTop).toHaveBeenCalledTimes(1);
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

  it("renders pane mermaid fences when markdown flips on", async () => {
    const text = "```mermaid\nflowchart LR\n  A --> B\n```";
    const { pre, rerender } = mount({ text: "" });

    rerender({ text, markdown: true });

    await waitFor(() => expect(pre.querySelector('[data-testid="pane-mermaid-svg"]')).not.toBeNull());
    expect(pre.querySelector(".markdown-preview-mermaid")?.getAttribute("data-mermaid-state")).toBe("rendered");
  });

  it("keeps the clicked DOM target stable from pointerdown through click dispatch", () => {
    const onPreviewImage = vi.fn();
    const { pre, rerender } = mount({ paneID: "%1", text: "", onPreviewImage });
    rerender({ paneID: "%1", text: "/work/shot.png", cwd: "/work", onPreviewImage });
    const target = pre.querySelector(".image-path") as HTMLElement;

    fireEvent.pointerDown(target, { pointerType: "mouse" });
    fireEvent.pointerUp(target, { pointerType: "mouse" });
    rerender({ paneID: "%1", text: "newest frame", cwd: "/work", onPreviewImage });

    expect(target.isConnected).toBe(true);
    expect(pre.querySelector(".image-path")).toBe(target);
    expect(document.querySelector('[role="status"]')).toHaveTextContent(
      "Live updates paused while selecting",
    );

    fireEvent.click(target, { ctrlKey: true });

    expect(onPreviewImage).toHaveBeenCalledWith("/work/shot.png", "/work", "");
    expect(pre.textContent).toBe("newest frame");
    expect(target.isConnected).toBe(false);
    expect(document.querySelector('[role="status"]')).toBeNull();
  });

  it("releases a pointer lock when the gesture is canceled outside the pane", () => {
    const { pre, rerender } = mount({ paneID: "%1", text: "" });
    rerender({ paneID: "%1", text: "stable frame" });
    fireEvent.pointerDown(pre, { pointerType: "touch" });
    rerender({ paneID: "%1", text: "latest frame" });

    fireEvent.pointerCancel(document.body, { pointerType: "touch" });

    expect(pre.textContent).toBe("latest frame");
    expect(document.querySelector('[role="status"]')).toBeNull();
  });

  it("retains a browser selection while incoming frames keep arriving", () => {
    const { pre, rerender } = mount({ paneID: "%1", text: "" });
    rerender({ paneID: "%1", text: "copy this text" });
    const textNode = pre.firstChild as Node;
    const selection = selectText(textNode);

    rerender({ paneID: "%1", text: "incoming replacement" });

    expect(pre.firstChild).toBe(textNode);
    expect(selection.toString()).toBe("copy this text");
    expect(pre.textContent).toBe("copy this text");
    expect(document.querySelector('[role="status"]')).toHaveAttribute("aria-live", "polite");
  });

  it("flushes only the newest pending frame when the pane selection collapses", () => {
    const { pre, rerender } = mount({ paneID: "%1", text: "" });
    rerender({ paneID: "%1", text: "selected text" });
    const selection = selectText(pre.firstChild as Node);

    rerender({ paneID: "%1", text: "intermediate frame" });
    rerender({ paneID: "%1", text: "latest frame" });
    expect(pre.textContent).toBe("selected text");

    selection.removeAllRanges();
    fireEvent(document, new Event("selectionchange"));

    expect(pre.textContent).toBe("latest frame");
    expect(document.querySelector('[role="status"]')).toBeNull();
  });

  it("locks live DOM commits for selection mode and flushes on exit", () => {
    const { pre, rerender } = mount({ paneID: "%1", text: "" });
    rerender({ paneID: "%1", text: "stable frame" });

    rerender({ paneID: "%1", text: "stable frame", selectionMode: true });
    rerender({ paneID: "%1", text: "pending frame", selectionMode: true });

    expect(pre.textContent).toBe("stable frame");
    expect(document.querySelector('[role="status"]')).toBeInTheDocument();

    rerender({ paneID: "%1", text: "pending frame", selectionMode: false });

    expect(pre.textContent).toBe("pending frame");
    expect(document.querySelector('[role="status"]')).toBeNull();
  });

  it("drops the old pending frame and selection when App switches panes", () => {
    vi.useFakeTimers();
    const onPreviewImage = vi.fn();
    const { pre, rerender } = mount({ paneID: "%1", text: "", onPreviewImage });
    rerender({ paneID: "%1", text: "/work/old.png", cwd: "/work", onPreviewImage });
    const oldTarget = pre.querySelector(".image-path") as HTMLElement;
    const oldNode = oldTarget.firstChild as Node;
    const selection = selectText(oldNode);
    fireEvent.pointerDown(oldTarget, { pointerType: "touch" });
    rerender({ paneID: "%1", text: "old pane pending", cwd: "/work", onPreviewImage });

    rerender({ paneID: "%2", text: "new pane now", cwd: "/other", onPreviewImage });
    vi.advanceTimersByTime(550);

    expect(pre.textContent).toBe("new pane now");
    expect(oldNode.isConnected).toBe(false);
    expect(selection.rangeCount).toBe(0);
    expect(onPreviewImage).not.toHaveBeenCalled();
    expect(document.querySelector('[role="status"]')).toBeNull();

    // Releasing the stale interaction cannot flush the prior pane's frame.
    fireEvent.pointerCancel(pre, { pointerType: "mouse" });
    expect(pre.textContent).toBe("new pane now");
  });
});
