/// <reference types="node" />

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { inflateSync } from "node:zlib";
import { describe, expect, it } from "vitest";

interface DecodedPng {
  width: number;
  height: number;
  colorType: number;
  pixels: Uint8Array;
}

interface Pixel {
  red: number;
  green: number;
  blue: number;
  alpha: number;
}

interface Bounds {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

const spritePath = resolve(
  process.cwd(),
  "src/assets/train-theme/sprites/train-carriage-empty-v2.png",
);

function paeth(left: number, up: number, upperLeft: number): number {
  const prediction = left + up - upperLeft;
  const leftDistance = Math.abs(prediction - left);
  const upDistance = Math.abs(prediction - up);
  const upperLeftDistance = Math.abs(prediction - upperLeft);
  if (leftDistance <= upDistance && leftDistance <= upperLeftDistance) {
    return left;
  }
  return upDistance <= upperLeftDistance ? up : upperLeft;
}

function decodeRgbaPng(path: string): DecodedPng {
  const png = readFileSync(path);
  expect(png.subarray(0, 8).toString("hex")).toBe("89504e470d0a1a0a");

  let width = 0;
  let height = 0;
  let bitDepth = 0;
  let colorType = 0;
  let interlace = 0;
  const compressed: Buffer[] = [];

  for (let offset = 8; offset < png.length; ) {
    const length = png.readUInt32BE(offset);
    const type = png.subarray(offset + 4, offset + 8).toString("ascii");
    const data = png.subarray(offset + 8, offset + 8 + length);
    if (type === "IHDR") {
      width = data.readUInt32BE(0);
      height = data.readUInt32BE(4);
      bitDepth = data[8]!;
      colorType = data[9]!;
      interlace = data[12]!;
    } else if (type === "IDAT") {
      compressed.push(data);
    }
    offset += length + 12;
    if (type === "IEND") break;
  }

  expect(bitDepth).toBe(8);
  expect(colorType).toBe(6);
  expect(interlace).toBe(0);

  const bytesPerPixel = 4;
  const stride = width * bytesPerPixel;
  const filtered = inflateSync(Buffer.concat(compressed));
  expect(filtered).toHaveLength(height * (stride + 1));
  const pixels = new Uint8Array(height * stride);

  let filteredOffset = 0;
  for (let y = 0; y < height; y++) {
    const filter = filtered[filteredOffset++]!;
    const rowOffset = y * stride;
    const previousRowOffset = rowOffset - stride;
    for (let x = 0; x < stride; x++) {
      const value = filtered[filteredOffset++]!;
      const left = x >= bytesPerPixel ? pixels[rowOffset + x - bytesPerPixel]! : 0;
      const up = y > 0 ? pixels[previousRowOffset + x]! : 0;
      const upperLeft =
        y > 0 && x >= bytesPerPixel
          ? pixels[previousRowOffset + x - bytesPerPixel]!
          : 0;
      const prediction =
        filter === 0
          ? 0
          : filter === 1
            ? left
            : filter === 2
              ? up
              : filter === 3
                ? Math.floor((left + up) / 2)
                : filter === 4
                  ? paeth(left, up, upperLeft)
                  : Number.NaN;
      expect(prediction, `unsupported PNG filter ${filter}`).not.toBeNaN();
      pixels[rowOffset + x] = (value + prediction) & 0xff;
    }
  }

  return { width, height, colorType, pixels };
}

function pixelAt(png: DecodedPng, x: number, y: number): Pixel {
  const offset = (y * png.width + x) * 4;
  return {
    red: png.pixels[offset]!,
    green: png.pixels[offset + 1]!,
    blue: png.pixels[offset + 2]!,
    alpha: png.pixels[offset + 3]!,
  };
}

function pixelsWithin(png: DecodedPng, bounds: Bounds): Pixel[] {
  const pixels: Pixel[] = [];
  for (let y = bounds.top; y <= bounds.bottom; y++) {
    for (let x = bounds.left; x <= bounds.right; x++) {
      pixels.push(pixelAt(png, x, y));
    }
  }
  return pixels;
}

const carriage = decodeRgbaPng(spritePath);
const paneBounds: readonly Bounds[] = [
  { left: 294, top: 82, right: 397, bottom: 137 },
  { left: 294, top: 219, right: 397, bottom: 274 },
];

describe("train carriage transparency asset", () => {
  it("keeps the exact RGBA canvas and transparent exterior corners", () => {
    expect(carriage).toMatchObject({
      width: 821,
      height: 383,
      colorType: 6,
    });
    for (const [x, y] of [
      [0, 0],
      [820, 0],
      [0, 382],
      [820, 382],
    ] as const) {
      expect(pixelAt(carriage, x, y).alpha, `${x},${y}`).toBe(0);
    }
  });

  it("makes both passenger-window interiors broadly and cleanly transparent", () => {
    for (const bounds of paneBounds) {
      const pane = pixelsWithin(carriage, bounds);
      expect(pane.filter(({ alpha }) => alpha === 0).length).toBeGreaterThan(5_500);
      expect(pane.every(({ alpha }) => alpha === 0 || alpha === 255)).toBe(true);
    }

    for (const [x, y] of [
      [310, 100],
      [342, 110],
      [380, 120],
      [310, 237],
      [342, 248],
      [380, 257],
    ] as const) {
      expect(pixelAt(carriage, x, y), `${x},${y}`).toEqual({
        red: 0,
        green: 0,
        blue: 0,
        alpha: 0,
      });
    }
  });

  it("keeps window frames, carriage details, and chroma-free edges opaque", () => {
    for (const [x, y] of [
      [342, 76],
      [288, 110],
      [400, 110],
      [342, 141],
      [342, 213],
      [288, 248],
      [400, 248],
      [342, 278],
      [200, 110],
      [269, 101],
      [650, 200],
    ] as const) {
      expect(pixelAt(carriage, x, y).alpha, `${x},${y}`).toBe(255);
    }

    const edgePixels = [
      ...pixelsWithin(carriage, {
        left: 286,
        top: 74,
        right: 405,
        bottom: 145,
      }),
      ...pixelsWithin(carriage, {
        left: 286,
        top: 211,
        right: 405,
        bottom: 282,
      }),
    ];
    expect(
      edgePixels.some(
        ({ red, green, blue, alpha }) =>
          alpha > 0 && green > 160 && green > red * 2 && green > blue * 2,
      ),
    ).toBe(false);
  });
});
