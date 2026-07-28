/// <reference types="node" />

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { inflateSync } from "node:zlib";
import { describe, expect, it } from "vitest";

import {
  TRAIN_SCENERY_TIME_GRADES,
  trainPaletteContrastRatio,
  trainPaletteLuminanceOrder,
} from "./TrainLayout";
import {
  generateRouteChunk,
  RouteChunkWindow,
  TRAIN_PARALLAX_LAYERS,
  TRAIN_REGION_CHUNK_LENGTH,
  TRAIN_ROUTE_CHUNK_WIDTH,
  TRAIN_ROUTE_OVERSCAN_CHUNKS,
  TRAIN_SET_PIECE_VISUAL_VARIANT_COUNT,
  trainParallaxLayerPosition,
  trainParallaxLayerTransform,
  trainSetPieceFocusForOccurrence,
  trainSetPieceScreenGeometry,
  type TrainRegionName,
  type TrainSetPieceType,
} from "./trainRoute";
import {
  TRAIN_SCENERY_ASSETS,
  TRAIN_SCENERY_DEPTH_GRAMMAR,
  TRAIN_TOWN_INDUSTRIAL_MIN_REPEAT_DISTANCE_PX,
  TRAIN_FOREST_MOUNTAIN_MIN_REPEAT_DISTANCE_PX,
  trainCoastSceneryBeatForChunk,
  trainForestMountainSceneryBeatForChunk,
  trainSceneryPlacementsForChunk,
  trainTownIndustrialSceneryBeatForChunk,
  type TrainSceneryAsset,
} from "./trainScenery";

const TRAIN_LAYOUT_SOURCE = readFileSync(
  resolve(process.cwd(), "src/components/TrainLayout.tsx"),
  "utf8",
);
const TRAIN_LAYOUT_STYLES = readFileSync(
  resolve(process.cwd(), "src/components/TrainLayout.css"),
  "utf8",
);
const AUDIT_SEEDS = [
  "train-053-aurora",
  "train-053-cascade",
  "train-053-harbour",
  "train-053-orchard",
  "train-053-summit",
] as const;
const AUDIT_VIEWPORTS = [390, 1_280, 2_560] as const;
const REGION_NAMES = [
  "forest",
  "mountain",
  "town",
  "coast",
  "industrial",
] as const satisfies readonly TrainRegionName[];
const SET_PIECE_TYPES = [
  "bridge",
  "tunnel",
  "town-edge",
  "coast-reveal",
  "station",
] as const satisfies readonly TrainSetPieceType[];

interface DecodedRgbaPng {
  width: number;
  height: number;
  pixels: Uint8Array;
}

function paeth(left: number, above: number, upperLeft: number): number {
  const estimate = left + above - upperLeft;
  const leftDistance = Math.abs(estimate - left);
  const aboveDistance = Math.abs(estimate - above);
  const upperLeftDistance = Math.abs(estimate - upperLeft);
  if (leftDistance <= aboveDistance && leftDistance <= upperLeftDistance) {
    return left;
  }
  return aboveDistance <= upperLeftDistance ? above : upperLeft;
}

function decodeSceneryPng(fileName: string): DecodedRgbaPng {
  const png = readFileSync(
    resolve(
      process.cwd(),
      "src/assets/train-theme/sprites/scenery",
      fileName,
    ),
  );
  expect(png.subarray(0, 8).toString("hex"), fileName).toBe(
    "89504e470d0a1a0a",
  );
  const width = png.readUInt32BE(16);
  const height = png.readUInt32BE(20);
  expect(png[24], `${fileName} bit depth`).toBe(8);
  expect(png[25], `${fileName} colour type`).toBe(6);
  expect(png[28], `${fileName} interlace`).toBe(0);

  const compressed: Buffer[] = [];
  for (let offset = 8; offset < png.length; ) {
    const length = png.readUInt32BE(offset);
    const type = png.subarray(offset + 4, offset + 8).toString("ascii");
    if (type === "IDAT") {
      compressed.push(png.subarray(offset + 8, offset + 8 + length));
    }
    offset += length + 12;
  }

  const encoded = inflateSync(Buffer.concat(compressed));
  const bytesPerPixel = 4;
  const stride = width * bytesPerPixel;
  const pixels = new Uint8Array(stride * height);
  let sourceOffset = 0;

  for (let y = 0; y < height; y++) {
    const filter = encoded[sourceOffset++]!;
    for (let x = 0; x < stride; x++) {
      const raw = encoded[sourceOffset++]!;
      const outputOffset = y * stride + x;
      const left =
        x >= bytesPerPixel ? pixels[outputOffset - bytesPerPixel]! : 0;
      const above = y > 0 ? pixels[outputOffset - stride]! : 0;
      const upperLeft =
        y > 0 && x >= bytesPerPixel
          ? pixels[outputOffset - stride - bytesPerPixel]!
          : 0;
      const predictor =
        filter === 0
          ? 0
          : filter === 1
            ? left
            : filter === 2
              ? above
              : filter === 3
                ? Math.floor((left + above) / 2)
                : filter === 4
                  ? paeth(left, above, upperLeft)
                  : Number.NaN;
      expect(Number.isNaN(predictor), `${fileName} PNG filter ${filter}`).toBe(
        false,
      );
      pixels[outputOffset] = (raw + predictor) & 0xff;
    }
  }

  return { width, height, pixels };
}

function assertBinaryAlpha(
  asset: TrainSceneryAsset,
  image: DecodedRgbaPng,
): void {
  expect([image.width, image.height], asset.id).toEqual([
    asset.width,
    asset.height,
  ]);
  let opaquePixels = 0;
  let transparentPixels = 0;
  let invalidAlpha: { offset: number; alpha: number } | null = null;
  for (let offset = 3; offset < image.pixels.length; offset += 4) {
    const alpha = image.pixels[offset]!;
    if (alpha !== 0 && alpha !== 255 && invalidAlpha === null) {
      invalidAlpha = { offset, alpha };
    }
    if (alpha === 255) opaquePixels++;
    else transparentPixels++;
  }
  expect(invalidAlpha, `${asset.id} binary alpha`).toBeNull();
  expect(opaquePixels, `${asset.id} opaque interior`).toBeGreaterThan(
    asset.width,
  );
  expect(transparentPixels, `${asset.id} transparent exterior`).toBeGreaterThan(
    0,
  );
}

describe("train final visual convergence", () => {
  it("keeps five deterministic routes bounded while covering every region and set-piece variant", () => {
    for (const seed of AUDIT_SEEDS) {
      const first = Array.from({ length: 2_401 }, (_, offset) =>
        generateRouteChunk(seed, offset - 1_200),
      );
      const repeated = Array.from({ length: first.length }, (_, offset) =>
        generateRouteChunk(seed, offset - 1_200),
      );
      expect(repeated, seed).toEqual(first);
      expect(new Set(first.map((chunk) => chunk.region)), seed).toEqual(
        new Set(REGION_NAMES),
      );

      const variants = new Map<TrainSetPieceType, Set<number>>(
        SET_PIECE_TYPES.map((type) => [type, new Set<number>()]),
      );
      for (const chunk of first) {
        if (chunk.setPiece?.role === "entry") {
          variants
            .get(chunk.setPiece.type)!
            .add(chunk.setPiece.visualVariant);
        }
      }
      for (const type of SET_PIECE_TYPES) {
        expect(variants.get(type), `${seed}/${type}`).toEqual(
          new Set(
            Array.from(
              { length: TRAIN_SET_PIECE_VISUAL_VARIANT_COUNT[type] },
              (_, variant) => variant,
            ),
          ),
        );
      }

      for (const viewportWidth of AUDIT_VIEWPORTS) {
        const route = new RouteChunkWindow(seed);
        const maximumMounted =
          Math.ceil(viewportWidth / TRAIN_ROUTE_CHUNK_WIDTH) +
          1 +
          TRAIN_ROUTE_OVERSCAN_CHUNKS * 2;
        for (const routePosition of [
          0,
          2_880,
          6_720,
          11_520,
          28_800,
          122_240,
          987_654,
        ]) {
          const snapshot = route.update(routePosition, viewportWidth);
          expect(
            snapshot.chunks.length,
            `${seed}/${viewportWidth}/${routePosition}`,
          ).toBeLessThanOrEqual(maximumMounted);
          expect(snapshot.chunks).toHaveLength(
            snapshot.lastIndex - snapshot.firstIndex + 1,
          );
        }
      }
    }
  });

  it("maintains regional density gaps and minimum repetition distance across seeds", () => {
    let repeatedPairs = 0;
    const regionCounts = new Map<TrainRegionName, number>(
      REGION_NAMES.map((region) => [region, 0]),
    );

    for (const seed of AUDIT_SEEDS) {
      for (let regionIndex = -120; regionIndex <= 120; regionIndex++) {
        const chunks = Array.from(
          { length: TRAIN_REGION_CHUNK_LENGTH },
          (_, offset) =>
            generateRouteChunk(
              seed,
              regionIndex * TRAIN_REGION_CHUNK_LENGTH + offset,
            ),
        );
        const region = chunks[0]!.region;
        regionCounts.set(region, regionCounts.get(region)! + 1);
        const lastCenterByAsset = new Map<string, number>();

        for (const chunk of chunks) {
          for (const placement of trainSceneryPlacementsForChunk(
            "midground",
            chunk,
            { includeSetPieces: false },
          ).filter((candidate) => !candidate.landmark)) {
            const center =
              chunk.index * TRAIN_ROUTE_CHUNK_WIDTH +
              (placement.offsetPercent / 100) * TRAIN_ROUTE_CHUNK_WIDTH;
            const previous = lastCenterByAsset.get(placement.asset.id);
            if (previous !== undefined) {
              repeatedPairs++;
              const minimum =
                region === "forest" || region === "mountain"
                  ? TRAIN_FOREST_MOUNTAIN_MIN_REPEAT_DISTANCE_PX
                  : region === "town" || region === "industrial"
                    ? TRAIN_TOWN_INDUSTRIAL_MIN_REPEAT_DISTANCE_PX
                    : 144;
              expect(
                center - previous,
                `${seed}/${region}/${placement.asset.id}`,
              ).toBeGreaterThanOrEqual(minimum);
            }
            lastCenterByAsset.set(placement.asset.id, center);
          }
        }
      }
    }

    expect(repeatedPairs).toBeGreaterThan(500);
    for (const region of REGION_NAMES) {
      expect(regionCounts.get(region), region).toBeGreaterThan(100);
    }
  });

  it("locks the observed TRAIN-054–060 region and traversal contracts across audit seeds", () => {
    const materialStyles =
      TRAIN_LAYOUT_STYLES.split(".train-terrain-base[data-terrain-material]")[1]
        ?.split(".train-coast-composition")[0] ?? "";
    expect(materialStyles).not.toContain("repeating-linear-gradient");
    expect(TRAIN_LAYOUT_STYLES).toContain(
      '.train-traversal-composition--tunnel[data-traversal-active="false"]',
    );
    expect(TRAIN_LAYOUT_STYLES).toMatch(
      /\.train-bridge-crossing-void\s*\{[^}]*height:\s*36px/s,
    );
    expect(TRAIN_LAYOUT_SOURCE).toContain(
      'data-bridge-forest-ground={',
    );
    expect(TRAIN_LAYOUT_SOURCE).toContain(
      'data-coast-contact-medium="dry-land"',
    );
    expect(TRAIN_LAYOUT_SOURCE).toContain(
      "data-built-fixture-pixel-density={",
    );
    expect(TRAIN_LAYOUT_SOURCE).toContain(
      "data-station-negative-space={",
    );

    for (const seed of AUDIT_SEEDS) {
      const forestRoles = new Set<string>();
      const mountainRoles = new Set<string>();
      const townScales = new Set<string>();
      const industrialScales = new Set<string>();
      const coastWaterKinds = new Set<string>();
      const coastFixtures = new Set<string>();

      for (let index = -1_200; index <= 1_200; index++) {
        const chunk = generateRouteChunk(seed, index);
        const naturalBeat = trainForestMountainSceneryBeatForChunk(chunk);
        if (naturalBeat?.region === "forest") {
          forestRoles.add(naturalBeat.role);
        } else if (naturalBeat?.region === "mountain") {
          mountainRoles.add(naturalBeat.role);
        }

        const builtBeat = trainTownIndustrialSceneryBeatForChunk(chunk);
        if (builtBeat?.region === "town") {
          townScales.add(builtBeat.scaleFamily);
        } else if (builtBeat?.region === "industrial") {
          industrialScales.add(builtBeat.scaleFamily);
        }

        const coastBeat = trainCoastSceneryBeatForChunk(chunk);
        if (coastBeat) {
          coastWaterKinds.add(coastBeat.waterKind);
          coastBeat.fixtures.forEach((fixture) => coastFixtures.add(fixture));
        }
      }

      expect(
        forestRoles.has("forest-canopy-cluster"),
        `${seed}/forest canopy`,
      ).toBe(true);
      expect(
        forestRoles.has("forest-clearing"),
        `${seed}/forest clearing`,
      ).toBe(true);
      expect(
        mountainRoles.has("mountain-layered-ridge"),
        `${seed}/mountain ridge`,
      ).toBe(true);
      expect(
        mountainRoles.has("mountain-open-vista"),
        `${seed}/mountain vista`,
      ).toBe(true);
      expect(townScales.size, `${seed}/town scale families`).toBeGreaterThan(2);
      expect(
        industrialScales.size,
        `${seed}/industrial scale families`,
      ).toBeGreaterThan(2);
      expect(coastWaterKinds.size, `${seed}/coast water rhythm`).toBeGreaterThan(
        4,
      );
      expect(coastFixtures.has("beach"), `${seed}/coast beach`).toBe(true);
      expect(coastFixtures.has("rock-shelf"), `${seed}/coast shelf`).toBe(true);
      expect(coastFixtures.has("pier"), `${seed}/coast pier`).toBe(true);
    }
  });

  it("centres visible geometry for every set-piece type and both visual variants", () => {
    for (const seed of AUDIT_SEEDS) {
      const variants = new Map<TrainSetPieceType, Set<number>>(
        SET_PIECE_TYPES.map((type) => [type, new Set<number>()]),
      );
      for (const viewportWidth of AUDIT_VIEWPORTS) {
        for (const type of SET_PIECE_TYPES) {
          for (let occurrence = 0; occurrence < 8; occurrence++) {
            const focus = trainSetPieceFocusForOccurrence(
              seed,
              type,
              viewportWidth,
              occurrence,
            );
            expect(focus, `${seed}/${type}/${occurrence}`).not.toBeNull();
            variants.get(type)!.add(focus!.visualVariant);
            const layer = TRAIN_PARALLAX_LAYERS.find(
              (candidate) => candidate.name === focus!.renderLayer,
            )!;
            const geometry = trainSetPieceScreenGeometry(
              focus!,
              layer.speedRatio,
            );
            expect(
              geometry.screenCenterPx,
              `${seed}/${type}/${viewportWidth}/${occurrence}`,
            ).toBeGreaterThanOrEqual(viewportWidth * 0.25);
            expect(
              geometry.screenCenterPx,
              `${seed}/${type}/${viewportWidth}/${occurrence}`,
            ).toBeLessThanOrEqual(viewportWidth * 0.75);
            expect(
              geometry.visibleWidthPx,
              `${seed}/${type}/${viewportWidth}/${occurrence}`,
            ).toBeGreaterThanOrEqual(Math.min(320, viewportWidth * 0.5));
            expect(new Set(focus!.expectedVisibleSegmentIDs).size).toBe(
              focus!.span,
            );
          }
        }
      }
      for (const type of SET_PIECE_TYPES) {
        expect(variants.get(type), `${seed}/${type}`).toEqual(
          new Set(
            Array.from(
              { length: TRAIN_SET_PIECE_VISUAL_VARIANT_COUNT[type] },
              (_, variant) => variant,
            ),
          ),
        );
      }
    }
  });

  it("preserves monotonic depth and palette contrast in every time mode", () => {
    const depths = ["ultra-far", "far", "midground", "near"] as const;
    for (let index = 1; index < depths.length; index++) {
      const previous = TRAIN_SCENERY_DEPTH_GRAMMAR[depths[index - 1]!];
      const current = TRAIN_SCENERY_DEPTH_GRAMMAR[depths[index]!];
      expect(current.scaleMultiplier).toBeGreaterThan(
        previous.scaleMultiplier,
      );
      expect(current.contrast).toBeGreaterThan(previous.contrast);
      expect(current.detailBudget).toBeGreaterThanOrEqual(
        previous.detailBudget,
      );
      expect(current.brightness).toBeLessThan(previous.brightness);
    }

    for (const mode of ["day", "sunset", "night"] as const) {
      const luminance = trainPaletteLuminanceOrder(mode);
      expect(luminance.skyBottom, mode).toBeGreaterThan(luminance.skyTop);
      expect(luminance.farSurface, mode).toBeGreaterThan(
        luminance.midSurface,
      );
      expect(luminance.midSurface, mode).toBeGreaterThan(
        luminance.nearSurface,
      );
      expect(trainPaletteContrastRatio(mode), mode).toBeGreaterThan(1.4);
    }
    expect(TRAIN_SCENERY_TIME_GRADES.day.brightness).toBeGreaterThan(
      TRAIN_SCENERY_TIME_GRADES.sunset.brightness,
    );
    expect(TRAIN_SCENERY_TIME_GRADES.sunset.brightness).toBeGreaterThan(
      TRAIN_SCENERY_TIME_GRADES.night.brightness,
    );
  });

  it("keeps every solid scenery sprite binary-alpha and every emissive mask owned by its base", () => {
    for (const asset of TRAIN_SCENERY_ASSETS) {
      const base = decodeSceneryPng(asset.fileName);
      if (asset.category === "cloud") {
        const alphas = new Set<number>();
        for (let offset = 3; offset < base.pixels.length; offset += 4) {
          alphas.add(base.pixels[offset]!);
        }
        expect(alphas.has(0), `${asset.id} transparent exterior`).toBe(true);
        expect(alphas.has(255), `${asset.id} opaque cloud core`).toBe(true);
        expect(
          [...alphas].some((alpha) => alpha > 0 && alpha < 255),
          `${asset.id} intentional soft atmosphere`,
        ).toBe(true);
        continue;
      }
      assertBinaryAlpha(asset, base);
      if (!asset.emissive) continue;

      const mask = decodeSceneryPng(asset.emissive.fileName);
      expect([mask.width, mask.height], asset.id).toEqual([
        base.width,
        base.height,
      ]);
      let litPixels = 0;
      let invalidMaskAlpha: { offset: number; alpha: number } | null = null;
      let detachedMaskPixel: number | null = null;
      for (let offset = 3; offset < mask.pixels.length; offset += 4) {
        const alpha = mask.pixels[offset]!;
        if (alpha !== 0 && alpha !== 255 && invalidMaskAlpha === null) {
          invalidMaskAlpha = { offset, alpha };
        }
        if (alpha === 255) {
          litPixels++;
          if (base.pixels[offset] !== 255 && detachedMaskPixel === null) {
            detachedMaskPixel = offset;
          }
        }
      }
      expect(invalidMaskAlpha, `${asset.id} emissive binary alpha`).toBeNull();
      expect(detachedMaskPixel, `${asset.id} detached emissive`).toBeNull();
      expect(litPixels, `${asset.id} emissive ownership`).toBeGreaterThan(0);
    }
  });

  it("freezes parallax without changing deterministic route geometry for reduced motion", () => {
    for (const seed of AUDIT_SEEDS) {
      for (const viewportWidth of AUDIT_VIEWPORTS) {
        const full = new RouteChunkWindow(seed).update(122_240, viewportWidth);
        const reduced = new RouteChunkWindow(seed).update(
          122_240,
          viewportWidth,
        );
        expect(reduced, `${seed}/${viewportWidth}`).toEqual(full);
      }
      for (const layer of TRAIN_PARALLAX_LAYERS) {
        expect(
          trainParallaxLayerPosition(122_240, layer.speedRatio, true),
          `${seed}/${layer.name}`,
        ).toBe(0);
        expect(
          trainParallaxLayerTransform(122_240, layer.speedRatio, true),
          `${seed}/${layer.name}`,
        ).toBe("none");
      }
    }
  });
});
