import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";

import { Chip } from "./Chip";
import type { PaneStatus } from "../types/server";

beforeAll(() => {
  // jsdom has no layout; selected chips scroll themselves into view.
  Element.prototype.scrollIntoView = vi.fn();
});

function pane(overrides: Partial<PaneStatus> = {}): PaneStatus {
  return {
    target: "s:0.0",
    pane_id: "%1",
    session: "very-long-session-name-that-should-not-overflow-mobile-statusline",
    window_index: 0,
    pane_index: 0,
    runtime: "codex",
    tag: "",
    state: "idle",
    idle: true,
    input_ready: true,
    running: false,
    asking: false,
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("Chip", () => {
  it("wraps the visible pane label in a shrinkable label span", () => {
    const label = "ndt_mxcp-extremely-long-pane-label";

    render(
      <Chip
        pane={pane()}
        label={label}
        hotkey={undefined}
        selected={false}
        onSelect={vi.fn()}
      />,
    );

    const chip = screen.getByTitle(/very-long-session-name/);
    const labelEl = chip.querySelector(".chip-label");
    expect(labelEl).not.toBeNull();
    expect(labelEl).toHaveTextContent(label);
  });

  it("is tabbable and activates with Enter or Space", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();

    render(
      <Chip
        pane={pane()}
        label="work"
        hotkey={undefined}
        selected={false}
        onSelect={onSelect}
      />,
    );

    const chip = screen.getByRole("button", { name: /work/ });
    await user.tab();
    expect(chip).toHaveFocus();

    await user.keyboard("{Enter}");
    await user.keyboard(" ");
    expect(onSelect).toHaveBeenCalledTimes(2);
  });

  it("exposes its selected state", () => {
    const props = {
      pane: pane(),
      label: "work",
      hotkey: undefined,
      onSelect: vi.fn(),
    };
    const { rerender } = render(<Chip {...props} selected={false} />);

    expect(screen.getByRole("button", { name: /work/ })).toHaveAttribute(
      "aria-pressed",
      "false",
    );

    rerender(<Chip {...props} selected />);
    expect(screen.getByRole("button", { name: /work/ })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });
});
