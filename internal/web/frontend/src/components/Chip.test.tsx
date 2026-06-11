import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { Chip } from "./Chip";
import type { PaneStatus } from "../types/server";

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
});
