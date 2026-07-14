import { useReducer, useRef } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { InputMsg } from "../types/server";
import KeyBar from "./KeyBar";

afterEach(cleanup);

function Harness({
  wsSend = vi.fn(() => true),
  showInputError = vi.fn(),
}: {
  wsSend?: (message: InputMsg) => boolean;
  showInputError?: (message: string) => void;
}) {
  const ctrlArmedRef = useRef(false);
  const [, bump] = useReducer((value: number) => value + 1, 0);

  return (
    <KeyBar
      wsSend={wsSend}
      showInputError={showInputError}
      ctrlArmedRef={ctrlArmedRef}
      bump={bump}
    />
  );
}

function makeKeyBarOverflow() {
  const bar = document.getElementById("key-bar")!;
  const firstKey = bar.querySelector("button")!;
  Object.defineProperty(firstKey, "offsetHeight", { configurable: true, value: 24 });
  Object.defineProperty(bar, "scrollHeight", { configurable: true, value: 60 });
  fireEvent(window, new Event("resize"));
}

describe("KeyBar accessibility", () => {
  it("gives symbolic helper keys understandable accessible names", () => {
    render(<Harness />);

    expect(screen.getByRole("button", { name: "Control C" })).toHaveTextContent("^C");
    expect(screen.getByRole("button", { name: "Shift Tab" })).toHaveTextContent("⇧Tab");
    expect(screen.getByRole("button", { name: "Enter" })).toHaveTextContent("↵");
    expect(screen.getByRole("button", { name: "Arrow up" })).toHaveTextContent("↑");
    expect(screen.getByRole("button", { name: "Arrow down" })).toHaveTextContent("↓");
    expect(screen.getByRole("button", { name: "Arrow left" })).toHaveTextContent("←");
    expect(screen.getByRole("button", { name: "Arrow right" })).toHaveTextContent("→");
  });

  it("exposes and updates sticky Control's pressed state", async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const control = screen.getByRole("button", { name: "Control modifier" });
    expect(control).toHaveAttribute("aria-pressed", "false");

    await user.click(control);
    expect(control).toHaveAttribute("aria-pressed", "true");
    expect(control).toHaveClass("armed");

    await user.click(control);
    expect(control).toHaveAttribute("aria-pressed", "false");
    expect(control).not.toHaveClass("armed");
  });

  it("associates the overflow toggle with the collapsed and expanded key bar", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    makeKeyBarOverflow();

    const toggle = screen.getByRole("button", { name: "Show all helper keys" });
    expect(toggle).toHaveAttribute("aria-controls", "key-bar");
    expect(toggle).toHaveAttribute("aria-expanded", "false");

    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    expect(toggle).toHaveAccessibleName("Collapse helper keys");
    expect(document.getElementById("key-area")).toHaveClass("expanded");

    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(toggle).toHaveAccessibleName("Show all helper keys");
    expect(document.getElementById("key-area")).not.toHaveClass("expanded");
  });

  it("keeps the focused input active when a helper key is tapped", () => {
    render(
      <>
        <input aria-label="terminal input" />
        <Harness />
      </>,
    );

    const input = screen.getByRole("textbox", { name: "terminal input" });
    const enter = screen.getByRole("button", { name: "Enter" });
    input.focus();

    expect(fireEvent.pointerDown(enter)).toBe(false);
    fireEvent.click(enter);
    expect(input).toHaveFocus();
  });
});
