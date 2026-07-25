/// <reference types="node" />

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { inflateSync } from "node:zlib";
import { describe, expect, it } from "vitest";

import {
  generateRouteChunk,
  RouteChunkWindow,
  TRAIN_REGION_CHUNK_LENGTH,
  TRAIN_ROUTE_CHUNK_WIDTH,
  type TrainRegionName,
} from "./trainRoute";
import {
  TRAIN_NEAR_TRACK_COOLDOWN_CHUNKS,
  TRAIN_NEAR_TRACK_MIN_SPACING_PX,
  TRAIN_NEAR_TRACK_PROP_POOLS,
  TRAIN_REGION_SCENERY_PROFILES,
  TRAIN_SCENERY_PROPS,
  TRAIN_SCENERY_VEGETATION,
  trainSceneryPlacementsForChunk,
  type TrainSceneryAsset,
} from "./trainScenery";

interface DecodedPng {
  width: number;
  height: number;
  pixels: Uint8Array;
}

const NEW_PROP_IDS = [
  "prop-milepost",
  "prop-signal-cabinet",
  "prop-crossing-marker",
  "prop-lamp-post",
  "prop-maintenance-equipment",
] as const;

const EXPECTED_NEW_PROP_GEOMETRY = {
  "prop-milepost": {
    width: 32,
    height: 68,
    collisionWidth: 24,
    safeScale: [0.7, 1],
  },
  "prop-signal-cabinet": {
    width: 68,
    height: 52,
    collisionWidth: 58,
    safeScale: [0.65, 0.95],
  },
  "prop-crossing-marker": {
    width: 54,
    height: 84,
    collisionWidth: 42,
    safeScale: [0.55, 0.85],
  },
  "prop-lamp-post": {
    width: 40,
    height: 92,
    collisionWidth: 24,
    safeScale: [0.55, 0.85],
  },
  "prop-maintenance-equipment": {
    width: 96,
    height: 48,
    collisionWidth: 82,
    safeScale: [0.65, 0.95],
  },
} as const;

const VEGETATION_AND_PROPS = [
  ...TRAIN_SCENERY_VEGETATION,
  ...TRAIN_SCENERY_PROPS,
] as const;

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

function decodeRgbaPng(asset: TrainSceneryAsset): DecodedPng {
  const png = readFileSync(
    resolve(
      process.cwd(),
      "src/assets/train-theme/sprites/scenery",
      asset.fileName,
    ),
  );
  expect(png.subarray(0, 8).toString("hex")).toBe("89504e470d0a1a0a");
  const width = png.readUInt32BE(16);
  const height = png.readUInt32BE(20);
  expect(png[24], `${asset.id} bit depth`).toBe(8);
  expect(png[25], `${asset.id} color type`).toBe(6);
  expect(png[28], `${asset.id} interlace`).toBe(0);

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
  const stride = width * 4;
  const pixels = new Uint8Array(stride * height);
  let sourceOffset = 0;

  for (let y = 0; y < height; y++) {
    const filter = encoded[sourceOffset++]!;
    for (let x = 0; x < stride; x++) {
      const raw = encoded[sourceOffset++]!;
      const outputOffset = y * stride + x;
      const left = x >= 4 ? pixels[outputOffset - 4]! : 0;
      const above = y > 0 ? pixels[outputOffset - stride]! : 0;
      const upperLeft =
        y > 0 && x >= 4 ? pixels[outputOffset - stride - 4]! : 0;
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
      expect(Number.isNaN(predictor), `unsupported PNG filter ${filter}`).toBe(
        false,
      );
      pixels[outputOffset] = (raw + predictor) & 0xff;
    }
  }

  return { width, height, pixels };
}

function nearTrackLine(seed: string, firstIndex: number, lastIndex: number) {
  return Array.from(
    { length: lastIndex - firstIndex + 1 },
    (_, offset) => generateRouteChunk(seed, firstIndex + offset),
  ).flatMap((chunk) =>
    trainSceneryPlacementsForChunk("near", chunk).map((placement) => ({
      chunkIndex: chunk.index,
      region: chunk.region,
      center:
        chunk.index * TRAIN_ROUTE_CHUNK_WIDTH +
        (placement.offsetPercent / 100) * TRAIN_ROUTE_CHUNK_WIDTH,
      ...placement,
    })),
  );
}

describe("train vegetation and near-track prop kit", () => {
  it("keeps audited vegetation and every prop on opaque solid-palette treatment", () => {
    expect(TRAIN_SCENERY_VEGETATION).toHaveLength(6);
    expect(TRAIN_SCENERY_PROPS).toHaveLength(8);
    expect(VEGETATION_AND_PROPS).toHaveLength(14);

    for (const asset of VEGETATION_AND_PROPS) {
      expect(asset.src).toBeTruthy();
      expect(asset.anchor).toBe("bottom-center");
      expect(asset.dayNightTreatment).toBe("solid-palette-grade");
      expect(asset.collisionWidth).toBeGreaterThan(0);
      expect(asset.collisionWidth).toBeLessThanOrEqual(asset.width);
    }

    for (const id of NEW_PROP_IDS) {
      const asset = TRAIN_SCENERY_PROPS.find((candidate) => candidate.id === id);
      expect(asset, id).toMatchObject({
        id,
        fileName: `${id}.png`,
        category: "prop",
        layer: "near",
        anchor: "bottom-center",
        dayNightTreatment: "solid-palette-grade",
        ...EXPECTED_NEW_PROP_GEOMETRY[id],
      });
    }
  });

  it("ships transparent exteriors and binary-alpha opaque sprite interiors", () => {
    for (const asset of VEGETATION_AND_PROPS) {
      const png = decodeRgbaPng(asset);
      expect(png.width, asset.id).toBe(asset.width);
      expect(png.height, asset.id).toBe(asset.height);
      let transparent = 0;
      let opaque = 0;
      const solidColors = new Set<string>();

      for (let offset = 0; offset < png.pixels.length; offset += 4) {
        const alpha = png.pixels[offset + 3]!;
        expect([0, 255], `${asset.id} alpha at ${offset / 4}`).toContain(alpha);
        if (alpha === 0) {
          transparent++;
          continue;
        }
        opaque++;
        solidColors.add(
          `${png.pixels[offset]},${png.pixels[offset + 1]},${png.pixels[offset + 2]}`,
        );
      }

      expect(transparent, asset.id).toBeGreaterThan(0);
      expect(opaque, asset.id).toBeGreaterThan(asset.height);
      expect(solidColors.size, asset.id).toBeGreaterThan(16);
    }
  });

  it("owns each prop through explicit regional pools and sparse near-track rules", () => {
    const allPropIDs = new Set(TRAIN_SCENERY_PROPS.map((asset) => asset.id));
    const pooledIDs = new Set<string>();

    for (const region of Object.keys(
      TRAIN_NEAR_TRACK_PROP_POOLS,
    ) as TrainRegionName[]) {
      const pool = TRAIN_NEAR_TRACK_PROP_POOLS[region];
      const rule = TRAIN_REGION_SCENERY_PROFILES[region].layers.near!;
      expect(pool.length, region).toBeGreaterThanOrEqual(4);
      expect(new Set(pool).size, region).toBe(pool.length);
      expect(pool.every((id) => allPropIDs.has(id)), region).toBe(true);
      pool.forEach((id) => pooledIDs.add(id));
      expect(rule.assetIds).toEqual(pool);
      expect(rule.maxPerChunk).toBe(1);
      expect(rule.minimumSpacingPx).toBe(TRAIN_NEAR_TRACK_MIN_SPACING_PX);
      expect(rule.cooldownChunks).toBe(TRAIN_NEAR_TRACK_COOLDOWN_CHUNKS);
      expect(rule.density).toBeLessThanOrEqual(0.5);
    }

    expect(pooledIDs).toEqual(allPropIDs);
    for (const id of NEW_PROP_IDS) {
      expect(
        Object.values(TRAIN_NEAR_TRACK_PROP_POOLS).filter((pool) =>
          (pool as readonly string[]).includes(id),
        ).length,
        id,
      ).toBeGreaterThanOrEqual(2);
    }
  });

  it("spaces and cools deterministic multi-seed placements without picket runs", () => {
    const seeds = [
      "vegetation-props-a",
      "vegetation-props-b",
      "vegetation-props-c",
      "vegetation-props-d",
    ];
    const signatures = seeds.map((seed) => {
      const line = nearTrackLine(seed, -1_200, 1_200);
      expect(nearTrackLine(seed, -1_200, 1_200)).toEqual(line);
      expect(new Set(line.map(({ asset }) => asset.id))).toEqual(
        new Set(TRAIN_SCENERY_PROPS.map((asset) => asset.id)),
      );

      for (let index = 1; index < line.length; index++) {
        const previous = line[index - 1]!;
        const current = line[index]!;
        const distance = current.center - previous.center;
        expect(distance).toBeGreaterThanOrEqual(
          TRAIN_NEAR_TRACK_MIN_SPACING_PX,
        );
        expect(distance).toBeGreaterThanOrEqual(
          previous.collisionWidth / 2 + current.collisionWidth / 2,
        );
        expect(current.chunkIndex - previous.chunkIndex).toBeGreaterThanOrEqual(
          2,
        );
      }
      expect(line.length).toBeLessThan(2_401 * 0.5);
      return line.map(
        ({ asset, chunkIndex, offsetPercent, scale }) =>
          `${chunkIndex}:${asset.id}:${offsetPercent.toFixed(3)}:${scale.toFixed(3)}`,
      );
    });

    expect(new Set(signatures.map((signature) => signature.join("|"))).size).toBe(
      seeds.length,
    );
  });

  it("keeps the mounted near-track vocabulary bounded across long travel and resizing", () => {
    const route = new RouteChunkWindow("bounded-near-track");
    let maximumMounted = 0;

    for (const viewportWidth of [375, 1440, 2560]) {
      for (let position = 0; position <= 10_000_000; position += 91_337) {
        const snapshot = route.update(position, viewportWidth);
        const mounted = snapshot.chunks.flatMap((chunk) =>
          trainSceneryPlacementsForChunk("near", chunk),
        );
        maximumMounted = Math.max(maximumMounted, mounted.length);
        expect(mounted.length).toBeLessThanOrEqual(snapshot.chunks.length);
        expect(
          snapshot.chunks.every(
            (chunk) =>
              trainSceneryPlacementsForChunk("near", chunk).length <= 1,
          ),
        ).toBe(true);
      }
    }

    const widest = route.update(10_000_000, 2560);
    expect(maximumMounted).toBeLessThanOrEqual(widest.chunks.length);
    expect(TRAIN_REGION_CHUNK_LENGTH).toBe(9);
  });
});
