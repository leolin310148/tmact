/// <reference types="node" />

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { inflateSync } from "node:zlib";
import { describe, expect, it } from "vitest";

import {
  TRAIN_SCENERY_DEPTH_PROFILES,
  TRAIN_SCENERY_TIME_GRADES,
  TRAIN_TIME_PALETTES,
} from "./TrainLayout";
import {
  generateRouteChunk,
  TRAIN_PARALLAX_LAYERS,
  TRAIN_PARALLAX_SEAM_OVERLAP,
} from "./trainRoute";
import {
  TRAIN_SCENERY_BRIDGES,
  TRAIN_SCENERY_COASTS,
  TRAIN_SCENERY_TERRAIN,
  trainSceneryPlacementsForChunk,
  type TrainSceneryAsset,
} from "./trainScenery";

interface DecodedPng {
  width: number;
  height: number;
  pixels: Uint8Array;
}

interface SolidPixelStats {
  opaque: number;
  transparent: number;
  colors: Set<string>;
  minimumLuminance: number;
  maximumLuminance: number;
  averageRed: number;
  averageGreen: number;
  averageBlue: number;
}

const DISTANT_KIT = [
  ...TRAIN_SCENERY_TERRAIN,
  ...TRAIN_SCENERY_COASTS,
  ...TRAIN_SCENERY_BRIDGES,
] as const;

const trainLayoutCss = readFileSync(
  resolve(process.cwd(), "src/components/TrainLayout.css"),
  "utf8",
);

const EXPECTED_GEOMETRY = {
  "terrain-foothills": {
    width: 244,
    height: 61,
    anchor: "bottom-center",
    safeScale: [0.8, 1.25],
  },
  "terrain-alpine": {
    width: 244,
    height: 71,
    anchor: "bottom-center",
    safeScale: [0.75, 1.15],
  },
  "terrain-mesa": {
    width: 244,
    height: 45,
    anchor: "bottom-center",
    safeScale: [0.85, 1.3],
  },
  "coast-shore": {
    width: 224,
    height: 33,
    anchor: "bottom-center",
    safeScale: [0.8, 1.15],
  },
  "bridge-truss": {
    width: 224,
    height: 67,
    anchor: "bottom-center",
    safeScale: [0.75, 1],
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

function solidPixelStats(asset: TrainSceneryAsset): SolidPixelStats {
  const png = decodeRgbaPng(asset.fileName);
  let opaque = 0;
  let transparent = 0;
  let minimumLuminance = Number.POSITIVE_INFINITY;
  let maximumLuminance = Number.NEGATIVE_INFINITY;
  let redTotal = 0;
  let greenTotal = 0;
  let blueTotal = 0;
  const colors = new Set<string>();

  for (let offset = 0; offset < png.pixels.length; offset += 4) {
    const alpha = png.pixels[offset + 3]!;
    expect([0, 255], `${asset.id} alpha at byte ${offset + 3}`).toContain(alpha);
    if (alpha === 0) {
      transparent++;
      continue;
    }

    const red = png.pixels[offset]!;
    const green = png.pixels[offset + 1]!;
    const blue = png.pixels[offset + 2]!;
    const luminance = 0.2126 * red + 0.7152 * green + 0.0722 * blue;
    opaque++;
    redTotal += red;
    greenTotal += green;
    blueTotal += blue;
    minimumLuminance = Math.min(minimumLuminance, luminance);
    maximumLuminance = Math.max(maximumLuminance, luminance);
    colors.add(`${red},${green},${blue}`);
  }

  return {
    opaque,
    transparent,
    colors,
    minimumLuminance,
    maximumLuminance,
    averageRed: redTotal / opaque,
    averageGreen: greenTotal / opaque,
    averageBlue: blueTotal / opaque,
  };
}

function hexLuminance(color: string): number {
  const channels = color
    .slice(1)
    .match(/.{2}/g)!
    .map((channel) => Number.parseInt(channel, 16));
  return (
    0.2126 * channels[0]! +
    0.7152 * channels[1]! +
    0.0722 * channels[2]!
  );
}

function collectSetPieceTraversal(seed: string) {
  const groups = new Map<
    string,
    Array<{
      chunkIndex: number;
      type: "coast-reveal";
      role: "entry" | "body" | "exit";
      segmentOffset: number;
      span: number;
    }>
  >();

  for (let index = -1_200; index <= 1_200; index++) {
    const chunk = generateRouteChunk(seed, index);
    if (chunk.setPiece?.type !== "coast-reveal") {
      continue;
    }

    const placements = TRAIN_PARALLAX_LAYERS.flatMap((layer) =>
      trainSceneryPlacementsForChunk(layer.name, chunk).filter(
        (placement) => placement.setPiece?.id === chunk.setPiece?.id,
      ),
    );
    expect(placements, `${chunk.setPiece.id}/${chunk.index}`).toHaveLength(0);
    const segments = groups.get(chunk.setPiece.id) ?? [];
    segments.push({
      chunkIndex: index,
      type: chunk.setPiece.type,
      role: chunk.setPiece.role,
      segmentOffset: chunk.setPiece.segmentOffset,
      span: chunk.setPiece.span,
    });
    groups.set(chunk.setPiece.id, segments);
  }

  return [...groups.entries()]
    .filter(([, segments]) => segments.length === segments[0]?.span)
    .map(([id, segments]) => ({
      id,
      segments: segments.sort(
        (left, right) => left.segmentOffset - right.segmentOffset,
      ),
    }));
}

describe("distant terrain, coast, and bridge asset kit", () => {
  it("preserves manifest geometry, anchors, perspective scales, and collision widths", () => {
    expect(DISTANT_KIT).toHaveLength(5);
    for (const asset of DISTANT_KIT) {
      const geometry =
        EXPECTED_GEOMETRY[asset.id as keyof typeof EXPECTED_GEOMETRY];
      expect(geometry, asset.id).toBeDefined();
      expect(asset).toMatchObject(geometry);
      expect(asset.src).toBeTruthy();
      expect(asset.category).toMatch(/^(terrain|coast|bridge)$/);
    }

    expect(TRAIN_PARALLAX_SEAM_OVERLAP).toBe(2);
    expect(
      TRAIN_SCENERY_COASTS[0].width *
        TRAIN_SCENERY_COASTS[0].safeScale[1],
    ).toBeCloseTo(257.6);
    expect(
      TRAIN_SCENERY_BRIDGES[0].width *
        TRAIN_SCENERY_BRIDGES[0].safeScale[1],
    ).toBe(224);
    expect(trainLayoutCss).toMatch(
      /\.train-set-piece\s*\{[\s\S]*?right:\s*-1px;[\s\S]*?left:\s*-1px;[\s\S]*?opacity:\s*1;/,
    );
    expect(trainLayoutCss).toMatch(
      /\.train-bridge-deck\s*\{[\s\S]*?bottom:\s*14px;[\s\S]*?background:\s*var\(--train-palette-mid-surface\);/,
    );
    const bridgeSupportsRule = trainLayoutCss.match(
      /\.train-bridge-supports\s*\{([^}]+)\}/,
    )?.[1];
    expect(bridgeSupportsRule).toMatch(/bottom:\s*18px;/);
    expect(bridgeSupportsRule).toMatch(/height:\s*58px;/);
    const bridgeCrossingRule = trainLayoutCss.match(
      /\.train-bridge-crossing-void\s*\{([^}]+)\}/,
    )?.[1];
    expect(bridgeCrossingRule).toMatch(/bottom:\s*18px;/);
    expect(bridgeCrossingRule).toMatch(/height:\s*36px;/);
    const bridgeTrackEdgeRule = trainLayoutCss.match(
      /\.train-bridge-track-edge\s*\{([^}]+)\}/,
    )?.[1];
    expect(bridgeTrackEdgeRule).toMatch(/bottom:\s*12px;/);
    expect(bridgeTrackEdgeRule).toMatch(/height:\s*7px;/);
    expect(trainLayoutCss).toMatch(
      /\.train-coast-reveal-water\s*\{[\s\S]*?height:\s*58px;[\s\S]*?linear-gradient\([\s\S]*?var\(--train-palette-water\)[\s\S]*?\);/,
    );
  });

  it("ships transparent exteriors with binary-alpha opaque solids and daylight color range", () => {
    const stats = Object.fromEntries(
      DISTANT_KIT.map((asset) => [asset.id, solidPixelStats(asset)]),
    );

    for (const asset of DISTANT_KIT) {
      const sample = stats[asset.id]!;
      expect(sample.transparent, asset.id).toBeGreaterThan(0);
      expect(sample.opaque, asset.id).toBeGreaterThan(asset.width);
      expect(sample.colors.size, asset.id).toBeGreaterThan(8);
      expect(
        sample.maximumLuminance - sample.minimumLuminance,
        asset.id,
      ).toBeGreaterThan(45);
      expect(sample.maximumLuminance, asset.id).toBeGreaterThan(135);
    }

    expect(stats["terrain-foothills"]!.averageGreen).toBeGreaterThan(
      stats["terrain-foothills"]!.averageRed,
    );
    expect(stats["terrain-alpine"]!.averageBlue).toBeGreaterThan(
      stats["terrain-alpine"]!.averageRed,
    );
    expect(stats["terrain-mesa"]!.averageRed).toBeGreaterThan(
      stats["terrain-mesa"]!.averageGreen,
    );
    expect(stats["terrain-mesa"]!.averageGreen).toBeGreaterThan(
      stats["terrain-mesa"]!.averageBlue,
    );
    expect(stats["coast-shore"]!.averageBlue).toBeGreaterThan(
      stats["coast-shore"]!.averageRed,
    );
  });

  it("keeps solid palette grading ordered without blanket time-of-day filters", () => {
    expect(TRAIN_SCENERY_TIME_GRADES.day.brightness).toBeGreaterThan(
      TRAIN_SCENERY_TIME_GRADES.sunset.brightness,
    );
    expect(TRAIN_SCENERY_TIME_GRADES.sunset.brightness).toBeGreaterThan(
      TRAIN_SCENERY_TIME_GRADES.night.brightness,
    );
    expect(TRAIN_SCENERY_TIME_GRADES.day.saturation).toBeGreaterThan(
      TRAIN_SCENERY_TIME_GRADES.sunset.saturation,
    );
    expect(TRAIN_SCENERY_TIME_GRADES.sunset.saturation).toBeGreaterThan(
      TRAIN_SCENERY_TIME_GRADES.night.saturation,
    );
    expect(TRAIN_SCENERY_TIME_GRADES.day.warmth).toBe(0);
    expect(TRAIN_SCENERY_TIME_GRADES.sunset.warmth).toBe(0);
    expect(TRAIN_SCENERY_TIME_GRADES.night.warmth).toBe(0);
    expect(TRAIN_TIME_PALETTES.sunset.horizonLight).toMatch(
      /^rgba\(255, 174, 101, 0\.58\)$/,
    );
    expect(TRAIN_TIME_PALETTES.sunset.skyTop).not.toBe(
      TRAIN_TIME_PALETTES.sunset.skyBottom,
    );

    for (const token of ["silhouette", "farSurface", "midSurface", "water"] as const) {
      expect(hexLuminance(TRAIN_TIME_PALETTES.day[token]), token).toBeGreaterThan(
        hexLuminance(TRAIN_TIME_PALETTES.sunset[token]),
      );
      expect(
        hexLuminance(TRAIN_TIME_PALETTES.sunset[token]),
        token,
      ).toBeGreaterThan(hexLuminance(TRAIN_TIME_PALETTES.night[token]));
    }
    expect(TRAIN_SCENERY_DEPTH_PROFILES["ultra-far"].contrast).toBeLessThan(
      TRAIN_SCENERY_DEPTH_PROFILES.far.contrast,
    );
    expect(TRAIN_SCENERY_DEPTH_PROFILES.far.contrast).toBeLessThan(
      TRAIN_SCENERY_DEPTH_PROFILES.midground.contrast,
    );
  });

  it("reserves coast spans continuously for DOM-owned transition geometry", () => {
    const first = collectSetPieceTraversal("terrain-kit-traversal");
    expect(collectSetPieceTraversal("terrain-kit-traversal")).toEqual(first);
    expect(new Set(first.map(({ segments }) => segments[0]!.type))).toEqual(
      new Set(["coast-reveal"]),
    );

    for (const { segments } of first) {
      expect(segments.map((segment) => segment.role)).toEqual([
        "entry",
        ...Array.from(
          { length: segments.length - 2 },
          () => "body" as const,
        ),
        "exit",
      ]);
      expect(segments.map((segment) => segment.chunkIndex)).toEqual(
        Array.from(
          { length: segments.length },
          (_, offset) => segments[0]!.chunkIndex + offset,
        ),
      );
    }
  });
});
