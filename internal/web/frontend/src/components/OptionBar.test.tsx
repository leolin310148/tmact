import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Question } from "../types/server";
import { OptionBar } from "./OptionBar";

afterEach(cleanup);

const question: Question = {
  prompt: "Choose a deployment target",
  choices: [
    { number: 1, label: "Local" },
    { number: 2, label: "Staging" },
    { number: 3, label: "Production" },
  ],
};

describe("OptionBar", () => {
  it("exposes the detected question as the choice group's accessible name", () => {
    render(<OptionBar question={question} onChoose={vi.fn()} />);

    const group = screen.getByRole("group", { name: question.prompt });
    const buttons = within(group).getAllByRole("button");
    expect(buttons).toHaveLength(3);
    expect(buttons[0]).toHaveAccessibleName("Option 1: Local");
    expect(buttons[1]).toHaveAccessibleName("Option 2: Staging");
    expect(buttons[2]).toHaveAccessibleName("Option 3: Production");
  });

  it("keeps an unlabelled choice number understandable to screen readers", () => {
    render(
      <OptionBar
        question={{ prompt: "Continue?", choices: [{ number: 7, label: "" }] }}
        onChoose={vi.fn()}
      />,
    );

    expect(screen.getByRole("group", { name: "Continue?" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Option 7" })).toHaveTextContent("7");
  });

  it("uses a stable fallback label and stays non-interactive without valid choices", () => {
    const { rerender } = render(
      <OptionBar question={{ choices: [{ number: 1, label: "Yes" }] }} onChoose={vi.fn()} />,
    );
    expect(screen.getByRole("group", { name: "Detected prompt choices" })).toBeInTheDocument();

    rerender(<OptionBar question={null} onChoose={vi.fn()} />);
    expect(screen.queryByRole("group")).not.toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(document.getElementById("option-bar")).toBeEmptyDOMElement();
  });

  it("traverses choices with arrows and Home/End, wrapping at either edge", async () => {
    const user = userEvent.setup();
    render(<OptionBar question={question} onChoose={vi.fn()} />);

    const buttons = screen.getAllByRole("button");
    buttons[0]!.focus();
    await user.keyboard("{ArrowRight}");
    expect(buttons[1]).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(buttons[2]).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(buttons[0]).toHaveFocus();
    await user.keyboard("{ArrowLeft}");
    expect(buttons[2]).toHaveFocus();
    await user.keyboard("{Home}");
    expect(buttons[0]).toHaveFocus();
    await user.keyboard("{End}");
    expect(buttons[2]).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    expect(buttons[1]).toHaveFocus();
  });

  it("activates the focused choice with Enter or Space", async () => {
    const onChoose = vi.fn();
    const user = userEvent.setup();
    render(<OptionBar question={question} onChoose={onChoose} />);

    const buttons = screen.getAllByRole("button");
    buttons[0]!.focus();
    await user.keyboard("{Enter}");
    buttons[1]!.focus();
    await user.keyboard(" ");

    expect(onChoose).toHaveBeenNthCalledWith(1, 1);
    expect(onChoose).toHaveBeenNthCalledWith(2, 2);
  });

  it("preserves focused input when a choice is tapped", () => {
    render(
      <>
        <input aria-label="terminal input" />
        <OptionBar question={question} onChoose={vi.fn()} />
      </>,
    );

    const input = screen.getByRole("textbox", { name: "terminal input" });
    const choice = screen.getByRole("button", { name: "Option 1: Local" });
    input.focus();

    expect(fireEvent.pointerDown(choice)).toBe(false);
    fireEvent.click(choice);
    expect(input).toHaveFocus();
  });
});
