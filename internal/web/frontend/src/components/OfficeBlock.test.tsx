import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

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

afterEach(() => {
  vi.useRealTimers();
  localStorage.removeItem("tmact.officeCollapsed");
});

describe("OfficeBlock", () => {
  it("renders an empty state without pane seats", () => {
    const { container } = render(<OfficeBlock panes={[]} selected={null} onSelect={vi.fn()} />);

    expect(screen.getByText("No panes")).toBeInTheDocument();
    expect(container.querySelectorAll(".office-seat")).toHaveLength(0);
    expect(screen.getByRole("button", { name: /collapse office layout/i })).toBeInTheDocument();
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

    const seats = Array.from(container.querySelectorAll("button.office-seat")) as HTMLElement[];
    const selectedOverlay = container.querySelector(".office-selected-overlay");
    expect(seats).toHaveLength(4);
    expect(selectedOverlay).toHaveTextContent("alpha");
    expect(selectedOverlay).toHaveAttribute("title", "alpha — working");
    expect(container.querySelector(".office-block")).toHaveStyle({ "--office-floorplan-base-h": "161px" });
    expect(container.querySelectorAll(".office-floorplan")).toHaveLength(1);
    expect(container.querySelectorAll(".office-floorplan-scale")).toHaveLength(1);
    expect(container.querySelectorAll(".office-shared-walls")).toHaveLength(1);
    expect(container.querySelectorAll(".office-floor-base")).toHaveLength(1);
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
    expect(first.querySelector(".office-name-tag")).toHaveTextContent("alpha");
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

  it("renders a bottom overlay for the selected pane name", () => {
    const { container } = render(
      <OfficeBlock
        panes={[
          pane({ pane_id: "%1", session: "hub@remote-work", peer: "hub", runtime: "codex" }),
          pane({ pane_id: "%2", session: "local-work", runtime: "claude" }),
        ]}
        selected="%1"
        onSelect={vi.fn()}
      />,
    );

    const overlay = container.querySelector(".office-selected-overlay");
    expect(overlay).toHaveTextContent("hub · remote-work");
    expect(overlay).toHaveAttribute("title", "hub · remote-work — idle");
    expect(overlay).not.toHaveClass("office-selected-overlay-empty");
  });

  it("keeps the bottom overlay visible even before a pane is selected", () => {
    const { container } = render(
      <OfficeBlock
        panes={[pane({ pane_id: "%1", session: "dev", runtime: "codex" })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    const overlay = container.querySelector(".office-selected-overlay");
    expect(overlay).toHaveTextContent("No pane selected");
    expect(overlay).toHaveClass("office-selected-overlay-empty");
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

    const [agent, shell, unknown] = Array.from(
      container.querySelectorAll("button.office-seat"),
    ) as [HTMLElement, HTMLElement, HTMLElement];
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

    for (const seat of document.querySelectorAll("button.office-seat")) {
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
    expect(seat.querySelector(".office-name-tag")).toHaveTextContent(longLabel);
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

  it("collapses and expands the office layout from the edge toggle", async () => {
    const user = userEvent.setup();
    const { container } = render(
      <OfficeBlock
        panes={[pane({ pane_id: "%7", session: "dev", runtime: "gemini" })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    const toggle = screen.getByRole("button", { name: /collapse office layout/i });
    expect(container.querySelector(".office-block")).not.toHaveClass("office-collapsed");
    expect(toggle).toHaveAttribute("aria-expanded", "true");

    await user.click(toggle);

    expect(container.querySelector(".office-block")).toHaveClass("office-collapsed");
    expect(localStorage.getItem("tmact.officeCollapsed")).toBe("1");
    expect(screen.getByRole("button", { name: /expand office layout/i })).toHaveAttribute(
      "aria-expanded",
      "false",
    );

    await user.click(screen.getByRole("button", { name: /expand office layout/i }));

    expect(container.querySelector(".office-block")).not.toHaveClass("office-collapsed");
    expect(localStorage.getItem("tmact.officeCollapsed")).toBe("0");
  });

  it("shows office seat names while holding Option on desktop", () => {
    const { container } = render(
      <OfficeBlock
        panes={[pane({ pane_id: "%7", session: "dev", runtime: "gemini" })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    const office = container.querySelector(".office-block");
    expect(office).not.toHaveClass("office-show-labels");

    fireEvent.keyDown(window, { key: "Alt", altKey: true });
    expect(office).toHaveClass("office-show-labels");

    fireEvent.keyUp(window, { key: "Alt", altKey: false });
    expect(office).not.toHaveClass("office-show-labels");
  });

  it("shows office seat names briefly after mobile pointer interaction", () => {
    vi.useFakeTimers();
    const { container } = render(
      <OfficeBlock
        panes={[pane({ pane_id: "%7", session: "dev", runtime: "gemini" })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    const office = container.querySelector(".office-block");
    const floorplan = container.querySelector(".office-floorplan")!;
    fireEvent.pointerDown(floorplan, { pointerType: "touch" });
    expect(office).toHaveClass("office-show-labels");

    fireEvent.pointerUp(floorplan, { pointerType: "touch" });
    act(() => vi.advanceTimersByTime(1999));
    expect(office).toHaveClass("office-show-labels");

    act(() => vi.advanceTimersByTime(1));
    expect(office).not.toHaveClass("office-show-labels");
    vi.useRealTimers();
  });
});
