import { createRef } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import DirectInput from "./DirectInput";

afterEach(cleanup);

function renderDirectInput(paneSelected: boolean, selectionMode: boolean) {
  const handlers = {
    onDirectKeyDown: vi.fn(),
    onDirectComposition: vi.fn(),
    onDirectCompositionStart: vi.fn(),
    onDirectBlur: vi.fn(),
    onDirectPaste: vi.fn(),
    onDirectInput: vi.fn(),
  };
  render(
    <DirectInput
      directRef={createRef<HTMLTextAreaElement>()}
      paneSelected={paneSelected}
      selectionMode={selectionMode}
      {...handlers}
    />,
  );
  const input = screen.getByRole("textbox", { name: "Direct terminal input" });
  return Object.assign(input, { handlers }) as HTMLElement & { handlers: typeof handlers };
}

describe("DirectInput accessibility state", () => {
  it("identifies enabled direct input for a selected pane", () => {
    const input = renderDirectInput(true, false);

    expect(input).toBeEnabled();
    expect(input).toHaveAccessibleDescription(
      "Input is available for the selected pane. Keystrokes are sent directly to the terminal.",
    );
  });

  it("disables and explains direct input when no pane is selected", () => {
    const input = renderDirectInput(false, false);

    expect(input).toBeDisabled();
    expect(input).toHaveAccessibleDescription("Select a pane to enable direct terminal input.");
  });

  it("disables and explains direct input while text selection mode is on", () => {
    const input = renderDirectInput(true, true);

    expect(input).toBeDisabled();
    expect(input).toHaveAccessibleDescription(
      "Direct terminal input is unavailable while text selection mode is on.",
    );
  });
});

// An IME composition holds its buffer in this textarea and relays nothing until
// compositionend, so App must be told when one starts (to reveal the otherwise
// transparent box) and when focus leaves mid-composition.
describe("DirectInput IME composition wiring", () => {
  it("reports composition start and end to App", () => {
    const input = renderDirectInput(true, false);

    fireEvent.compositionStart(input, { data: "" });
    fireEvent.compositionEnd(input, { data: "你好" });

    expect(input.handlers.onDirectCompositionStart).toHaveBeenCalledTimes(1);
    expect(input.handlers.onDirectComposition).toHaveBeenCalledTimes(1);
  });

  it("reports blur so an uncommitted buffer can be dropped", () => {
    const input = renderDirectInput(true, false);

    fireEvent.blur(input);

    expect(input.handlers.onDirectBlur).toHaveBeenCalledTimes(1);
  });

  it("labels the composing box for the reader", () => {
    renderDirectInput(true, false);

    expect(document.querySelector(".ime-hint")?.textContent).toContain("組字中");
  });
});
