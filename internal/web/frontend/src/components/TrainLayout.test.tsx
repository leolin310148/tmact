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
  vi.unstubAllGlobals();
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
    expect(queryByTestId("train-route-diagnostics")).toHaveTextContent(
      /seed infinite-journey .* position 0\.0px .* chunks .* mounted \d+/,
    );
  });

  it("mounts only the diagnosed route chunk window", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world");
    const chunks = container.querySelectorAll(".train-route-chunk");

    expect(world).not.toBeNull();
    expect(chunks).toHaveLength(Number(world!.dataset.routeMountedChunks));
    expect(
      [...chunks].map((chunk) => chunk.getAttribute("data-route-chunk-index")).join(","),
    ).toBe(world!.dataset.routeChunkIndices);
    expect(world).toHaveAttribute("data-route-seed", "infinite-journey");
    expect(world).toHaveAttribute(
      "data-route-seed-version",
      "tmact-train-route-v1",
    );
  });

  it("renders ordered bounded chunks for all five parallax layers", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const layers = [
      ...container.querySelectorAll<HTMLElement>(".train-world-layer"),
    ];

    expect(layers.map((layer) => layer.dataset.worldLayer)).toEqual([
      "sky",
      "ultra-far",
      "far",
      "midground",
      "near",
    ]);
    expect(layers.map((layer) => layer.dataset.speedRatio)).toEqual([
      "0",
      "0.1",
      "0.25",
      "0.55",
      "1",
    ]);
    expect(layers.map((layer) => layer.dataset.layerOrder)).toEqual([
      "0",
      "1",
      "2",
      "3",
      "4",
    ]);
    for (const layer of layers) {
      expect(
        layer.querySelectorAll(".train-parallax-chunk").length,
      ).toBeGreaterThan(0);
    }
  });

  it("renders manifest-backed scenery sprites with explicit anchors and scale bounds", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const chunks = container.querySelectorAll(".train-parallax-chunk");
    const sprites =
      container.querySelectorAll<HTMLImageElement>(".train-scenery-asset");

    expect(sprites.length).toBeGreaterThan(0);
    expect(sprites.length).toBeLessThanOrEqual(chunks.length * 2);
    expect(container.querySelector("[data-scenery-category='cloud']")).not.toBeNull();
    expect(
      container.querySelector("[data-scenery-category='terrain']"),
    ).not.toBeNull();
    expect(
      container.querySelector("[data-scenery-category='building'], [data-scenery-category='vegetation'], [data-scenery-category='bridge']"),
    ).not.toBeNull();
    expect(container.querySelector("[data-scenery-category='prop']")).not.toBeNull();

    for (const sprite of sprites) {
      expect(sprite).toHaveAttribute("aria-hidden", "true");
      expect(sprite).toHaveAttribute("data-scenery-asset");
      expect(sprite.dataset.sceneryAnchor).toMatch(/^(center|bottom-center)$/);
      expect(sprite.dataset.scenerySafeScale).toMatch(/^\d+(\.\d+)?-\d+(\.\d+)?$/);
      expect(sprite.dataset.sceneryLandmark).toMatch(/^(true|false)$/);
      expect(Number(sprite.dataset.sceneryCollisionWidth)).toBeGreaterThan(0);
      expect(sprite.width).toBeGreaterThan(0);
      expect(sprite.height).toBeGreaterThan(0);
      expect(sprite.style.getPropertyValue("--train-scenery-scale")).not.toBe("");
    }

    for (const chunk of chunks) {
      expect(chunk.getAttribute("data-route-region")).toMatch(
        /^(forest|mountain|town|coast|industrial)$/,
      );
      expect(chunk).toHaveAttribute("data-route-region-index");
      expect(chunk).toHaveAttribute("data-route-region-offset");
    }
  });

  it("overlaps adjacent chunks to hide fractional-pixel seams", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const nearChunks = [
      ...container.querySelectorAll<HTMLElement>(
        '[data-world-layer="near"] .train-parallax-chunk',
      ),
    ].sort(
      (left, right) =>
        Number.parseFloat(left.style.left) - Number.parseFloat(right.style.left),
    );
    const first = nearChunks[0]!;
    const second = nearChunks[1]!;
    const firstLeft = Number.parseFloat(first.style.left);
    const firstRight = firstLeft + Number.parseFloat(first.style.width);
    const secondLeft = Number.parseFloat(second.style.left);

    expect(first).toHaveAttribute("data-seam-overlap", "2");
    expect(firstRight - secondLeft).toBe(2);
  });

  it("mounts a complete static scene for reduced-motion users", () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn().mockReturnValue({
        matches: true,
        media: "(prefers-reduced-motion: reduce)",
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    );

    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector(".train-layout-world");
    const layers = container.querySelectorAll(".train-world-layer");

    expect(world).toHaveAttribute("data-motion", "reduced");
    expect(layers).toHaveLength(5);
    for (const layer of layers) {
      expect(layer).toHaveAttribute("data-motion", "reduced");
      expect((layer as HTMLElement).dataset.layerPosition).toBe("0.000px");
    }
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
