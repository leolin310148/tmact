/// <reference types="node" />

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { inflateSync } from "node:zlib";
import { describe, expect, it } from "vitest";

import { generateRouteChunk, TRAIN_PARALLAX_LAYERS } from "./trainRoute";
import {
  TRAIN_REGION_SCENERY_PROFILES,
  TRAIN_SCENERY_BUILDINGS,
  trainSceneryPlacementsForChunk,
  type TrainSceneryAsset,
} from "./trainScenery";

interface DecodedPng {
  width: number;
  height: number;
  pixels: Uint8Array;
}

const INDUSTRIAL_BUILDING_IDS = [
  "building-workshop",
  "building-warehouse",
  "building-water-tower",
] as const;

const EXPECTED_OPAQUE_PIXELS: Record<string, number> = {
  "building-workshop": 2127,
  "building-warehouse": 6347,
  "building-water-tower": 2748,
} as const;

const EXPECTED_EMISSIVE_PIXELS: Record<string, number> = {
  "building-workshop": 28,
  "building-warehouse": 26,
  "building-water-tower": 8,
} as const;

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

function decodeRgbaPng(fileName: string): DecodedPng {
  const png = readFileSync(
    resolve(
      process.cwd(),
      "src/assets/train-theme/sprites/scenery",
      fileName,
    ),
  );
  expect(png.subarray(0, 8).toString("hex")).toBe("89504e470d0a1a0a");
  const width = png.readUInt32BE(16);
  const height = png.readUInt32BE(20);
  expect(png[24]).toBe(8);
  expect(png[25]).toBe(6);
  expect(png[28]).toBe(0);

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
      expect(Number.isNaN(predictor), `unsupported PNG filter ${filter}`).toBe(
        false,
      );
      pixels[outputOffset] = (raw + predictor) & 0xff;
    }
  }

  return { width, height, pixels };
}

function industrialBuildings(): TrainSceneryAsset[] {
  return INDUSTRIAL_BUILDING_IDS.map((id) => {
    const asset = TRAIN_SCENERY_BUILDINGS.find((candidate) => candidate.id === id);
    expect(asset).toBeDefined();
    return asset!;
  });
}

describe("industrial building base and emissive assets", () => {
  it("records exact geometry-aligned masks for every industrial structure", () => {
    for (const asset of industrialBuildings()) {
      expect(asset.emissive).toMatchObject({
        kind: "windows",
        width: asset.width,
        height: asset.height,
      });
      expect(asset.emissive?.fileName).toBe(`${asset.id}-emissive.png`);
      expect(asset.emissive?.src).toBeTruthy();
    }

    expect(
      TRAIN_SCENERY_BUILDINGS.filter((asset) => !asset.emissive),
    ).toHaveLength(0);
  });

  it("ships opaque daylight bases with sparse masks on real solid pixels", () => {
    for (const asset of industrialBuildings()) {
      const base = decodeRgbaPng(asset.fileName);
      const emissive = decodeRgbaPng(asset.emissive!.fileName);
      expect([base.width, base.height]).toEqual([asset.width, asset.height]);
      expect([emissive.width, emissive.height]).toEqual([
        asset.width,
        asset.height,
      ]);

      let opaqueBasePixels = 0;
      let litPixels = 0;
      const materialColors = new Set<string>();

      for (let offset = 0; offset < base.pixels.length; offset += 4) {
        const baseAlpha = base.pixels[offset + 3]!;
        const maskAlpha = emissive.pixels[offset + 3]!;
        expect([0, 255]).toContain(baseAlpha);
        expect([0, 255]).toContain(maskAlpha);

        if (baseAlpha === 255) {
          opaqueBasePixels++;
          materialColors.add(
            `${base.pixels[offset]},${base.pixels[offset + 1]},${base.pixels[offset + 2]}`,
          );
        }

        if (maskAlpha === 255) {
          litPixels++;
          expect(baseAlpha).toBe(255);
          const red = base.pixels[offset]!;
          const green = base.pixels[offset + 1]!;
          const blue = base.pixels[offset + 2]!;
          expect(green).toBeGreaterThan(red);
          expect(blue).toBeGreaterThan(green);
        }
      }

      expect(opaqueBasePixels).toBe(EXPECTED_OPAQUE_PIXELS[asset.id]);
      expect(litPixels).toBe(EXPECTED_EMISSIVE_PIXELS[asset.id]);
      expect(materialColors.size).toBeGreaterThan(24);
      expect(litPixels).toBeLessThan(base.width * base.height * 0.02);
    }
  });

  it("keeps industrial structures owned only by the industrial region", () => {
    const owners = new Map<string, Set<string>>(
      INDUSTRIAL_BUILDING_IDS.map((id) => [id, new Set<string>()]),
    );

    for (const [region, profile] of Object.entries(
      TRAIN_REGION_SCENERY_PROFILES,
    )) {
      for (const rule of Object.values(profile.layers)) {
        for (const id of rule?.assetIds ?? []) {
          owners.get(id)?.add(region);
        }
      }
      for (const id of "landmark" in profile ? profile.landmark.assetIds : []) {
        owners.get(id)?.add(region);
      }
    }

    for (const id of INDUSTRIAL_BUILDING_IDS) {
      expect(owners.get(id), id).toEqual(new Set(["industrial"]));
    }
  });

  it("renders industrial ownership deterministically without changing bounds", () => {
    const collect = (seed: string) =>
      Array.from({ length: 2401 }, (_, offset) => offset - 1200).flatMap(
        (index) => {
          const chunk = generateRouteChunk(seed, index);
          return TRAIN_PARALLAX_LAYERS.flatMap((layer) =>
            trainSceneryPlacementsForChunk(layer.name, chunk)
              .filter((placement) =>
                INDUSTRIAL_BUILDING_IDS.includes(
                  placement.asset.id as (typeof INDUSTRIAL_BUILDING_IDS)[number],
                ),
              )
              .map((placement) => ({
                chunk: chunk.index,
                region: chunk.region,
                layer: layer.name,
                asset: placement.asset.id,
                offset: placement.offsetPercent,
                scale: placement.scale,
                landmark: placement.landmark,
              })),
          );
        },
      );

    const first = collect("industrial-lighting");
    expect(collect("industrial-lighting")).toEqual(first);
    expect(new Set(first.map((placement) => placement.asset))).toEqual(
      new Set(INDUSTRIAL_BUILDING_IDS),
    );
    expect(
      first.every(
        (placement) =>
          placement.region === "industrial" &&
          placement.layer === "midground",
      ),
    ).toBe(true);
    expect(first.length).toBeLessThan(2401 * 2);
  });

  it("keeps committed PNG decoding deterministic", () => {
    for (const asset of industrialBuildings()) {
      expect(decodeRgbaPng(asset.fileName)).toEqual(
        decodeRgbaPng(asset.fileName),
      );
      expect(decodeRgbaPng(asset.emissive!.fileName)).toEqual(
        decodeRgbaPng(asset.emissive!.fileName),
      );
    }
  });
});
