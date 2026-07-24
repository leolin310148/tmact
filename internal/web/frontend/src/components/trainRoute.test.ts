import { describe, expect, it } from "vitest";

import {
  generateRouteChunk,
  RouteChunkWindow,
  routeChunkWindowRange,
  TRAIN_PARALLAX_LAYERS,
  TRAIN_PARALLAX_SEAM_OVERLAP,
  TRAIN_REGION_CHUNK_LENGTH,
  TRAIN_REGION_PROFILES,
  TRAIN_ROUTE_CHUNK_WIDTH,
  TRAIN_ROUTE_OVERSCAN_CHUNKS,
  trainRegionAtIndex,
  trainParallaxLayerPosition,
  trainParallaxLayerTransform,
  type TrainRegionProfile,
} from "./trainRoute";

describe("train route chunks", () => {
  it("generates stable chunks from a versioned seed and integer index", () => {
    const first = generateRouteChunk("alpine-line", 42);
    const repeated = generateRouteChunk("alpine-line", 42);

    expect(repeated).toEqual(first);
    expect(first).toEqual({
      index: 42,
      seedKey: "tmact-train-route-v1:alpine-line:42",
      routeSeed: "alpine-line",
      seedVersion: "tmact-train-route-v1",
      variant: 1,
      terrainHeight: 40,
      ridgeHeight: 56,
      featureOffset: 14,
      region: "mountain",
      regionIndex: 4,
      regionChunkOffset: 6,
      regionChunkLength: 9,
    });
    expect(() => generateRouteChunk("alpine-line", 1.5)).toThrow(
      "route chunk index must be an integer",
    );
  });

  it("varies generated scenery when the seed changes", () => {
    const coast = generateRouteChunk("coast", 8);
    const forest = generateRouteChunk("forest", 8);

    expect({
      variant: coast.variant,
      terrainHeight: coast.terrainHeight,
      ridgeHeight: coast.ridgeHeight,
      featureOffset: coast.featureOffset,
    }).not.toEqual({
      variant: forest.variant,
      terrainHeight: forest.terrainHeight,
      ridgeHeight: forest.ridgeHeight,
      featureOffset: forest.featureOffset,
    });
  });

  it("builds deterministic nine-chunk regions with weighted allowed transitions", () => {
    const firstPass = Array.from({ length: 400 }, (_, offset) =>
      trainRegionAtIndex("grammar-line", offset - 200),
    );
    const repeated = Array.from({ length: 400 }, (_, offset) =>
      trainRegionAtIndex("grammar-line", offset - 200),
    );

    expect(repeated).toEqual(firstPass);
    expect(new Set(firstPass)).toEqual(
      new Set(["forest", "mountain", "town", "coast", "industrial"]),
    );
    expect(TRAIN_REGION_CHUNK_LENGTH).toBeGreaterThanOrEqual(6);
    expect(TRAIN_REGION_CHUNK_LENGTH).toBeLessThanOrEqual(12);

    for (let index = 1; index < firstPass.length; index++) {
      const previous = firstPass[index - 1]!;
      const current = firstPass[index]!;
      expect(
        (TRAIN_REGION_PROFILES[previous] as TrainRegionProfile)
          .transitionWeights[current],
        `${previous} → ${current}`,
      ).toBeGreaterThan(0);
    }
  });

  it("keeps every chunk in a region coherent before changing profile", () => {
    for (let regionIndex = -40; regionIndex <= 40; regionIndex++) {
      const chunks = Array.from({ length: TRAIN_REGION_CHUNK_LENGTH }, (_, offset) =>
        generateRouteChunk(
          "coherent-line",
          regionIndex * TRAIN_REGION_CHUNK_LENGTH + offset,
        ),
      );
      expect(new Set(chunks.map((chunk) => chunk.region))).toEqual(
        new Set([trainRegionAtIndex("coherent-line", regionIndex)]),
      );
      expect(chunks.map((chunk) => chunk.regionChunkOffset)).toEqual(
        Array.from({ length: TRAIN_REGION_CHUNK_LENGTH }, (_, offset) => offset),
      );
    }
  });

  it("reports the default seed when an empty seed falls back", () => {
    const route = new RouteChunkWindow("");
    const snapshot = route.update(0, 320);

    expect(route.seed).toBe("infinite-journey");
    expect(snapshot.chunks[0]?.seedKey).toContain(
      "tmact-train-route-v1:infinite-journey:",
    );
  });

  it("recycles a bounded window during long-distance travel", () => {
    const route = new RouteChunkWindow("long-haul");
    const viewportWidth = 1440;
    const maximumMounted =
      Math.ceil(viewportWidth / TRAIN_ROUTE_CHUNK_WIDTH) +
      1 +
      TRAIN_ROUTE_OVERSCAN_CHUNKS * 2;
    let maximumObserved = 0;

    for (let position = 0; position <= 10_000_000; position += 79_321) {
      const snapshot = route.update(position, viewportWidth);
      maximumObserved = Math.max(maximumObserved, snapshot.chunks.length);
      expect(snapshot.chunks).toHaveLength(
        snapshot.lastIndex - snapshot.firstIndex + 1,
      );
      expect(snapshot.chunks.length).toBeLessThanOrEqual(maximumMounted);
    }

    expect(maximumObserved).toBe(maximumMounted);
  });

  it("retains visible chunk objects and complete coverage across resizing", () => {
    const route = new RouteChunkWindow("resize-line");
    const compact = route.update(775, 375);
    const retained = compact.chunks.find((chunk) => chunk.index === 2);
    const wide = route.update(775, 1920);
    const sameChunk = wide.chunks.find((chunk) => chunk.index === 2);

    expect(sameChunk).toBe(retained);
    expect(wide.firstVisibleIndex).toBeLessThan(compact.firstVisibleIndex);
    expect(wide.lastVisibleIndex).toBe(compact.lastVisibleIndex);

    const leftmostVisibleX =
      wide.routePosition - wide.lastVisibleIndex * TRAIN_ROUTE_CHUNK_WIDTH;
    const rightmostVisibleEdge =
      wide.routePosition -
      wide.firstVisibleIndex * TRAIN_ROUTE_CHUNK_WIDTH +
      TRAIN_ROUTE_CHUNK_WIDTH;
    expect(leftmostVisibleX).toBeLessThanOrEqual(0);
    expect(rightmostVisibleEdge).toBeGreaterThanOrEqual(wide.viewportWidth);
  });

  it("keeps adjacent ranges continuous at exact chunk boundaries", () => {
    const before = routeChunkWindowRange(319.999, 640);
    const boundary = routeChunkWindowRange(320, 640);
    const after = routeChunkWindowRange(320.001, 640);

    expect(before.firstVisibleIndex).toBe(-1);
    expect(boundary.firstVisibleIndex).toBe(0);
    expect(boundary.lastVisibleIndex).toBe(1);
    expect(after.firstVisibleIndex).toBe(boundary.firstVisibleIndex);
    expect(after.lastVisibleIndex).toBe(boundary.lastVisibleIndex + 1);
    expect(before.lastIndex - after.firstIndex).toBeGreaterThanOrEqual(
      TRAIN_ROUTE_OVERSCAN_CHUNKS * 2,
    );
  });

  it("defines five ordered parallax layers with restrained speed ratios", () => {
    expect(TRAIN_PARALLAX_LAYERS).toEqual([
      { name: "sky", speedRatio: 0 },
      { name: "ultra-far", speedRatio: 0.1 },
      { name: "far", speedRatio: 0.25 },
      { name: "midground", speedRatio: 0.55 },
      { name: "near", speedRatio: 1 },
    ]);
    expect(TRAIN_PARALLAX_SEAM_OVERLAP).toBe(2);
  });

  it("calculates layer transforms from route position and pauses reduced motion", () => {
    expect(trainParallaxLayerPosition(240, 0.25)).toBe(60);
    expect(trainParallaxLayerPosition(240, 0.55)).toBe(132);
    expect(trainParallaxLayerTransform(240, 0.1)).toBe(
      "translate3d(24.000px, 0, 0)",
    );
    expect(trainParallaxLayerTransform(240, 1)).toBe(
      "translate3d(240.000px, 0, 0)",
    );
    expect(trainParallaxLayerPosition(240, 1, true)).toBe(0);
    expect(trainParallaxLayerTransform(240, 1, true)).toBe("none");
    expect(trainParallaxLayerPosition(Number.NaN, 1)).toBe(0);
    expect(trainParallaxLayerPosition(240, Number.NaN)).toBe(0);
  });
});
