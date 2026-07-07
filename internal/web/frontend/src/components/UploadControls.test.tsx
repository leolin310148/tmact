// Regression coverage for the "right-side buttons do nothing" bug.
//
// The upload / selection / clear-pane buttons are enabled IMPERATIVELY by App
// (syncQuickDock / syncSelectionButton / selectPane write `el.disabled = false`
// on the DOM node), NOT through a React `disabled` prop. That is deliberate —
// but it means the JSX must NOT carry a static `disabled` literal. React's
// synthetic event system decides whether to dispatch a click from the element's
// FIBER PROPS (shouldPreventMouseEvent), never from the live DOM `disabled`. So
// a static `disabled` leaves props.disabled === true forever; App's DOM write
// makes the button look/behave enabled natively, yet React silently drops the
// onClick. (pointerdown is NOT suppressed, which is exactly why the broken
// buttons still ran onPointerDownNoBlur but never fired their handler.)
//
// These tests pin the contract: after the imperative enable, a click reaches the
// handler. They fail if anyone re-introduces a static `disabled` prop.

import { cleanup, fireEvent, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UploadControls } from "./UploadControls";

afterEach(cleanup);

function renderControls(selectionMode = false) {
  const handlers = {
    onUpload: vi.fn(),
    onSelection: vi.fn(),
    onClear: vi.fn(),
    onHelp: vi.fn(),
    onSettings: vi.fn(),
    onFiles: vi.fn(),
    onDownloadList: vi.fn(),
  };
  render(<UploadControls {...handlers} selectionMode={selectionMode} />);
  return handlers;
}

const ACTION_BUTTONS = [
  ["upload-btn", "onUpload"],
  ["selection-btn", "onSelection"],
  ["clear-pane-btn", "onClear"],
] as const;

describe("UploadControls", () => {
  it.each(ACTION_BUTTONS)(
    "#%s fires %s after App enables it on the DOM (no static `disabled` prop)",
    (id, handlerName) => {
      const handlers = renderControls();
      const btn = document.getElementById(id) as HTMLButtonElement | null;
      expect(btn).not.toBeNull();
      // Mirror App's imperative enable (syncQuickDock / syncSelectionButton /
      // selectPane). With a static `disabled` prop this DOM write would be
      // ineffective for click dispatch — React would still suppress onClick.
      btn!.disabled = false;
      fireEvent.click(btn!);
      expect(handlers[handlerName]).toHaveBeenCalledTimes(1);
    },
  );

  it.each(ACTION_BUTTONS.map(([id]) => id))(
    "#%s renders without a `disabled` attribute (React must not own it)",
    (id) => {
      renderControls();
      const btn = document.getElementById(id) as HTMLButtonElement;
      expect(btn.hasAttribute("disabled")).toBe(false);
    },
  );

  it('#selection-btn seeds aria-pressed="false" (App then owns it via syncSelectionButton)', () => {
    // UploadControls.tsx seeds this attribute to match the vanilla index.html
    // markup; App mutates it imperatively afterward. Guards against a future
    // edit dropping the seed or converting it to a React-controlled prop.
    renderControls();
    const btn = document.getElementById("selection-btn") as HTMLButtonElement;
    expect(btn.getAttribute("aria-pressed")).toBe("false");
  });

  it("#help-btn fires onHelp on click", () => {
    const handlers = renderControls();
    fireEvent.click(document.getElementById("help-btn") as HTMLButtonElement);
    expect(handlers.onHelp).toHaveBeenCalledTimes(1);
  });

  it("#gear-btn fires onSettings on click", () => {
    const handlers = renderControls();
    fireEvent.click(document.getElementById("gear-btn") as HTMLButtonElement);
    expect(handlers.onSettings).toHaveBeenCalledTimes(1);
  });

  it("#upload-btn becomes the download button in selection mode", () => {
    const handlers = renderControls(true);
    const btn = document.getElementById("upload-btn") as HTMLButtonElement;
    expect(btn.getAttribute("aria-label")).toBe("download files from pane");
    btn.disabled = false;
    fireEvent.click(btn);
    expect(handlers.onDownloadList).toHaveBeenCalledTimes(1);
    expect(handlers.onUpload).not.toHaveBeenCalled();
  });

  it("#file-upload change forwards the selected files then clears its value", () => {
    const handlers = renderControls();
    const input = document.getElementById("file-upload") as HTMLInputElement;
    const file = new File(["x"], "note.txt", { type: "text/plain" });
    fireEvent.change(input, { target: { files: [file] } });
    expect(handlers.onFiles).toHaveBeenCalledTimes(1);
    const [filesArg] = handlers.onFiles.mock.calls[0] ?? [];
    expect(filesArg).toHaveLength(1);
    expect(input.value).toBe("");
  });
});
