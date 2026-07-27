import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { PaneStatus } from "../types/server";
import {
  advanceTrainWorldRoutePosition,
  minimumCarriagesForWidth,
  TRAIN_CARRIAGE_WHEEL_COUNT,
  TRAIN_ARTWORK_SCALE,
  TRAIN_LOCOMOTIVE_WHEEL_COUNT,
  TRAIN_MIN_SEAT_TARGET_PX,
  TRAIN_SCENERY_DEPTH_PROFILES,
  TRAIN_SCENERY_TIME_GRADES,
  TRAIN_TERRAIN_LAYER_ENVELOPES,
  TRAIN_TERRAIN_REGION_MATERIALS,
  TRAIN_TIME_PALETTES,
  TRAIN_WORLD_TRACK_PERSPECTIVE,
  TRAIN_WORLD_TRACK_TILE_WIDTH,
  trainPaletteContrastRatio,
  trainPaletteLuminanceOrder,
  trainTerrainContourForChunk,
  trainTerrainHeightAtPercent,
  trainTraversalActiveRole,
  trainWorldCruiseSpeed,
  trainWorldReducedMotionForced,
  trainWorldRoutePosition,
  trainWorldRouteSeed,
  trainWorldSetPieceFocus,
  trainWorldTrackTransform,
  TrainLayout,
  TrainRouteChunk,
} from "./TrainLayout";
import {
  trainWheelRotationDegrees,
  TRAIN_SKY_CLOUD_SPEED_RATIO,
  TRAIN_SKY_SUN_SPEED_RATIO,
  TRAIN_SKY_WISP_SPEED_RATIO,
  TRAIN_WORLD_MAX_FRAME_ELAPSED_MS,
  TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS,
  TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS,
} from "./trainMotion";
import {
  TRAIN_STATION_DEFAULT_DWELL_MS,
  TRAIN_STATION_PLATFORM_SETTLE_MS,
} from "./trainStation";
import {
  TRAIN_JOURNEY_PERSIST_INTERVAL_MS,
  TRAIN_JOURNEY_STORAGE_KEY,
  type TrainJourneySnapshot,
} from "./trainJourneySnapshot";
import {
  generateRouteChunk,
  TRAIN_PARALLAX_LAYERS,
  TRAIN_REGION_CHUNK_LENGTH,
} from "./trainRoute";
import {
  TRAIN_NIGHT_LIFE_MAX_INTENSITY,
  TRAIN_NIGHT_LIFE_MIN_INTENSITY,
  TRAIN_REGION_NIGHT_LIFE,
  trainCoastSceneryBeatForChunk,
  trainForestMountainSceneryBeatForChunk,
  trainNightLifeForPlacement,
  trainSceneryPlacementsForChunk,
  trainTownIndustrialSceneryBeatForChunk,
} from "./trainScenery";

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
  localStorage.clear();
  document
    .querySelectorAll("style[data-train-layout-test-styles]")
    .forEach((style) => style.remove());
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
        chunk.dataset.routeSetPieceVariant,
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
    expect(advanceTrainWorldRoutePosition(10, 200)).toBe(14.8);
    expect(advanceTrainWorldRoutePosition(10, 500)).toBe(16);
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

  it("seats the complete consist into the track with one responsive-safe offset", () => {
    const { container } = render(
      <TrainLayout
        panes={[pane({ pane_id: "%1", session: "alpha", runtime: "codex" })]}
        selected={null}
        onSelect={vi.fn()}
      />,
    );
    const consist = container.querySelector<HTMLElement>(
      ".train-layout-consist",
    )!;
    const track = container.querySelector<HTMLElement>(".train-world-track")!;

    expect(consist).toContainElement(
      container.querySelector<HTMLElement>(".train-locomotive-more"),
    );
    expect(consist).toContainElement(
      container.querySelector<HTMLElement>(".train-carriage"),
    );
    expect(consist).toContainElement(
      container.querySelector<HTMLElement>(".train-seat"),
    );
    expect(consist).not.toContainElement(track);
    expect(
      trainLayoutCss.match(/--train-consist-track-overlap:\s*12px;/g),
    ).toHaveLength(1);
    expect(trainLayoutCss).toMatch(
      /\.train-layout-consist\s*\{[\s\S]*?height:\s*calc\(100% - var\(--train-track-h\)\);[\s\S]*?margin-bottom:\s*var\(--train-track-h\);[\s\S]*?transform:\s*translateY\(var\(--train-consist-track-overlap\)\);/,
    );

    const trackHeight = Number(
      trainLayoutCss.match(/--train-track-h:\s*(\d+)px;/)?.[1],
    );
    const overlap = Number(
      trainLayoutCss.match(/--train-consist-track-overlap:\s*(\d+)px;/)?.[1],
    );
    expect(overlap).toBeGreaterThan(0);
    expect(overlap).toBeLessThan(trackHeight);

    const compactRules = trainLayoutCss.slice(
      trainLayoutCss.indexOf("@media (max-width: 760px)"),
    );
    expect(compactRules).not.toContain("--train-consist-track-overlap:");
    expect(trainLayoutCss).toMatch(
      /\.train-carriage\s*\{[\s\S]*?height:\s*var\(--train-artwork-scale\);[\s\S]*?margin-left:\s*clamp\(-19\.8px,\s*-1\.8vh,\s*-10\.8px\);/,
    );
    expect(trainLayoutCss).toMatch(
      /--train-seat-target-size:\s*max\(\s*var\(--train-seat-target-min\),\s*18\.8888888889cqw\s*\);/,
    );
    expect(trainLayoutCss).not.toMatch(
      /\.(?:train-locomotive-more|train-layout-locomotive|train-carriage|train-seat)\s*\{[^}]*translateY\(/,
    );
  });

  it("mounts code-native rims at bounded locomotive and carriage wheel centers", () => {
    const { container, rerender } = render(
      <TrainLayout
        panes={Array.from({ length: 12 }, (_, index) =>
          pane({
            pane_id: `%${index + 1}`,
            target: `s:0.${index}`,
            session: `session-${index}`,
            runtime: "codex",
          }),
        )}
        selected={null}
        onSelect={vi.fn()}
      />,
    );
    const layout = container.querySelector<HTMLElement>(".train-layout")!;
    const locomotiveLayer = container.querySelector(
      '[data-wheel-layer="locomotive"]',
    );
    const carriageLayers = container.querySelectorAll(
      '[data-wheel-layer="carriage"]',
    );

    expect(locomotiveLayer).toHaveAttribute(
      "data-wheel-count",
      String(TRAIN_LOCOMOTIVE_WHEEL_COUNT),
    );
    expect(
      locomotiveLayer?.querySelectorAll('[data-wheel-rim="locomotive"]'),
    ).toHaveLength(TRAIN_LOCOMOTIVE_WHEEL_COUNT);
    expect(carriageLayers).toHaveLength(3);
    for (const layer of carriageLayers) {
      expect(layer).toHaveAttribute(
        "data-wheel-count",
        String(TRAIN_CARRIAGE_WHEEL_COUNT),
      );
    }
    expect(container.querySelectorAll(".train-wheel-rim")).toHaveLength(15);
    expect(layout).toHaveAttribute("data-wheel-node-count", "15");
    expect(
      [...container.querySelectorAll<HTMLElement>(".train-wheel-rim")].every(
        (rim) =>
          Boolean(rim.dataset.wheelCenter) &&
          rim.style.getPropertyValue("--train-wheel-center-x").endsWith("%") &&
          rim.style.getPropertyValue("--train-wheel-center-y").endsWith("%"),
      ),
    ).toBe(true);

    rerender(<TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />);
    expect(container.querySelectorAll(".train-wheel-rim")).toHaveLength(7);
    expect(layout).toHaveAttribute("data-wheel-node-count", "7");
    expect(trainLayoutCss).toMatch(
      /\.train-wheel-rim\s*\{[\s\S]*?transform:\s*translate\(-50%, -50%\) rotate\(var\(--train-wheel-rotation\)\);/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-wheel-layer\s*\{[\s\S]*?pointer-events:\s*none;/,
    );
    expect(trainLayoutCss).not.toMatch(
      /\.(?:train-carriage|train-layout-locomotive)\s*\{[^}]*rotate\(/,
    );
  });

  it("updates all wheel rims from route distance in the existing motion owner", () => {
    const animation = installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const layout = container.querySelector<HTMLElement>(".train-layout")!;
    const consist = container.querySelector(".train-layout-consist");
    const rims = [...container.querySelectorAll(".train-wheel-rim")];

    expect(layout).toHaveAttribute("data-wheel-rotation", "0.000deg");
    animation.run(1_000);
    animation.run(1_200);
    expect(layout).toHaveAttribute(
      "data-wheel-rotation",
      `${trainWheelRotationDegrees(4.8).toFixed(3)}deg`,
    );
    expect(container.querySelector(".train-layout-consist")).toBe(consist);
    expect([...container.querySelectorAll(".train-wheel-rim")]).toEqual(rims);
    expect(
      layout.style.getPropertyValue("--train-wheel-rotation"),
    ).toBe(layout.dataset.wheelRotation);
  });

  it("accepts a bounded development cruise-speed override", () => {
    expect(trainWorldCruiseSpeed("?train-cruise-speed=24")).toBe(24);
    expect(trainWorldCruiseSpeed("?train-cruise-speed=999")).toBe(96);
    expect(trainWorldCruiseSpeed("?train-cruise-speed=nope")).toBe(24);
  });

  it("accepts a bounded development route-position override", () => {
    expect(trainWorldRoutePosition("?train-route-position=5760")).toBe(5760);
    expect(trainWorldRoutePosition("?train-route-position=9999999")).toBe(
      1_000_000,
    );
    expect(trainWorldRoutePosition("?train-route-position=-1")).toBe(0);
    expect(trainWorldRoutePosition("?train-route-position=nope")).toBe(0);
  });

  it("offers a development-only reduced-motion browser diagnostic", () => {
    expect(trainWorldReducedMotionForced("?train-reduced-motion=1")).toBe(true);
    expect(trainWorldReducedMotionForced("?train-reduced-motion=0")).toBe(false);
    expect(trainWorldReducedMotionForced("")).toBe(false);
  });

  it("focuses actual projected geometry and reserves every participating layer", () => {
    vi.spyOn(window, "innerWidth", "get").mockReturnValue(1_280);
    window.history.replaceState(
      null,
      "",
      "/?train-route-seed=layout-focus&train-set-piece-focus=coast-reveal&train-reduced-motion=1",
    );
    installAnimationFrame();
    mockVisibility();
    const focus = trainWorldSetPieceFocus(
      window.location.search,
      "layout-focus",
      1_280,
    )!;
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;

    expect(world).toHaveAttribute("data-set-piece-focus-type", "coast-reveal");
    expect(world).toHaveAttribute("data-set-piece-focus-id", focus.id);
    expect(world).toHaveAttribute(
      "data-set-piece-focus-position",
      focus.journeyPosition.toFixed(3),
    );
    expect(world).toHaveAttribute(
      "data-set-piece-focus-segments",
      focus.expectedVisibleSegmentIDs.join(","),
    );

    const rendered = [
      ...container.querySelectorAll<HTMLElement>(
        '.train-set-piece-projection-segment[data-parallax-layer="far"] ' +
          '[data-set-piece-type="coast-reveal"]',
      ),
    ];
    expect(rendered).toHaveLength(focus.span);
    expect(rendered.map((segment) => segment.dataset.setPieceSegment)).toEqual(
      Array.from({ length: focus.span }, (_, offset) => String(offset)),
    );

    for (const layer of ["far", "midground", "near"]) {
      const reservations = container.querySelectorAll<HTMLElement>(
        `.train-set-piece-projection-segment[data-parallax-layer="${layer}"]` +
          `[data-set-piece-reservation="${focus.id}"]`,
      );
      expect(reservations).toHaveLength(focus.span);
    }
    for (const reservedChunk of container.querySelectorAll<HTMLElement>(
      '[data-scenery-reserved="projected-set-piece"]',
    )) {
      expect(
        reservedChunk.querySelector("[data-scenery-asset]"),
      ).not.toBeInTheDocument();
      expect(
        reservedChunk.querySelector(
          [
            "[data-built-ground]",
            "[data-built-fixture]",
            "[data-coast-composition]",
            ".train-forest-ground-details",
          ].join(","),
        ),
      ).not.toBeInTheDocument();
    }
    expect(
      Number(world.dataset.projectedSetPieceSegments),
    ).toBeLessThanOrEqual(48);
    expect(container.querySelectorAll(".train-parallax-chunk")).toHaveLength(
      Number(world.dataset.routeTotalMountedChunks),
    );
  });

  it("keeps an explicitly focused traversal or station visible during ultrawide collision arbitration", () => {
    vi.spyOn(window, "innerWidth", "get").mockReturnValue(2_560);
    installAnimationFrame();
    mockVisibility();

    for (const proof of [
      {
        search:
          "/?train-route-seed=train-053-aurora&train-set-piece-focus=bridge" +
          "&train-set-piece-occurrence=0&train-reduced-motion=1",
        type: "bridge",
      },
      {
        search:
          "/?train-route-seed=train-053-cascade&train-set-piece-focus=tunnel" +
          "&train-set-piece-occurrence=0&train-reduced-motion=1",
        type: "tunnel",
      },
      {
        search:
          "/?train-route-seed=train-053-summit&train-set-piece-focus=station" +
          "&train-set-piece-occurrence=0&train-reduced-motion=1",
        type: "station",
      },
    ] as const) {
      window.history.replaceState(null, "", proof.search);
      const focus = trainWorldSetPieceFocus(
        window.location.search,
        trainWorldRouteSeed(window.location.search),
        2_560,
      )!;
      const rendered = render(
        <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
      );
      const world =
        rendered.container.querySelector<HTMLElement>(".train-layout-world")!;
      const focusedSegments = rendered.container.querySelectorAll(
        `[data-set-piece-id="${focus.id}"]`,
      );

      expect(world).toHaveAttribute("data-set-piece-focus-type", proof.type);
      expect(focusedSegments.length, proof.type).toBeGreaterThanOrEqual(
        focus.span,
      );
      expect(world.dataset.setPieceCollisionExcludedIds, proof.type).not.toContain(
        focus.id,
      );
      rendered.unmount();
    }
  });

  it("excludes incompatible projected stations from a readable coast entry during cruise", () => {
    vi.spyOn(window, "innerWidth", "get").mockReturnValue(390);
    window.history.replaceState(
      null,
      "",
      "/?train-route-seed=train-051-proof&train-route-position=56195&train-reduced-motion=1",
    );
    installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const coastID =
      "tmact-train-route-v1:train-051-proof:set-piece:20:coast-reveal";
    const stationID = "station:train-051-proof:9";

    expect(
      container.querySelectorAll(
        `[data-set-piece-id="${coastID}"][data-set-piece-layer="far"]`,
      ),
    ).toHaveLength(4);
    expect(
      container.querySelector(`[data-set-piece-id="${stationID}"]`),
    ).not.toBeInTheDocument();
    expect(Number(world.dataset.setPieceCollisionExclusions)).toBeGreaterThan(
      0,
    );
    expect(world.dataset.setPieceCollisionExcludedIds).toContain(stationID);
  });

  it("persists at a restrained cadence and remounts at identical route geometry", () => {
    vi.useFakeTimers();
    const startedAt = new Date(2026, 0, 1, 12, 0).getTime();
    vi.setSystemTime(startedAt);
    window.history.replaceState(
      null,
      "",
      "/?train-route-seed=resume-geometry",
    );
    const animation = installAnimationFrame();
    mockVisibility();
    const first = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world =
      first.container.querySelector<HTMLElement>(".train-layout-world")!;

    animation.run(0);
    vi.setSystemTime(startedAt + TRAIN_JOURNEY_PERSIST_INTERVAL_MS - 1);
    animation.run(100);
    expect(localStorage.getItem(TRAIN_JOURNEY_STORAGE_KEY)).toBeNull();

    vi.setSystemTime(startedAt + TRAIN_JOURNEY_PERSIST_INTERVAL_MS);
    animation.run(200);
    const persisted = JSON.parse(
      localStorage.getItem(TRAIN_JOURNEY_STORAGE_KEY)!,
    ) as TrainJourneySnapshot;
    expect(persisted.routeSeed).toBe("resume-geometry");
    expect(persisted.routePosition).toBeGreaterThan(0);
    expect(world).toHaveAttribute("data-journey-persistence", "saved");
    const savedPosition = world.dataset.routePosition;
    const savedGeometry = routeGeometryFingerprint(first.container);
    const savedSkyAnchors = [
      ...first.container.querySelectorAll<HTMLElement>(
        "[data-day-sky-anchor]",
      ),
    ].map((anchor) => [
      anchor.dataset.daySkyAnchorId,
      anchor.dataset.skyPosition,
      anchor.dataset.skyMotionDistance,
    ]);
    const savedCloudPosition =
      first.container.querySelector<HTMLElement>('[data-world-layer="sky"]')!
        .dataset.layerPosition;

    first.unmount();
    vi.setSystemTime(startedAt + 86_400_000);
    const second = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const restored =
      second.container.querySelector<HTMLElement>(".train-layout-world")!;

    expect(restored).toHaveAttribute("data-journey-restored", "true");
    expect(restored).toHaveAttribute("data-route-position", savedPosition);
    expect(restored).toHaveAttribute(
      "data-journey-checkpoint-position",
      savedPosition,
    );
    expect(routeGeometryFingerprint(second.container)).toEqual(savedGeometry);
    expect(
      [
        ...second.container.querySelectorAll<HTMLElement>(
          "[data-day-sky-anchor]",
        ),
      ].map((anchor) => [
        anchor.dataset.daySkyAnchorId,
        anchor.dataset.skyPosition,
        anchor.dataset.skyMotionDistance,
      ]),
    ).toEqual(savedSkyAnchors);
    expect(
      second.container.querySelector<HTMLElement>('[data-world-layer="sky"]')!
        .dataset.layerPosition,
    ).toBe(savedCloudPosition);
    animation.run(86_400_000);
    expect(restored).toHaveAttribute("data-route-position", savedPosition);
    animation.run(86_400_100);
    expect(
      Number.parseFloat(restored.dataset.routePosition!),
    ).toBeCloseTo(Number.parseFloat(savedPosition!) + 2.4, 3);
  });

  it("keeps station phases out of lifecycle checkpoints and resumes safely after departure", () => {
    window.history.replaceState(
      null,
      "",
      "/?train-route-seed=station-resume&train-cruise-speed=96&train-station-trigger=approach",
    );
    const animation = installAnimationFrame();
    mockVisibility();
    const first = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world =
      first.container.querySelector<HTMLElement>(".train-layout-world")!;
    const firstStation = world.dataset.stationEventId;
    const unsafeStates = new Set([
      "approach",
      "decelerate",
      "platform",
      "dwell",
      "depart",
    ]);
    const observedUnsafeStates = new Set<string>();
    let timestamp = 0;

    animation.run(timestamp);
    for (
      let frame = 0;
      frame < 400 &&
      !(
        world.dataset.stationState === "cruise" &&
        world.dataset.stationEventId !== firstStation
      );
      frame++
    ) {
      const state = world.dataset.stationState!;
      if (unsafeStates.has(state) && !observedUnsafeStates.has(state)) {
        observedUnsafeStates.add(state);
        act(() => window.dispatchEvent(new PageTransitionEvent("pagehide")));
        expect(localStorage.getItem(TRAIN_JOURNEY_STORAGE_KEY)).toBeNull();
      }
      timestamp += 250;
      animation.run(timestamp);
    }

    expect(observedUnsafeStates).toEqual(unsafeStates);
    expect(world).toHaveAttribute("data-station-state", "cruise");
    act(() => window.dispatchEvent(new PageTransitionEvent("pagehide")));
    const checkpoint = JSON.parse(
      localStorage.getItem(TRAIN_JOURNEY_STORAGE_KEY)!,
    ) as TrainJourneySnapshot;
    expect(checkpoint.routePosition).toBe(
      Number.parseFloat(world.dataset.routePosition!),
    );

    first.unmount();
    window.history.replaceState(
      null,
      "",
      "/?train-route-seed=station-resume&train-cruise-speed=96",
    );
    const second = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const restored =
      second.container.querySelector<HTMLElement>(".train-layout-world")!;
    expect(restored).toHaveAttribute("data-journey-restored", "true");
    expect(restored).toHaveAttribute("data-station-state", "cruise");
    expect(restored).toHaveAttribute(
      "data-route-position",
      `${checkpoint.routePosition.toFixed(3)}px`,
    );
  });

  it("falls back deterministically when a valid-schema checkpoint is not station-safe", () => {
    localStorage.setItem(
      TRAIN_JOURNEY_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        seedVersion: "tmact-train-route-v1",
        routeSeed: "unsafe-station-checkpoint",
        routePosition: 3_680,
      }),
    );
    installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;

    expect(world).toHaveAttribute(
      "data-route-seed",
      "unsafe-station-checkpoint",
    );
    expect(world).toHaveAttribute("data-journey-restored", "false");
    expect(world).toHaveAttribute("data-route-position", "0.000px");
    expect(world).toHaveAttribute("data-station-state", "cruise");
  });

  it("persists and restores the discrete reduced-motion checkpoint", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 12, 0));
    mockReducedMotion();
    mockVisibility();
    window.history.replaceState(
      null,
      "",
      "/?train-route-seed=reduced-resume",
    );
    const first = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world =
      first.container.querySelector<HTMLElement>(".train-layout-world")!;

    act(() =>
      vi.advanceTimersByTime(TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS * 2),
    );
    const checkpointPosition = world.dataset.routePosition;
    expect(Number.parseFloat(checkpointPosition!)).toBeGreaterThan(0);
    act(() => window.dispatchEvent(new PageTransitionEvent("pagehide")));
    first.unmount();

    const second = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const restored =
      second.container.querySelector<HTMLElement>(".train-layout-world")!;
    expect(restored).toHaveAttribute("data-motion", "reduced");
    expect(restored).toHaveAttribute("data-journey-restored", "true");
    expect(restored).toHaveAttribute("data-route-position", checkpointPosition);
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
    const layout = container.querySelector<HTMLElement>(".train-layout")!;
    const initialPosition = world.dataset.routePosition;
    const initialWheelRotation = layout.dataset.wheelRotation;
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
    expect(container.querySelectorAll(".train-wheel-rim")).toHaveLength(
      Number(layout.dataset.wheelNodeCount),
    );

    rerender(<div data-active-theme="office" />);
    expect(container.querySelector(".train-layout")).not.toBeInTheDocument();
    expect(container.querySelector(".train-wheel-rim")).not.toBeInTheDocument();
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
    const remountedLayout =
      container.querySelector<HTMLElement>(".train-layout")!;
    expect(remountedWorld).not.toBe(world);
    expect(remountedWorld.dataset.routePosition).toBe(initialPosition);
    expect(remountedLayout.dataset.wheelRotation).toBe(initialWheelRotation);
    expect(container.querySelectorAll(".train-wheel-rim")).toHaveLength(
      Number(remountedLayout.dataset.wheelNodeCount),
    );
    expect(routeGeometryFingerprint(container)).toEqual(initialGeometry);
    expect(remountedWorld).toHaveAttribute("data-route-apply-count", "1");
  });

  it("renders an articulated station campus behind the train without changing its span", () => {
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
    expect(world).toHaveAttribute("data-station-target-speed", "16.800");
    expect(stationSegments).toHaveLength(6);
    expect(stationSegments.map((segment) => segment.dataset.setPieceRole)).toEqual(
      ["entry", "body", "body", "body", "body", "exit"],
    );
    expect(
      new Set(stationSegments.map((segment) => segment.dataset.setPieceId)),
    ).toHaveProperty("size", 1);
    expect(stationSegments.map((segment) => segment.dataset.stationAssets)).toEqual(
      [
        "platform,campus,canopy,fixtures,service",
        "platform,campus,canopy,fixtures,service",
        "platform,campus,canopy,fixtures,service",
        "platform,campus,fixtures,service",
        "platform,campus,fixtures,service",
        "platform,campus,canopy,fixtures,service",
      ],
    );
    expect(
      stationSegments.every(
        (segment) => segment.dataset.stationVerticalZone === "behind-train",
      ),
    ).toBe(true);
    expect(
      container.querySelectorAll("[data-station-asset='platform']"),
    ).toHaveLength(6);
    expect(
      container.querySelectorAll("[data-station-asset='building']"),
    ).toHaveLength(3);
    expect(
      container.querySelectorAll("[data-station-asset='canopy']"),
    ).toHaveLength(4);
    expect(
      container.querySelectorAll("[data-station-asset='canopy-support']"),
    ).toHaveLength(7);
    expect(
      container.querySelectorAll("[data-station-asset='lamp']"),
    ).toHaveLength(4);
    expect(
      container.querySelectorAll("[data-station-asset='signal']"),
    ).toHaveLength(2);
    expect(
      [...container.querySelectorAll<HTMLElement>(
        "[data-station-asset='signal']",
      )].map((signal) => signal.dataset.stationSignalAspect),
    ).toEqual(["approach", "proceed"]);
    expect(
      [...container.querySelectorAll<HTMLElement>(
        "[data-station-architecture]",
      )].map((architecture) => architecture.dataset.stationBay),
    ).toEqual([
      "entrance",
      "west-waiting",
      "ticket-hall",
      "garden-platform",
      "service-yard",
      "departure",
    ]);
    expect(
      [...container.querySelectorAll<HTMLElement>(
        "[data-station-architecture]",
      )].map((architecture) => architecture.dataset.stationStructure),
    ).toEqual([
      "gatehouse",
      "waiting-bay",
      "station-house",
      "garden-bay",
      "service-shed",
      "exit-platform",
    ]);
    expect(
      [...container.querySelectorAll<HTMLElement>(
        "[data-station-asset='building']",
      )].map((building) => building.dataset.stationBuildingKind),
    ).toEqual(["gatehouse", "station-house", "service-shed"]);
    expect(
      container.querySelectorAll("[data-station-framed-opening]"),
    ).toHaveLength(6);
    expect(
      container.querySelectorAll("[data-station-asset='service']"),
    ).toHaveLength(8);
    expect(
      container.querySelectorAll("[data-station-ambient-detail='steam']"),
    ).toHaveLength(1);
    expect(
      container.querySelectorAll("[data-station-asset='sign']"),
    ).toHaveLength(1);
  });

  it("keeps all six campus roles whole while clipping only platform transitions", () => {
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);
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
    const stationSegments = [
      ...container.querySelectorAll<HTMLElement>(
        '.train-set-piece[data-set-piece-type="station"]',
      ),
    ].sort(
      (left, right) =>
        Number(left.dataset.setPieceSegment) -
        Number(right.dataset.setPieceSegment),
    );
    const architectures = stationSegments.map((segment) =>
      segment.querySelector<HTMLElement>("[data-station-architecture]"),
    );
    const transitions = stationSegments.map((segment) =>
      segment.querySelector<HTMLElement>(
        "[data-station-transition-geometry]",
      ),
    );

    expect(stationSegments).toHaveLength(6);
    expect(
      stationSegments.map((segment) => segment.dataset.setPieceRole),
    ).toEqual(["entry", "body", "body", "body", "body", "exit"]);
    expect(
      architectures.map((architecture) => architecture?.dataset.stationRole),
    ).toEqual(["entry", "body", "body", "body", "body", "exit"]);
    expect(
      transitions.map(
        (transition) => transition?.dataset.stationTransitionGeometry,
      ),
    ).toEqual(["entry", "body", "body", "body", "body", "exit"]);

    for (const [index, segment] of stationSegments.entries()) {
      const architecture = architectures[index]!;
      const transition = transitions[index]!;
      const role = segment.dataset.setPieceRole;

      expect(architecture).toHaveAttribute(
        "data-station-segment",
        `${index}`,
      );
      if ([0, 2, 4].includes(index)) {
        expect(architecture).toContainElement(
          segment.querySelector("[data-station-asset='building']"),
        );
      } else {
        expect(
          segment.querySelector("[data-station-asset='building']"),
        ).not.toBeInTheDocument();
      }
      if ([0, 1, 2, 5].includes(index)) {
        expect(architecture).toContainElement(
          segment.querySelector("[data-station-asset='canopy']"),
        );
      } else {
        expect(
          segment.querySelector("[data-station-asset='canopy']"),
        ).not.toBeInTheDocument();
      }
      expect(
        architecture.querySelectorAll("[data-station-asset='lamp']"),
      ).toHaveLength([0, 1, 1, 1, 0, 1][index]!);
      expect(architecture).not.toContainElement(
        segment.querySelector("[data-station-asset='platform']"),
      );
      expect(transition).toContainElement(
        segment.querySelector("[data-station-asset='platform']"),
      );
      expect(getComputedStyle(segment).clipPath).toBe("none");
      expect(getComputedStyle(segment).overflow).toBe("visible");
      expect(getComputedStyle(architecture).clipPath).toBe("none");
      expect(getComputedStyle(architecture).overflow).toBe("visible");
      if (role === "body") {
        expect(getComputedStyle(transition).clipPath).toBe("none");
      } else {
        expect(getComputedStyle(transition).clipPath).toContain("polygon(");
      }
    }

    expect(
      getComputedStyle(transitions[0]!).clipPath.replaceAll(" ", ""),
    ).toContain("100%0,100%100%");
    expect(
      getComputedStyle(transitions[5]!).clipPath.replaceAll(" ", ""),
    ).toContain("polygon(00,62%0");
    expect(
      architectures[2]!.querySelector("[data-station-asset='windows']"),
    ).toBeInTheDocument();
    expect(
      architectures[2]!.querySelector("[data-station-asset='sign']"),
    ).toBeInTheDocument();
    expect(
      container.querySelectorAll("[data-station-asset='signal']"),
    ).toHaveLength(2);
    expect(
      architectures[3]!.querySelector(
        "[data-station-framed-opening='garden-vista']",
      ),
    ).toBeInTheDocument();
  });

  it("joins six role-owned campus bays through one platform with intentional world openings", () => {
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
    const stationSegments = [
      ...container.querySelectorAll<HTMLElement>(
        '.train-set-piece[data-set-piece-type="station"]',
      ),
    ].sort(
      (left, right) =>
        Number(left.dataset.setPieceSegment) -
        Number(right.dataset.setPieceSegment),
    );

    expect(
      stationSegments.map(
        (segment) =>
          segment.querySelector<HTMLElement>("[data-station-architecture]")
            ?.dataset.stationBay,
      ),
    ).toEqual([
      "entrance",
      "west-waiting",
      "ticket-hall",
      "garden-platform",
      "service-yard",
      "departure",
    ]);
    expect(
      stationSegments.map(
        (segment) =>
          segment.querySelectorAll("[data-station-asset='lamp']").length,
      ),
    ).toEqual([0, 1, 1, 1, 0, 1]);
    expect(
      stationSegments.map(
        (segment) =>
          segment.querySelectorAll("[data-station-asset='canopy-support']")
            .length,
      ),
    ).toEqual([1, 2, 2, 0, 0, 2]);
    expect(
      [...container.querySelectorAll<HTMLElement>(
        "[data-station-asset='signal']",
      )].map((signal) => [
        signal.dataset.stationOwnerSegment,
        signal.dataset.stationSignalAspect,
      ]),
    ).toEqual([
      ["0", "approach"],
      ["5", "proceed"],
    ]);

    const framedOpenings = [
      ...container.querySelectorAll<HTMLElement>(
        "[data-station-framed-opening]",
      ),
    ];
    expect(
      framedOpenings.map((opening) => opening.dataset.stationFramedOpening),
    ).toEqual([
      "entry-vista",
      "waiting-vista",
      "ticket-vista",
      "garden-vista",
      "service-vista",
      "exit-vista",
    ]);
    expect(
      framedOpenings.every(
        (opening) =>
          opening.dataset.stationOpeningOwner === "world-parallax" &&
          opening.querySelector(
            "[data-station-view-owner='world-parallax']",
          ) !== null,
      ),
    ).toBe(true);
    expect(
      stationSegments.map(
        (segment) =>
          segment.querySelectorAll("[data-station-asset='platform']").length,
      ),
    ).toEqual([1, 1, 1, 1, 1, 1]);
    expect(trainLayoutCss).toMatch(
      /\.train-station-building--station-house\s*\{[^}]*right:\s*26%;[^}]*left:\s*26%;/,
    );
    expect(trainLayoutCss).not.toMatch(
      /\.train-station-building\s*\{[^}]*(?:right|left):\s*-2px;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-station-open-air-view\s*\{[^}]*background:\s*transparent;/,
    );
  });

  it("limits campus masses and fixtures while preserving platform contact and negative space", () => {
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
    const stationSegments = [
      ...container.querySelectorAll<HTMLElement>(
        '.train-set-piece[data-set-piece-type="station"]',
      ),
    ].sort(
      (left, right) =>
        Number(left.dataset.setPieceSegment) -
        Number(right.dataset.setPieceSegment),
    );
    const architectures = stationSegments.map(
      (segment) =>
        segment.querySelector<HTMLElement>("[data-station-architecture]")!,
    );

    expect(
      architectures.map((architecture) => architecture.dataset.stationMassRole),
    ).toEqual([
      "entry-house",
      "open-waiting",
      "ticket-house",
      "open-garden",
      "service-house",
      "open-departure",
    ]);
    expect(
      architectures.map(
        (architecture) => architecture.dataset.stationNegativeSpace,
      ),
    ).toEqual([
      "entry-vista",
      "waiting-vista",
      "ticket-vista",
      "garden-vista",
      "service-vista",
      "exit-vista",
    ]);
    expect(
      [...container.querySelectorAll<HTMLElement>(
        "[data-station-asset='canopy'][data-station-canopy-role]",
      )].map((canopy) => canopy.dataset.stationCanopyRole),
    ).toEqual([
      "entry-awning",
      "waiting-shelter",
      "ticket-awning",
      "departure-shelter",
    ]);
    expect(
      [...container.querySelectorAll<HTMLElement>(
        "[data-station-entrance]",
      )].map((entrance) => entrance.dataset.stationEntrance),
    ).toEqual(["side-entrance", "main-entrance", "service-entrance"]);
    expect(
      [...container.querySelectorAll<HTMLElement>(
        "[data-station-platform-contact]",
      )].every(
        (platform) =>
          platform.dataset.stationPlatformContact === "fixed-train-overlap",
      ),
    ).toBe(true);
    expect(
      stationSegments.every(
        (segment) =>
          segment.querySelectorAll("[data-station-asset='service']").length <=
            2 &&
          segment.querySelectorAll("[data-station-asset='lamp']").length <= 1,
      ),
    ).toBe(true);

    expect(trainLayoutCss).toMatch(
      /\.train-station-building--gatehouse\s*\{[^}]*right:\s*8%;[^}]*left:\s*62%;[^}]*height:\s*50px;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-station-building--station-house\s*\{[^}]*right:\s*26%;[^}]*left:\s*26%;[^}]*height:\s*60px;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-station-building--service-shed\s*\{[^}]*right:\s*44%;[^}]*left:\s*16%;[^}]*height:\s*48px;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-station-open-air-bay\s*\{[^}]*bottom:\s*20px;[^}]*height:\s*46px;[^}]*background:\s*transparent;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-station-canopy-support\s*\{[^}]*height:\s*calc\(var\(--train-station-canopy-bottom\) - 20px\);/,
    );
    expect(trainLayoutCss).not.toMatch(
      /\.train-station-architecture\[data-station-bay="(?:garden-platform|service-yard)"\]\s*\.train-station-canopy/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-consist\s*\{[^}]*z-index:\s*2;[\s\S]*?margin-bottom:\s*var\(--train-track-h\);[\s\S]*?transform:\s*translateY\(var\(--train-consist-track-overlap\)\);/,
    );
  });

  it("keeps station solids opaque while time palettes own sparse illumination", () => {
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);
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
    const buildings = [
      ...container.querySelectorAll<HTMLElement>(
        "[data-station-asset='building']",
      ),
    ];
    const stationEmissives = [
      ...container.querySelectorAll<HTMLElement>(
        ".train-station-emissive",
      ),
    ];
    const sunsetOwned = stationEmissives.filter(
      (emissive) =>
        emissive.dataset.stationLightSchedule === "sunset-night",
    );
    const nightOnly = stationEmissives.filter(
      (emissive) => emissive.dataset.stationLightSchedule === "night",
    );

    expect(buildings).toHaveLength(3);
    expect(stationEmissives).toHaveLength(8);
    expect(sunsetOwned).toHaveLength(4);
    expect(nightOnly).toHaveLength(4);
    expect(trainLayoutCss).toMatch(
      /\.train-station-building\s*\{[^}]*background-color:\s*var\(--train-station-wall\);[^}]*opacity:\s*1;[^}]*filter:\s*none;/,
    );
    const stationBuildingRule =
      trainLayoutCss.match(/\.train-station-building\s*\{([^}]*)\}/)?.[1] ??
      "";
    expect(stationBuildingRule).not.toContain("--train-palette-haze");
    expect(stationBuildingRule).not.toContain("transparent");
    expect(
      container.querySelectorAll("[data-station-solid-surface='opaque']"),
    ).toHaveLength(26);

    for (const mode of ["day", "sunset", "night"] as const) {
      world.dataset.timeOfDay = mode;
      for (const building of buildings) {
        expect(getComputedStyle(building).opacity).toBe("1");
        expect(getComputedStyle(building).filter).toBe("none");
      }
      for (const fixture of container.querySelectorAll<HTMLElement>(
        ".train-station-lamp-fixture, .train-station-signal-head, .train-station-window-row",
      )) {
        expect(getComputedStyle(fixture).filter).toBe("none");
        expect(getComputedStyle(fixture).boxShadow).toBe("none");
      }

      if (mode === "day") {
        expect(
          stationEmissives.every(
            (emissive) =>
              getComputedStyle(emissive).opacity === "0" &&
              getComputedStyle(emissive).filter === "none",
          ),
        ).toBe(true);
      } else if (mode === "sunset") {
        expect(
          sunsetOwned.every(
            (emissive) => getComputedStyle(emissive).opacity === "0.24",
          ),
        ).toBe(true);
        expect(
          nightOnly.every(
            (emissive) => getComputedStyle(emissive).opacity === "0",
          ),
        ).toBe(true);
      } else {
        expect(
          stationEmissives.every(
            (emissive) =>
              Number.parseFloat(getComputedStyle(emissive).opacity) > 0,
          ),
        ).toBe(true);
      }
    }
  });

  it("keeps station scenery pointer-inert below the train and preserves compact campus ends", () => {
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
      /\.train-set-piece--station\s*\{[^}]*clip-path:\s*none;[^}]*overflow:\s*visible;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-station-architecture\s*\{[^}]*overflow:\s*visible;[^}]*clip-path:\s*none;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-station-transition--entry\s*\{\s*clip-path:\s*polygon\(/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-station-transition--exit\s*\{\s*clip-path:\s*polygon\(/,
    );
    expect(trainLayoutCss).not.toMatch(
      /\.train-set-piece--station\.train-set-piece--(?:entry|exit)\s*\{[^}]*clip-path:/,
    );
    expect(trainLayoutCss).toMatch(
      /@media \(max-width:\s*760px\)[\s\S]*?\.train-station-building--station-house\s*\{[\s\S]*?right:\s*28%;[\s\S]*?left:\s*28%;/,
    );
    expect(trainLayoutCss).not.toMatch(
      /@media \(max-width:\s*760px\)[\s\S]*?\.train-station-canopy\s*\{[^}]*(?:right|left):\s*-1px;/,
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
    const layout = container.querySelector<HTMLElement>(".train-layout")!;
    const states = new Set<string>([world.dataset.stationState!]);
    const stationStructure = () => ({
      segments: container.querySelectorAll(
        '.train-set-piece[data-set-piece-type="station"]',
      ).length,
      buildings: container.querySelectorAll(
        "[data-station-asset='building']",
      ).length,
      canopies: container.querySelectorAll(
        "[data-station-asset='canopy']",
      ).length,
      entrances: container.querySelectorAll("[data-station-entrance]").length,
      openings: container.querySelectorAll("[data-station-negative-space]")
        .length,
      platformContacts: container.querySelectorAll(
        "[data-station-platform-contact='fixed-train-overlap']",
      ).length,
    });
    const campusByState = new Map<string, ReturnType<typeof stationStructure>>([
      [world.dataset.stationState!, stationStructure()],
    ]);
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
      campusByState.set(world.dataset.stationState!, stationStructure());
    }

    expect(world).toHaveAttribute("data-station-state", "dwell");
    expect(world).toHaveAttribute("data-station-positional-motion", "stopped");
    expect(world).toHaveAttribute("data-station-ambient", "running");
    const dwellPosition = world.dataset.routePosition;
    const dwellTrackPosition = track.dataset.trackPosition;
    const dwellWheelRotation = layout.dataset.wheelRotation;
    const dwellSkyPositions = [
      ...container.querySelectorAll<HTMLElement>("[data-day-sky-anchor]"),
    ].map((anchor) => anchor.dataset.skyPosition);
    const dwellCloudPosition =
      container.querySelector<HTMLElement>('[data-world-layer="sky"]')!
        .dataset.layerPosition;
    expect(dwellTrackPosition).toBe(dwellPosition);
    expect(dwellWheelRotation).toBe(
      `${trainWheelRotationDegrees(
        Number.parseFloat(dwellPosition!),
      ).toFixed(3)}deg`,
    );

    timestamp += 250;
    animation.run(timestamp);
    expect(world.dataset.routePosition).toBe(dwellPosition);
    expect(track.dataset.trackPosition).toBe(dwellTrackPosition);
    expect(layout.dataset.wheelRotation).toBe(dwellWheelRotation);
    expect(
      [...container.querySelectorAll<HTMLElement>("[data-day-sky-anchor]")].map(
        (anchor) => anchor.dataset.skyPosition,
      ),
    ).toEqual(dwellSkyPositions);
    expect(
      container.querySelector<HTMLElement>('[data-world-layer="sky"]')!
        .dataset.layerPosition,
    ).toBe(dwellCloudPosition);

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
      campusByState.set(world.dataset.stationState!, stationStructure());
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
    for (const state of [
      "approach",
      "decelerate",
      "platform",
      "dwell",
      "depart",
    ]) {
      expect(campusByState.get(state)).toEqual({
        segments: 6,
        buildings: 3,
        canopies: 4,
        entrances: 3,
        openings: 6,
        platformContacts: 6,
      });
    }
    expect(world.dataset.stationEventId).not.toBe(firstStation);
    expect(Number.parseFloat(world.dataset.routePosition!)).toBeGreaterThan(
      Number.parseFloat(dwellPosition!),
    );
    expect(layout.dataset.wheelRotation).not.toBe(dwellWheelRotation);
    expect(world).toHaveAttribute("data-station-target-speed", "24.000");
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
    const layout = container.querySelector<HTMLElement>(".train-layout")!;

    animation.run(1_000);
    animation.run(1_250);
    const pausedPosition = world.dataset.routePosition;
    const pausedSpeed = world.dataset.stationCurrentSpeed;
    const pausedTrackPosition = track.dataset.trackPosition;
    const pausedWheelRotation = layout.dataset.wheelRotation;

    visibility.set("hidden");
    expect(world).toHaveAttribute("data-motion-state", "suspended");
    expect(track).toHaveAttribute("data-motion-state", "suspended");
    expect(layout).toHaveAttribute("data-wheel-motion-state", "suspended");
    expect(world).toHaveAttribute("data-station-ambient", "suspended");
    expect(animation.pending()).toBe(0);

    visibility.set("visible");
    animation.run(100_000);
    expect(world.dataset.routePosition).toBe(pausedPosition);
    expect(track.dataset.trackPosition).toBe(pausedTrackPosition);
    expect(layout.dataset.wheelRotation).toBe(pausedWheelRotation);
    expect(world.dataset.stationCurrentSpeed).toBe(pausedSpeed);
    animation.run(100_250);
    expect(Number.parseFloat(world.dataset.routePosition!)).toBeGreaterThan(
      Number.parseFloat(pausedPosition!),
    );
    expect(layout.dataset.wheelRotation).not.toBe(pausedWheelRotation);
    expect(layout).toHaveAttribute("data-wheel-motion-state", "running");
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

    expect(world).toHaveAttribute("data-cruise-speed", "24");
    expect(world).toHaveAttribute("data-route-position", "4.800px");
    expect(world).toHaveAttribute("data-route-apply-count", "2");
    expect(world).toHaveAttribute("data-route-window-updates", "1");
    expect(track).toHaveAttribute("data-track-position", "4.800px");
    expect(track).toHaveAttribute("data-track-transform", "4.800px");
    expect(track.style.getPropertyValue("--train-track-transform")).toBe(
      "4.800px",
    );
    expect(container.querySelector(".train-layout-consist")).toBe(consist);
  });

  it("moves sun, wisps, routed clouds, far, and near scenery in strict depth order", () => {
    window.history.replaceState(null, "", "/?train-cruise-speed=96");
    const animation = installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const sun = container.querySelector<HTMLElement>(
      '[data-day-sky-anchor="sun"]',
    )!;
    const wisp = container.querySelector<HTMLElement>(
      '[data-day-sky-anchor="wisp"]',
    )!;
    const layerPosition = (name: string) =>
      Number.parseFloat(
        container.querySelector<HTMLElement>(
          `[data-world-layer="${name}"]`,
        )!.dataset.layerPosition!,
      );
    const initial = {
      sun: Number.parseFloat(sun.dataset.skyPosition!),
      wisp: Number.parseFloat(wisp.dataset.skyPosition!),
      cloud: layerPosition("sky"),
      far: layerPosition("far"),
      near: layerPosition("near"),
    };

    animation.run(0);
    for (let frame = 1; frame <= 40; frame++) {
      animation.run(frame * 250);
    }

    const displacement = {
      sun: Math.abs(Number.parseFloat(sun.dataset.skyPosition!) - initial.sun),
      wisp: Math.abs(
        Number.parseFloat(wisp.dataset.skyPosition!) - initial.wisp,
      ),
      cloud: Math.abs(layerPosition("sky") - initial.cloud),
      far: Math.abs(layerPosition("far") - initial.far),
      near: Math.abs(layerPosition("near") - initial.near),
    };
    expect(world).toHaveAttribute("data-route-position", "960.000px");
    expect(displacement.sun).toBeCloseTo(
      960 * TRAIN_SKY_SUN_SPEED_RATIO,
      3,
    );
    expect(displacement.wisp).toBeCloseTo(
      960 * TRAIN_SKY_WISP_SPEED_RATIO,
      3,
    );
    expect(displacement.cloud).toBeCloseTo(
      960 * TRAIN_SKY_CLOUD_SPEED_RATIO,
      3,
    );
    expect(
      [
        displacement.sun,
        displacement.wisp,
        displacement.cloud,
        displacement.far,
        displacement.near,
      ].every((distance, index, distances) =>
        index === 0 ? distance > 0 : distance > distances[index - 1]!,
      ),
    ).toBe(true);
    expect(world).toHaveAttribute("data-route-total-mounted-chunks");
    expect(container.querySelectorAll("[data-day-sky-anchor]").length).toBe(
      Number(world.dataset.daySkyCount),
    );
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

    expect(world).toHaveAttribute("data-route-position", "6.000px");
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
    expect(world).toHaveAttribute("data-route-position", "2.400px");

    visibility.set("hidden");
    expect(world).toHaveAttribute("data-motion-state", "suspended");
    expect(animation.pending()).toBe(0);

    visibility.set("visible");
    animation.run(100_000);
    expect(world).toHaveAttribute("data-route-position", "2.400px");
    animation.run(100_100);
    expect(world).toHaveAttribute("data-route-position", "4.800px");
    expect(world).toHaveAttribute("data-motion-state", "running");
  });

  it("cleans up scheduled motion and visibility ownership on unmount", () => {
    const animation = installAnimationFrame();
    mockVisibility();
    const removeListener = vi.spyOn(document, "removeEventListener");
    const { container, unmount } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );

    expect(animation.pending()).toBeGreaterThan(0);
    expect(container.querySelectorAll(".train-star").length).toBeGreaterThan(0);
    expect(container.querySelector("[data-night-sky-catalogue]")).toBeInTheDocument();
    unmount();

    expect(animation.pending()).toBe(0);
    expect(container.querySelector(".train-star")).not.toBeInTheDocument();
    expect(container.querySelector("[data-night-sky-catalogue]"))
      .not.toBeInTheDocument();
    expect(container.querySelector("[data-day-sky-anchor]"))
      .not.toBeInTheDocument();
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
    const stars = [...container.querySelectorAll(".train-star")];

    animation.run(0);
    for (let frame = 1; frame <= 120; frame++) {
      animation.run(frame * TRAIN_WORLD_MAX_FRAME_ELAPSED_MS);
    }

    const nearChunks = container.querySelectorAll(
      '[data-world-layer="near"] .train-route-chunk',
    );
    const skyChunks = container.querySelectorAll(
      '[data-world-layer="sky"] .train-parallax-chunk',
    );
    const clouds = container.querySelectorAll(
      '[data-world-layer="sky"] [data-scenery-category="cloud"]',
    );
    const allChunks = container.querySelectorAll(".train-parallax-chunk");
    expect(Number(world.dataset.routeWindowUpdates)).toBeGreaterThan(0);
    expect(nearChunks).toHaveLength(Number(world.dataset.routeMountedChunks));
    expect(allChunks.length).toBeLessThanOrEqual(50);
    expect(clouds.length).toBeLessThanOrEqual(skyChunks.length * 2);
    expect([...container.querySelectorAll(".train-star")]).toEqual(stars);
    expect(stars).toHaveLength(Number(world.dataset.starCount));
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
    const compactStarCount = Number(world.dataset.starCount);
    const compactMoon = container.querySelector<HTMLElement>("[data-moon-id]")!;
    const compactMoonPhase = compactMoon.dataset.moonPhase;
    const compactMoonLeft = compactMoon.style.left;
    const compactMoonExclusion = compactMoon.dataset.moonExclusion;
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
    expect(world).toHaveAttribute("data-star-viewport-width", "1920");
    expect(Number(world.dataset.starCount)).toBeGreaterThan(compactStarCount);
    expect(container.querySelectorAll(".train-star")).toHaveLength(
      Number(world.dataset.starCount),
    );
    const wideMoon = container.querySelector<HTMLElement>("[data-moon-id]")!;
    expect(wideMoon).toHaveAttribute("data-moon-phase", compactMoonPhase);
    expect(wideMoon.style.left).toBe(compactMoonLeft);
    expect(wideMoon.dataset.moonExclusion).not.toBe(compactMoonExclusion);
    expect(nearChunks).toHaveLength(wideCount);
    expect(rightmostEdge).toBeGreaterThanOrEqual(1_920);
  });

  it("reproduces cloud placements after a compact-wide-compact resize cycle", () => {
    installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    let width = window.innerWidth;
    Object.defineProperty(world, "clientWidth", {
      configurable: true,
      get: () => width,
    });
    const cloudSignature = () =>
      [
        ...container.querySelectorAll<HTMLElement>(
          '[data-world-layer="sky"] [data-scenery-category="cloud"]',
        ),
      ]
        .map(
          (cloud) =>
            `${cloud.dataset.sceneryAsset}:` +
            `${cloud.dataset.cloudRoutePosition}:` +
            `${cloud.dataset.cloudAltitude}`,
        )
        .sort();
    const initial = cloudSignature();

    width = 2_560;
    act(() => window.dispatchEvent(new Event("resize")));

    const wideClouds = container.querySelectorAll(
      '[data-world-layer="sky"] [data-scenery-category="cloud"]',
    );
    const wideSkyChunks = container.querySelectorAll(
      '[data-world-layer="sky"] .train-parallax-chunk',
    );
    expect(wideClouds.length).toBeLessThanOrEqual(wideSkyChunks.length * 2);
    expect(new Set(cloudSignature()).size).toBe(wideClouds.length);

    width = window.innerWidth;
    act(() => window.dispatchEvent(new Event("resize")));

    expect(cloudSignature()).toEqual(initial);
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
      String(TRAIN_SKY_CLOUD_SPEED_RATIO),
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

  it("composes monotonic depth and time grading through named CSS tokens", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 12, 0));
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const depthLayers = ["ultra-far", "far", "midground", "near"] as const;
    const contrast = depthLayers.map(
      (name) => TRAIN_SCENERY_DEPTH_PROFILES[name].contrast,
    );

    expect(contrast).toEqual([...contrast].sort((left, right) => left - right));
    expect(new Set(contrast).size).toBe(contrast.length);
    for (const name of depthLayers) {
      const layer = container.querySelector<HTMLElement>(
        `[data-world-layer="${name}"]`,
      )!;
      const profile = TRAIN_SCENERY_DEPTH_PROFILES[name];
      expect(layer).toHaveAttribute("data-depth-saturation", String(profile.saturation));
      expect(layer).toHaveAttribute("data-depth-brightness", String(profile.brightness));
      expect(layer).toHaveAttribute("data-depth-contrast", String(profile.contrast));
      expect(layer).toHaveAttribute(
        "data-depth-scale",
        String(profile.scaleMultiplier),
      );
      expect(layer).toHaveAttribute(
        "data-depth-detail-budget",
        String(profile.detailBudget),
      );
      expect(layer).toHaveAttribute(
        "data-depth-anchor-tolerance",
        String(profile.anchorToContourTolerancePx),
      );
      expect(layer).toHaveAttribute(
        "data-depth-overlap-limit",
        String(profile.maximumCollisionOverlapRatio),
      );
      expect(layer).toHaveAttribute("data-atmosphere-inside-sprite", "false");
      expect(layer.style.getPropertyValue("--train-depth-saturation")).toBe(
        String(profile.saturation),
      );
      expect(layer.style.getPropertyValue("--train-depth-brightness")).toBe(
        String(profile.brightness),
      );
      expect(layer.style.getPropertyValue("--train-depth-contrast")).toBe(
        String(profile.contrast),
      );
    }
    const depthVeils = [
      ...container.querySelectorAll<HTMLElement>("[data-depth-veil]"),
    ];
    expect(depthVeils).toHaveLength(2);
    for (const veil of depthVeils) {
      expect(veil).toHaveAttribute("data-atmosphere-owner", "depth-compositor");
      expect(veil).toHaveAttribute("data-haze-owner", "dedicated-plane");
    }

    expect(world.style.getPropertyValue("--train-time-scenery-saturation")).toBe(
      String(TRAIN_SCENERY_TIME_GRADES.day.saturation),
    );
    expect(world.style.getPropertyValue("--train-time-scenery-brightness")).toBe(
      String(TRAIN_SCENERY_TIME_GRADES.day.brightness),
    );
    expect(world.style.getPropertyValue("--train-time-scenery-warmth")).toBe(
      String(TRAIN_SCENERY_TIME_GRADES.day.warmth),
    );
    expect(trainLayoutCss).toMatch(
      /\.train-scenery-asset\s*\{[\s\S]*?saturate\(var\(--train-depth-saturation\)\)[\s\S]*?brightness\(var\(--train-depth-brightness\)\)[\s\S]*?contrast\(var\(--train-depth-contrast\)\)[\s\S]*?saturate\(var\(--train-time-scenery-saturation\)\)[\s\S]*?brightness\(var\(--train-time-scenery-brightness\)\)[\s\S]*?sepia\(var\(--train-time-scenery-warmth\)\)/,
    );
    expect(trainLayoutCss).not.toMatch(
      /\.train-layout-world\[data-time-of-day="[^"]+"\] \.train-scenery-asset/,
    );
    expect(trainLayoutCss).not.toMatch(
      /\.train-parallax-chunk--variant-\d+\s*\{[^}]*filter:/,
    );
  });

  it("keeps solid scenery effectively opaque through every time palette", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 0, 1, 12, 0));
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const initialGeometry = routeGeometryFingerprint(container);
    const timeToggle = screen.getByRole("button", {
      name: "Cycle train lighting (day / sunset / night)",
    });
    const assertOpaqueSolids = () => {
      const solidSprites = [
        ...container.querySelectorAll<HTMLElement>(
          ".train-scenery-asset:not([data-scenery-category='cloud'])",
        ),
      ];
      expect(solidSprites.length).toBeGreaterThan(0);
      for (const sprite of solidSprites) {
        const chunk = sprite.closest<HTMLElement>(".train-parallax-chunk")!;
        const spriteOpacity = Number.parseFloat(getComputedStyle(sprite).opacity);
        const chunkOpacity = Number.parseFloat(getComputedStyle(chunk).opacity);
        expect(spriteOpacity).toBe(1);
        expect(chunkOpacity).toBe(1);
        expect(spriteOpacity * chunkOpacity).toBe(1);
      }
      for (const setPiece of container.querySelectorAll<HTMLElement>(
        ".train-set-piece",
      )) {
        expect(getComputedStyle(setPiece).opacity).toBe("1");
      }
    };

    for (const mode of ["day", "sunset", "night"] as const) {
      expect(world).toHaveAttribute("data-time-of-day", mode);
      assertOpaqueSolids();
      expect(routeGeometryFingerprint(container)).toEqual(initialGeometry);
      if (mode !== "night") fireEvent.click(timeToggle);
    }
    const cloud = container.querySelector<HTMLElement>(
      "[data-scenery-category='cloud']",
    )!;
    expect(Number.parseFloat(getComputedStyle(cloud).opacity)).toBeLessThan(1);
  });

  it("keeps multi-seed terrain envelopes irregular, deterministic, and seamless across region boundaries", () => {
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);
    const seeds = [
      "train-045-terrain-alpha",
      "train-045-terrain-bravo",
      "train-045-terrain-charlie",
    ];
    const representatives = new Map<
      string,
      ReturnType<typeof generateRouteChunk>
    >();
    const solidLayers = [
      "ultra-far",
      "far",
      "midground",
      "near",
    ] as const;
    const regions = new Set<string>();
    let boundaryCount = 0;
    for (const seed of seeds) {
      for (let index = -360; index <= 360; index++) {
        const chunk = generateRouteChunk(seed, index);
        representatives.set(`${chunk.region}:${chunk.variant}`, chunk);
        regions.add(chunk.region);
        const rightNeighbour = generateRouteChunk(
          seed,
          chunk.index - 1,
          chunk.seedVersion,
        );
        const leftNeighbour = generateRouteChunk(
          seed,
          chunk.index + 1,
          chunk.seedVersion,
        );
        if (rightNeighbour.region !== chunk.region) boundaryCount++;

        for (const layerName of solidLayers) {
          const contour = trainTerrainContourForChunk(chunk, layerName);
          const envelope = TRAIN_TERRAIN_LAYER_ENVELOPES[layerName];
          expect(trainTerrainContourForChunk(chunk, layerName)).toEqual(
            contour,
          );
          expect(contour.points.map((point) => point.xPercent)).toEqual([
            0, 11, 26, 43, 61, 77, 91, 100,
          ]);
          expect(contour.material).toBe(
            TRAIN_TERRAIN_REGION_MATERIALS[chunk.region],
          );
          expect(contour.transitionMaterial).toBe(
            rightNeighbour.region === chunk.region
              ? null
              : TRAIN_TERRAIN_REGION_MATERIALS[rightNeighbour.region],
          );
          expect(contour.points[0]!.heightPx).toBe(contour.seamLeftHeightPx);
          expect(contour.points.at(-1)!.heightPx).toBe(
            contour.seamRightHeightPx,
          );
          const heights = contour.points.map((point) => point.heightPx);
          expect(Math.min(...heights), `${seed}/${index}/${layerName}`).toBeGreaterThanOrEqual(
            envelope.minimumHeightPx,
          );
          expect(Math.max(...heights), `${seed}/${index}/${layerName}`).toBeLessThanOrEqual(
            envelope.maximumHeightPx,
          );
          expect(
            Math.round(
              (Math.max(...heights) - Math.min(...heights)) * 1000,
            ) / 1000,
            `${seed}/${index}/${layerName}`,
          ).toBeGreaterThanOrEqual(envelope.minimumVariationPx);
          for (const percent of [0, 7, 19, 34, 52, 69, 85, 96, 100]) {
            const height = trainTerrainHeightAtPercent(contour, percent);
            expect(height).toBeGreaterThanOrEqual(envelope.minimumHeightPx);
            expect(height).toBeLessThanOrEqual(envelope.maximumHeightPx);
          }
          expect(contour.seamRightHeightPx).toBe(
            trainTerrainContourForChunk(rightNeighbour, layerName)
              .seamLeftHeightPx,
          );
          expect(contour.seamLeftHeightPx).toBe(
            trainTerrainContourForChunk(leftNeighbour, layerName)
              .seamRightHeightPx,
          );
        }
      }
    }
    expect(boundaryCount).toBeGreaterThan(100);
    expect(regions).toEqual(
      new Set(["forest", "mountain", "town", "coast", "industrial"]),
    );
    expect(representatives.size).toBe(25);

    const { container } = render(
      <>
        {[...representatives.values()].flatMap((chunk) =>
          solidLayers.map((layerName) => (
            <TrainRouteChunk
              chunk={chunk}
              layer={
                TRAIN_PARALLAX_LAYERS.find(
                  (layer) => layer.name === layerName,
                )!
              }
              key={`${chunk.region}:${chunk.variant}:${layerName}`}
            />
          )),
        )}
      </>,
    );
    const chunks = [
      ...container.querySelectorAll<HTMLElement>(".train-parallax-chunk"),
    ];
    const terrainBases = [
      ...container.querySelectorAll<HTMLElement>(".train-terrain-base"),
    ];
    expect(chunks).toHaveLength(representatives.size * solidLayers.length);
    expect(terrainBases).toHaveLength(chunks.length);
    for (const terrain of terrainBases) {
      expect(terrain).toHaveAttribute("data-terrain-owner", "chunk-contour");
      expect(terrain.dataset.terrainMaterial).toBe(
        TRAIN_TERRAIN_REGION_MATERIALS[
          terrain.dataset
            .terrainRegion as keyof typeof TRAIN_TERRAIN_REGION_MATERIALS
        ],
      );
      expect(terrain.dataset.terrainContour?.split(",")).toHaveLength(8);
      const envelope =
        TRAIN_TERRAIN_LAYER_ENVELOPES[
          terrain.dataset
            .terrainLayer as keyof typeof TRAIN_TERRAIN_LAYER_ENVELOPES
        ];
      expect(terrain.dataset.terrainEnvelope).toBe(
        `${envelope.minimumHeightPx}:${envelope.maximumHeightPx}`,
      );
      expect(getComputedStyle(terrain).opacity).toBe("1");
      expect(getComputedStyle(terrain).pointerEvents).toBe("none");
      expect(getComputedStyle(terrain).clipPath).toContain("polygon(");
      expect(getComputedStyle(terrain).backgroundImage).toBe("none");
      expect(getComputedStyle(terrain).backgroundColor).not.toBe(
        "transparent",
      );
    }

    const groundedSprites = [
      ...container.querySelectorAll<HTMLElement>(
        ".train-scenery-asset:not([data-scenery-category='cloud'])",
      ),
    ];
    expect(groundedSprites.length).toBeGreaterThan(0);
    for (const sprite of groundedSprites) {
      const chunkElement = sprite.closest<HTMLElement>(
        ".train-parallax-chunk",
      )!;
      const chunk = representatives.get(
        `${chunkElement.dataset.routeRegion}:${chunkElement.dataset.routeChunkVariant}`,
      )!;
      const layerName =
        chunkElement.dataset.parallaxLayer as (typeof solidLayers)[number];
      const expectedGround = trainTerrainHeightAtPercent(
        trainTerrainContourForChunk(chunk, layerName),
        Number.parseFloat(sprite.style.left),
      );
      const groundInset = Number(sprite.dataset.sceneryGroundInset);
      const expectedCanvasBottom = expectedGround - groundInset;
      expect(Number(sprite.dataset.sceneryContourHeight)).toBe(expectedGround);
      expect(
        Math.abs(
          Number(sprite.dataset.sceneryGroundHeight) - expectedCanvasBottom,
        ),
      ).toBeLessThanOrEqual(0.001_1);
      expect(
        Math.abs(
          Number.parseFloat(
            sprite.style.getPropertyValue("--train-scenery-ground-height"),
          ) - expectedCanvasBottom,
        ),
      ).toBeLessThanOrEqual(0.001_1);
      expect(
        Math.abs(Number(sprite.dataset.sceneryAnchorError)),
      ).toBeLessThanOrEqual(
        TRAIN_SCENERY_DEPTH_PROFILES[layerName].anchorToContourTolerancePx,
      );
      expect(groundInset).toBeLessThanOrEqual(
        TRAIN_SCENERY_DEPTH_PROFILES[layerName].maximumGroundInsetPx,
      );
      expect(sprite.dataset.sceneryTerrainOwner).toBe(
        `${chunk.index}:${layerName}`,
      );
    }

    for (const layerName of solidLayers) {
      const rule = trainLayoutCss.match(
        new RegExp(
          `\\.train-parallax-chunk--${layerName}\\s*\\{([^}]*)\\}`,
        ),
      )?.[1];
      expect(rule).toBeDefined();
      expect(rule).toMatch(/background:\s*none;/);
      expect(rule).not.toMatch(/gradient\(/);
    }
    const coastCompositionStart = trainLayoutCss.indexOf(
      ".train-coast-composition",
    );
    const regionalTerrainStart = trainLayoutCss.indexOf(
      '.train-parallax-chunk--midground[data-regional-scenery-role="forest-canopy-cluster"]',
    );
    const terrainCss =
      trainLayoutCss.slice(
        trainLayoutCss.indexOf(".train-terrain-base"),
        coastCompositionStart,
      ) +
      trainLayoutCss.slice(
        regionalTerrainStart,
        trainLayoutCss.indexOf(".train-depth-veil"),
      );
    expect(terrainCss).toMatch(/clip-path:\s*polygon\(/);
    expect(terrainCss).toMatch(/100%\s+100%,\s*0\s+100%/);
    expect(terrainCss).toMatch(
      /background-color:\s*var\(--train-terrain-surface\);/,
    );
    for (const material of Object.values(TRAIN_TERRAIN_REGION_MATERIALS)) {
      expect(terrainCss).toContain(
        `.train-terrain-base[data-terrain-material="${material}"]`,
      );
    }
    for (const variant of [1, 2, 3, 4]) {
      expect(terrainCss).toContain(
        `.train-parallax-chunk--variant-${variant} .train-terrain-base`,
      );
    }
    expect(terrainCss).not.toMatch(/#[0-9a-f]{3,8}|rgba?\(/i);
    expect(terrainCss).not.toMatch(/opacity:\s*0(?:\D|$)|transform:|animation:/);
  });

  it("keeps terrain materials opaque while confining sparse accents to owned pixel marks", () => {
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);

    const materials = Object.values(TRAIN_TERRAIN_REGION_MATERIALS);
    const materialTokens = new Map([
      ["forest-soil", "--train-palette-forest-soil"],
      ["mountain-rock", "--train-palette-mountain-rock"],
      ["town-ground", "--train-palette-life-town"],
      ["coast-shore", "--train-palette-mid-surface"],
      ["industrial-fill", "--train-palette-life-industrial"],
    ]);
    const commonAccentRule = trainLayoutCss.match(
      /\.train-terrain-base\[data-terrain-material\]::before\s*\{([^}]*)\}/,
    )?.[1];
    expect(commonAccentRule).toMatch(/position:\s*absolute;/);
    expect(commonAccentRule).toMatch(/content:\s*"";/);
    expect(commonAccentRule).toMatch(/pointer-events:\s*none;/);

    for (const material of materials) {
      const escapedMaterial = material.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      const baseRule = trainLayoutCss.match(
        new RegExp(
          `\\.train-terrain-base\\[data-terrain-material="${escapedMaterial}"\\]\\s*\\{([^}]*)\\}`,
        ),
      )?.[1];
      const accentRule = trainLayoutCss.match(
        new RegExp(
          `\\.train-terrain-base\\[data-terrain-material="${escapedMaterial}"\\]::before\\s*\\{([^}]*)\\}`,
        ),
      )?.[1];
      expect(baseRule, `${material}:base`).toBeDefined();
      expect(accentRule, `${material}:accent`).toBeDefined();
      expect(baseRule).toContain("background-color:");
      expect(baseRule).toContain(materialTokens.get(material));
      expect(baseRule).not.toMatch(
        /background-image|(?:repeating-)?linear-gradient|radial-gradient/,
      );
      expect(accentRule).toMatch(/left:\s*calc\(/);
      expect(accentRule).toMatch(/bottom:\s*\d+px;/);
      expect(accentRule).toMatch(/background-color:\s*currentColor;/);
      expect(accentRule).toMatch(/box-shadow:/);
      expect(accentRule).not.toMatch(/gradient|repeat|opacity|transparent/);

      const width = Number.parseInt(
        accentRule!.match(/width:\s*(\d+)px;/)?.[1] ?? "999",
        10,
      );
      const height = Number.parseInt(
        accentRule!.match(/height:\s*(\d+)px;/)?.[1] ?? "999",
        10,
      );
      expect(width, `${material}:width`).toBeLessThanOrEqual(31);
      expect(height, `${material}:height`).toBeLessThanOrEqual(4);
    }

    const representatives = new Map<
      string,
      ReturnType<typeof generateRouteChunk>
    >();
    for (let index = -500; index <= 500 && representatives.size < 5; index++) {
      const chunk = generateRouteChunk("train-054-material-contract", index);
      representatives.set(
        TRAIN_TERRAIN_REGION_MATERIALS[chunk.region],
        chunk,
      );
    }
    expect(new Set(representatives.keys())).toEqual(new Set(materials));

    const midground = TRAIN_PARALLAX_LAYERS.find(
      (layer) => layer.name === "midground",
    )!;
    const { container } = render(
      <div className="train-layout-world" data-time-of-day="day">
        {[...representatives.entries()].map(([material, chunk]) => (
          <TrainRouteChunk
            chunk={chunk}
            layer={midground}
            includeSetPieces={false}
            key={material}
          />
        ))}
      </div>,
    );
    const terrain = [
      ...container.querySelectorAll<HTMLElement>(".train-terrain-base"),
    ];
    expect(terrain).toHaveLength(materials.length);
    for (const element of terrain) {
      const material = element.dataset.terrainMaterial!;
      const chunk = representatives.get(material)!;
      const rightNeighbour = generateRouteChunk(
        chunk.routeSeed,
        chunk.index - 1,
        chunk.seedVersion,
      );
      const contour = trainTerrainContourForChunk(chunk, "midground");
      expect(element).toHaveAttribute("data-terrain-owner", "chunk-contour");
      expect(element.dataset.terrainRegion).toBe(chunk.region);
      expect(material).toBe(TRAIN_TERRAIN_REGION_MATERIALS[chunk.region]);
      expect(getComputedStyle(element).opacity).toBe("1");
      expect(getComputedStyle(element).backgroundColor).not.toBe("transparent");
      expect(contour.seamRightHeightPx).toBe(
        trainTerrainContourForChunk(rightNeighbour, "midground")
          .seamLeftHeightPx,
      );
    }

    for (const mode of ["day", "sunset", "night"] as const) {
      const order = trainPaletteLuminanceOrder(mode);
      expect(order.farSurface, `${mode}:far-mid`).toBeGreaterThan(
        order.midSurface,
      );
      expect(order.midSurface, `${mode}:mid-near`).toBeGreaterThan(
        order.nearSurface,
      );
    }
  });

  it("builds both town-edge variants as opaque road-and-yard density gradients", () => {
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);

    const representatives = new Map<
      number,
      ReturnType<typeof generateRouteChunk>[]
    >();
    for (let index = -3_600; index <= 3_600; index++) {
      const chunk = generateRouteChunk("train-039-town-edge", index);
      const setPiece = chunk.setPiece;
      if (
        setPiece?.type !== "town-edge" ||
        setPiece.role !== "entry" ||
        representatives.has(setPiece.visualVariant)
      ) {
        continue;
      }
      representatives.set(
        setPiece.visualVariant,
        Array.from({ length: setPiece.span }, (_, offset) =>
          generateRouteChunk(
            "train-039-town-edge",
            setPiece.startIndex + offset,
          ),
        ),
      );
    }
    expect(new Set(representatives.keys())).toEqual(new Set([0, 1]));

    const { container } = render(
      <>
        {(["day", "sunset", "night"] as const).map((mode) => (
          <div
            className="train-layout-world"
            data-time-of-day={mode}
            data-testid={`town-edge-${mode}`}
            key={mode}
          >
            {[...representatives.entries()].flatMap(([variant, chunks]) =>
              chunks.map((chunk) => (
                <TrainRouteChunk
                  chunk={chunk}
                  layer={
                    TRAIN_PARALLAX_LAYERS.find(
                      (layer) => layer.name === "midground",
                    )!
                  }
                  key={`${variant}:${chunk.index}`}
                />
              )),
            )}
          </div>
        ))}
      </>,
    );
    const townEdgeBaseRule = trainLayoutCss.match(
      /\.train-set-piece--town-edge\s*\{([\s\S]*?)\n\}/,
    )?.[1];
    expect(townEdgeBaseRule).toBeDefined();
    expect(townEdgeBaseRule).toMatch(/opacity:\s*1;/);
    expect(townEdgeBaseRule).toMatch(/background:\s*none;/);
    expect(trainLayoutCss).toContain(".train-town-edge-road");
    expect(trainLayoutCss).toContain(".train-town-edge-yard");
    expect(trainLayoutCss).toContain(".train-town-edge-foreground");

    for (const mode of ["day", "sunset", "night"] as const) {
      const world = screen.getByTestId(`town-edge-${mode}`);
      const segments = [
        ...world.querySelectorAll<HTMLElement>(
          ".train-set-piece--town-edge",
        ),
      ];
      expect(segments).toHaveLength(6);
      for (const variant of [0, 1]) {
        const variantSegments = segments.filter(
          (segment) => segment.dataset.setPieceVariant === String(variant),
        );
        expect(
          variantSegments.map((segment) => segment.dataset.setPieceRole),
        ).toEqual(["entry", "body", "exit"]);
        expect(
          variantSegments.map(
            (segment) => segment.dataset.townEdgeContinuity,
          ),
        ).toEqual(
          variantSegments.map(
            (segment, offset) =>
              `${segment.dataset.setPieceStart}:${offset}`,
          ),
        );
        expect(
          variantSegments.map(
            (segment) =>
              segment.querySelector<HTMLElement>(
                "[data-town-edge-transition]",
              )?.dataset.townEdgeDensity,
          ),
        ).toEqual(["open-edge", "gathering", "settled-block"]);
        expect(
          variantSegments.map(
            (segment) =>
              segment.querySelectorAll("[data-town-edge-building]").length,
          ),
        ).toEqual([1, 3, 4]);
        expect(
          variantSegments.every(
            (segment) =>
              segment.querySelectorAll("[data-town-edge-geometry='road']")
                .length === 1 &&
              segment.querySelectorAll("[data-town-edge-geometry='yard']")
                .length === 1,
          ),
        ).toBe(true);
        const buildings = variantSegments.flatMap((segment) => [
          ...segment.querySelectorAll<HTMLElement>(
            "[data-town-edge-building]",
          ),
        ]);
        expect(buildings).toHaveLength(8);
        expect(
          buildings.map((building) => building.dataset.townEdgeBuilding),
        ).toEqual(
          expect.arrayContaining([
            "building-rowhouse",
            "building-apartments",
            "building-cottage",
            "landmark-town-church",
          ]),
        );
        expect(
          variantSegments.flatMap((segment) => [
            ...segment.querySelectorAll<HTMLElement>(
              "[data-town-edge-slot]",
            ),
          ]).map((shell) => Number(shell.dataset.townEdgeSlot)),
        ).toEqual([0, 1, 2, 3, 4, 5, 6, 7]);
        expect(
          new Set(
            variantSegments.flatMap((segment) => [
              ...segment.querySelectorAll<HTMLElement>(
                "[data-town-edge-material]",
              ),
            ]).map((shell) => shell.dataset.townEdgeMaterial),
          ),
        ).toEqual(new Set(["brick", "stone", "plaster"]));
      }
      for (const segment of segments) {
        expect(getComputedStyle(segment).opacity).toBe("1");
        expect(getComputedStyle(segment).pointerEvents).toBe("none");
      }
      for (const building of world.querySelectorAll<HTMLElement>(
        "[data-town-edge-building]",
      )) {
        expect(getComputedStyle(building).opacity).toBe("1");
        expect(building.tagName).toBe("IMG");
        expect(Number(building.getAttribute("width"))).toBeGreaterThan(0);
        expect(Number(building.getAttribute("height"))).toBeGreaterThan(0);
      }
      for (const mask of world.querySelectorAll<HTMLElement>(
        "[data-emissive='town-edge-windows']",
      )) {
        expect(mask.dataset.emissiveOwner).toMatch(/^building-/);
        expect(mask.dataset.townEdgeWindowAlignment).toBe(
          `${mask.getAttribute("width")}x${mask.getAttribute("height")}`,
        );
        expect(getComputedStyle(mask).opacity).toBe(
          mode === "day" ? "0" : mode === "sunset" ? "0.28" : "0.68",
        );
      }
    }
    const sequences = [0, 1].map((variant) =>
      [
        ...screen
          .getByTestId("town-edge-day")
          .querySelectorAll<HTMLElement>(
            `[data-set-piece-variant="${variant}"] [data-town-edge-building]`,
          ),
      ].map((building) => building.dataset.townEdgeBuilding),
    );
    expect(sequences[0]).not.toEqual(sequences[1]);
  });

  it("owns atmospheric alpha in two fixed veils between solid depth layers", () => {
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const veils = [
      ...container.querySelectorAll<HTMLElement>(".train-depth-veil"),
    ];

    expect(veils).toHaveLength(2);
    expect(veils.map((veil) => veil.dataset.depthVeil)).toEqual([
      "ultra-far",
      "far",
    ]);
    expect(veils.map((veil) => veil.dataset.betweenLayers)).toEqual([
      "ultra-far,far",
      "far,midground",
    ]);
    for (const veil of veils) {
      expect(veil).toHaveAttribute("data-atmosphere-owner", "depth-compositor");
      expect(getComputedStyle(veil).pointerEvents).toBe("none");
    }
    expect(getComputedStyle(veils[0]!).zIndex).toBe("1");
    expect(getComputedStyle(veils[1]!).zIndex).toBe("2");
    const paletteLayers = [
      ...container.querySelectorAll<HTMLElement>(
        ".train-depth-veil-palette",
      ),
    ];
    expect(paletteLayers).toHaveLength(6);
    expect(
      paletteLayers.map((layer) => layer.dataset.depthVeilPalette),
    ).toEqual(["day", "sunset", "night", "day", "sunset", "night"]);
    expect(
      paletteLayers.filter((layer) => layer.classList.contains("is-active")),
    ).toHaveLength(2);
    const atmosphereRule = trainLayoutCss.match(
      /\.train-world-atmosphere\s*\{([^}]*)\}/,
    )?.[1];
    expect(atmosphereRule).toBeDefined();
    expect(atmosphereRule).not.toContain("--train-atmosphere-haze");
    expect(trainLayoutCss).toMatch(
      /\.train-depth-veil-palette\s*\{[\s\S]*?opacity:\s*0;[\s\S]*?transition:\s*opacity 450ms ease;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-depth-veil-palette\.is-active\s*\{\s*opacity:\s*1;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-depth-veil--far \.train-depth-veil-palette\s*\{[\s\S]*?color-mix\(in srgb, var\(--train-depth-veil-color\), transparent 38%\)/,
    );
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

    const clouds = container.querySelectorAll<HTMLImageElement>(
      "[data-scenery-category='cloud']",
    );
    for (const cloud of clouds) {
      expect(Number(cloud.dataset.cloudAltitude)).toBeGreaterThanOrEqual(10);
      expect(Number(cloud.dataset.cloudAltitude)).toBeLessThanOrEqual(42);
      expect(cloud.dataset.cloudPattern).toMatch(
        /^(open|grouped|scattered)$/,
      );
      expect(
        Number.isFinite(Number(cloud.dataset.cloudRoutePosition)),
      ).toBe(true);
      expect(cloud.style.top).toMatch(/%$/);
    }

    for (const chunk of chunks) {
      expect(chunk.getAttribute("data-route-region")).toMatch(
        /^(forest|mountain|town|coast|industrial)$/,
      );
      expect(chunk).toHaveAttribute("data-route-region-index");
      expect(chunk).toHaveAttribute("data-route-region-offset");
    }
  });

  it("renders forest and mountain rhythm diagnostics with terrain-owned details", () => {
    const requestedRoles = [
      "forest-stream",
      "forest-canopy-cluster",
      "forest-undergrowth",
      "forest-clearing",
      "mountain-cliff",
      "mountain-rock-field",
      "mountain-open-vista",
    ] as const;
    const chunks = requestedRoles.map((requestedRole) => {
      for (let index = -2_000; index <= 2_000; index++) {
        const chunk = generateRouteChunk("train-047-render-contract", index);
        if (
          trainForestMountainSceneryBeatForChunk(chunk)?.role === requestedRole
        ) {
          return chunk;
        }
      }
      throw new Error(`missing regional scenery role: ${requestedRole}`);
    });
    const midground = TRAIN_PARALLAX_LAYERS.find(
      (layer) => layer.name === "midground",
    )!;
    const { container } = render(
      <>
        {chunks.map((chunk) => (
          <TrainRouteChunk
            chunk={chunk}
            layer={midground}
            includeSetPieces={false}
            key={chunk.index}
          />
        ))}
      </>,
    );

    const renderedChunks = [
      ...container.querySelectorAll<HTMLElement>(
        "[data-regional-scenery-role]",
      ),
    ];
    expect(renderedChunks).toHaveLength(requestedRoles.length);
    expect(
      renderedChunks.map((chunk) => chunk.dataset.regionalSceneryRole),
    ).toEqual(requestedRoles);
    for (const chunk of renderedChunks) {
      expect(chunk.dataset.regionalSilhouette).toBeTruthy();
      expect(chunk.dataset.regionalRhythmVariant).toMatch(/^[012]$/);
      expect(chunk.dataset.regionalDensity).toMatch(
        /^(dense|medium|sparse|gap)$/,
      );
      expect(chunk.querySelectorAll(".train-terrain-base")).toHaveLength(1);
    }

    const sprites = [
      ...container.querySelectorAll<HTMLElement>(
        "[data-scenery-regional-role]",
      ),
    ];
    expect(sprites.length).toBeGreaterThan(0);
    for (const sprite of sprites) {
      expect(sprite.dataset.scenerySilhouette).toBeTruthy();
      expect(sprite.dataset.sceneryRhythmVariant).toMatch(/^[012]$/);
      expect(sprite.dataset.sceneryTransition).toMatch(
        /^(entry|interior|exit)$/,
      );
      expect(sprite.dataset.sceneryScaleFamily).toMatch(
        /^(forest|mountain)-/,
      );
    }
    expect(trainLayoutCss).toContain(
      '[data-regional-scenery-role="forest-stream"]',
    );
    expect(trainLayoutCss).toContain(
      '[data-regional-scenery-role="forest-undergrowth"]',
    );
    expect(trainLayoutCss).toContain(
      '[data-regional-scenery-role="mountain-cliff"]',
    );
    expect(trainLayoutCss).toContain(
      '[data-regional-scenery-role="mountain-rock-field"]',
    );

    const near = TRAIN_PARALLAX_LAYERS.find(
      (layer) => layer.name === "near",
    )!;
    const forestChunks = chunks.filter(
      (chunk) => chunk.region === "forest",
    );
    const nearRender = render(
      <>
        {forestChunks.map((chunk) => (
          <TrainRouteChunk
            chunk={chunk}
            layer={near}
            includeSetPieces={false}
            key={chunk.index}
          />
        ))}
      </>,
    );
    const groundDetails = [
      ...nearRender.container.querySelectorAll<HTMLElement>(
        "[data-forest-ground-role]",
      ),
    ];
    expect(groundDetails).toHaveLength(forestChunks.length);
    for (const [index, details] of groundDetails.entries()) {
      expect(details.dataset.forestGroundOwner).toBe(
        `${forestChunks[index]!.index}:near`,
      );
      expect(details.dataset.forestGroundFamily).toBeTruthy();
      expect(details.dataset.forestGroundVariant).toMatch(/^[012]$/);
      const accents = [
        ...details.querySelectorAll<HTMLElement>(
          "[data-forest-ground-detail]",
        ),
      ];
      expect(accents).toHaveLength(3);
      for (const accent of accents) {
        expect(accent.dataset.forestGroundAnchor).toBe("contour");
        expect(Number(accent.dataset.forestGroundHeight)).toBeCloseTo(
          Number.parseFloat(accent.style.bottom),
          3,
        );
        expect(accent.style.left).toMatch(/%$/);
        if (accent instanceof HTMLImageElement) {
          expect(accent.dataset.forestGroundAsset).toMatch(
            /^vegetation-/,
          );
          expect(accent.width).toBeGreaterThan(0);
          expect(accent.height).toBeGreaterThan(0);
          expect(accent).toHaveAttribute("loading", "lazy");
          expect(accent).toHaveAttribute("decoding", "async");
        }
      }
    }
    const forestGroundCss = trainLayoutCss.slice(
      trainLayoutCss.indexOf(".train-forest-ground-details"),
      trainLayoutCss.indexOf(
        '.train-parallax-chunk--midground[data-regional-scenery-role="forest-canopy-cluster"]',
      ),
    );
    expect(forestGroundCss).not.toContain("repeating-linear-gradient");
    expect(forestGroundCss).toContain(
      ".train-forest-ground-detail--sprite",
    );
    expect(forestGroundCss).toContain(".train-forest-ground-detail--meadow");
  });

  it("renders grounded town blocks and industrial fixtures with owner-attached light", () => {
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);
    const requestedRoles = [
      "town-residential-block",
      "town-commercial-main-street",
      "town-civic-square",
      "industrial-shed-district",
      "industrial-tank-yard",
      "industrial-stack-line",
      "industrial-crane-yard",
      "industrial-service-gap",
    ] as const;
    const chunks = requestedRoles.map((requestedRole) => {
      for (let index = -3_000; index <= 3_000; index++) {
        const chunk = generateRouteChunk("train-048-render-contract", index);
        if (
          trainTownIndustrialSceneryBeatForChunk(chunk)?.role ===
          requestedRole
        ) {
          return chunk;
        }
      }
      throw new Error(`missing built-environment role: ${requestedRole}`);
    });
    const midground = TRAIN_PARALLAX_LAYERS.find(
      (layer) => layer.name === "midground",
    )!;
    const renderBuiltEnvironment = (
      timeOfDay: "day" | "sunset" | "night",
    ) => (
      <div className="train-layout-world" data-time-of-day={timeOfDay}>
        {chunks.map((chunk) => (
          <TrainRouteChunk
            chunk={chunk}
            layer={midground}
            includeSetPieces={false}
            key={chunk.index}
          />
        ))}
      </div>
    );
    const rendered = render(renderBuiltEnvironment("day"));
    const renderedChunks = [
      ...rendered.container.querySelectorAll<HTMLElement>(
        "[data-built-environment-family]",
      ),
    ];

    expect(renderedChunks).toHaveLength(requestedRoles.length);
    expect(
      renderedChunks.map((chunk) => chunk.dataset.regionalSceneryRole),
    ).toEqual(requestedRoles);
    for (const chunk of renderedChunks) {
      expect(chunk.dataset.builtEnvironmentGround).toBeTruthy();
      expect(chunk.dataset.builtEnvironmentFamily).toBeTruthy();
      expect(chunk.dataset.builtEnvironmentScale).toMatch(
        /^(small|medium|tall|mixed)$/,
      );
      expect(
        chunk.querySelectorAll("[data-built-ground-surface='opaque']"),
      ).toHaveLength(1);
      const fixtures = [
        ...chunk.querySelectorAll<HTMLElement>(
          "[data-built-fixture-surface='opaque']",
        ),
      ];
      expect(fixtures.length).toBeGreaterThan(0);
      expect(fixtures.length).toBeLessThanOrEqual(2);
      for (const fixture of fixtures) {
        expect(Number(fixture.dataset.builtFixtureGroundHeight)).toBeGreaterThan(
          0,
        );
        expect(fixture.dataset.builtFixtureOwner).toContain(
          `:${fixture.closest<HTMLElement>("[data-route-chunk-index]")!.dataset.routeChunkIndex}:`,
        );
        expect(Number(fixture.dataset.builtFixtureFoundationError)).toBeCloseTo(
          0,
          3,
        );
        expect(Number(fixture.dataset.builtFixtureCollisionWidth)).toBeGreaterThan(
          0,
        );
        if (fixture.dataset.builtFixtureArt === "raster") {
          expect(fixture.dataset.builtFixturePixelDensity).toBe("native-1x");
          expect(fixture.dataset.builtFixturePerspective).toBe(
            TRAIN_WORLD_TRACK_PERSPECTIVE,
          );
          expect(
            fixture.querySelector("[data-built-raster-asset]"),
          ).toHaveAttribute("data-built-raster-pixel-density", "native-1x");
          expect(
            fixture.querySelector("[data-built-foundation-contact='contour']"),
          ).not.toBeNull();
        }
        const overlay = fixture.querySelector<HTMLElement>(
          "[data-emissive='regional-fixture']",
        );
        if (overlay) {
          expect(overlay.dataset.emissiveOwner).toBe(
            fixture.dataset.builtFixtureOwner,
          );
          expect(overlay).toHaveAttribute(
            "data-emissive-schedule",
            "sunset-night",
          );
        }
      }
      const orderedFixtures = [...fixtures].sort(
        (left, right) =>
          Number(left.dataset.builtFixtureXPercent) -
          Number(right.dataset.builtFixtureXPercent),
      );
      for (let index = 1; index < orderedFixtures.length; index++) {
        const left = orderedFixtures[index - 1]!;
        const right = orderedFixtures[index]!;
        const centerDistance =
          ((Number(right.dataset.builtFixtureXPercent) -
            Number(left.dataset.builtFixtureXPercent)) /
            100) *
          320;
        const requiredDistance =
          (Number(left.dataset.builtFixtureCollisionWidth) +
            Number(right.dataset.builtFixtureCollisionWidth)) /
            2 +
          12;
        expect(centerDistance).toBeGreaterThanOrEqual(requiredDistance);
      }
    }

    const fixtureKinds = new Set(
      [
        ...rendered.container.querySelectorAll<HTMLElement>(
          "[data-built-fixture]",
        ),
      ].map((fixture) => fixture.dataset.builtFixture),
    );
    expect(fixtureKinds).toEqual(
      new Set([
        "street-tree",
        "townhouse-block",
        "shop-awning",
        "civic-clock",
        "industrial-shed",
        "vent-stack",
        "service-pipe",
        "storage-tank",
        "furnace-stack",
        "gantry-crane",
        "utility-pole",
      ]),
    );
    const rasterFixtures = [
      ...rendered.container.querySelectorAll<HTMLElement>(
        "[data-built-fixture-art='raster']",
      ),
    ];
    expect(
      new Set(rasterFixtures.map((fixture) => fixture.dataset.builtFixture)),
    ).toEqual(
      new Set([
        "townhouse-block",
        "shop-awning",
        "industrial-shed",
        "gantry-crane",
      ]),
    );
    const rasterAssetIDs = rasterFixtures.map(
      (fixture) => fixture.dataset.builtFixtureAsset!,
    );
    expect(
      rasterAssetIDs.every((assetID) =>
        [
          "building-rowhouse",
          "building-apartments",
          "building-cottage",
          "building-workshop",
          "building-warehouse",
          "landmark-industrial-gantry",
        ].includes(assetID),
      ),
    ).toBe(true);
    expect(
      rasterAssetIDs.some(
        (assetID) =>
          assetID === "building-rowhouse" ||
          assetID === "building-apartments" ||
          assetID === "building-cottage",
      ),
    ).toBe(true);
    expect(
      rasterAssetIDs.some(
        (assetID) =>
          assetID === "building-workshop" ||
          assetID === "building-warehouse",
      ),
    ).toBe(true);
    const sceneryFoundations = [
      ...rendered.container.querySelectorAll<HTMLElement>(
        ".train-scenery-foundation",
      ),
    ];
    expect(sceneryFoundations.length).toBeGreaterThan(0);
    for (const foundation of sceneryFoundations) {
      expect(foundation.dataset.builtFoundationContact).toBe("contour");
      expect(Number(foundation.dataset.builtFoundationError)).toBeCloseTo(0, 3);
      expect(Number(foundation.dataset.builtFoundationGroundHeight)).toBeGreaterThan(
        0,
      );
    }
    const overlays = [
      ...rendered.container.querySelectorAll<HTMLElement>(
        "[data-emissive='regional-fixture']",
      ),
    ];
    expect(overlays.length).toBeGreaterThan(3);
    expect(overlays.every((overlay) => getComputedStyle(overlay).opacity === "0")).toBe(
      true,
    );
    rendered.rerender(renderBuiltEnvironment("sunset"));
    expect(
      overlays.every(
        (overlay) =>
          getComputedStyle(overlay).opacity ===
          (overlay.dataset.emissiveRegion === "town" ? "0.16" : "0.14"),
      ),
    ).toBe(true);
    rendered.rerender(renderBuiltEnvironment("night"));
    expect(
      overlays.every((overlay) => getComputedStyle(overlay).opacity === "0.88"),
    ).toBe(true);

    expect(trainLayoutCss).toContain(
      ".train-built-environment-ground--residential-street",
    );
    expect(trainLayoutCss).toContain(
      ".train-built-environment-ground--service-road",
    );
    expect(trainLayoutCss).toContain(
      ".train-built-environment-fixture--storage-tank",
    );
    expect(trainLayoutCss).toContain(
      '.train-built-environment-fixture[data-built-fixture-art="raster"]',
    );
    expect(trainLayoutCss).toContain(".train-built-environment-raster");
    expect(trainLayoutCss).not.toContain(
      ".train-built-environment-fixture--townhouse-block {",
    );
    expect(trainLayoutCss).not.toContain(
      ".train-built-environment-fixture--industrial-shed {",
    );
    expect(trainLayoutCss).not.toContain(
      ".train-built-environment-fixture--shop-awning {",
    );
    expect(trainLayoutCss).not.toContain(
      ".train-built-environment-fixture--gantry-crane {",
    );
  });

  it("renders deterministic set-piece segments with restrained occlusion", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const segments = [
      ...container.querySelectorAll<HTMLElement>("[data-set-piece-id]"),
    ].filter((element) => element.classList.contains("train-set-piece"));
    const townEdge = segments.filter(
      (segment) =>
        segment.dataset.setPieceType === "town-edge" &&
        segment.dataset.setPieceParticipation === "primary",
    );
    const supportingTownEdge = segments.filter(
      (segment) =>
        segment.dataset.setPieceType === "town-edge" &&
        segment.dataset.setPieceParticipation === "supporting",
    );

    expect(townEdge.map((segment) => segment.dataset.setPieceRole)).toEqual([
      "entry",
      "body",
      "exit",
    ]);
    expect(
      new Set(townEdge.map((segment) => segment.dataset.setPieceId)),
    ).toHaveProperty("size", 1);
    expect(
      supportingTownEdge.map((segment) => segment.dataset.setPieceRole),
    ).toEqual(["entry", "body", "exit"]);
    expect(
      new Set(
        supportingTownEdge.map((segment) => segment.dataset.setPieceLayer),
      ),
    ).toEqual(new Set(["near"]));
    for (const segment of segments) {
      expect(segment).toHaveAttribute("data-set-piece-occlusion", "restrained");
      expect(segment.dataset.setPieceRole).toMatch(/^(entry|body|exit)$/);
      expect(segment.dataset.setPieceVariant).toMatch(/^[01]$/);
      expect(segment).toHaveClass(
        `train-set-piece--variant-${segment.dataset.setPieceVariant}`,
      );
      expect(Number(segment.dataset.setPieceSpan)).toBeGreaterThanOrEqual(3);
      expect(Number(segment.dataset.setPieceStart)).toBeLessThanOrEqual(
        Number(segment.dataset.setPieceEnd),
      );
    }
  });

  it("renders two continuous deterministic compositions for every major set piece", () => {
    const majorTypes = [
      "bridge",
      "tunnel",
      "coast-reveal",
      "town-edge",
    ] as const;
    const representatives = new Map<
      string,
      ReturnType<typeof generateRouteChunk>[]
    >();

    for (let index = -3_600; index <= 3_600; index++) {
      const chunk = generateRouteChunk("layout-variant-catalogue", index);
      const setPiece = chunk.setPiece;
      if (
        !setPiece ||
        setPiece.role !== "entry" ||
        !majorTypes.includes(setPiece.type as (typeof majorTypes)[number])
      ) {
        continue;
      }
      const key = `${setPiece.type}:${setPiece.visualVariant}`;
      if (representatives.has(key)) continue;
      representatives.set(
        key,
        Array.from({ length: setPiece.span }, (_, offset) =>
          generateRouteChunk(
            "layout-variant-catalogue",
            setPiece.startIndex + offset,
          ),
        ),
      );
    }

    expect(new Set(representatives.keys())).toEqual(
      new Set(
        majorTypes.flatMap((type) => [`${type}:0`, `${type}:1`]),
      ),
    );

    const { container } = render(
      <>
        {[...representatives.entries()].flatMap(([key, chunks]) =>
          chunks.map((chunk) => (
            <TrainRouteChunk
              chunk={chunk}
              layer={
                TRAIN_PARALLAX_LAYERS.find(
                  (layer) => layer.name === chunk.setPiece!.renderLayer,
                )!
              }
              key={`${key}:${chunk.index}`}
            />
          )),
        )}
      </>,
    );

    for (const [key, chunks] of representatives) {
      const [type, variant] = key.split(":");
      const rendered = [
        ...container.querySelectorAll<HTMLElement>(
          `[data-set-piece-type="${type}"][data-set-piece-variant="${variant}"]`,
        ),
      ];
      expect(rendered).toHaveLength(chunks.length);
      expect(rendered.map((segment) => segment.dataset.setPieceRole)).toEqual([
        "entry",
        ...Array.from({ length: chunks.length - 2 }, () => "body"),
        "exit",
      ]);
      expect(
        rendered.map((segment) =>
          segment.style.getPropertyValue("--train-set-piece-phase"),
        ),
      ).toEqual(
        Array.from(
          { length: chunks.length },
          (_, offset) => `${-offset * 320}px`,
        ),
      );
      expect(
        new Set(rendered.map((segment) => segment.dataset.setPieceId)),
      ).toHaveProperty("size", 1);
    }

    for (const type of ["bridge", "tunnel", "coast-reveal"] as const) {
      expect(trainLayoutCss).toContain(
        `.train-set-piece--${type}.train-set-piece--variant-1`,
      );
    }
    expect(
      container.querySelectorAll(
        '[data-town-edge-composition="density-gradient-settlement"]',
      ),
    ).toHaveLength(6);
    const variantRules = [
      ...trainLayoutCss.matchAll(
        /\.train-set-piece--(?:bridge|tunnel|coast-reveal)\.train-set-piece--variant-1[\s\S]*?\n\}/g,
      ),
    ].map((match) => match[0]);
    expect(variantRules.length).toBeGreaterThanOrEqual(3);
    expect(variantRules.join("\n")).not.toMatch(/#[0-9a-f]{3,8}|rgba?\(/i);
  });

  it("coordinates both transition variants across their reserved layers with broad readable coverage", () => {
    const entries = new Map<
      string,
      NonNullable<ReturnType<typeof generateRouteChunk>["setPiece"]>
    >();
    for (let index = -4_800; index <= 4_800; index++) {
      const segment = generateRouteChunk(
        "transition-layout-catalogue",
        index,
      ).setPiece;
      if (
        !segment ||
        segment.role !== "entry" ||
        (segment.type !== "town-edge" && segment.type !== "coast-reveal")
      ) {
        continue;
      }
      entries.set(`${segment.type}:${segment.visualVariant}`, segment);
    }
    expect(new Set(entries.keys())).toEqual(
      new Set([
        "town-edge:0",
        "town-edge:1",
        "coast-reveal:0",
        "coast-reveal:1",
      ]),
    );

    const { container } = render(
      <>
        {[...entries.entries()].flatMap(([key, entry]) =>
          entry.reservedLayers.flatMap((layerName) => {
            const layer = TRAIN_PARALLAX_LAYERS.find(
              (candidate) => candidate.name === layerName,
            )!;
            return Array.from({ length: entry.span }, (_, offset) => (
              <TrainRouteChunk
                chunk={generateRouteChunk(
                  "transition-layout-catalogue",
                  entry.startIndex + offset,
                )}
                layer={layer}
                key={`${key}:${layerName}:${offset}`}
              />
            ));
          }),
        )}
      </>,
    );

    for (const [key, entry] of entries) {
      const [type, variant] = key.split(":");
      const segments = [
        ...container.querySelectorAll<HTMLElement>(
          `[data-set-piece-id="${entry.id}"][data-set-piece-variant="${variant}"]`,
        ),
      ];
      expect(segments, key).toHaveLength(
        entry.span * entry.reservedLayers.length,
      );
      for (const layer of entry.reservedLayers) {
        const layerSegments = segments.filter(
          (segment) => segment.dataset.setPieceLayer === layer,
        );
        expect(
          layerSegments.map((segment) => segment.dataset.setPieceRole),
          `${key}/${layer}`,
        ).toEqual([
          "entry",
          ...Array.from({ length: entry.span - 2 }, () => "body"),
          "exit",
        ]);
        expect(
          layerSegments.map(
            (segment) => segment.dataset.setPieceParticipation,
          ),
          `${key}/${layer}`,
        ).toEqual(
          Array.from(
            { length: entry.span },
            () => (layer === entry.renderLayer ? "primary" : "supporting"),
          ),
        );
      }

      if (type === "town-edge") {
        const midground = segments.filter(
          (segment) => segment.dataset.setPieceLayer === "midground",
        );
        expect(
          midground.map(
            (segment) =>
              segment.querySelector<HTMLElement>(
                "[data-town-edge-transition]",
              )?.dataset.townEdgeDensity,
          ),
          key,
        ).toEqual(["open-edge", "gathering", "settled-block"]);
        expect(
          midground.map(
            (segment) =>
              segment.querySelectorAll("[data-town-edge-building]").length,
          ),
          key,
        ).toEqual([1, 3, 4]);
        expect(
          new Set(
            midground.map(
              (segment) =>
                segment.querySelector<HTMLElement>(
                  "[data-town-edge-transition]",
                )?.dataset.townEdgeRoadGrammar,
            ),
          ),
          key,
        ).toEqual(
          new Set([variant === "0" ? "market-road" : "garden-lane"]),
        );
        expect(
          segments.filter(
            (segment) => segment.dataset.setPieceLayer === "near",
          ).every(
            (segment) =>
              segment.querySelector(
                "[data-town-edge-geometry='foreground-clearing']",
              ) !== null,
          ),
          key,
        ).toBe(true);
      } else {
        const far = segments.filter(
          (segment) => segment.dataset.setPieceLayer === "far",
        );
        const midground = segments.filter(
          (segment) => segment.dataset.setPieceLayer === "midground",
        );
        const near = segments.filter(
          (segment) => segment.dataset.setPieceLayer === "near",
        );
        expect(
          far.every(
            (segment) =>
              segment.querySelector<HTMLElement>(
                "[data-coast-reveal-composition]",
              )?.dataset.coastRevealLayerRole === "water-horizon",
          ),
          key,
        ).toBe(true);
        expect(
          midground.every(
            (segment) =>
              segment.querySelector<HTMLElement>(
                "[data-coast-reveal-composition]",
              )?.dataset.coastRevealLayerRole === "shoreline-frame",
          ),
          key,
        ).toBe(true);
        expect(
          near.every(
            (segment) =>
              segment.querySelector<HTMLElement>(
                "[data-coast-reveal-composition]",
              )?.dataset.coastRevealLayerRole === "track-foreground",
          ),
          key,
        ).toBe(true);
        expect(
          far.map(
            (segment) =>
              Number(
                segment.querySelector<HTMLElement>(
                  "[data-coast-reveal-composition]",
                )?.dataset.coastRevealWaterCoverage,
              ),
          ),
          key,
        ).toEqual(
          variant === "0" ? [58, 100, 100, 62] : [64, 100, 100, 70],
        );
        expect(
          far.every(
            (segment) =>
              segment.querySelector(
                "[data-coast-reveal-geometry='broad-water']",
              ) !== null,
          ),
          key,
        ).toBe(true);
        expect(
          midground.every(
            (segment) =>
              segment.querySelector(
                "[data-coast-reveal-geometry='shoreline-frame']",
              ) !== null,
          ),
          key,
        ).toBe(true);
        expect(
          near.every(
            (segment) =>
              segment.querySelector(
                "[data-coast-reveal-geometry='foreground-opening']",
              ) !== null,
          ),
          key,
        ).toBe(true);
        for (const segment of segments) {
          const geometry = [
            ...segment.querySelectorAll<HTMLElement>(
              "[data-coast-reveal-geometry]",
            ),
          ];
          const expectedGeometryCount =
            segment.dataset.setPieceLayer === "far" ? 2 : 1;
          expect(geometry, key).toHaveLength(expectedGeometryCount);
          expect(
            new Set(
              geometry.map(
                (candidate) => candidate.dataset.coastRevealGeometryOwner,
              ),
            ),
            key,
          ).toHaveProperty("size", 1);
          expect(
            segment.querySelector<HTMLElement>(
              "[data-coast-reveal-composition]",
            ),
            key,
          ).toHaveAttribute("data-coast-reveal-single-owner", "true");
        }
      }
    }
  });

  it("builds both bridge and tunnel variants as aligned traversal roles on participating layers", () => {
    const representatives = new Map<
      string,
      ReturnType<typeof generateRouteChunk>[]
    >();

    for (let index = -4_800; index <= 4_800; index++) {
      const chunk = generateRouteChunk("traversal-layout-catalogue", index);
      const segment = chunk.setPiece;
      if (
        !segment ||
        segment.role !== "entry" ||
        (segment.type !== "bridge" && segment.type !== "tunnel")
      ) {
        continue;
      }
      const key = `${segment.type}:${segment.visualVariant}`;
      if (representatives.has(key)) continue;
      representatives.set(
        key,
        Array.from({ length: segment.span }, (_, offset) =>
          generateRouteChunk(
            "traversal-layout-catalogue",
            segment.startIndex + offset,
          ),
        ),
      );
    }

    expect(new Set(representatives.keys())).toEqual(
      new Set(["bridge:0", "bridge:1", "tunnel:0", "tunnel:1"]),
    );

    const participatingLayers = TRAIN_PARALLAX_LAYERS.filter(
      ({ name }) => name === "midground" || name === "near",
    );
    const { container } = render(
      <>
        {[...representatives.entries()].flatMap(([key, chunks]) =>
          participatingLayers.flatMap((layer) =>
            chunks.map((chunk) => (
              <TrainRouteChunk
                chunk={chunk}
                layer={layer}
                key={`${key}:${layer.name}:${chunk.index}`}
              />
            )),
          ),
        )}
      </>,
    );

    for (const [key, chunks] of representatives) {
      const [type, variant] = key.split(":");
      const expectedRoles = [
        "entry",
        ...Array.from({ length: chunks.length - 2 }, () => "body"),
        "exit",
      ];
      const primary = [
        ...container.querySelectorAll<HTMLElement>(
          `[data-set-piece-type="${type}"][data-set-piece-variant="${variant}"]` +
            '[data-set-piece-layer="midground"]',
        ),
      ];
      const supporting = [
        ...container.querySelectorAll<HTMLElement>(
          `[data-set-piece-type="${type}"][data-set-piece-variant="${variant}"]` +
            '[data-set-piece-layer="near"]',
        ),
      ];

      expect(primary).toHaveLength(chunks.length);
      expect(supporting).toHaveLength(chunks.length);
      expect(primary.map((segment) => segment.dataset.setPieceRole)).toEqual(
        expectedRoles,
      );
      expect(supporting.map((segment) => segment.dataset.setPieceRole)).toEqual(
        expectedRoles,
      );
      expect(
        primary.map((segment) =>
          segment.style.getPropertyValue("--train-set-piece-phase"),
        ),
      ).toEqual(
        supporting.map((segment) =>
          segment.style.getPropertyValue("--train-set-piece-phase"),
        ),
      );
      expect(
        primary.every(
          (segment) => segment.dataset.setPieceParticipation === "primary",
        ),
      ).toBe(true);
      expect(
        supporting.every(
          (segment) => segment.dataset.setPieceParticipation === "supporting",
        ),
      ).toBe(true);

      for (const segment of [...primary, ...supporting]) {
        const composition = segment.querySelector<HTMLElement>(
          `[data-traversal-composition="${type}"]`,
        );
        expect(composition).not.toBeNull();
        expect(composition).toHaveAttribute(
          "data-traversal-layer",
          segment.dataset.setPieceLayer,
        );
        expect(composition).toHaveAttribute("data-traversal-track-contact", "17");
        expect(composition).toHaveAttribute(
          "data-traversal-geometry-owner",
          `${segment.dataset.setPieceId}:${segment.dataset.setPieceLayer}:${segment.dataset.setPieceSegment}`,
        );
      }

      if (type === "bridge") {
        expect(
          primary.every(
            (segment) =>
              segment.querySelectorAll(
                '[data-bridge-geometry="track-deck"]',
              ).length === 1,
          ),
        ).toBe(true);
        expect(
          primary.every(
            (segment) =>
              segment.querySelectorAll(
                '[data-bridge-geometry="supports-below-deck"]',
              ).length === 1,
          ),
        ).toBe(true);
        expect(
          primary.filter(
            (segment) => segment.dataset.setPieceRole !== "body",
          ).every((segment) =>
            Boolean(
              segment.querySelector(
                `[data-bridge-geometry="${segment.dataset.setPieceRole}-approach"]`,
              ),
            ),
          ),
        ).toBe(true);
        expect(
          primary.filter(
            (segment) => segment.dataset.setPieceRole === "body",
          ).every(
            (segment) =>
              segment.querySelector("[data-bridge-ground-owner]") === null,
          ),
        ).toBe(true);
        expect(
          primary.every((segment) => {
            const composition = segment.querySelector<HTMLElement>(
              '[data-traversal-composition="bridge"]',
            );
            const crossing = segment.querySelector<HTMLElement>(
              "[data-bridge-crossing-medium]",
            );
            const deck = segment.querySelector<HTMLElement>(
              '[data-bridge-geometry="track-deck"]',
            );
            const supports = segment.querySelector<HTMLElement>(
              '[data-bridge-geometry="supports-below-deck"]',
            );
            const structure = segment.querySelector<HTMLElement>(
              `[data-bridge-geometry="${segment.dataset.setPieceRole}-${
                variant === "0" ? "pony-truss" : "stone-parapet"
              }"]`,
            );
            return (
              composition?.dataset.bridgeLayerResponsibility ===
                "crossing-deck-structure" &&
              composition.dataset.bridgeCrossingSubject ===
                (variant === "0" ? "river" : "gorge") &&
              composition.dataset.bridgeSingleOwner === "true" &&
              composition.dataset.bridgeState ===
                segment.dataset.setPieceRole &&
              crossing?.dataset.bridgeCrossingMedium ===
                (variant === "0" ? "river" : "gorge") &&
              crossing.dataset.bridgeSolidSurface === "opaque" &&
              deck?.dataset.trackContact === "17" &&
              deck.dataset.bridgeDeckContinuity ===
                `${segment.dataset.setPieceRole}:${segment.dataset.setPieceSegment}` &&
              supports !== null &&
              structure !== null
            );
          }),
        ).toBe(true);
        expect(
          supporting.every((segment) => {
            const composition = segment.querySelector<HTMLElement>(
              '[data-traversal-composition="bridge"]',
            );
            const geometry = [
              ...segment.querySelectorAll<HTMLElement>(
                "[data-bridge-geometry]",
              ),
            ];
            return (
              composition?.dataset.bridgeLayerResponsibility ===
                "trackside-contact" &&
              composition.dataset.bridgeCrossingSubject === "none" &&
              composition.dataset.bridgeSingleOwner === "true" &&
              geometry.length === 1 &&
              geometry[0]?.dataset.bridgeGeometry ===
                `${segment.dataset.setPieceRole}-track-edge` &&
              geometry[0]?.dataset.trackContact === "17"
            );
          }),
        ).toBe(true);
        expect(
          supporting.some((segment) =>
            Boolean(
              segment.querySelector(
                ".train-bridge-crossing-void,.train-bridge-approach," +
                  ".train-bridge-span,.train-bridge-deck,.train-bridge-supports",
              ),
            ),
          ),
        ).toBe(false);
      } else {
        expect(
          primary.every((segment) =>
            Boolean(
              segment.querySelector(
                '[data-tunnel-geometry="enclosing-mountain"]',
              ),
            ),
          ),
        ).toBe(true);
        expect(
          primary.every((segment) =>
            segment.querySelectorAll("[data-tunnel-opening-owner]").length === 1,
          ),
        ).toBe(true);
        expect(
          primary.filter(
            (segment) => segment.dataset.setPieceRole !== "body",
          ).every((segment) =>
            segment
              .querySelector('[data-tunnel-portal-visible="portal"]')
              ?.getAttribute("data-tunnel-geometry") ===
            `${segment.dataset.setPieceRole}-portal-frame`,
          ),
        ).toBe(true);
        expect(
          primary
            .filter((segment) => segment.dataset.setPieceRole === "body")
            .every(
              (segment) =>
                segment
                  .querySelector('[data-tunnel-portal-visible="passage"]')
                  ?.getAttribute("data-tunnel-geometry") === "bore-lining",
            ),
        ).toBe(true);
        expect(
          primary.every((segment) => {
            const composition = segment.querySelector<HTMLElement>(
              '[data-traversal-composition="tunnel"]',
            );
            const opening = segment.querySelector<HTMLElement>(
              "[data-tunnel-opening-owner]",
            );
            const portal = segment.querySelector<HTMLElement>(
              "[data-tunnel-portal-silhouette]",
            );
            return (
              composition?.dataset.tunnelLayerResponsibility ===
                "rock-portal-bore" &&
              composition.dataset.tunnelOpeningCount === "1" &&
              composition.dataset.tunnelState ===
                segment.dataset.setPieceRole &&
              opening?.dataset.trackContact === "17" &&
              opening.dataset.tunnelBoreContinuity === "rail-passage" &&
              portal?.dataset.trackContact === "17" &&
              portal.dataset.tunnelPortalSilhouette ===
                (variant === "0" ? "round-arch" : "stepped-arch")
            );
          }),
        ).toBe(true);
        expect(
          supporting.every((segment) => {
            const composition = segment.querySelector<HTMLElement>(
              '[data-traversal-composition="tunnel"]',
            );
            const geometry = [
              ...segment.querySelectorAll<HTMLElement>(
                "[data-tunnel-geometry]",
              ),
            ];
            return (
              composition?.dataset.tunnelLayerResponsibility ===
                "trackside-contact" &&
              composition.dataset.tunnelOpeningCount === "0" &&
              geometry.length === 1 &&
              geometry[0]?.dataset.tunnelGeometry ===
                `${segment.dataset.setPieceRole}-track-edge` &&
              geometry[0]?.dataset.trackContact === "17"
            );
          }),
        ).toBe(true);
        expect(
          supporting.some((segment) =>
            Boolean(
              segment.querySelector(
                ".train-tunnel-mountain-mass,.train-tunnel-opening,.train-tunnel-portal",
              ),
            ),
          ),
        ).toBe(false);
      }
    }

    expect(trainLayoutCss).toContain(".train-bridge-crossing-void");
    expect(trainLayoutCss).toContain(".train-bridge-supports");
    expect(trainLayoutCss).toContain(".train-bridge-track-edge");
    const bridgeCss = trainLayoutCss.slice(
      trainLayoutCss.indexOf(".train-bridge-crossing-void"),
      trainLayoutCss.indexOf(".train-tunnel-mountain-mass {"),
    );
    expect(bridgeCss).not.toContain("repeating-linear-gradient");
    expect(bridgeCss).toMatch(
      /\.train-set-piece--bridge\.train-set-piece--body \.train-bridge-span\s*\{[\s\S]*?right:\s*10px;[\s\S]*?left:\s*10px;/,
    );
    expect(bridgeCss).toMatch(
      /\.train-set-piece--bridge\.train-set-piece--variant-1\s+\.train-bridge-lattice\s*\{[\s\S]*?display:\s*none;/,
    );
    expect(bridgeCss).toMatch(
      /\.train-bridge-crossing-void\s*\{[\s\S]*?bottom:\s*18px;[\s\S]*?height:\s*68px;/,
    );
    expect(bridgeCss).toMatch(
      /\.train-set-piece--bridge\.train-set-piece--variant-1\.train-set-piece--entry[\s\S]*?\.train-bridge-crossing-void[\s\S]*?i:first-child,[\s\S]*?i:last-child\s*\{[\s\S]*?bottom:\s*0;[\s\S]*?width:\s*42%;/,
    );
    expect(bridgeCss).toMatch(
      /\.train-set-piece--bridge\.train-set-piece--variant-1[\s\S]*?\.train-bridge-supports[\s\S]*?i\s*\{[\s\S]*?height:\s*46px;/,
    );
    expect(trainLayoutCss).toContain(".train-tunnel-mountain-mass");
    expect(trainLayoutCss).toContain(".train-tunnel-opening");
    expect(trainLayoutCss).toMatch(
      /\.train-tunnel-opening\s*\{[\s\S]*?#02070d 76%/,
    );
    const bodyOpeningRule = trainLayoutCss.match(
      /\.train-set-piece--tunnel\.train-set-piece--body \.train-tunnel-opening\s*\{([^}]+)\}/,
    )?.[1];
    const steppedBodyOpeningRule = trainLayoutCss.match(
      /\.train-set-piece--tunnel\.train-set-piece--variant-1\.train-set-piece--body\s+\.train-tunnel-opening\s*\{([^}]+)\}/,
    )?.[1];
    const entryOpeningRule = trainLayoutCss.match(
      /\.train-tunnel-opening\s*\{([^}]+)\}/,
    )?.[1];
    const exitOpeningRule = trainLayoutCss.match(
      /\.train-set-piece--tunnel\.train-set-piece--exit \.train-tunnel-opening\s*\{([^}]+)\}/,
    )?.[1];
    const steppedEntryOpeningRule = trainLayoutCss.match(
      /\.train-set-piece--tunnel\.train-set-piece--variant-1\s+\.train-tunnel-opening\s*\{([^}]+)\}/,
    )?.[1];
    const steppedExitOpeningRule = trainLayoutCss.match(
      /\.train-set-piece--tunnel\.train-set-piece--variant-1\.train-set-piece--exit\s+\.train-tunnel-opening\s*\{([^}]+)\}/,
    )?.[1];
    expect(entryOpeningRule).toBeDefined();
    expect(entryOpeningRule).toMatch(/right:\s*0;[\s\S]*?left:\s*24%;/);
    expect(exitOpeningRule).toBeDefined();
    expect(exitOpeningRule).toMatch(/right:\s*24%;[\s\S]*?left:\s*0;/);
    expect(steppedEntryOpeningRule).toBeDefined();
    expect(steppedEntryOpeningRule).toMatch(
      /right:\s*0;[\s\S]*?left:\s*28%;/,
    );
    expect(steppedExitOpeningRule).toBeDefined();
    expect(steppedExitOpeningRule).toMatch(
      /right:\s*28%;[\s\S]*?left:\s*0;/,
    );
    expect(bodyOpeningRule).toBeDefined();
    expect(bodyOpeningRule).toMatch(/right:\s*3%;[\s\S]*left:\s*3%;/);
    expect(bodyOpeningRule).toMatch(/border-radius:\s*30% 30% 0 0;/);
    expect(steppedBodyOpeningRule).toBeDefined();
    expect(steppedBodyOpeningRule).toMatch(/right:\s*4%;[\s\S]*left:\s*4%;/);
    expect(steppedBodyOpeningRule).toMatch(/border-radius:\s*14% 14% 0 0;/);
    expect(bodyOpeningRule).not.toMatch(/(?:right|left):\s*-1px/);
    expect(steppedBodyOpeningRule).not.toMatch(/(?:right|left):\s*-1px/);
    expect(bodyOpeningRule).not.toMatch(/border-radius:\s*0/);
    expect(steppedBodyOpeningRule).not.toMatch(/border-radius:\s*0/);
    expect(trainLayoutCss).not.toMatch(
      /\.train-set-piece--tunnel[\s\S]*?\.train-tunnel-opening\s*\{[^}]*clip-path:\s*none/,
    );
    expect(trainLayoutCss).not.toContain(".train-set-piece--bridge::after");
    expect(trainLayoutCss).not.toContain(".train-set-piece--tunnel::after");
  });

  it("focuses both bridge variants with one crossing owner and continuous rail contact", () => {
    vi.spyOn(window, "innerWidth", "get").mockReturnValue(1_280);
    installAnimationFrame();
    mockVisibility();
    const seed = "train-053-aurora";
    const proofs = new Map<
      number,
      { occurrence: number; search: string }
    >();

    for (let occurrence = 0; occurrence < 16; occurrence++) {
      const search =
        `?train-route-seed=${seed}&train-set-piece-focus=bridge` +
        `&train-set-piece-occurrence=${occurrence}&train-reduced-motion=1`;
      const focus = trainWorldSetPieceFocus(search, seed, 1_280);
      if (focus) {
        proofs.set(focus.visualVariant, { occurrence, search });
      }
      if (proofs.size === 2) break;
    }

    expect(new Set(proofs.keys())).toEqual(new Set([0, 1]));

    for (const [variant, proof] of proofs) {
      window.history.replaceState(null, "", `/${proof.search}`);
      const rendered = render(
        <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
      );
      const world =
        rendered.container.querySelector<HTMLElement>(".train-layout-world")!;
      const focus = trainWorldSetPieceFocus(
        window.location.search,
        seed,
        1_280,
      )!;
      const primary = [
        ...rendered.container.querySelectorAll<HTMLElement>(
          `[data-set-piece-id="${focus.id}"][data-set-piece-layer="midground"]`,
        ),
      ];
      const supporting = [
        ...rendered.container.querySelectorAll<HTMLElement>(
          `[data-set-piece-id="${focus.id}"][data-set-piece-layer="near"]`,
        ),
      ];

      expect(world).toHaveAttribute("data-set-piece-focus-type", "bridge");
      expect(world.dataset.setPieceCollisionExcludedIds).not.toContain(
        focus.id,
      );
      expect(world).toHaveAttribute("data-set-piece-collision-exclusions", "0");
      expect(primary.map((segment) => segment.dataset.setPieceRole)).toEqual([
        "entry",
        "body",
        "body",
        "exit",
      ]);
      expect(supporting.map((segment) => segment.dataset.setPieceRole)).toEqual([
        "entry",
        "body",
        "body",
        "exit",
      ]);
      expect(
        primary.map(
          (segment) =>
            segment.querySelector<HTMLElement>(
              '[data-bridge-geometry="track-deck"]',
            )?.dataset.bridgeDeckContinuity,
        ),
      ).toEqual(["entry:0", "body:1", "body:2", "exit:3"]);
      expect(
        primary.every(
          (segment) =>
            segment.querySelectorAll("[data-bridge-crossing-medium]").length ===
              1 &&
            segment.querySelectorAll(
              '[data-bridge-geometry="supports-below-deck"]',
            ).length === 1,
        ),
      ).toBe(true);
      expect(
        supporting.every(
          (segment) =>
            segment.querySelectorAll("[data-bridge-geometry]").length === 1 &&
            segment.querySelector<HTMLElement>("[data-bridge-geometry]")
              ?.dataset.trackContact === "17",
        ),
      ).toBe(true);
      expect(
        primary[1]?.querySelector("[data-bridge-crossing-medium]"),
      ).toHaveAttribute(
        "data-bridge-crossing-medium",
        variant === 0 ? "river" : "gorge",
      );
      if (variant === 1) {
        const framing = [
          ...rendered.container.querySelectorAll<HTMLElement>(
            `[data-set-piece-id="${focus.id}"] [data-bridge-forest-framing]`,
          ),
        ];
        expect(framing.map((owner) => owner.dataset.bridgeForestFraming)).toEqual(
          ["entry", "body", "body", "exit"],
        );
        expect(
          framing.every(
            (owner) =>
              (owner.dataset.bridgeForestGround === "approach" ||
                owner.dataset.bridgeForestGround === "gorge-rim") &&
              owner.querySelectorAll("[data-bridge-forest-detail]").length === 2,
          ),
        ).toBe(true);
        expect(
          primary
            .filter((segment) => segment.dataset.setPieceRole === "body")
            .every(
              (segment) =>
                segment
                  .querySelector("[data-bridge-forest-framing]")
                  ?.getAttribute("data-bridge-forest-ground") === "gorge-rim",
            ),
        ).toBe(true);
      }
      rendered.unmount();
    }
  });

  it("preserves tunnel portal, bore, and rail-contact geometry with reduced motion", () => {
    window.history.replaceState(
      null,
      "",
      "/?train-route-seed=train-053-cascade&train-set-piece-focus=tunnel" +
        "&train-set-piece-occurrence=0&train-reduced-motion=1",
    );
    mockReducedMotion();

    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const primary = [
      ...container.querySelectorAll<HTMLElement>(
        '[data-set-piece-type="tunnel"][data-set-piece-layer="midground"]',
      ),
    ];
    const supporting = [
      ...container.querySelectorAll<HTMLElement>(
        '[data-set-piece-type="tunnel"][data-set-piece-layer="near"]',
      ),
    ];
    const focus = trainWorldSetPieceFocus(
      window.location.search,
      "train-053-cascade",
      1_280,
    )!;

    expect(world).toHaveAttribute("data-motion", "reduced");
    expect(primary.map((segment) => segment.dataset.setPieceRole)).toEqual([
      "entry",
      "body",
      "exit",
    ]);
    expect(supporting.map((segment) => segment.dataset.setPieceRole)).toEqual([
      "entry",
      "body",
      "exit",
    ]);
    expect(trainTraversalActiveRole(focus, focus.journeyPosition - 600)).toBe(
      "entry",
    );
    expect(trainTraversalActiveRole(focus, focus.journeyPosition)).toBe("body");
    expect(trainTraversalActiveRole(focus, focus.journeyPosition + 600)).toBe(
      "exit",
    );
    expect(
      primary.map(
        (segment) =>
          segment.querySelector<HTMLElement>(
            '[data-traversal-composition="tunnel"]',
          )?.dataset.traversalActive,
      ),
    ).toEqual(["false", "true", "false"]);
    expect(
      primary.every(
        (segment) =>
          segment.querySelectorAll("[data-tunnel-opening-owner]").length === 1 &&
          segment.querySelectorAll("[data-tunnel-portal-silhouette]").length ===
            1,
      ),
    ).toBe(true);
    expect(
      supporting.every(
        (segment) =>
          segment.querySelectorAll("[data-tunnel-opening-owner]").length === 0 &&
          segment.querySelectorAll('[data-track-contact="17"]').length === 1,
      ),
    ).toBe(true);
    expect(trainLayoutCss).toMatch(
      /\.train-traversal-composition--tunnel\[data-traversal-active="false"\][\s\S]*?\.train-tunnel-opening,[\s\S]*?\.train-tunnel-portal\s*\{[\s\S]*?display:\s*none;/,
    );
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
    const layout = container.querySelector<HTMLElement>(".train-layout")!;
    const initialWheelRotation = layout.dataset.wheelRotation;

    act(() => vi.advanceTimersByTime(TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS - 1));
    expect(world).toHaveAttribute("data-route-position", "0.000px");
    expect(track).toHaveAttribute("data-track-position", "0.000px");
    act(() => vi.advanceTimersByTime(1));
    expect(world).toHaveAttribute("data-route-position", "2.400px");
    expect(track).toHaveAttribute("data-track-position", "2.400px");
    expect(track).toHaveAttribute("data-track-transform", "2.400px");
    expect(layout.dataset.wheelRotation).not.toBe(initialWheelRotation);
    expect(layout.dataset.wheelRotation).toBe(
      `${trainWheelRotationDegrees(2.4).toFixed(3)}deg`,
    );
    expect(
      container.querySelector<HTMLElement>('[data-world-layer="near"]')!
        .dataset.layerPosition,
    ).toBe("2.400px");
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

  it("gives ordinary coast one midground owner and grounds every land-owned fixture on opaque shore", () => {
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);
    const far = TRAIN_PARALLAX_LAYERS.find(
      (candidate) => candidate.name === "far",
    )!;
    const midground = TRAIN_PARALLAX_LAYERS.find(
      (candidate) => candidate.name === "midground",
    )!;
    const coastRegionIndex = Array.from(
      { length: 401 },
      (_, offset) => offset - 200,
    ).find(
      (regionIndex) =>
        generateRouteChunk(
          "coast-water-render",
          regionIndex * TRAIN_REGION_CHUNK_LENGTH,
        ).region === "coast",
    )!;
    const chunks = Array.from(
      { length: TRAIN_REGION_CHUNK_LENGTH },
      (_, offset) =>
      generateRouteChunk(
        "coast-water-render",
        coastRegionIndex * TRAIN_REGION_CHUNK_LENGTH + offset,
      ),
    );
    const rendered = render(
      <div className="train-layout-world" data-time-of-day="day">
        {chunks.map((chunk) => (
          <div key={chunk.index}>
            <TrainRouteChunk
              chunk={chunk}
              layer={far}
              includeSetPieces={false}
            />
            <TrainRouteChunk
              chunk={chunk}
              layer={midground}
              includeSetPieces={false}
            />
          </div>
        ))}
      </div>,
    );

    const farWater = [
      ...rendered.container.querySelectorAll<HTMLElement>(
        ".train-coast-water-plane--far",
      ),
    ];
    const midWater = [
      ...rendered.container.querySelectorAll<HTMLElement>(
        ".train-coast-water-plane--midground",
      ),
    ];
    const shores = [
      ...rendered.container.querySelectorAll<HTMLElement>(
        ".train-coast-shore-profile",
      ),
    ];
    expect(farWater).toHaveLength(0);
    expect(midWater).toHaveLength(TRAIN_REGION_CHUNK_LENGTH);
    expect(shores).toHaveLength(TRAIN_REGION_CHUNK_LENGTH);
    expect(trainLayoutCss).not.toContain(".train-coast-water-plane--far");
    expect(
      new Set(midWater.map((plane) => plane.dataset.waterOwner)).size,
    ).toBe(TRAIN_REGION_CHUNK_LENGTH);
    for (const plane of midWater) {
      expect(plane).toHaveAttribute("data-water-plane", "continuous");
      expect(plane).toHaveAttribute("data-water-surface", "owned");
      expect(plane).toHaveAttribute("data-coast-contact-medium", "water");
      expect(plane.dataset.waterSeamLeft).toMatch(/^\d+(?:\.\d+)?$/);
      expect(plane.dataset.waterSeamRight).toMatch(/^\d+(?:\.\d+)?$/);
      const cues = plane.querySelectorAll<HTMLElement>(
        "[data-water-movement-cue]",
      );
      expect(cues).toHaveLength(3);
      cues.forEach((cue) =>
        expect(cue.dataset.waterDepthOwner).toBe(plane.dataset.waterOwner),
      );
    }
    for (const shore of shores) {
      expect(shore.dataset.shoreContinuity).toMatch(
        /^\d+(?:\.\d+)?:\d+(?:\.\d+)?$/,
      );
      expect(shore).toHaveAttribute("data-shore-surface", "opaque");
      expect(shore).toHaveAttribute(
        "data-coast-contact-medium",
        "dry-land",
      );
    }
    expect(
      rendered.container.querySelectorAll(
        '[data-coast-layer-role="water-shore-fixtures"]',
      ),
    ).toHaveLength(TRAIN_REGION_CHUNK_LENGTH);
    rendered.container
      .querySelectorAll<HTMLElement>("[data-coast-composition]")
      .forEach((composition) =>
        expect(composition).toHaveAttribute(
          "data-coast-single-owner",
          "midground",
        ),
      );
    const fixtures = rendered.container.querySelectorAll<HTMLElement>(
      "[data-coast-fixture]",
    );
    expect(fixtures.length).toBeGreaterThan(8);
    fixtures.forEach((fixture) =>
      expect(fixture).toHaveAttribute(
        "data-coast-fixture-surface",
        "opaque",
      ),
    );
    fixtures.forEach((fixture) => {
      const waterOwned = ["boat", "buoy"].includes(
        fixture.dataset.coastFixture!,
      );
      expect(fixture).toHaveAttribute(
        "data-coast-fixture-medium",
        waterOwned ? "water" : "dry-land",
      );
      if (waterOwned) {
        expect(fixture.dataset.waterOwner).toMatch(/:midground:water$/);
        expect(fixture.dataset.coastFixtureWaterlineHeight).toMatch(/^\d+$/);
      } else {
        expect(fixture).not.toHaveAttribute("data-water-owner");
        expect(Number(fixture.dataset.coastFixtureGroundHeight)).toBeGreaterThan(
          0,
        );
      }
    });
    expect(
      new Set(
        chunks.flatMap(
          (chunk) => trainCoastSceneryBeatForChunk(chunk)?.fixtures ?? [],
        ),
      ),
    ).toEqual(
      new Set([
        "beach",
        "rock-shelf",
        "pier",
        "boat",
        "buoy",
        "harbour-post",
        "dune-grass",
      ]),
    );

    rendered.unmount();
    const lighthouseChunk = Array.from({ length: 6001 }, (_, offset) =>
      generateRouteChunk("coast-reflection-render", offset - 3000),
    ).find(
      (chunk) =>
        chunk.region === "coast" &&
        trainSceneryPlacementsForChunk("midground", chunk, {
          includeSetPieces: false,
        }).some(
          (placement, ordinal) =>
            placement.asset.id === "landmark-coast-lighthouse" &&
            trainNightLifeForPlacement(chunk, placement, ordinal) !== null,
        ),
    )!;
    expect(lighthouseChunk).toBeDefined();
    const lighthouse = render(
      <div className="train-layout-world" data-time-of-day="night">
        <TrainRouteChunk
          chunk={lighthouseChunk}
          layer={midground}
          includeSetPieces={false}
        />
      </div>,
    );
    const water = lighthouse.container.querySelector<HTMLElement>(
      ".train-coast-water-plane--midground",
    )!;
    const lighthouseAsset = lighthouse.container.querySelector<HTMLElement>(
      '[data-scenery-asset="landmark-coast-lighthouse"]',
    )!;
    const lighthouseFoundation =
      lighthouse.container.querySelector<HTMLElement>(
        `[data-built-foundation-owner="${lighthouseAsset.dataset.sceneryInstanceId}"]`,
      )!;
    expect(lighthouseAsset).toHaveAttribute(
      "data-coast-contact-medium",
      "dry-land",
    );
    expect(lighthouseFoundation).toHaveAttribute(
      "data-coast-foundation",
      "opaque-shelf",
    );
    expect(lighthouseFoundation).toHaveAttribute(
      "data-coast-contact-medium",
      "dry-land",
    );
    const clip = lighthouse.container.querySelector<HTMLElement>(
      ".train-coast-reflection-clip",
    )!;
    const reflection = clip.querySelector<HTMLElement>(
      '[data-emissive="lighthouse-water-reflection"]',
    )!;
    const owner = reflection.closest<HTMLElement>(".train-night-life")!;
    expect(clip).toHaveAttribute("data-reflection-clip", "water-only");
    expect(clip.dataset.reflectionClipOwner).toBe(water.dataset.waterOwner);
    expect(clip.dataset.waterOwner).toBe(water.dataset.waterOwner);
    expect(reflection.dataset.reflectionSourceOwner).toBe(
      owner.dataset.emissiveOwnerInstance,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-coast-reflection-clip\s*\{[\s\S]*?overflow:\s*hidden;[\s\S]*?clip-path:/,
    );
  });

  it("aligns bounded industrial masks, isolates palette state, and fails unlit", () => {
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);
    const layer = TRAIN_PARALLAX_LAYERS.find(
      (candidate) => candidate.name === "midground",
    )!;
    const chunk = Array.from({ length: 2401 }, (_, offset) =>
      generateRouteChunk("town-mask-render", offset - 1200),
    ).find(
      (candidate) =>
        candidate.region === "industrial" &&
        trainSceneryPlacementsForChunk("midground", candidate).some(
          (placement, ordinal) =>
            [
              "building-workshop",
              "building-warehouse",
              "building-water-tower",
            ].includes(placement.asset.id) &&
            trainNightLifeForPlacement(candidate, placement, ordinal) !== null,
        ),
    )!;
    expect(chunk).toBeDefined();
    const routeChunk = (timeOfDay: "day" | "sunset" | "night") => (
      <div className="train-layout-world" data-time-of-day={timeOfDay}>
        <TrainRouteChunk chunk={chunk} layer={layer} />
      </div>
    );
    const rendered = render(routeChunk("day"));
    const masks = [
      ...rendered.container.querySelectorAll<HTMLImageElement>(
        ".train-scenery-emissive-mask",
      ),
    ];

    expect(masks.length).toBeGreaterThan(0);
    const { container } = rendered;
    const bases = container.querySelectorAll<HTMLImageElement>(
      "[data-scenery-asset^='building-']",
    );
    expect(masks.length).toBe(bases.length);

    for (const mask of masks) {
      const ownerInstance = mask.dataset.emissiveOwnerInstance!;
      const chunk = mask.closest<HTMLElement>(".train-parallax-chunk")!;
      const base = chunk.querySelector<HTMLImageElement>(
        `[data-scenery-instance-id="${ownerInstance}"]`,
      )!;
      expect(base).not.toBeNull();
      expect(mask).toHaveAttribute("data-emissive-kind", "windows");
      expect(mask).toHaveAttribute("data-emissive-plane", "owner-attached");
      expect(mask).toHaveAttribute("data-emissive-load", "pending");
      expect(mask.dataset.emissiveOwner).toBe(base.dataset.sceneryAsset);
      expect(mask.dataset.emissiveGroundHeight).toBe(
        base.dataset.sceneryGroundHeight,
      );
      expect(mask.dataset.emissiveContourHeight).toBe(
        base.dataset.sceneryContourHeight,
      );
      expect(mask.dataset.emissiveGroundInset).toBe(
        base.dataset.sceneryGroundInset,
      );
      expect(mask.dataset.emissiveScale).toBe(
        Number.parseFloat(
          base.style.getPropertyValue("--train-scenery-scale"),
        ).toFixed(3),
      );
      expect(mask.dataset.sceneryAnchor).toBe(base.dataset.sceneryAnchor);
      expect(mask.dataset.sceneryManifestLayer).toBe(
        base.dataset.sceneryManifestLayer,
      );
      expect(mask.width).toBe(base.width);
      expect(mask.height).toBe(base.height);
      expect(mask.style.left).toBe(base.style.left);
      expect(mask.style.top).toBe(base.style.top);
      expect(mask.style.getPropertyValue("--train-scenery-scale")).toBe(
        base.style.getPropertyValue("--train-scenery-scale"),
      );
      expect(
        chunk.querySelectorAll(".train-emissive-overlay--windows"),
      ).toHaveLength(0);
    }

    const mask = masks.find(
      (candidate) => candidate.dataset.emissiveEnabled === "true",
    )!;
    expect(mask).toBeDefined();
    const ownerBase = mask
      .closest<HTMLElement>(".train-parallax-chunk")!
      .querySelector<HTMLImageElement>(
        `[data-scenery-instance-id="${mask.dataset.emissiveOwnerInstance}"]`,
      )!;
    expect(getComputedStyle(mask).opacity).toBe("0");
    rendered.rerender(routeChunk("sunset"));
    expect(getComputedStyle(mask).opacity).toBe("0.14");
    rendered.rerender(routeChunk("night"));
    expect(getComputedStyle(mask).opacity).toBe("0.88");

    fireEvent.load(mask);
    expect(mask).toHaveAttribute("data-emissive-load", "loaded");
    fireEvent.error(mask);
    expect(mask).toHaveAttribute("data-emissive-load", "failed");
    expect(mask.hidden).toBe(true);
    expect(ownerBase.hidden).toBe(false);
  });

  it("keeps every emissive treatment separate from its owning solid art", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const emissiveKinds = new Set(
      [...container.querySelectorAll<HTMLElement>("[data-emissive]")].map(
        (overlay) => overlay.dataset.emissive,
      ),
    );

    const requiredKinds = ["stars", "moon"];
    const allowedKinds = [
      ...requiredKinds,
      "airglow",
      "celestial-accent",
      "building-windows",
      "town-edge-windows",
      "regional-fixture",
      ...Object.values(TRAIN_REGION_NIGHT_LIFE).map((rule) => rule.kind),
      "lighthouse-water-reflection",
    ];
    for (const kind of requiredKinds) {
      expect(emissiveKinds).toContain(kind);
    }
    expect(
      [...emissiveKinds].every(
        (kind) => allowedKinds.includes(kind!),
      ),
    ).toBe(true);
    const ownerlessSkyKinds = new Set([
      "stars",
      "moon",
      "airglow",
      "celestial-accent",
    ]);
    for (const overlay of container.querySelectorAll<HTMLElement>(
      "[data-emissive]",
    )) {
      if (overlay.tagName === "IMG") {
        if (overlay.dataset.emissive === "town-edge-windows") {
          expect(overlay).toHaveClass("train-town-edge-building-emissive");
        } else if (overlay.dataset.emissive === "regional-fixture") {
          expect(overlay).toHaveClass(
            "train-built-environment-raster-emissive",
          );
        } else {
          expect(overlay).toHaveClass("train-scenery-emissive-mask");
          expect(overlay).toHaveAttribute("data-emissive", "building-windows");
        }
        expect(overlay).toHaveAttribute("data-emissive-owner");
      } else if (!ownerlessSkyKinds.has(overlay.dataset.emissive!)) {
        expect(overlay).toHaveAttribute("data-emissive-owner");
      }
    }
    for (const overlay of container.querySelectorAll<HTMLElement>(
      '[data-emissive-plane="owner-attached"]',
    )) {
      const ownerInstance = overlay.dataset.emissiveOwnerInstance;
      expect(ownerInstance).toBeTruthy();
      expect(
        overlay
          .closest<HTMLElement>(".train-parallax-chunk")
          ?.querySelector(`[data-scenery-instance-id="${ownerInstance}"]`),
      ).not.toBeNull();
      expect(overlay.dataset.emissiveGroundHeight).toBeDefined();
      expect(overlay.dataset.emissiveContourHeight).toBeDefined();
      expect(overlay.dataset.emissiveGroundInset).toBeDefined();
      expect(overlay.dataset.emissiveScale).toBeDefined();
    }
    expect(
      container.querySelectorAll(".train-emissive-overlay--windows"),
    ).toHaveLength(0);
    expect(
      container.querySelectorAll(".train-scenery-emissive-mask").length,
    ).toBeLessThanOrEqual(
      container.querySelectorAll("[data-scenery-category='building']").length,
    );
    expect(trainLayoutCss).not.toMatch(
      /train-emissive-overlay--(?:streetlight|station-lamp|signal|water-reflection)/,
    );
  });

  it("aligns one sparse owned nighttime signature for every region", () => {
    const styles = document.createElement("style");
    styles.dataset.trainLayoutTestStyles = "true";
    styles.textContent = trainLayoutCss;
    document.head.append(styles);
    const layer = TRAIN_PARALLAX_LAYERS.find(
      (candidate) => candidate.name === "midground",
    )!;
    const samples = Object.entries(TRAIN_REGION_NIGHT_LIFE).map(
      ([region, rule]) => {
        const landmarkOwner = rule.owners[0]!.assetId;
        const chunk = Array.from({ length: 3600 }, (_, index) =>
          generateRouteChunk(`night-life-${region}`, index - 1800),
        ).find(
          (candidate) =>
            candidate.region === region &&
            trainSceneryPlacementsForChunk("midground", candidate).some(
              (placement, ordinal) =>
                placement.asset.id === landmarkOwner &&
                trainNightLifeForPlacement(
                  candidate,
                  placement,
                  ordinal,
                ) !== null,
            ),
        );
        expect(chunk, `${region}/${landmarkOwner}`).toBeDefined();
        return chunk!;
      },
    );

    const view = (timeOfDay: "day" | "night") => (
      <div
        className="train-layout-world"
        data-time-of-day={timeOfDay}
        data-motion-state="running"
      >
        {samples.map((chunk) => (
          <TrainRouteChunk
            chunk={chunk}
            layer={layer}
            key={`${chunk.region}:${chunk.index}`}
          />
        ))}
      </div>
    );
    const rendered = render(view("day"));

    for (const chunk of samples) {
      const signature = rendered.container.querySelector<HTMLElement>(
        `[data-night-life-region="${chunk.region}"]`,
      )!;
      expect(signature, chunk.region).not.toBeNull();
      expect(signature).toHaveAttribute(
        "data-night-life-kind",
        TRAIN_REGION_NIGHT_LIFE[chunk.region].kind,
      );
      expect(signature).toHaveAttribute(
        "data-night-life-plane",
        "midground-behind-train",
      );
      const owner = signature
        .closest<HTMLElement>(".train-parallax-chunk")!
        .querySelector<HTMLImageElement>(
          `[data-scenery-asset="${signature.dataset.emissiveOwner}"]`,
        )!;
      expect(owner).not.toBeNull();
      expect(signature.style.left).toBe(owner.style.left);
      expect(
        signature.style.getPropertyValue("--train-scenery-scale"),
      ).toBe(owner.style.getPropertyValue("--train-scenery-scale"));
      expect(getComputedStyle(signature).opacity).toBe("0");
    }

    const lighthouse = rendered.container.querySelector<HTMLElement>(
      '[data-night-life-kind="coast-lighthouse-beacon"]',
    )!;
    const reflection = lighthouse.querySelector<HTMLElement>(
      '[data-night-life-detail="paired-reflection"]',
    )!;
    expect(lighthouse).toHaveAttribute("data-night-life-reflection", "paired");
    expect(reflection.dataset.emissiveOwner).toBe(
      lighthouse.dataset.emissiveOwner,
    );

    rendered.rerender(view("night"));
    const nightSignatures =
      rendered.container.querySelectorAll<HTMLElement>(
        "[data-night-life-region]",
      );
    expect(nightSignatures).toHaveLength(5);
    for (const signature of nightSignatures) {
      const intensity = Number.parseFloat(
        signature.style.getPropertyValue("--train-night-life-intensity"),
      );
      expect(intensity).toBeGreaterThanOrEqual(
        TRAIN_NIGHT_LIFE_MIN_INTENSITY,
      );
      expect(intensity).toBeLessThan(TRAIN_NIGHT_LIFE_MAX_INTENSITY);
      expect(signature).toHaveAttribute("data-emissive-owner");
    }
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\[data-time-of-day="night"\] \.train-night-life\s*\{\s*opacity:\s*var\(--train-night-life-intensity\);/,
    );
    expect(trainLayoutCss).toMatch(
      /@media \(prefers-reduced-motion: reduce\)[\s\S]*?\.train-night-life \*[\s\S]*?animation:\s*none;/,
    );
  });

  it("renders one bounded seeded star catalogue instead of repeating CSS grids", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const catalogue = container.querySelector<HTMLElement>(
      "[data-star-catalogue]",
    )!;
    const nightSky = container.querySelector<HTMLElement>(
      "[data-night-sky-catalogue]",
    )!;
    const moon = nightSky.querySelector<HTMLElement>("[data-moon-id]")!;
    const stars = [...catalogue.querySelectorAll<HTMLElement>(".train-star")];
    const bands = nightSky.querySelectorAll("[data-celestial-band]");
    const accents = nightSky.querySelectorAll("[data-celestial-accent]");

    expect(container.querySelectorAll("[data-star-catalogue]")).toHaveLength(1);
    expect(container.querySelectorAll("[data-night-sky-catalogue]")).toHaveLength(
      1,
    );
    expect(catalogue).toHaveAttribute("data-star-catalogue", "infinite-journey");
    expect(nightSky).toHaveAttribute(
      "data-night-sky-catalogue",
      "infinite-journey",
    );
    expect(nightSky).toHaveAttribute("data-night-sky-version", "night-sky-v2");
    expect(nightSky).toHaveAttribute("data-sky-plane", "behind-terrain");
    expect(nightSky).toHaveAttribute("data-control-contrast", "preserved");
    expect(stars).toHaveLength(Number(catalogue.dataset.starCount));
    expect(stars).toHaveLength(Number(world.dataset.starCount));
    expect(stars.length).toBeLessThanOrEqual(38);
    expect(Number(nightSky.dataset.nightSkyCount)).toBe(
      stars.length + 1 + bands.length + accents.length,
    );
    expect(Number(nightSky.dataset.nightSkyCount)).toBeLessThanOrEqual(41);
    expect(moon.dataset.moonPhase).toMatch(
      /^(crescent|quarter|gibbous|full)$/,
    );
    expect(moon.dataset.moonDirection).toMatch(/^(waxing|waning)$/);
    expect(moon.style.left).toMatch(/%$/);
    expect(moon.style.top).toMatch(/%$/);
    expect(bands.length).toBeLessThanOrEqual(1);
    expect(accents.length).toBeLessThanOrEqual(1);
    expect(
      [...new Set(stars.map((star) => star.dataset.starTint))],
    ).toEqual(expect.arrayContaining(["cool", "neutral", "warm"]));
    expect(
      stars.some((star) => star.dataset.starIntensity === "bright"),
    ).toBe(true);
    expect(
      stars.some((star) => star.dataset.starIntensity === "dim"),
    ).toBe(true);
    expect(
      stars.some((star) => Boolean(star.dataset.starGroup)),
    ).toBe(true);

    const starRule = trainLayoutCss.match(
      /\.train-emissive-overlay--stars\s*\{([^}]*)\}/,
    )?.[1];
    expect(starRule).toBeDefined();
    expect(starRule).not.toContain("radial-gradient");
    expect(starRule).not.toContain("background-size");
    expect(trainLayoutCss).toMatch(
      /\.train-emissive-overlay\s*\{[\s\S]*?opacity:\s*0;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\[data-time-of-day="day"\] \.train-emissive-overlay--stars\s*\{\s*opacity:\s*0;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\[data-time-of-day="sunset"\] \.train-emissive-overlay--stars\s*\{\s*opacity:\s*0\.09;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\[data-time-of-day="night"\] \.train-emissive-overlay--stars\s*\{\s*opacity:\s*1;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\[data-time-of-day="day"\] \.train-celestial-band,\s*\.train-layout-world\[data-time-of-day="sunset"\] \.train-celestial-band\s*\{\s*opacity:\s*0;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\[data-time-of-day="night"\] \.train-celestial-band\s*\{\s*opacity:\s*var\(--train-celestial-band-opacity\);/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\[data-time-of-day="sunset"\] \.train-emissive-overlay--moon\s*\{\s*opacity:\s*0\.14;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\[data-time-of-day="day"\] \.train-celestial-accent,\s*\.train-layout-world\[data-time-of-day="sunset"\] \.train-celestial-accent\s*\{\s*opacity:\s*0;/,
    );
  });

  it("renders a bounded seeded day-sky catalogue behind terrain and controls", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const catalogue = container.querySelector<HTMLElement>(
      "[data-day-sky-catalogue]",
    )!;
    const anchors = [
      ...catalogue.querySelectorAll<HTMLElement>("[data-day-sky-anchor]"),
    ];
    const [gapStart, gapEnd] = catalogue.dataset.daySkyNegativeSpace!
      .split("-")
      .map(Number);

    expect(container.querySelectorAll("[data-day-sky-catalogue]")).toHaveLength(
      1,
    );
    expect(catalogue).toHaveAttribute("data-sky-plane", "behind-terrain");
    expect(catalogue).toHaveAttribute("data-control-contrast", "preserved");
    expect(catalogue.dataset.daySkyWeather).toMatch(
      /^(clear|fair|breezy|showery)$/,
    );
    expect(anchors).toHaveLength(Number(catalogue.dataset.daySkyCount));
    expect(anchors).toHaveLength(Number(world.dataset.daySkyCount));
    expect(anchors.length).toBeLessThanOrEqual(5);
    expect(
      catalogue.querySelectorAll("[data-day-sky-anchor='sun']"),
    ).toHaveLength(1);
    expect(
      catalogue.querySelectorAll("[data-day-sky-anchor='wisp']").length,
    ).toBeGreaterThanOrEqual(1);
    expect(gapEnd! - gapStart!).toBeGreaterThanOrEqual(22);
    expect(gapEnd! - gapStart!).toBeLessThanOrEqual(32);
    expect(trainLayoutCss).toMatch(
      /\.train-sky-emissive\s*\{[\s\S]*?z-index:\s*1;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-inspection\s*\{[\s\S]*?z-index:\s*1;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\s*\{[\s\S]*?z-index:\s*0;/,
    );
  });

  it("keeps cloud and sky-anchor geometry isolated from palette changes", () => {
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    const geometry = () => ({
      sky: [
        ...container.querySelectorAll<HTMLElement>(
          "[data-day-sky-anchor]",
        ),
      ].map((anchor) => `${anchor.dataset.daySkyAnchor}:${anchor.style.cssText}`),
      clouds: [
        ...container.querySelectorAll<HTMLElement>(
          "[data-scenery-category='cloud']",
        ),
      ].map(
        (cloud) =>
          `${cloud.dataset.sceneryAsset}:${cloud.dataset.cloudRoutePosition}:${cloud.style.cssText}`,
      ),
      night: [
        ...container.querySelectorAll<HTMLElement>(
          "[data-night-sky-catalogue] [data-moon-id], " +
            "[data-night-sky-catalogue] [data-star-id], " +
            "[data-night-sky-catalogue] [data-celestial-band], " +
            "[data-night-sky-catalogue] [data-celestial-accent]",
        ),
      ].map(
        (element) =>
          `${element.dataset.moonId ?? element.dataset.starId ?? element.dataset.celestialBand ?? element.dataset.celestialAccent}:` +
          element.style.cssText,
      ),
    });
    const beforeMode = world.dataset.timeOfDay;
    const before = geometry();

    fireEvent.click(
      screen.getByRole("button", {
        name: "Cycle train lighting (day / sunset / night)",
      }),
    );

    expect(world.dataset.timeOfDay).not.toBe(beforeMode);
    expect(geometry()).toEqual(before);
    for (const cloud of container.querySelectorAll(
      "[data-scenery-category='cloud']",
    )) {
      expect(cloud).toHaveAttribute(
        "data-cloud-rendering",
        "palette-specific",
      );
    }
  });

  it("keeps sky motion phase stable across palette changes and resize", () => {
    window.history.replaceState(null, "", "/?train-cruise-speed=96");
    const animation = installAnimationFrame();
    mockVisibility();
    const width = vi
      .spyOn(window, "innerWidth", "get")
      .mockReturnValue(1_024);
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const world = container.querySelector<HTMLElement>(".train-layout-world")!;
    animation.run(0);
    animation.run(250);
    const phases = new Map(
      [...container.querySelectorAll<HTMLElement>("[data-day-sky-anchor]")].map(
        (anchor) => [
          anchor.dataset.daySkyAnchorId!,
          anchor.dataset.skyMotionDistance!,
        ] as const,
      ),
    );
    const cloudPhase =
      container.querySelector<HTMLElement>('[data-world-layer="sky"]')!
        .dataset.layerPosition;
    const routePosition = world.dataset.routePosition;

    fireEvent.click(
      screen.getByRole("button", {
        name: "Cycle train lighting (day / sunset / night)",
      }),
    );
    expect(world.dataset.routePosition).toBe(routePosition);
    for (const anchor of container.querySelectorAll<HTMLElement>(
      "[data-day-sky-anchor]",
    )) {
      expect(anchor.dataset.skyMotionDistance).toBe(
        phases.get(anchor.dataset.daySkyAnchorId!),
      );
    }

    width.mockReturnValue(1_280);
    act(() => window.dispatchEvent(new Event("resize")));
    expect(world.dataset.routePosition).toBe(routePosition);
    expect(
      container.querySelector<HTMLElement>('[data-world-layer="sky"]')!
        .dataset.layerPosition,
    ).toBe(cloudPhase);
    for (const [anchorID, phase] of phases) {
      const anchor = [
        ...container.querySelectorAll<HTMLElement>("[data-day-sky-anchor]"),
      ].find((candidate) => candidate.dataset.daySkyAnchorId === anchorID);
      expect(anchor).toBeDefined();
      expect(anchor!).toHaveAttribute("data-sky-motion-distance", phase);
    }
  });

  it("uses crisp palette-owned cloud grading without a blanket sunset sepia", () => {
    const dayRule = trainLayoutCss.match(
      /\.train-layout-world\[data-time-of-day="day"\]\s+\.train-parallax-chunk--sky\s+\.train-scenery-asset--cloud\s*\{([^}]*)\}/,
    )?.[1];
    const sunsetRule = trainLayoutCss.match(
      /\.train-layout-world\[data-time-of-day="sunset"\]\s+\.train-parallax-chunk--sky\s+\.train-scenery-asset--cloud\s*\{([^}]*)\}/,
    )?.[1];

    expect(dayRule).toContain("opacity: 0.96");
    expect(dayRule).toContain("saturate(0.28)");
    expect(dayRule).toContain("brightness(3.1)");
    expect(dayRule).toContain("contrast(0.78)");
    expect(dayRule).toContain("var(--train-palette-cloud-light)");
    expect(dayRule).toContain("var(--train-palette-cloud-shadow)");
    expect(sunsetRule).toContain("opacity: 0.88");
    expect(sunsetRule).toContain("var(--train-palette-cloud-light)");
    expect(sunsetRule).toContain("var(--train-palette-cloud-shadow)");
    expect(sunsetRule).not.toContain("sepia");
    expect(trainLayoutCss).toMatch(
      /\.train-layout-world\[data-time-of-day="night"\] \.train-day-sky\s*\{\s*opacity:\s*0;/,
    );
  });

  it("keeps the seeded star field static under reduced motion", () => {
    vi.useFakeTimers();
    mockReducedMotion();
    const animation = installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const catalogue = container.querySelector<HTMLElement>(
      "[data-star-catalogue]",
    )!;
    const nightSky = container.querySelector<HTMLElement>(
      "[data-night-sky-catalogue]",
    )!;
    const stars = [...catalogue.querySelectorAll(".train-star")];
    const celestialElements = [...nightSky.children];

    expect(catalogue).toHaveAttribute("data-motion", "reduced");
    expect(nightSky).toHaveAttribute("data-motion", "reduced");
    act(() => vi.advanceTimersByTime(TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS * 2));
    expect([...catalogue.querySelectorAll(".train-star")]).toEqual(stars);
    expect([...nightSky.children]).toEqual(celestialElements);
    expect(animation.pending()).toBeLessThanOrEqual(1);
    expect(trainLayoutCss).toMatch(
      /\.train-emissive-overlay--stars\[data-motion="reduced"\] \.train-star\s*\{[\s\S]*?animation:\s*none;/,
    );
    expect(trainLayoutCss).toMatch(
      /@media \(prefers-reduced-motion: reduce\)\s*\{[\s\S]*?\.train-night-sky,[\s\S]*?\.train-celestial-accent\s*\{[\s\S]*?animation:\s*none;/,
    );
  });

  it("keeps day-sky anchors static under reduced motion", () => {
    vi.useFakeTimers();
    mockReducedMotion();
    installAnimationFrame();
    mockVisibility();
    const { container } = render(
      <TrainLayout panes={[]} selected={null} onSelect={vi.fn()} />,
    );
    const catalogue = container.querySelector<HTMLElement>(
      "[data-day-sky-catalogue]",
    )!;
    const anchors = [...catalogue.querySelectorAll("[data-day-sky-anchor]")];
    const positions = anchors.map(
      (anchor) => (anchor as HTMLElement).dataset.skyPosition,
    );
    const cloudLayer = container.querySelector<HTMLElement>(
      '[data-world-layer="sky"]',
    )!;

    expect(catalogue).toHaveAttribute("data-motion", "reduced");
    act(() => vi.advanceTimersByTime(TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS * 2));
    expect([...catalogue.querySelectorAll("[data-day-sky-anchor]")]).toEqual(
      anchors,
    );
    expect(
      anchors.map((anchor) => (anchor as HTMLElement).dataset.skyPosition),
    ).toEqual(positions);
    expect(cloudLayer).toHaveAttribute("data-layer-position", "0.000px");
    expect(
      anchors.every(
        (anchor) =>
          (anchor as HTMLElement).dataset.skyMotionDistance === "0.000px",
      ),
    ).toBe(true);
    expect(trainLayoutCss).toMatch(
      /@media \(prefers-reduced-motion: reduce\)\s*\{[\s\S]*?\.train-day-sky-anchor\s*\{[\s\S]*?animation:\s*none;/,
    );
  });

  it("provides accessible foreground contrast in every palette", () => {
    expect(trainPaletteContrastRatio("day")).toBeGreaterThanOrEqual(4.5);
    expect(trainPaletteContrastRatio("sunset")).toBeGreaterThanOrEqual(4.5);
    expect(trainPaletteContrastRatio("night")).toBeGreaterThanOrEqual(4.5);
  });

  it("uses palette-owned sky, material, and regional-life tokens without a blanket sunset wash", () => {
    const rgbChannels = (hex: string) =>
      hex
        .slice(1)
        .match(/.{2}/g)!
        .map((channel) => Number.parseInt(channel, 16));
    const blueBias = (hex: string) => {
      const [red, , blue] = rgbChannels(hex);
      return blue! - red!;
    };
    const chroma = (hex: string) => {
      const channels = rgbChannels(hex);
      return Math.max(...channels) - Math.min(...channels);
    };

    expect(blueBias(TRAIN_TIME_PALETTES.day.skyTop)).toBeGreaterThan(100);
    expect(blueBias(TRAIN_TIME_PALETTES.day.skyBottom)).toBeGreaterThan(35);
    expect(chroma(TRAIN_TIME_PALETTES.day.skyTop)).toBeGreaterThan(120);
    expect(TRAIN_TIME_PALETTES.sunset.skyTop).toBe("#465b82");
    expect(TRAIN_TIME_PALETTES.sunset.skyBottom).toBe("#efa16f");
    expect(TRAIN_TIME_PALETTES.sunset.horizonLight).toContain("rgba");
    expect(TRAIN_SCENERY_TIME_GRADES.sunset.warmth).toBe(0);

    const solidTokenNames = [
      "skyTop",
      "skyBottom",
      "cloudLight",
      "cloudShadow",
      "silhouette",
      "farSurface",
      "midSurface",
      "nearSurface",
      "water",
      "forestSoil",
      "mountainRock",
      "forestLife",
      "mountainLife",
      "townLife",
      "coastLife",
      "industrialLife",
      "foregroundContrast",
      "controlSurface",
      "emissive",
    ] as const;
    for (const mode of ["day", "sunset", "night"] as const) {
      const palette = TRAIN_TIME_PALETTES[mode];
      expect(Object.keys(palette)).toEqual(Object.keys(TRAIN_TIME_PALETTES.day));
      for (const token of solidTokenNames) {
        expect(palette[token], `${mode}:${token}`).toMatch(/^#[\da-f]{6}$/i);
      }
      expect(palette.haze, `${mode}:haze`).toMatch(/^rgba\(/);
      expect(palette.horizonLight, `${mode}:horizon`).toMatch(/^rgba\(/);
    }

    expect(trainLayoutCss).toMatch(
      /\.train-day-sky-anchor--sun::after\s*\{[\s\S]*?radial-gradient\([\s\S]*?--train-palette-horizon-light/,
    );
    expect(trainLayoutCss).toMatch(
      /\[data-time-of-day="sunset"\]\s*\.train-emissive-overlay\s*\{\s*opacity:\s*0;/,
    );
    expect(trainLayoutCss).toMatch(
      /data-emissive-region="town"[\s\S]*?opacity:\s*0\.18;/,
    );
  });

  it("keeps palette luminance ordered so geometry remains readable at every time", () => {
    for (const mode of ["day", "sunset", "night"] as const) {
      const order = trainPaletteLuminanceOrder(mode);
      expect(order.skyBottom, `${mode}:sky`).toBeGreaterThan(order.skyTop);
      expect(order.farSurface, `${mode}:far-mid`).toBeGreaterThan(
        order.midSurface,
      );
      expect(order.midSurface, `${mode}:mid-near`).toBeGreaterThan(
        order.nearSurface,
      );
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
