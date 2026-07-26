import { describe, expect, it } from "vitest";

import {
  generateRouteChunk,
  RouteChunkWindow,
  routeChunkWindowRange,
  TRAIN_PARALLAX_LAYERS,
  TRAIN_PARALLAX_SEAM_OVERLAP,
  TRAIN_REGION_CHUNK_LENGTH,
  TRAIN_REGION_PROFILES,
  TRAIN_SET_PIECE_DEFINITIONS,
  TRAIN_SET_PIECE_VISUAL_VARIANT_COUNT,
  TRAIN_ROUTE_CHUNK_WIDTH,
  TRAIN_ROUTE_OVERSCAN_CHUNKS,
  trainSetPiecesAreIncompatible,
  trainSetPieceVisualVariant,
  trainRegionAtIndex,
  trainParallaxLayerPosition,
  trainParallaxLayerTransform,
  type TrainRegionProfile,
} from "./trainRoute";
import { TRAIN_SKY_CLOUD_SPEED_RATIO } from "./trainMotion";

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
      setPiece: {
        id: "tmact-train-route-v1:alpine-line:set-piece:4:bridge",
        type: "bridge",
        role: "body",
        startIndex: 40,
        endIndex: 43,
        span: 4,
        segmentOffset: 2,
        visualVariant: 0,
        renderLayer: "midground",
        reservedLayers: ["midground", "near"],
        incompatibleWith: ["tunnel", "coast-reveal", "town-edge", "station"],
      },
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

  it("reserves deterministic non-overlapping multi-chunk set pieces", () => {
    const firstPass = Array.from({ length: 1_801 }, (_, offset) =>
      generateRouteChunk("set-piece-line", offset - 900).setPiece,
    );
    const repeated = Array.from({ length: 1_801 }, (_, offset) =>
      generateRouteChunk("set-piece-line", offset - 900).setPiece,
    );
    const entries = firstPass.filter(
      (setPiece) => setPiece?.role === "entry",
    );

    expect(repeated).toEqual(firstPass);
    expect(new Set(entries.map((setPiece) => setPiece?.type))).toEqual(
      new Set(["bridge", "tunnel", "coast-reveal", "town-edge", "station"]),
    );

    const reservations = entries
      .map((setPiece) => setPiece!)
      .sort((left, right) => left.startIndex - right.startIndex);
    for (let index = 1; index < reservations.length; index++) {
      const previous = reservations[index - 1]!;
      const current = reservations[index]!;
      expect(previous.endIndex).toBeLessThan(current.startIndex);
      if (
        trainSetPiecesAreIncompatible(previous.type, current.type) ||
        trainSetPiecesAreIncompatible(current.type, previous.type)
      ) {
        expect(previous.endIndex).toBeLessThan(current.startIndex);
      }
    }
  });

  it("emits continuous entry/body/exit segments for bridge and transition traversals", () => {
    const segmentsByID = new Map<
      string,
      NonNullable<ReturnType<typeof generateRouteChunk>["setPiece"]>[]
    >();

    for (let index = -900; index <= 900; index++) {
      const setPiece = generateRouteChunk("traversal-line", index).setPiece;
      if (!setPiece) continue;
      const segments = segmentsByID.get(setPiece.id) ?? [];
      segments.push(setPiece);
      segmentsByID.set(setPiece.id, segments);
    }

    const complete = [...segmentsByID.values()].filter(
      (segments) => segments.length === segments[0]?.span,
    );
    for (const segments of complete) {
      segments.sort((left, right) => left.segmentOffset - right.segmentOffset);
      expect(segments.map((segment) => segment.role)).toEqual([
        "entry",
        ...Array.from({ length: segments.length - 2 }, () => "body" as const),
        "exit",
      ]);
      expect(segments.map((segment) => segment.startIndex + segment.segmentOffset))
        .toEqual(
          Array.from(
            { length: segments.length },
            (_, offset) => segments[0]!.startIndex + offset,
          ),
        );
      expect(new Set(segments.map((segment) => segment.id)).size).toBe(1);
      expect(
        new Set(segments.map((segment) => segment.visualVariant)).size,
      ).toBe(1);
    }

    expect(complete.some((segments) => segments[0]?.type === "bridge")).toBe(true);
    expect(
      complete.some(
        (segments) =>
          segments[0]?.type === "coast-reveal" ||
          segments[0]?.type === "tunnel",
      ),
    ).toBe(true);
  });

  it("selects two stable visual compositions per major set piece without changing route contracts", () => {
    const majorTypes = [
      "bridge",
      "tunnel",
      "coast-reveal",
      "town-edge",
    ] as const;
    const observed = new Map(
      majorTypes.map((type) => [type, new Set<number>()]),
    );
    const firstPass = Array.from({ length: 7_201 }, (_, offset) =>
      generateRouteChunk("variant-frequency-line", offset - 3_600),
    );
    const repeated = Array.from({ length: firstPass.length }, (_, offset) =>
      generateRouteChunk("variant-frequency-line", offset - 3_600),
    );

    expect(repeated).toEqual(firstPass);
    for (const chunk of firstPass) {
      const setPiece = chunk.setPiece;
      if (!setPiece || setPiece.type === "station") {
        if (setPiece) expect(setPiece.visualVariant).toBe(0);
        continue;
      }
      observed.get(setPiece.type)!.add(setPiece.visualVariant);
      expect(setPiece.span).toBe(TRAIN_SET_PIECE_DEFINITIONS[setPiece.type].span);
      expect(setPiece.incompatibleWith).toEqual(
        TRAIN_SET_PIECE_DEFINITIONS[setPiece.type].incompatibleWith,
      );
    }

    for (const type of majorTypes) {
      expect(TRAIN_SET_PIECE_VISUAL_VARIANT_COUNT[type]).toBe(2);
      expect(observed.get(type), type).toEqual(new Set([0, 1]));
    }
  });

  it("keys visual variants by route version, seed, and set-piece identity", () => {
    const ids = Array.from(
      { length: 64 },
      (_, index) => `tmact-train-route-v1:visual-key:set-piece:${index}:bridge`,
    );
    const catalogue = ids.map((id) =>
      trainSetPieceVisualVariant("visual-key", id, "bridge"),
    );
    const repeated = ids.map((id) =>
      trainSetPieceVisualVariant("visual-key", id, "bridge"),
    );
    const otherSeed = ids.map((id) =>
      trainSetPieceVisualVariant("visual-key-b", id, "bridge"),
    );
    const otherVersion = ids.map((id) =>
      trainSetPieceVisualVariant(
        "visual-key",
        id,
        "bridge",
        "tmact-train-route-v2",
      ),
    );

    expect(repeated).toEqual(catalogue);
    expect(otherSeed).not.toEqual(catalogue);
    expect(otherVersion).not.toEqual(catalogue);
    expect(new Set(catalogue)).toEqual(new Set([0, 1]));
  });

  it("declares symmetric incompatibility and bounded region reservations", () => {
    for (const definition of Object.values(TRAIN_SET_PIECE_DEFINITIONS)) {
      expect(definition.span).toBeGreaterThanOrEqual(3);
      expect(definition.span).toBeLessThan(TRAIN_REGION_CHUNK_LENGTH);
      for (const incompatible of definition.incompatibleWith) {
        expect(trainSetPiecesAreIncompatible(incompatible, definition.type)).toBe(
          true,
        );
      }
    }

    for (let index = -900; index <= 900; index++) {
      const chunk = generateRouteChunk("bounded-pieces", index);
      if (!chunk.setPiece) continue;
      const regionStart = chunk.regionIndex * TRAIN_REGION_CHUNK_LENGTH;
      const regionEnd = regionStart + TRAIN_REGION_CHUNK_LENGTH - 1;
      expect(chunk.setPiece.startIndex).toBeGreaterThanOrEqual(regionStart);
      expect(chunk.setPiece.endIndex).toBeLessThan(regionEnd);
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
      { name: "sky", speedRatio: TRAIN_SKY_CLOUD_SPEED_RATIO },
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
