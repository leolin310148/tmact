/// <reference types="node" />

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { inflateSync } from "node:zlib";
import { describe, expect, it } from "vitest";

import {
  TRAIN_SCENERY_BUILDINGS,
  type TrainSceneryAsset,
} from "./trainScenery";

interface DecodedPng {
  width: number;
  height: number;
  pixels: Uint8Array;
}

const TOWN_BUILDING_IDS = [
  "building-rowhouse",
  "building-apartments",
  "building-cottage",
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

function townBuildings(): TrainSceneryAsset[] {
  return TOWN_BUILDING_IDS.map((id) => {
    const asset = TRAIN_SCENERY_BUILDINGS.find((candidate) => candidate.id === id);
    expect(asset).toBeDefined();
    return asset!;
  });
}

describe("town building base and emissive assets", () => {
  it("records exact geometry-aligned mask metadata without changing base geometry", () => {
    for (const asset of townBuildings()) {
      expect(asset.emissive).toMatchObject({
        kind: "windows",
        width: asset.width,
        height: asset.height,
      });
      expect(asset.emissive?.fileName).toBe(
        `${asset.id}-emissive.png`,
      );
      expect(asset.emissive?.src).toBeTruthy();
    }

    expect(
      TRAIN_SCENERY_BUILDINGS.filter((asset) => asset.emissive).map(
        (asset) => asset.id,
      ),
    ).toEqual(TOWN_BUILDING_IDS);
  });

  it("ships binary-alpha opaque bases and sparse masks aligned only to solid pixels", () => {
    for (const asset of townBuildings()) {
      const base = decodeRgbaPng(asset.fileName);
      const emissive = decodeRgbaPng(asset.emissive!.fileName);
      expect([base.width, base.height]).toEqual([asset.width, asset.height]);
      expect([emissive.width, emissive.height]).toEqual([
        asset.width,
        asset.height,
      ]);

      let transparentBasePixels = 0;
      let opaqueBasePixels = 0;
      let litPixels = 0;
      const materialColors = new Set<string>();

      for (let offset = 0; offset < base.pixels.length; offset += 4) {
        const baseAlpha = base.pixels[offset + 3]!;
        const maskAlpha = emissive.pixels[offset + 3]!;
        expect([0, 255]).toContain(baseAlpha);
        expect([0, 255]).toContain(maskAlpha);

        if (baseAlpha === 0) {
          transparentBasePixels++;
        } else {
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

      expect(transparentBasePixels).toBeGreaterThan(0);
      expect(opaqueBasePixels).toBeGreaterThan(base.width);
      expect(materialColors.size).toBeGreaterThan(24);
      expect(litPixels).toBeGreaterThan(8);
      expect(litPixels).toBeLessThan(base.width * base.height * 0.08);
    }
  });

  it("keeps the committed PNG bytes deterministic across repeated decoding", () => {
    for (const asset of townBuildings()) {
      expect(decodeRgbaPng(asset.fileName)).toEqual(
        decodeRgbaPng(asset.fileName),
      );
      expect(decodeRgbaPng(asset.emissive!.fileName)).toEqual(
        decodeRgbaPng(asset.emissive!.fileName),
      );
    }
  });
});
