import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { PaneStatus } from "../types/server";
import {
  advanceTrainWorldRoutePosition,
  minimumCarriagesForWidth,
  TrainLayout,
} from "./TrainLayout";

vi.mock("../api/client", () => ({
  loadClosedSessions: vi.fn(() =>
    Promise.resolve({ res: { ok: true } as Response, data: { sessions: [] } }),
  ),
  killSession: vi.fn(() =>
    Promise.resolve({ res: { ok: true } as Response, data: { ok: true } }),
  ),
  reopenSession: vi.fn(() =>
    Promise.resolve({ res: { ok: true } as Response, data: { ok: true } }),
  ),
  reportHumanActivity: vi.fn(),
}));

afterEach(() => {
  cleanup();
  window.history.replaceState(null, "", "/");
});

function pane(overrides: Partial<PaneStatus> = {}): PaneStatus {
  return {
    target: "s:0.0",
    pane_id: "%1",
    session: "sess",
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

describe("TrainLayout", () => {
  it("mounts a clipped world below an independent train inspection layer", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );

    const layout = container.querySelector(".train-layout");
    const world = container.querySelector(".train-layout-world");
    const inspection = container.querySelector(".train-layout-inspection");
    expect(layout).toContainElement(world as HTMLElement);
    expect(layout).toContainElement(inspection as HTMLElement);
    expect(world).toHaveAttribute("data-layer", "world");
    expect(world).toHaveAttribute("data-route-direction", "right");
    expect(inspection).toHaveAttribute("data-layer", "train");
    expect(world?.nextElementSibling).toBe(inspection);
  });

  it("advances the world to the right from a single route position", () => {
    expect(advanceTrainWorldRoutePosition(10, 500)).toBe(16);
    expect(advanceTrainWorldRoutePosition(16, -100)).toBe(16);
  });

  it("shows the motion grid only when its development flag is enabled", () => {
    const { queryByTestId, rerender } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    expect(queryByTestId("train-world-debug-grid")).not.toBeInTheDocument();

    window.history.replaceState(null, "", "/?train-world-debug=1");
    rerender(<TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />);
    expect(queryByTestId("train-world-debug-grid")).toHaveTextContent("world →");
  });

  it("keeps world position independent from horizontal train inspection", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world");
    const inspection = container.querySelector<HTMLElement>(
      ".train-layout-inspection",
    );
    expect(world).not.toBeNull();
    expect(inspection).not.toBeNull();

    const routePosition = world!.dataset.routePosition;
    inspection!.scrollLeft = 240;
    fireEvent.scroll(inspection!);

    expect(inspection).toHaveProperty("scrollLeft", 240);
    expect(world!.dataset.routePosition).toBe(routePosition);
    expect(inspection!.contains(world)).toBe(false);
  });

  it("renders an empty starter carriage when no panes exist", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );

    expect(
      screen.getByRole("complementary", { name: "Train pane switcher" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Show recently closed sessions" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("group", { name: "Train carriage 1" })).toHaveAttribute(
      "data-filler-carriage",
      "true",
    );
    expect(container.querySelectorAll(".train-seat--empty")).toHaveLength(4);
    expect(container.querySelector(".train-layout-track")).toBeInTheDocument();
  });

  it("calculates enough overlapping carriages to cover a wide viewport", () => {
    expect(minimumCarriagesForWidth(1920, 143, 242)).toBe(8);
    expect(minimumCarriagesForWidth(900, 143, 242)).toBe(4);
    expect(minimumCarriagesForWidth(0, 0, 0)).toBe(1);
  });

  it("packs panes into four ordered seats per carriage", () => {
    const panes = Array.from({ length: 6 }, (_, i) =>
      pane({
        target: `s${i}:0.0`,
        pane_id: `%${i + 1}`,
        session: `session-${i + 1}`,
        runtime: "codex",
      }),
    );

    const { container } = render(
      <TrainLayout panes={panes} selected={null} onSelect={vi.fn()} />,
    );

    expect(screen.getAllByRole("group", { name: /Train carriage/ })).toHaveLength(2);
    const passengers = screen.getAllByRole("button", { name: /Select pane/ });
    expect(passengers).toHaveLength(6);
    expect(
      passengers.map((passenger) => passenger.getAttribute("data-seat-index")),
    ).toEqual(["0", "1", "2", "3", "0", "1"]);
    expect(
      passengers.map((passenger) => passenger.getAttribute("data-character-index")),
    ).toEqual(["0", "1", "2", "3", "4", "5"]);
    expect(container.querySelectorAll(".train-seat--empty")).toHaveLength(2);
    expect(container.querySelectorAll(".train-seat--facing-left")).toHaveLength(4);
    expect(container.querySelectorAll(".train-seat--facing-right")).toHaveLength(4);
  });

  it("selects the pane represented by a passenger", () => {
    const onSelect = vi.fn();
    render(
      <TrainLayout
        panes={[
          pane({ pane_id: "%1", session: "alpha", runtime: "codex" }),
          pane({ pane_id: "%2", session: "beta", runtime: "codex" }),
        ]}
        selected="%2"
        onSelect={onSelect}
      />,
    );

    const selectedPassenger = screen.getByRole("button", {
      name: "Select pane beta, idle",
    });
    expect(selectedPassenger).toHaveAttribute("aria-pressed", "true");
    expect(selectedPassenger).toHaveClass("selected");

    fireEvent.click(screen.getByRole("button", { name: "Select pane alpha, idle" }));
    expect(onSelect).toHaveBeenCalledWith("%1");
  });

  it("keeps inactive panes off the train and lists them from the locomotive", () => {
    const onSelect = vi.fn();
    render(
      <TrainLayout
        panes={[
          pane({ pane_id: "%1", session: "active", runtime: "codex" }),
          pane({ pane_id: "%2", session: "inactive" }),
        ]}
        selected={null}
        onSelect={onSelect}
      />,
    );

    expect(screen.getAllByRole("group", { name: /Train carriage/ })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: /Select pane/ })).toHaveLength(1);
    expect(
      screen.queryByRole("button", { name: "Select pane inactive, idle" }),
    ).not.toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", {
        name: "Show 1 more pane and recently closed sessions",
      }),
    );

    const overflowPane = screen.getByRole("menuitem", {
      name: "Select pane inactive",
    });
    fireEvent.click(overflowPane);
    expect(onSelect).toHaveBeenCalledWith("%2");
  });
});
