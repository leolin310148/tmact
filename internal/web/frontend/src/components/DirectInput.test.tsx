import { createRef } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import DirectInput from "./DirectInput";

afterEach(cleanup);

function renderDirectInput(paneSelected: boolean, selectionMode: boolean) {
  render(
    <DirectInput
      directRef={createRef<HTMLTextAreaElement>()}
      paneSelected={paneSelected}
      selectionMode={selectionMode}
      onDirectKeyDown={vi.fn()}
      onDirectComposition={vi.fn()}
      onDirectPaste={vi.fn()}
      onDirectInput={vi.fn()}
    />,
  );
  return screen.getByRole("textbox", { name: "Direct terminal input" });
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
