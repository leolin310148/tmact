/// <reference types="node" />

import { readFileSync, statSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

interface ImageDimensions {
  width: number;
  height: number;
}

const componentRoot = resolve(process.cwd(), "src/components");

function pngDimensions(path: string): ImageDimensions {
  const png = readFileSync(path);
  expect(png.subarray(0, 8).toString("hex"), path).toBe(
    "89504e470d0a1a0a",
  );
  return {
    width: png.readUInt32BE(16),
    height: png.readUInt32BE(20),
  };
}

function webpDimensions(path: string): ImageDimensions {
  const webp = readFileSync(path);
  expect(webp.subarray(0, 4).toString("ascii"), path).toBe("RIFF");
  expect(webp.subarray(8, 12).toString("ascii"), path).toBe("WEBP");
  expect(
    webp.indexOf(Buffer.from("VP8L")),
    `${path} lossless payload`,
  ).toBeGreaterThanOrEqual(12);

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

    offset = dataOffset + length + (length % 2);
  }

  throw new Error(`No supported WebP dimension chunk in ${path}`);
}

function officeRuntimeAssets(): string[] {
  const component = readFileSync(
    resolve(componentRoot, "OfficeDesks.tsx"),
    "utf8",
  );
  const css = readFileSync(resolve(componentRoot, "OfficeDesks.css"), "utf8");
  const imports = [
    ...component.matchAll(/from "([^"]*pixel-agents\/[^"]+)"/g),
  ].map(([, path]) => path!);
  const backgrounds = [
    ...css.matchAll(/url\(([^)]*pixel-agents\/[^)]+)\)/g),
  ].map(([, path]) => path!.replaceAll(/["']/g, ""));

  return [...new Set([...imports, ...backgrounds])].sort();
}

describe("optimized office runtime assets", () => {
  it("loads lossless WebP derivatives while retaining PNG masters", () => {
    const assets = officeRuntimeAssets();

    expect(assets).toHaveLength(20);
    expect(assets.every((path) => path.endsWith(".webp"))).toBe(true);

    let pngBytes = 0;
    let webpBytes = 0;
    for (const webpImport of assets) {
      const webpPath = resolve(componentRoot, webpImport);
      const pngPath = webpPath.replace(/\.webp$/, ".png");

      expect(statSync(webpPath).isFile(), webpImport).toBe(true);
      expect(statSync(pngPath).isFile(), pngPath).toBe(true);
      expect(webpDimensions(webpPath)).toEqual(pngDimensions(pngPath));
      webpBytes += statSync(webpPath).size;
      pngBytes += statSync(pngPath).size;
    }

    expect(webpBytes).toBeLessThan(pngBytes * 0.7);
  });
});
