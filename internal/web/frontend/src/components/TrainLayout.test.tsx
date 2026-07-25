import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { PaneStatus } from "../types/server";
import {
  advanceTrainWorldRoutePosition,
  minimumCarriagesForWidth,
  TRAIN_ARTWORK_SCALE,
  TRAIN_MIN_SEAT_TARGET_PX,
  TRAIN_WORLD_TRACK_PERSPECTIVE,
  TRAIN_WORLD_TRACK_TILE_WIDTH,
  trainPaletteContrastRatio,
  trainWorldCruiseSpeed,
  trainWorldTrackTransform,
  TrainLayout,
} from "./TrainLayout";
import {
  TRAIN_WORLD_MAX_FRAME_ELAPSED_MS,
  TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS,
  TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS,
} from "./trainMotion";
import {
  TRAIN_STATION_DEFAULT_DWELL_MS,
  TRAIN_STATION_PLATFORM_SETTLE_MS,
} from "./trainStation";

const trainLayoutCss = readFileSync(
  resolve(process.cwd(), "src/components/TrainLayout.css"),
  "utf8",
);

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

function mockReducedMotion() {
  vi.stubGlobal(
    "matchMedia",
    vi.fn().mockReturnValue({
      matches: true,
      media: "(prefers-reduced-motion: reduce)",
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    }),
  );
}

function advanceReducedMotionToState(
  world: HTMLElement,
  expectedState: string,
  limit = 200,
) {
  const routePositions: number[] = [
    Number.parseFloat(world.dataset.routePosition!),
  ];
  for (
    let step = 0;
    step < limit && world.dataset.stationState !== expectedState;
    step++
  ) {
    act(() => vi.advanceTimersByTime(TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS));
    routePositions.push(Number.parseFloat(world.dataset.routePosition!));
  }
  expect(world).toHaveAttribute("data-station-state", expectedState);
  return routePositions;
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

function routeGeometryFingerprint(container: HTMLElement): string[] {
  return [...container.querySelectorAll<HTMLElement>(".train-parallax-chunk")].map(
    (chunk) =>
      [
        chunk.dataset.parallaxLayer,
        chunk.dataset.routeChunkIndex,
        chunk.dataset.routeRegion,
        chunk.dataset.routeSetPiece,
        chunk.style.left,
        chunk.style.width,
      ].join(":"),
  );
}

describe("TrainLayout", () => {
  it("mounts a clipped world below an independent train inspection layer", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );

    const layout = container.querySelector(".train-layout");
    const world = container.querySelector(".train-layout-world");
    const inspection = container.querySelector(".train-layout-inspection");
    const track = container.querySelector(".train-world-track");
    expect(layout).toContainElement(world as HTMLElement);
    expect(layout).toContainElement(inspection as HTMLElement);
    expect(world).toContainElement(track as HTMLElement);
    expect(world).toHaveAttribute("data-layer", "world");
    expect(world).toHaveAttribute("data-route-direction", "right");
    expect(track).toHaveAttribute("data-world-track", "railway");
    expect(track).toHaveAttribute(
      "data-track-perspective",
      TRAIN_WORLD_TRACK_PERSPECTIVE,
    );
    expect(track).toHaveAttribute(
      "data-track-tile-width",
      String(TRAIN_WORLD_TRACK_TILE_WIDTH),
    );
    expect(track).toHaveAttribute("data-route-direction", "right");
    expect(track).toHaveAttribute("data-speed-ratio", "1");
    expect(inspection).toHaveAttribute("data-layer", "train");
    expect(inspection).not.toContainElement(track as HTMLElement);
    expect(container.querySelectorAll(".train-world-track")).toHaveLength(1);
    expect(container.querySelector(".train-layout-track")).not.toBeInTheDocument();
    expect(world?.nextElementSibling).toBe(inspection);
  });

  it("advances the world to the right from a single route position", () => {
    expect(advanceTrainWorldRoutePosition(10, 200)).toBe(12.4);
    expect(advanceTrainWorldRoutePosition(10, 500)).toBe(13);
    expect(advanceTrainWorldRoutePosition(16, -100)).toBe(16);
  });

  it("wraps the right-moving track transform to one bounded tile", () => {
    expect(TRAIN_WORLD_TRACK_TILE_WIDTH).toBe(240);
    expect(trainWorldTrackTransform(0)).toBe(0);
    expect(trainWorldTrackTransform(12.4)).toBeCloseTo(12.4);
    expect(trainWorldTrackTransform(252.4)).toBeCloseTo(12.4);
    expect(trainWorldTrackTransform(-12)).toBe(228);
    expect(trainWorldTrackTransform(Number.NaN)).toBe(0);
  });

  it("shares wheel clearance with one overscanned perspective track plane", () => {
    expect(trainLayoutCss).toMatch(
      /\.train-layout\s*\{[\s\S]*?--train-track-h:\s*19px;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-consist\s*\{[\s\S]*?height:\s*calc\(100% - var\(--train-track-h\)\);[\s\S]*?margin-bottom:\s*var\(--train-track-h\);/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-world-track\s*\{[\s\S]*?--train-track-tile-width:\s*240px;[\s\S]*?z-index:\s*5;[\s\S]*?right:\s*-240px;[\s\S]*?bottom:\s*0;[\s\S]*?left:\s*-240px;[\s\S]*?height:\s*var\(--train-track-h\);/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-world-track\s*\{[\s\S]*?repeating-linear-gradient\(\s*104deg,[\s\S]*?background-size:\s*100% 100%,\s*var\(--train-track-tile-width\) 100%,\s*100% 100%;[\s\S]*?background-repeat:\s*no-repeat,\s*repeat-x,\s*no-repeat;[\s\S]*?transform:\s*translate3d\(var\(--train-track-transform\), 0, 0\);/,
    );
    expect(trainLayoutCss).not.toContain("train-track-tile.png");
    expect(trainLayoutCss).not.toContain(".train-layout-track");
  });

  it("accepts a bounded development cruise-speed override", () => {
    expect(trainWorldCruiseSpeed("?train-cruise-speed=24")).toBe(24);
    expect(trainWorldCruiseSpeed("?train-cruise-speed=999")).toBe(96);
    expect(trainWorldCruiseSpeed("?train-cruise-speed=nope")).toBe(12);
  });

  it("completes a deterministic journey across regions, station, resize, theme switch, and remount", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 12, 0));
    window.history.replaceState(
      null,
      "",
      "/?train-route-seed=train-010-e2e&train-cruise-speed=96&train-station-trigger=approach",
    );
    const animation = installAnimationFrame();
    mockVisibility();
    const removeDocumentListener = vi.spyOn(document, "removeEventListener");
    const removeWindowListener = vi.spyOn(window, "removeEventListener");
    const journey = () => (
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />
    );
    const { container, rerender } = render(journey());
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const initialPosition = world.dataset.routePosition;
    const initialGeometry = routeGeometryFingerprint(container);
    const firstStation = world.dataset.stationEventId;
    const visitedRegions = new Set<string>();
    const visitedLandmarks = new Set<string>();
    const visitedStationStates = new Set<string>();
    let maximumMountedChunks = 0;
    let timestamp = 0;

    const observeJourney = () => {
      for (const chunk of container.querySelectorAll<HTMLElement>(
        '[data-world-layer="near"] .train-route-chunk',
      )) {
        if (chunk.dataset.routeRegion) {
          visitedRegions.add(chunk.dataset.routeRegion);
        }
        const setPiece = chunk.dataset.routeSetPiece;
        if (setPiece && setPiece !== "none" && setPiece !== "station") {
          visitedLandmarks.add(setPiece);
        }
      }
      if (world.dataset.stationState) {
        visitedStationStates.add(world.dataset.stationState);
      }
      maximumMountedChunks = Math.max(
        maximumMountedChunks,
        Number(world.dataset.routeTotalMountedChunks),
      );
    };

    expect(world).toHaveAttribute("data-route-seed", "train-010-e2e");
    expect(world).toHaveAttribute("data-time-of-day", "day");
    expect(world).toHaveAttribute("data-station-state", "approach");
    observeJourney();
    fireEvent.click(
      screen.getByRole("button", {
        name: "Cycle train lighting (day / sunset / night)",
      }),
    );
    expect(world).toHaveAttribute("data-time-of-day", "sunset");
    expect(world).toHaveAttribute("data-palette-transition", "crossfading");
    expect(routeGeometryFingerprint(container)).toEqual(initialGeometry);
    act(() => vi.advanceTimersByTime(450));
    expect(world).toHaveAttribute("data-palette-transition", "settled");

    animation.run(timestamp);
    for (
      let frame = 0;
      frame < 360 &&
      !(
        world.dataset.stationState === "cruise" &&
        world.dataset.stationEventId !== firstStation
      );
      frame++
    ) {
      timestamp += 250;
      animation.run(timestamp);
      observeJourney();
    }

    expect(visitedStationStates).toEqual(
      new Set([
        "approach",
        "decelerate",
        "platform",
        "dwell",
        "depart",
        "cruise",
      ]),
    );
    expect(world.dataset.stationEventId).not.toBe(firstStation);

    Object.defineProperty(world, "clientWidth", {
      configurable: true,
      get: () => 2_560,
    });
    act(() => window.dispatchEvent(new Event("resize")));
    observeJourney();
    expect(Number(world.dataset.routeTotalMountedChunks)).toBeGreaterThan(
      initialGeometry.length,
    );

    for (
      let frame = 0;
      frame < 480 &&
      (visitedRegions.size < 3 || visitedLandmarks.size === 0);
      frame++
    ) {
      timestamp += 250;
      animation.run(timestamp);
      observeJourney();
    }

    expect(visitedRegions.size).toBeGreaterThanOrEqual(3);
    expect(visitedLandmarks.size).toBeGreaterThan(0);
    expect(maximumMountedChunks).toBeLessThanOrEqual(80);
    expect(container.querySelectorAll(".train-parallax-chunk").length).toBe(
      Number(world.dataset.routeTotalMountedChunks),
    );

    rerender(<div data-active-theme="office" />);
    expect(container.querySelector(".train-layout")).not.toBeInTheDocument();
    expect(animation.pending()).toBe(0);
    expect(removeDocumentListener).toHaveBeenCalledWith(
      "visibilitychange",
      expect.any(Function),
    );
    expect(removeWindowListener).toHaveBeenCalledWith(
      "resize",
      expect.any(Function),
    );

    rerender(journey());
    const remountedWorld =
      container.querySelector<HTMLElement>(".train-layout-world")!;
    expect(remountedWorld).not.toBe(world);
    expect(remountedWorld.dataset.routePosition).toBe(initialPosition);
    expect(routeGeometryFingerprint(container)).toEqual(initialGeometry);
    expect(remountedWorld).toHaveAttribute("data-route-apply-count", "1");
  });

  it("renders a legible station composition behind the train without changing its span", () => {
    window.history.replaceState(
      null,
      "",
      "/?train-station-trigger=approach",
    );
    installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const stationSegments = [
      ...container.querySelectorAll<HTMLElement>(
        '.train-set-piece[data-set-piece-type="station"]',
      ),
    ].sort(
      (left, right) =>
        Number(left.dataset.setPieceSegment) -
        Number(right.dataset.setPieceSegment),
    );

    expect(world).toHaveAttribute("data-station-state", "approach");
    expect(world).toHaveAttribute("data-station-target-speed", "8.400");
    expect(stationSegments).toHaveLength(6);
    expect(stationSegments.map((segment) => segment.dataset.setPieceRole)).toEqual(
      ["entry", "body", "body", "body", "body", "exit"],
    );
    expect(
      new Set(stationSegments.map((segment) => segment.dataset.setPieceId)),
    ).toHaveProperty("size", 1);
    expect(
      stationSegments.every(
        (segment) =>
          segment.dataset.stationAssets ===
            "platform,building,canopy,lamps" &&
          segment.dataset.stationVerticalZone === "behind-train",
      ),
    ).toBe(true);
    expect(
      container.querySelectorAll("[data-station-asset='platform']"),
    ).toHaveLength(6);
    expect(
      container.querySelectorAll("[data-station-asset='building']"),
    ).toHaveLength(6);
    expect(
      container.querySelectorAll("[data-station-asset='canopy']"),
    ).toHaveLength(6);
    expect(
      container.querySelectorAll("[data-station-asset='lamp']"),
    ).toHaveLength(12);
    expect(
      container.querySelectorAll("[data-station-asset='signal']"),
    ).toHaveLength(4);
    expect(
      [...container.querySelectorAll<HTMLElement>(
        "[data-station-asset='signal']",
      )].map((signal) => signal.dataset.stationSignalAspect),
    ).toEqual(["approach", "approach", "proceed", "proceed"]);
    expect(
      container.querySelectorAll("[data-station-ambient-detail='steam']"),
    ).toHaveLength(1);
    expect(
      container.querySelectorAll("[data-station-asset='sign']"),
    ).toHaveLength(1);
  });

  it("keeps station scenery pointer-inert below the train and caps compact station geometry", () => {
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\s*\{[\s\S]*?z-index:\s*0;[\s\S]*?pointer-events:\s*none;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-inspection\s*\{[\s\S]*?z-index:\s*1;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-set-piece--station\s*\{[\s\S]*?bottom:\s*17px;[\s\S]*?height:\s*112px;/,
    );
    expect(trainLayoutCss).toMatch(
      /@media \(max-width:\s*760px\)[\s\S]*?\.train-station-canopy\s*\{[\s\S]*?right:\s*0;[\s\S]*?left:\s*0;/,
    );
  });

  it("stops positional scenery for dwell while ambient details continue, then departs continuously", () => {
    window.history.replaceState(
      null,
      "",
      "/?train-station-trigger=approach",
    );
    const animation = installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const track = container.querySelector<HTMLElement>(".train-world-track")!;
    const states = new Set<string>([world.dataset.stationState!]);
    const firstStation = world.dataset.stationEventId;
    let timestamp = 0;

    animation.run(timestamp);
    for (
      let frame = 0;
      frame < 300 && world.dataset.stationState !== "dwell";
      frame++
    ) {
      timestamp += 250;
      animation.run(timestamp);
      states.add(world.dataset.stationState!);
    }

    expect(world).toHaveAttribute("data-station-state", "dwell");
    expect(world).toHaveAttribute("data-station-positional-motion", "stopped");
    expect(world).toHaveAttribute("data-station-ambient", "running");
    const dwellPosition = world.dataset.routePosition;
    const dwellTrackPosition = track.dataset.trackPosition;
    expect(dwellTrackPosition).toBe(dwellPosition);

    timestamp += 250;
    animation.run(timestamp);
    expect(world.dataset.routePosition).toBe(dwellPosition);
    expect(track.dataset.trackPosition).toBe(dwellTrackPosition);

    for (
      let frame = 0;
      frame < 300 &&
      !(
        world.dataset.stationState === "cruise" &&
        world.dataset.stationEventId !== firstStation
      );
      frame++
    ) {
      timestamp += 250;
      animation.run(timestamp);
      states.add(world.dataset.stationState!);
    }

    expect(states).toEqual(
      new Set([
        "approach",
        "decelerate",
        "platform",
        "dwell",
        "depart",
        "cruise",
      ]),
    );
    expect(world.dataset.stationEventId).not.toBe(firstStation);
    expect(Number.parseFloat(world.dataset.routePosition!)).toBeGreaterThan(
      Number.parseFloat(dwellPosition!),
    );
    expect(world).toHaveAttribute("data-station-target-speed", "12.000");
  });

  it("pauses station departure while hidden and resumes without a leap", () => {
    window.history.replaceState(
      null,
      "",
      "/?train-station-trigger=depart",
    );
    const animation = installAnimationFrame();
    const visibility = mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const track = container.querySelector<HTMLElement>(".train-world-track")!;

    animation.run(1_000);
    animation.run(1_250);
    const pausedPosition = world.dataset.routePosition;
    const pausedSpeed = world.dataset.stationCurrentSpeed;
    const pausedTrackPosition = track.dataset.trackPosition;

    visibility.set("hidden");
    expect(world).toHaveAttribute("data-motion-state", "suspended");
    expect(track).toHaveAttribute("data-motion-state", "suspended");
    expect(world).toHaveAttribute("data-station-ambient", "suspended");
    expect(animation.pending()).toBe(0);

    visibility.set("visible");
    animation.run(100_000);
    expect(world.dataset.routePosition).toBe(pausedPosition);
    expect(track.dataset.trackPosition).toBe(pausedTrackPosition);
    expect(world.dataset.stationCurrentSpeed).toBe(pausedSpeed);
    animation.run(100_250);
    expect(Number.parseFloat(world.dataset.routePosition!)).toBeGreaterThan(
      Number.parseFloat(pausedPosition!),
    );
  });

  it("progresses from the animation clock without rerendering the train", () => {
    const animation = installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const track = container.querySelector<HTMLElement>(".train-world-track")!;
    const consist = container.querySelector(".train-layout-consist");

    animation.run(1_000);
    animation.run(1_200);

    expect(world).toHaveAttribute("data-cruise-speed", "12");
    expect(world).toHaveAttribute("data-route-position", "2.400px");
    expect(world).toHaveAttribute("data-route-apply-count", "2");
    expect(world).toHaveAttribute("data-route-window-updates", "1");
    expect(track).toHaveAttribute("data-track-position", "2.400px");
    expect(track).toHaveAttribute("data-track-transform", "2.400px");
    expect(track.style.getPropertyValue("--train-track-transform")).toBe(
      "2.400px",
    );
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
    expect(container.querySelectorAll(".train-world-track")).toHaveLength(1);
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
      /seed infinite-journey .* position 0\.0px .* chunks .* near \d+ .* total \d+/,
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
    expect(Number(world!.dataset.routeTotalMountedChunks)).toBe(
      container.querySelectorAll(".train-parallax-chunk").length,
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

  it("renders deterministic set-piece segments with restrained occlusion", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const segments = [
      ...container.querySelectorAll<HTMLElement>("[data-set-piece-id]"),
    ].filter((element) => element.classList.contains("train-set-piece"));
    const townEdge = segments.filter(
      (segment) => segment.dataset.setPieceType === "town-edge",
    );

    expect(townEdge.map((segment) => segment.dataset.setPieceRole)).toEqual([
      "entry",
      "body",
      "exit",
    ]);
    expect(
      new Set(townEdge.map((segment) => segment.dataset.setPieceId)),
    ).toHaveProperty("size", 1);
    for (const segment of segments) {
      expect(segment).toHaveAttribute("data-set-piece-occlusion", "restrained");
      expect(segment.dataset.setPieceRole).toMatch(/^(entry|body|exit)$/);
      expect(Number(segment.dataset.setPieceSpan)).toBeGreaterThanOrEqual(3);
      expect(Number(segment.dataset.setPieceStart)).toBeLessThanOrEqual(
        Number(segment.dataset.setPieceEnd),
      );
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
    expect(container.querySelector(".train-world-track")).toHaveAttribute(
      "data-motion",
      "reduced",
    );
    expect(layers).toHaveLength(5);
    for (const layer of layers) {
      expect(layer).toHaveAttribute("data-motion", "reduced");
      expect((layer as HTMLElement).dataset.layerPosition).toBe("0.000px");
    }
  });

  it("uses restrained infrequent route steps for reduced motion", () => {
    vi.useFakeTimers();
    mockVisibility();
    mockReducedMotion();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const track = container.querySelector<HTMLElement>(".train-world-track")!;

    act(() => vi.advanceTimersByTime(TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS - 1));
    expect(world).toHaveAttribute("data-route-position", "0.000px");
    expect(track).toHaveAttribute("data-track-position", "0.000px");
    act(() => vi.advanceTimersByTime(1));
    expect(world).toHaveAttribute("data-route-position", "1.200px");
    expect(track).toHaveAttribute("data-track-position", "1.200px");
    expect(track).toHaveAttribute("data-track-transform", "1.200px");
    expect(
      container.querySelector<HTMLElement>('[data-world-layer="near"]')!
        .dataset.layerPosition,
    ).toBe("1.200px");
  });

  it("keeps reduced-motion approach restrained while station phases use wall-clock time", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 12, 0));
    window.history.replaceState(
      null,
      "",
      "/?train-cruise-speed=96&train-station-trigger=approach",
    );
    mockVisibility();
    mockReducedMotion();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;

    const routePositions = advanceReducedMotionToState(world, "platform");
    const routeDeltas = routePositions.slice(1).map(
      (position, index) => position - routePositions[index]!,
    );
    expect(
      routeDeltas.every(
        (delta) =>
          delta >= 0 &&
          delta <=
            (96 * TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS) / 1_000,
      ),
    ).toBe(true);
    const stopPosition = world.dataset.routePosition;

    act(() => vi.advanceTimersByTime(TRAIN_STATION_PLATFORM_SETTLE_MS - 1));
    expect(world).toHaveAttribute("data-station-state", "platform");
    expect(world.dataset.routePosition).toBe(stopPosition);
    act(() => vi.advanceTimersByTime(1));
    expect(world).toHaveAttribute("data-station-state", "dwell");

    act(() => vi.advanceTimersByTime(TRAIN_STATION_DEFAULT_DWELL_MS - 1));
    expect(world).toHaveAttribute("data-station-state", "dwell");
    expect(world.dataset.routePosition).toBe(stopPosition);
    act(() => vi.advanceTimersByTime(1));
    expect(world).toHaveAttribute("data-station-state", "depart");
    expect(world.dataset.routePosition).toBe(stopPosition);

    act(() => vi.advanceTimersByTime(TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS));
    expect(Number.parseFloat(world.dataset.routePosition!)).toBeGreaterThan(
      Number.parseFloat(stopPosition!),
    );
  });

  it("suspends the reduced-motion station wall clock while hidden and cleans it up", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 12, 0));
    window.history.replaceState(
      null,
      "",
      "/?train-cruise-speed=96&train-station-trigger=approach",
    );
    const visibility = mockVisibility();
    mockReducedMotion();
    const removeListener = vi.spyOn(document, "removeEventListener");
    const { container, unmount } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const track = container.querySelector<HTMLElement>(".train-world-track")!;

    advanceReducedMotionToState(world, "platform");
    act(() => vi.advanceTimersByTime(100));
    const hiddenTrackPosition = track.dataset.trackPosition;
    visibility.set("hidden");
    expect(world).toHaveAttribute("data-motion-state", "suspended");
    expect(track).toHaveAttribute("data-motion-state", "suspended");
    // Positional/station motion is fully suspended; only the independent
    // next-palette-boundary clock remains scheduled.
    expect(vi.getTimerCount()).toBe(1);
    act(() => vi.advanceTimersByTime(60_000));
    expect(world).toHaveAttribute("data-station-state", "platform");
    expect(track.dataset.trackPosition).toBe(hiddenTrackPosition);

    visibility.set("visible");
    act(() => vi.advanceTimersByTime(149));
    expect(world).toHaveAttribute("data-station-state", "platform");
    act(() => vi.advanceTimersByTime(1));
    expect(world).toHaveAttribute("data-station-state", "dwell");

    act(() => vi.advanceTimersByTime(2_000));
    visibility.set("hidden");
    act(() => vi.advanceTimersByTime(60_000));
    expect(world).toHaveAttribute("data-station-state", "dwell");
    visibility.set("visible");
    act(() => vi.advanceTimersByTime(1_999));
    expect(world).toHaveAttribute("data-station-state", "dwell");
    act(() => vi.advanceTimersByTime(1));
    expect(world).toHaveAttribute("data-station-state", "depart");

    unmount();
    expect(vi.getTimerCount()).toBe(0);
    expect(removeListener).toHaveBeenCalledWith(
      "visibilitychange",
      expect.any(Function),
    );
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

  it("crosses from day to sunset at the 17:00 local-time boundary", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 16, 59, 59, 500));
    installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const geometryBefore = routeGeometryFingerprint(container);
    const positionBefore = world.dataset.routePosition;

    act(() => vi.advanceTimersByTime(499));
    expect(world).toHaveAttribute("data-time-of-day", "day");

    act(() => vi.advanceTimersByTime(1));
    expect(world).toHaveAttribute("data-time-of-day", "sunset");
    expect(world).toHaveAttribute("data-time-source", "clock");
    expect(world).toHaveAttribute("data-palette-transition", "crossfading");
    expect(world.dataset.routePosition).toBe(positionBefore);
    expect(routeGeometryFingerprint(container)).toEqual(geometryBefore);

    act(() => vi.advanceTimersByTime(450));
    expect(world).toHaveAttribute("data-palette-transition", "settled");
  });

  it("crosses from sunset to night at the 18:30 local-time boundary", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 18, 29, 59, 500));
    installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const geometryBefore = routeGeometryFingerprint(container);
    const positionBefore = world.dataset.routePosition;

    act(() => vi.advanceTimersByTime(499));
    expect(world).toHaveAttribute("data-time-of-day", "sunset");

    act(() => vi.advanceTimersByTime(1));
    expect(world).toHaveAttribute("data-time-of-day", "night");
    expect(world).toHaveAttribute("data-time-source", "clock");
    expect(world).toHaveAttribute("data-palette-transition", "crossfading");
    expect(world.dataset.routePosition).toBe(positionBefore);
    expect(routeGeometryFingerprint(container)).toEqual(geometryBefore);
  });

  it("keeps a manual palette stable while clock boundaries pass", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 16, 59, 59, 500));
    installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const geometryBefore = routeGeometryFingerprint(container);
    const positionBefore = world.dataset.routePosition;

    fireEvent.click(
      screen.getByRole("button", {
        name: "Cycle train lighting (day / sunset / night)",
      }),
    );
    expect(world).toHaveAttribute("data-time-of-day", "sunset");
    expect(world).toHaveAttribute("data-time-source", "manual");

    act(() => vi.advanceTimersByTime(90 * 60 * 1_000 + 500));
    expect(world).toHaveAttribute("data-time-of-day", "sunset");
    expect(world).toHaveAttribute("data-time-source", "manual");
    expect(world.dataset.routePosition).toBe(positionBefore);
    expect(routeGeometryFingerprint(container)).toEqual(geometryBefore);
  });

  it("cleans up its clock timer and reschedules from current time on remount", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 16, 59, 59));
    installAnimationFrame();
    mockVisibility();
    const journey = () => (
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />
    );
    const firstMount = render(journey());

    expect(vi.getTimerCount()).toBeGreaterThan(0);
    firstMount.unmount();
    expect(vi.getTimerCount()).toBe(0);

    vi.setSystemTime(new Date(2026, 0, 1, 18, 29, 59));
    const secondMount = render(journey());
    const remountedWorld =
      secondMount.container.querySelector<HTMLElement>(".train-layout-world")!;
    expect(remountedWorld).toHaveAttribute("data-time-of-day", "sunset");

    act(() => vi.advanceTimersByTime(1_000));
    expect(remountedWorld).toHaveAttribute("data-time-of-day", "night");
    secondMount.unmount();
    expect(vi.getTimerCount()).toBe(0);
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
    const track = container.querySelector<HTMLElement>(".train-world-track");
    expect(world).not.toBeNull();
    expect(inspection).not.toBeNull();
    expect(track).not.toBeNull();

    const routePosition = world!.dataset.routePosition;
    const trackPosition = track!.dataset.trackPosition;
    const trackTransform = track!.style.transform;
    inspection!.scrollLeft = 240;
    fireEvent.scroll(inspection!);

    expect(inspection).toHaveProperty("scrollLeft", 240);
    expect(world!.dataset.routePosition).toBe(routePosition);
    expect(track!.dataset.trackPosition).toBe(trackPosition);
    expect(track!.style.transform).toBe(trackTransform);
    expect(container.querySelector(".train-world-track")).toBe(track);
    expect(container.querySelectorAll(".train-world-track")).toHaveLength(1);
    expect(inspection!.contains(world)).toBe(false);
    expect(inspection!.contains(track)).toBe(false);
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
    expect(container.querySelector(".train-world-track")).toBeInTheDocument();
  });

  it("calculates enough overlapping carriages to cover a wide viewport", () => {
    expect(minimumCarriagesForWidth(1920, 143, 242 * TRAIN_ARTWORK_SCALE)).toBe(9);
    expect(minimumCarriagesForWidth(900, 143, 242)).toBe(4);
    expect(minimumCarriagesForWidth(0, 0, 0)).toBe(1);
  });

  it("scales only train artwork to 90% and preserves the fixed world baseline", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );

    expect(TRAIN_ARTWORK_SCALE).toBe(0.9);
    expect(container.querySelector(".train-layout")).toHaveAttribute(
      "data-artwork-scale",
      "0.9",
    );
    expect(container.querySelector(".train-carriage")).toHaveAttribute(
      "data-artwork-scale",
      "0.9",
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout\s*\{[\s\S]*?--train-artwork-scale:\s*90%;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-locomotive\s*\{[\s\S]*?height:\s*var\(--train-artwork-scale\);/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-carriage\s*\{[\s\S]*?height:\s*var\(--train-artwork-scale\);/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-world-track\s*\{[\s\S]*?bottom:\s*0;[\s\S]*?height:\s*var\(--train-track-h\);/,
    );
    expect(trainLayoutCss).not.toMatch(
      /\.train-layout-world\s*\{[^}]*transform:\s*scale/,
    );
  });

  it("keeps seat hit targets at least 44px without scaling their focus rings", () => {
    const { container } = render(
      <TrainLayout
        panes={[pane({ pane_id: "%1", session: "alpha", runtime: "codex" })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );
    const passenger = screen.getByRole("button", {
      name: "Select pane alpha, idle",
    });

    expect(TRAIN_MIN_SEAT_TARGET_PX).toBe(44);
    expect(passenger).toHaveAttribute("data-min-hit-size", "44");
    expect(passenger.querySelector(".train-seat-artwork")).toBeInTheDocument();
    expect(container.querySelectorAll(".train-seat-artwork")).toHaveLength(4);
    expect(trainLayoutCss).toMatch(
      /--train-seat-target-size:\s*max\(\s*var\(--train-seat-target-min\),\s*18\.8888888889cqw\s*\);/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-seat-artwork\s*\{[\s\S]*?width:\s*17cqw;[\s\S]*?height:\s*17cqw;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-seat:focus-visible\s*\{[\s\S]*?outline:\s*2px solid var\(--accent\);/,
    );
    expect(trainLayoutCss).not.toMatch(
      /\.train-seat\s*\{[^}]*transform:\s*scale/,
    );
  });

  it("keeps packed seat targets non-overlapping and routes edge clicks to one pane", () => {
    const onSelect = vi.fn();
    render(
      <TrainLayout
        panes={[
          pane({ pane_id: "%1", session: "alpha", runtime: "codex" }),
          pane({ pane_id: "%2", session: "beta", runtime: "codex" }),
          pane({ pane_id: "%3", session: "gamma", runtime: "codex" }),
          pane({ pane_id: "%4", session: "delta", runtime: "codex" }),
        ]}
        selected={null}
        onSelect={onSelect}
      />,
    );

    expect(trainLayoutCss).toMatch(
      /--train-seat-column-gap:\s*max\([\s\S]*?var\(--train-seat-target-size\),[\s\S]*?calc\(34cqw - 18px\)[\s\S]*?\);/,
    );
    expect(trainLayoutCss).toMatch(
      /--train-seat-row-gap:\s*max\(var\(--train-seat-target-size\),\s*38cqh\);/,
    );
    const beta = screen.getByRole("button", {
      name: "Select pane beta, idle",
    });
    fireEvent.pointerDown(beta, { clientX: 1, clientY: 1 });
    fireEvent.click(beta, { clientX: 1, clientY: 1 });
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith("%2");
  });

  it("preserves keyboard navigation and activation on enlarged seat targets", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(
      <TrainLayout
        panes={[pane({ pane_id: "%1", session: "alpha", runtime: "codex" })]}
        selected={null}
        onSelect={onSelect}
      />,
    );

    await user.tab();
    expect(
      screen.getByRole("button", { name: "Show recently closed sessions" }),
    ).toHaveFocus();
    await user.tab();
    expect(
      screen.getByRole("button", { name: "Select pane alpha, idle" }),
    ).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(onSelect).toHaveBeenCalledWith("%1");
  });

  it("keeps a golden set aura exclusive to the selected passenger", () => {
    const panes = [
      pane({ pane_id: "%1", session: "alpha", runtime: "codex" }),
      pane({ pane_id: "%2", session: "beta", runtime: "codex" }),
    ];
    const { container, rerender } = render(
      <TrainLayout panes={panes} selected="%1" onSelect={vi.fn()} />,
    );

    const alpha = screen.getByRole("button", {
      name: "Select pane alpha, idle",
    });
    const beta = screen.getByRole("button", {
      name: "Select pane beta, idle",
    });
    expect(alpha.querySelector(".train-selected-set-aura")).toBeInTheDocument();
    expect(beta.querySelector(".train-selected-set-aura")).not.toBeInTheDocument();
    expect(container.querySelectorAll(".train-selected-set-aura")).toHaveLength(1);

    rerender(<TrainLayout panes={panes} selected="%2" onSelect={vi.fn()} />);
    expect(alpha.querySelector(".train-selected-set-aura")).not.toBeInTheDocument();
    expect(beta.querySelector(".train-selected-set-aura")).toBeInTheDocument();
    expect(container.querySelectorAll(".train-selected-set-aura")).toHaveLength(1);

    rerender(<TrainLayout panes={panes} selected={null} onSelect={vi.fn()} />);
    expect(container.querySelector(".train-selected-set-aura")).not.toBeInTheDocument();
  });

  it("keeps keyboard focus independent from the selected golden treatment", async () => {
    const user = userEvent.setup();
    const { container } = render(
      <TrainLayout
        panes={[pane({ pane_id: "%1", session: "alpha", runtime: "codex" })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    await user.tab();
    await user.tab();
    const passenger = screen.getByRole("button", {
      name: "Select pane alpha, idle",
    });
    expect(passenger).toHaveFocus();
    expect(passenger).not.toHaveClass("selected");
    expect(container.querySelector(".train-selected-set-aura")).not.toBeInTheDocument();
    expect(trainLayoutCss).toMatch(
      /\.train-seat:focus-visible\s*\{[\s\S]*?outline:\s*2px solid var\(--accent\);/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-seat\.selected \.train-seat-sprite\s*\{[\s\S]*?#fff1ad[\s\S]*?rgba\(224,\s*145,\s*29,\s*0\.95\)/,
    );
  });

  it("preserves a legible golden selection when the passenger is stale", () => {
    const { container, rerender } = render(
      <TrainLayout
        panes={[
          pane({
            pane_id: "%1",
            session: "alpha",
            runtime: "codex",
            stale: true,
          }),
        ]}
        selected="%1"
        onSelect={vi.fn()}
      />,
    );

    const passenger = screen.getByRole("button", {
      name: "Select pane alpha, stale",
    });
    expect(passenger).toHaveClass("selected", "stale");
    expect(passenger).toHaveAttribute("aria-pressed", "true");
    expect(
      passenger.querySelector(".train-seat-artwork .train-seat-sprite"),
    ).toBeInTheDocument();
    expect(passenger.querySelector(".train-selected-set-aura")).toBeInTheDocument();
    expect(trainLayoutCss).toMatch(
      /\.train-seat\.selected\.stale \.train-seat-sprite\s*\{[\s\S]*?opacity:\s*0\.68;[\s\S]*?grayscale\(0\.32\)[\s\S]*?#fff1ad/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-seat\.stale:not\(\.selected\) \.train-seat-sprite\s*\{[\s\S]*?opacity:\s*0\.55;/,
    );

    rerender(
      <TrainLayout
        panes={[
          pane({
            pane_id: "%1",
            session: "alpha",
            runtime: "codex",
            stale: true,
          }),
        ]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );
    expect(container.querySelector(".train-selected-set-aura")).not.toBeInTheDocument();
  });

  it("uses a static but equally explicit golden aura for reduced motion", () => {
    mockReducedMotion();
    const { container } = render(
      <TrainLayout
        panes={[pane({ pane_id: "%1", session: "alpha", runtime: "codex" })]}
        selected="%1"
        onSelect={vi.fn()}
      />,
    );

    expect(container.querySelectorAll(".train-selected-set-aura")).toHaveLength(1);
    expect(trainLayoutCss).toMatch(
      /@keyframes train-selected-set-shimmer\s*\{[\s\S]*?filter:\s*brightness\(1\.08\);/,
    );
    expect(trainLayoutCss).toMatch(
      /@media \(prefers-reduced-motion: reduce\)\s*\{[\s\S]*?\.train-selected-set-aura\s*\{[\s\S]*?animation:\s*none;[\s\S]*?opacity:\s*0\.78;[\s\S]*?filter:\s*none;/,
    );
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
