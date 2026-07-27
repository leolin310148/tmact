/// <reference types="node" />

import { readFileSync, readdirSync, statSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

interface ImageDimensions {
  width: number;
  height: number;
}

const assetRoot = resolve(process.cwd(), "src/assets/train-theme/sprites");
const componentRoot = resolve(process.cwd(), "src/components");

function webpDimensions(path: string): ImageDimensions {
  const webp = readFileSync(path);
  expect(webp.subarray(0, 4).toString("ascii"), path).toBe("RIFF");
  expect(webp.subarray(8, 12).toString("ascii"), path).toBe("WEBP");

  for (let offset = 12; offset + 8 <= webp.length; ) {
    const type = webp.subarray(offset, offset + 4).toString("ascii");
    const length = webp.readUInt32LE(offset + 4);
    const dataOffset = offset + 8;

    if (type === "VP8X") {
      return {
        width: webp.readUIntLE(dataOffset + 4, 3) + 1,
        height: webp.readUIntLE(dataOffset + 7, 3) + 1,
      };
    }
    if (type === "VP8L") {
      expect(webp[dataOffset], `${path} VP8L signature`).toBe(0x2f);
      const bits = webp.readUInt32LE(dataOffset + 1);
      return {
        width: (bits & 0x3fff) + 1,
        height: ((bits >>> 14) & 0x3fff) + 1,
      };
    }
    if (
      type === "VP8 " &&
      webp.subarray(dataOffset + 3, dataOffset + 6).toString("hex") ===
        "9d012a"
    ) {
      return {
        width: webp.readUInt16LE(dataOffset + 6) & 0x3fff,
        height: webp.readUInt16LE(dataOffset + 8) & 0x3fff,
      };
    }

    offset = dataOffset + length + (length % 2);
  }

  throw new Error(`No supported WebP dimension chunk in ${path}`);
}

function importedTrainSprites(sourceFile: string): string[] {
  const source = readFileSync(resolve(componentRoot, sourceFile), "utf8");
  return [...source.matchAll(/from "([^"]*train-theme\/sprites\/[^"]+)"/g)].map(
    ([, path]) => path!,
  );
}

function pairedAssetBytes(directory: string, prefix = ""): {
  pngBytes: number;
  webpBytes: number;
} {
  const files = readdirSync(directory);
  const pngStems = files
    .filter((file) => file.startsWith(prefix) && file.endsWith(".png"))
    .map((file) => file.slice(0, -4))
    .sort();
  const webpStems = files
    .filter((file) => file.startsWith(prefix) && file.endsWith(".webp"))
    .map((file) => file.slice(0, -5))
    .sort();

  expect(webpStems).toEqual(pngStems);
  return {
    pngBytes: pngStems.reduce(
      (total, stem) => total + statSync(resolve(directory, `${stem}.png`)).size,
      0,
    ),
    webpBytes: webpStems.reduce(
      (total, stem) => total + statSync(resolve(directory, `${stem}.webp`)).size,
      0,
    ),
  };
}

describe("optimized train runtime assets", () => {
  it("loads WebP derivatives while retaining PNG masters", () => {
    const imports = [
      ...importedTrainSprites("TrainLayout.tsx"),
      ...importedTrainSprites("trainScenery.ts"),
    ];

    expect(imports).toHaveLength(48);
    expect(imports.every((path) => path.endsWith(".webp"))).toBe(true);
    for (const importedPath of imports) {
      expect(
        statSync(resolve(componentRoot, importedPath)).isFile(),
        importedPath,
      ).toBe(true);
    }
  });

  it("keeps the large consist artwork at the two-device-pixel ceiling", () => {
    const carriagePath = resolve(assetRoot, "train-carriage-empty-v2.webp");
    const locomotivePath = resolve(assetRoot, "train-locomotive.webp");

    expect(webpDimensions(carriagePath)).toEqual({ width: 617, height: 288 });
    expect(webpDimensions(locomotivePath)).toEqual({
      width: 573,
      height: 288,
    });
    expect(statSync(carriagePath).size).toBeLessThan(60 * 1024);
    expect(statSync(locomotivePath).size).toBeLessThan(60 * 1024);
  });

  it("keeps seat sprites lossless-sized for a two-device-pixel target", () => {
    const characterDirectory = resolve(assetRoot, "characters");
    const { pngBytes, webpBytes } = pairedAssetBytes(
      characterDirectory,
      "train-seat-",
    );

    for (const file of readdirSync(characterDirectory).filter(
      (name) => name.startsWith("train-seat-") && name.endsWith(".webp"),
    )) {
      expect(webpDimensions(resolve(characterDirectory, file))).toEqual({
        width: 96,
        height: 96,
      });
    }
    expect(webpBytes).toBeLessThan(pngBytes * 0.6);
  });

  it("keeps every scenery master paired with a smaller lossless derivative", () => {
    const { pngBytes, webpBytes } = pairedAssetBytes(
      resolve(assetRoot, "scenery"),
    );

    expect(webpBytes).toBeLessThan(pngBytes * 0.75);
  });
});
