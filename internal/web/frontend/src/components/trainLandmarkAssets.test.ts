/// <reference types="node" />

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { inflateSync } from "node:zlib";
import { describe, expect, it } from "vitest";

import {
  TRAIN_SCENERY_LANDMARKS,
  type TrainSceneryAsset,
} from "./trainScenery";

interface DecodedPng {
  width: number;
  height: number;
  pixels: Uint8Array;
}

const EXPECTED_LANDMARKS = {
  "landmark-forest-clearing": {
    width: 192,
    height: 96,
    collisionWidth: 176,
    safeScale: [0.62, 0.82],
    category: "vegetation",
  },
  "landmark-mountain-lookout": {
    width: 176,
    height: 104,
    collisionWidth: 160,
    safeScale: [0.58, 0.78],
    category: "building",
  },
  "landmark-town-church": {
    width: 168,
    height: 112,
    collisionWidth: 150,
    safeScale: [0.58, 0.78],
    category: "building",
  },
  "landmark-coast-lighthouse": {
    width: 184,
    height: 112,
    collisionWidth: 160,
    safeScale: [0.58, 0.78],
    category: "building",
  },
  "landmark-industrial-gantry": {
    width: 200,
    height: 96,
    collisionWidth: 184,
    safeScale: [0.6, 0.82],
    category: "building",
  },
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
  for (let offset = 8; offset < png.length;) {
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

describe("train regional daylight landmarks", () => {
  it("records exact bounded geometry and daylight-neutral rendering contracts", () => {
    expect(TRAIN_SCENERY_LANDMARKS).toHaveLength(5);
    for (const asset of TRAIN_SCENERY_LANDMARKS) {
      expect(asset).toMatchObject({
        id: asset.id,
        fileName: `${asset.id}.png`,
        layer: "midground",
        anchor: "bottom-center",
        dayNightTreatment: "solid-palette-grade",
        ...EXPECTED_LANDMARKS[asset.id as keyof typeof EXPECTED_LANDMARKS],
      });
      expect(asset.emissive).toBeUndefined();
    }
  });

  it("ships crisp RGBA sprites with transparent exteriors and opaque interiors", () => {
    for (const asset of TRAIN_SCENERY_LANDMARKS) {
      const png = decodeRgbaPng(asset);
      expect(png.width, asset.id).toBe(asset.width);
      expect(png.height, asset.id).toBe(asset.height);
      let transparent = 0;
      let opaque = 0;
      let luminanceTotal = 0;
      let minimumLuminance = 255;
      let maximumLuminance = 0;
      const colors = new Set<string>();

      for (let offset = 0; offset < png.pixels.length; offset += 4) {
        const alpha = png.pixels[offset + 3]!;
        expect([0, 255], `${asset.id} alpha at ${offset / 4}`).toContain(alpha);
        if (alpha === 0) {
          transparent++;
          continue;
        }
        const red = png.pixels[offset]!;
        const green = png.pixels[offset + 1]!;
        const blue = png.pixels[offset + 2]!;
        const luminance = red * 0.2126 + green * 0.7152 + blue * 0.0722;
        expect(
          red > 220 && green < 70 && blue > 180,
          `${asset.id} retained chroma-key pixel`,
        ).toBe(false);
        opaque++;
        luminanceTotal += luminance;
        minimumLuminance = Math.min(minimumLuminance, luminance);
        maximumLuminance = Math.max(maximumLuminance, luminance);
        colors.add(`${red},${green},${blue}`);
      }

      expect(transparent, asset.id).toBeGreaterThan(png.width);
      expect(opaque, asset.id).toBeGreaterThan(png.height * 8);
      expect(colors.size, asset.id).toBeGreaterThan(32);
      expect(luminanceTotal / opaque, asset.id).toBeGreaterThan(48);
      expect(maximumLuminance - minimumLuminance, asset.id).toBeGreaterThan(90);
    }
  });
});
