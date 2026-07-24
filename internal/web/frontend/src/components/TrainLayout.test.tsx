import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { PaneStatus } from "../types/server";
import {
  advanceTrainWorldRoutePosition,
  minimumCarriagesForWidth,
  trainPaletteContrastRatio,
  trainWorldCruiseSpeed,
  TrainLayout,
} from "./TrainLayout";
import {
  TRAIN_WORLD_MAX_FRAME_ELAPSED_MS,
  TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS,
} from "./trainMotion";

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
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.history.replaceState(null, "", "/");
});

function installAnimationFrame() {
  let nextID = 1;
  const callbacks = new Map<number, FrameRequestCallback>();
  vi.stubGlobal(
    "requestAnimationFrame",
    vi.fn((callback: FrameRequestCallback) => {
      const id = nextID++;
      callbacks.set(id, callback);
      return id;
    }),
  );
  vi.stubGlobal(
    "cancelAnimationFrame",
    vi.fn((id: number) => callbacks.delete(id)),
  );

  return {
    pending: () => callbacks.size,
    run(timestamp: number) {
      const pending = [...callbacks.values()];
      callbacks.clear();
      act(() => {
        for (const callback of pending) callback(timestamp);
      });
    },
  };
}

function mockVisibility(initial: DocumentVisibilityState = "visible") {
  let visibility = initial;
  vi.spyOn(document, "visibilityState", "get").mockImplementation(
    () => visibility,
  );
  return {
    set(next: DocumentVisibilityState) {
      visibility = next;
      act(() => document.dispatchEvent(new Event("visibilitychange")));
    },
  };
}

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
    expect(advanceTrainWorldRoutePosition(10, 200)).toBe(12.4);
    expect(advanceTrainWorldRoutePosition(10, 500)).toBe(13);
    expect(advanceTrainWorldRoutePosition(16, -100)).toBe(16);
  });

  it("accepts a bounded development cruise-speed override", () => {
    expect(trainWorldCruiseSpeed("?train-cruise-speed=24")).toBe(24);
    expect(trainWorldCruiseSpeed("?train-cruise-speed=999")).toBe(96);
    expect(trainWorldCruiseSpeed("?train-cruise-speed=nope")).toBe(12);
  });

  it("progresses from the animation clock without rerendering the train", () => {
    const animation = installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const consist = container.querySelector(".train-layout-consist");

    animation.run(1_000);
    animation.run(1_200);

    expect(world).toHaveAttribute("data-cruise-speed", "12");
    expect(world).toHaveAttribute("data-route-position", "2.400px");
    expect(world).toHaveAttribute("data-route-apply-count", "2");
    expect(world).toHaveAttribute("data-route-window-updates", "1");
    expect(container.querySelector(".train-layout-consist")).toBe(consist);
  });

  it("clamps a throttled frame instead of leaping forward", () => {
    const animation = installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;

    animation.run(100);
    animation.run(100 + TRAIN_WORLD_MAX_FRAME_ELAPSED_MS + 20_000);

    expect(world).toHaveAttribute("data-route-position", "3.000px");
  });

  it("suspends while hidden and resumes from a fresh timestamp", () => {
    const animation = installAnimationFrame();
    const visibility = mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;

    animation.run(1_000);
    animation.run(1_100);
    expect(world).toHaveAttribute("data-route-position", "1.200px");

    visibility.set("hidden");
    expect(world).toHaveAttribute("data-motion-state", "suspended");
    expect(animation.pending()).toBe(0);

    visibility.set("visible");
    animation.run(100_000);
    expect(world).toHaveAttribute("data-route-position", "1.200px");
    animation.run(100_100);
    expect(world).toHaveAttribute("data-route-position", "2.400px");
    expect(world).toHaveAttribute("data-motion-state", "running");
  });

  it("cleans up scheduled motion and visibility ownership on unmount", () => {
    const animation = installAnimationFrame();
    mockVisibility();
    const removeListener = vi.spyOn(document, "removeEventListener");
    const { unmount } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );

    expect(animation.pending()).toBeGreaterThan(0);
    unmount();

    expect(animation.pending()).toBe(0);
    expect(removeListener).toHaveBeenCalledWith(
      "visibilitychange",
      expect.any(Function),
    );
  });

  it("recycles chunks without unbounded mounts or train renders", () => {
    window.history.replaceState(
      null,
      "",
      "/?train-cruise-speed=96",
    );
    const animation = installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const consist = container.querySelector(".train-layout-consist");

    animation.run(0);
    for (let frame = 1; frame <= 120; frame++) {
      animation.run(frame * TRAIN_WORLD_MAX_FRAME_ELAPSED_MS);
    }

    const nearChunks = container.querySelectorAll(
      '[data-world-layer="near"] .train-route-chunk',
    );
    const allChunks = container.querySelectorAll(".train-parallax-chunk");
    expect(Number(world.dataset.routeWindowUpdates)).toBeGreaterThan(0);
    expect(nearChunks).toHaveLength(Number(world.dataset.routeMountedChunks));
    expect(allChunks.length).toBeLessThanOrEqual(50);
    expect(container.querySelector(".train-layout-consist")).toBe(consist);
  });

  it("expands the overscanned route window before a wider viewport can gap", () => {
    installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const compactCount = Number(world.dataset.routeMountedChunks);
    Object.defineProperty(world, "clientWidth", {
      configurable: true,
      get: () => 1_920,
    });

    act(() => window.dispatchEvent(new Event("resize")));

    const wideCount = Number(world.dataset.routeMountedChunks);
    const nearChunks = [
      ...container.querySelectorAll<HTMLElement>(
        '[data-world-layer="near"] .train-route-chunk',
      ),
    ];
    const rightmostEdge = Math.max(
      ...nearChunks.map(
        (chunk) =>
          Number.parseFloat(chunk.style.left) +
          Number.parseFloat(chunk.style.width),
      ),
    );
    expect(wideCount).toBeGreaterThan(compactCount);
    expect(nearChunks).toHaveLength(wideCount);
    expect(rightmostEdge).toBeGreaterThanOrEqual(1_920);
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
      expect(sprite).toHaveAttribute("loading", "lazy");
      expect(sprite).toHaveAttribute("decoding", "async");
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

  it("uses restrained infrequent route steps for reduced motion", () => {
    vi.useFakeTimers();
    mockVisibility();
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
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;

    act(() => vi.advanceTimersByTime(TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS - 1));
    expect(world).toHaveAttribute("data-route-position", "0.000px");
    act(() => vi.advanceTimersByTime(1));
    expect(world).toHaveAttribute("data-route-position", "1.200px");
    expect(
      container.querySelector<HTMLElement>('[data-world-layer="near"]')!
        .dataset.layerPosition,
    ).toBe("1.200px");
  });

  it("manually crossfades palettes without changing visible route geometry", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 12, 0));
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const geometryBefore = [
      ...container.querySelectorAll<HTMLElement>(".train-parallax-chunk"),
    ].map((chunk) => ({
      layer: chunk.dataset.parallaxLayer,
      index: chunk.dataset.routeChunkIndex,
      region: chunk.dataset.routeRegion,
      left: chunk.style.left,
      width: chunk.style.width,
    }));
    const positionBefore = world.dataset.routePosition;

    expect(world).toHaveAttribute("data-time-of-day", "day");
    expect(world).toHaveAttribute("data-time-source", "clock");
    expect(world).toHaveAttribute("data-palette-transition", "settled");

    fireEvent.click(
      screen.getByRole("button", {
        name: "Cycle train lighting (day / sunset / night)",
      }),
    );

    expect(world).toHaveAttribute("data-time-of-day", "sunset");
    expect(world).toHaveAttribute("data-time-source", "manual");
    expect(world).toHaveAttribute("data-palette-transition", "crossfading");
    expect(world.dataset.routePosition).toBe(positionBefore);
    expect(
      [...container.querySelectorAll<HTMLElement>(".train-parallax-chunk")].map(
        (chunk) => ({
          layer: chunk.dataset.parallaxLayer,
          index: chunk.dataset.routeChunkIndex,
          region: chunk.dataset.routeRegion,
          left: chunk.style.left,
          width: chunk.style.width,
        }),
      ),
    ).toEqual(geometryBefore);

    act(() => vi.advanceTimersByTime(450));
    expect(world).toHaveAttribute("data-palette-transition", "settled");
  });

  it("keeps every emissive treatment separate from the scenery sprites", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const emissiveKinds = new Set(
      [...container.querySelectorAll<HTMLElement>("[data-emissive]")].map(
        (overlay) => overlay.dataset.emissive,
      ),
    );

    expect(emissiveKinds).toEqual(
      new Set([
        "stars",
        "moon",
        "windows",
        "streetlight",
        "station-lamp",
        "signal",
        "water-reflection",
      ]),
    );
    for (const overlay of container.querySelectorAll("[data-emissive]")) {
      expect(overlay.tagName).not.toBe("IMG");
    }
  });

  it("provides accessible foreground contrast in every palette", () => {
    expect(trainPaletteContrastRatio("day")).toBeGreaterThanOrEqual(4.5);
    expect(trainPaletteContrastRatio("sunset")).toBeGreaterThanOrEqual(4.5);
    expect(trainPaletteContrastRatio("night")).toBeGreaterThanOrEqual(4.5);
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
