import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { OfficeBlock } from "./OfficeBlock";
import type { PaneStatus } from "../types/server";

function pane(overrides: Partial<PaneStatus>): PaneStatus {
  return {
    target: "s:0." + (overrides.pane_index ?? 0),
    pane_id: "%" + (overrides.pane_index ?? 0),
    session: "work",
    window_index: 0,
    pane_index: 0,
    runtime: "",
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

describe("OfficeBlock", () => {
  it("renders an empty state without pane seats", () => {
    render(<OfficeBlock panes={[]} selected={null} onSelect={vi.fn()} />);

    expect(screen.getByText("No panes")).toBeInTheDocument();
    expect(screen.queryAllByRole("button")).toHaveLength(0);
  });

  it("renders mixed pane states in statusline order", () => {
    const { container } = render(
      <OfficeBlock
        panes={[
          pane({ pane_id: "%2", session: "zeta", pane_index: 0, runtime: "", asking: true }),
          pane({ pane_id: "%1", session: "alpha", pane_index: 0, runtime: "codex", running: true, idle: false, state: "working" }),
          pane({ pane_id: "%3", session: "zeta", window_index: 1, pane_index: 1, runtime: "claude", stale: true }),
          pane({ pane_id: "%4", session: "zeta", window_index: 2, pane_index: 2, runtime: "shell" }),
        ]}
        selected="%1"
        onSelect={vi.fn()}
      />,
    );

    const seats = screen.getAllByRole("button");
    expect(seats).toHaveLength(4);
    expect(container.querySelector(".office-block")).toHaveStyle({ "--office-scroll-h": "161px" });
    expect(container.querySelectorAll(".office-shared-walls")).toHaveLength(1);
    expect(container.querySelector(".office-top-wall-edge")).not.toBeNull();
    expect(container.querySelector(".office-top-wall-face")).not.toBeNull();
    expect(container.querySelector(".office-top-decor-floor")).not.toBeNull();
    expect(container.querySelector(".office-left-decor-floor")).not.toBeNull();
    expect(container.querySelector(".office-right-decor-floor")).not.toBeNull();
    expect(container.querySelector(".office-left-wall-edge")).toBeNull();
    expect(container.querySelector(".office-left-wall-face")).toBeNull();
    expect(container.querySelector(".office-left-wall-cap")).toBeNull();
    const [first, second, third, fourth] = seats as [HTMLElement, HTMLElement, HTMLElement, HTMLElement];
    expect(first).toHaveClass("office-seat-left", "occupied", "state-running", "selected");
    expect(first.querySelector(".office-label")).toBeNull();
    expect(first.querySelector(".office-floor")).not.toBeNull();
    expect(first.querySelector(".office-work-area")).not.toBeNull();
    expect(first.querySelector(".office-shared-walls")).toBeNull();
    expect(first.querySelector(".office-small-table-side")).not.toBeNull();
    expect(first.querySelector(".office-pc-side")).not.toBeNull();
    expect(first.querySelector(".office-wooden-chair-side")).not.toBeNull();
    expect(first.querySelector(".office-seated-agent")).not.toBeNull();
    expect(first.querySelector(".office-person")).toBeNull();
    expect(first.querySelector(".office-sprite-desk")).toBeNull();
    expect(first.querySelector(".office-sprite-monitor")).toBeNull();
    expect(second).toHaveClass("office-seat-right", "empty-seat", "asking");
    expect(second.querySelector(".office-label")).toBeNull();
    expect(second.querySelector(".office-alert")).toBeNull();
    expect(third).toHaveClass("office-seat-left", "occupied", "state-stale", "stale");
    expect(third.querySelector(".office-person")).toBeNull();
    expect(fourth).toHaveClass("office-seat-right", "empty-seat");
    expect(fourth.querySelector(".office-person")).toBeNull();
  });

  it("renders the floor base with a compact side table and without people or monitors", () => {
    const { container } = render(
      <OfficeBlock
        panes={[
          pane({ pane_id: "%1", session: "agent", runtime: "codex" }),
          pane({ pane_id: "%2", session: "shell", runtime: "shell" }),
          pane({ pane_id: "%3", session: "unknown", runtime: "custom-agent" }),
        ]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    const [agent, shell, unknown] = screen.getAllByRole("button") as [HTMLElement, HTMLElement, HTMLElement];
    expect(container.querySelectorAll(".office-shared-walls")).toHaveLength(1);
    expect(container.querySelectorAll(".office-left-wall-face")).toHaveLength(0);
    expect(container.querySelectorAll(".office-right-decor-floor")).toHaveLength(1);
    expect(container.querySelectorAll(".office-top-decor-floor")).toHaveLength(1);
    expect(container.querySelectorAll(".office-seat-filler")).toHaveLength(1);
    expect(container.querySelector(".office-seat-filler .office-floor")).not.toBeNull();
    expect(agent).toHaveClass("occupied");
    expect(shell).toHaveClass("empty-seat");
    expect(unknown).toHaveClass("empty-seat");
    for (const seat of [agent, shell, unknown]) {
      expect(seat.querySelector(".office-floor")).not.toBeNull();
      expect(seat.querySelector(".office-work-area")).not.toBeNull();
      expect(seat.querySelector(".office-shared-walls")).toBeNull();
      expect(seat.querySelector(".office-small-table-side")).not.toBeNull();
      expect(seat.querySelector(".office-pc-side")).not.toBeNull();
      expect(seat.querySelector(".office-wooden-chair-side")).not.toBeNull();
      expect(seat.querySelector(".office-person")).toBeNull();
      expect(seat.querySelector(".office-sprite-monitor")).toBeNull();
    }
    expect(agent.querySelector(".office-seated-agent")).not.toBeNull();
    expect(shell.querySelector(".office-seated-agent")).toBeNull();
    expect(unknown.querySelector(".office-seated-agent")).toBeNull();
  });

  it("keeps state classes without rendering visible labels", () => {
    render(
      <OfficeBlock
        panes={[
          pane({ pane_id: "%1", session: "idle", runtime: "claude" }),
          pane({ pane_id: "%2", session: "question", runtime: "codex", asking: true }),
          pane({ pane_id: "%3", session: "running", runtime: "gemini", running: true, idle: false, state: "working" }),
          pane({ pane_id: "%4", session: "stale", runtime: "copilot", stale: true }),
        ]}
        selected="%2"
        onSelect={vi.fn()}
      />,
    );

    for (const seat of screen.getAllByRole("button")) {
      expect(seat.querySelector(".office-label")).toBeNull();
      expect(seat.querySelector(".office-monitor .office-label")).toBeNull();
    }

    expect(screen.getByRole("button", { name: /select pane idle, idle/i })).toHaveClass("state-idle");
    expect(screen.getByRole("button", { name: /select pane question, asking/i })).toHaveClass("state-asking", "asking", "selected");
    expect(screen.getByRole("button", { name: /select pane running, working/i })).toHaveClass("state-running");
    expect(screen.getByRole("button", { name: /select pane stale, stale/i })).toHaveClass("state-stale", "stale");
  });

  it("puts the full pane label in title and aria label without a visible label", () => {
    const longLabel = "very-long-session-name-for-test";
    render(
      <OfficeBlock
        panes={[pane({ pane_id: "%9", session: longLabel, runtime: "codex" })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    const seat = screen.getByRole("button", { name: /very-long-session-name-for-test/i });
    expect(seat).toHaveAttribute("title", expect.stringContaining(longLabel));
    expect(seat).toHaveAttribute("aria-label", expect.stringContaining(longLabel));
    expect(seat.querySelector(".office-label")).toBeNull();
  });

  it("selects the clicked pane", async () => {
    const onSelect = vi.fn();
    render(
      <OfficeBlock
        panes={[pane({ pane_id: "%7", session: "dev", runtime: "gemini" })]}
        selected={null}
        onSelect={onSelect}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: /select pane dev/i }));

    expect(onSelect).toHaveBeenCalledWith("%7");
  });
});
