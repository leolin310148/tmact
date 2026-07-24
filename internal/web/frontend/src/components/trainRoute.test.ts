import { describe, expect, it } from "vitest";

import {
  generateRouteChunk,
  RouteChunkWindow,
  routeChunkWindowRange,
  TRAIN_ROUTE_CHUNK_WIDTH,
  TRAIN_ROUTE_OVERSCAN_CHUNKS,
} from "./trainRoute";

describe("train route chunks", () => {
  it("generates stable chunks from a versioned seed and integer index", () => {
    const first = generateRouteChunk("alpine-line", 42);
    const repeated = generateRouteChunk("alpine-line", 42);

    expect(repeated).toEqual(first);
    expect(first).toEqual({
      index: 42,
      seedKey: "tmact-train-route-v1:alpine-line:42",
      variant: 1,
      terrainHeight: 40,
      ridgeHeight: 56,
      featureOffset: 14,
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
});
