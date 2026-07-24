/// <reference types="node" />

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import { TRAIN_PARALLAX_LAYERS } from "./trainRoute";
import {
  TRAIN_SCENERY_ASSETS,
  TRAIN_SCENERY_BRIDGES,
  TRAIN_SCENERY_BUILDINGS,
  TRAIN_SCENERY_CLOUDS,
  TRAIN_SCENERY_COASTS,
  TRAIN_SCENERY_PROPS,
  TRAIN_SCENERY_TERRAIN,
  TRAIN_SCENERY_VEGETATION,
  trainSceneryAssetsForChunk,
  trainSceneryScale,
} from "./trainScenery";

describe("train scenery asset kit", () => {
  it("records the complete reusable kit and rendering metadata", () => {
    expect(TRAIN_SCENERY_CLOUDS).toHaveLength(3);
    expect(TRAIN_SCENERY_TERRAIN).toHaveLength(3);
    expect(TRAIN_SCENERY_VEGETATION).toHaveLength(6);
    expect(TRAIN_SCENERY_BUILDINGS).toHaveLength(6);
    expect(TRAIN_SCENERY_BRIDGES).toHaveLength(1);
    expect(TRAIN_SCENERY_COASTS).toHaveLength(1);
    expect(TRAIN_SCENERY_PROPS).toHaveLength(3);
    expect(TRAIN_SCENERY_ASSETS).toHaveLength(23);
    expect(new Set(TRAIN_SCENERY_ASSETS.map((asset) => asset.id)).size).toBe(
      TRAIN_SCENERY_ASSETS.length,
    );

    for (const asset of TRAIN_SCENERY_ASSETS) {
      expect(asset.src).toBeTruthy();
      expect(asset.anchor).toMatch(/^(center|bottom-center)$/);
      expect(asset.width).toBeGreaterThan(0);
      expect(asset.height).toBeGreaterThan(0);
      expect(asset.safeScale[0]).toBeGreaterThan(0);
      expect(asset.safeScale[1]).toBeGreaterThanOrEqual(asset.safeScale[0]);
      expect(asset.dayNightTreatment).toMatch(
        /^(atmospheric-filter|emissive-windows|water-reflection)$/,
      );
    }
  });

  it("ships scale-consistent RGBA PNG files at their manifest dimensions", () => {
    for (const asset of TRAIN_SCENERY_ASSETS) {
      const assetPath = resolve(
        process.cwd(),
        "src/assets/train-theme/sprites/scenery",
        asset.fileName,
      );
      const png = readFileSync(assetPath);

      expect(png.subarray(0, 8).toString("hex")).toBe("89504e470d0a1a0a");
      expect(png.readUInt32BE(16)).toBe(asset.width);
      expect(png.readUInt32BE(20)).toBe(asset.height);
      expect(png[25]).toBe(6);
    }
  });

  it("selects every asset deterministically across a long chunk sample", () => {
    const firstPass = new Map<string, readonly string[]>();
    const selectedIDs = new Set<string>();

    for (let index = -180; index <= 180; index++) {
      const variant = ((index % 5) + 5) % 5;
      for (const layer of TRAIN_PARALLAX_LAYERS) {
        const assets = trainSceneryAssetsForChunk(
          layer.name,
          index,
          variant,
        );
        const key = `${layer.name}:${index}:${variant}`;
        const ids = assets.map((asset) => asset.id);
        firstPass.set(key, ids);
        ids.forEach((id) => selectedIDs.add(id));
        expect(
          trainSceneryAssetsForChunk(layer.name, index, variant).map(
            (asset) => asset.id,
          ),
        ).toEqual(ids);
      }
    }

    expect(firstPass.size).toBe(TRAIN_PARALLAX_LAYERS.length * 361);
    expect(selectedIDs).toEqual(
      new Set(TRAIN_SCENERY_ASSETS.map((asset) => asset.id)),
    );
  });

  it("keeps deterministic variant scaling inside each asset safe range", () => {
    for (const asset of TRAIN_SCENERY_ASSETS) {
      const scales = [0, 1, 2, 3, 4].map((variant) =>
        trainSceneryScale(asset, variant),
      );
      expect(scales[0]).toBe(asset.safeScale[0]);
      expect(scales[4]).toBe(asset.safeScale[1]);
      expect(scales).toEqual([...scales].sort((left, right) => left - right));
    }
  });
});
