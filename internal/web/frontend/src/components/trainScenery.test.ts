/// <reference types="node" />

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import {
  generateRouteChunk,
  TRAIN_PARALLAX_LAYERS,
  TRAIN_REGION_CHUNK_LENGTH,
  TRAIN_ROUTE_CHUNK_WIDTH,
} from "./trainRoute";
import {
  TRAIN_REGION_SCENERY_PROFILES,
  TRAIN_SCENERY_ASSETS,
  TRAIN_SCENERY_BRIDGES,
  TRAIN_SCENERY_BUILDINGS,
  TRAIN_SCENERY_CLOUDS,
  TRAIN_SCENERY_COASTS,
  TRAIN_SCENERY_PROPS,
  TRAIN_SCENERY_TERRAIN,
  TRAIN_SCENERY_VEGETATION,
  trainSceneryPlacementsForChunk,
  trainSceneryScale,
  type TrainRegionSceneryProfile,
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

  it("selects every region-allowed asset deterministically across a long route", () => {
    const firstPass = new Map<string, readonly string[]>();
    const selectedIDs = new Set<string>();

    for (let index = -1800; index <= 1800; index++) {
      const chunk = generateRouteChunk("asset-line", index);
      for (const layer of TRAIN_PARALLAX_LAYERS) {
        const placements = trainSceneryPlacementsForChunk(layer.name, chunk);
        const key = `${layer.name}:${index}`;
        const ids = placements.map((placement) => placement.asset.id);
        firstPass.set(key, ids);
        ids.forEach((id) => selectedIDs.add(id));
        expect(
          trainSceneryPlacementsForChunk(layer.name, chunk).map(
            (placement) => placement.asset.id,
          ),
        ).toEqual(ids);
      }
    }

    expect(firstPass.size).toBe(TRAIN_PARALLAX_LAYERS.length * 3601);
    expect(selectedIDs).toEqual(
      new Set(TRAIN_SCENERY_ASSETS.map((asset) => asset.id)),
    );
  });

  it("enforces regional asset pools, density bounds, and one landmark per region", () => {
    const landmarkChunks = new Map<number, Set<number>>();

    for (let index = -1200; index <= 1200; index++) {
      const chunk = generateRouteChunk("constraint-line", index);
      const profile = TRAIN_REGION_SCENERY_PROFILES[
        chunk.region
      ] as TrainRegionSceneryProfile;
      for (const layer of TRAIN_PARALLAX_LAYERS) {
        const rule = profile.layers[layer.name];
        const placements = trainSceneryPlacementsForChunk(layer.name, chunk);
        expect(placements.length).toBeLessThanOrEqual(rule?.maxPerChunk ?? 0);

        for (const placement of placements) {
          const isAllowedNormal = rule?.assetIds.includes(placement.asset.id);
          const isAllowedLandmark =
            profile.landmark?.layer === layer.name &&
            profile.landmark.assetIds.includes(placement.asset.id);
          expect(
            isAllowedNormal ||
              isAllowedLandmark ||
              placement.setPiece !== null,
            `${chunk.region}/${layer.name}/${placement.asset.id}`,
          ).toBe(true);

          if (placement.landmark) {
            const chunks =
              landmarkChunks.get(chunk.regionIndex) ?? new Set<number>();
            chunks.add(chunk.index);
            landmarkChunks.set(chunk.regionIndex, chunks);
          }
        }
      }
    }

    for (const chunks of landmarkChunks.values()) {
      expect(chunks.size).toBeLessThanOrEqual(1);
    }
  });

  it("keeps placed objects spaced and collision-free across chunk boundaries", () => {
    for (const layer of TRAIN_PARALLAX_LAYERS) {
      const placements = [];
      for (let index = -500; index <= 500; index++) {
        const chunk = generateRouteChunk("spacing-line", index);
        for (const placement of trainSceneryPlacementsForChunk(
          layer.name,
          chunk,
        )) {
          if (placement.minimumSpacingPx <= 0) continue;
          placements.push({
            center:
              index * TRAIN_ROUTE_CHUNK_WIDTH +
              (placement.offsetPercent / 100) * TRAIN_ROUTE_CHUNK_WIDTH,
            ...placement,
          });
        }
      }
      placements.sort((left, right) => left.center - right.center);

      for (let index = 1; index < placements.length; index++) {
        const previous = placements[index - 1]!;
        const current = placements[index]!;
        const distance = current.center - previous.center;
        expect(distance).toBeGreaterThanOrEqual(
          Math.min(previous.minimumSpacingPx, current.minimumSpacingPx),
        );
        expect(distance).toBeGreaterThanOrEqual(
          previous.collisionWidth / 2 + current.collisionWidth / 2,
        );
      }
    }
  });

  it("reserves set-piece layers before placing incompatible small scenery", () => {
    for (let index = -1_200; index <= 1_200; index++) {
      const chunk = generateRouteChunk("reservation-line", index);
      if (!chunk.setPiece) continue;

      for (const layer of TRAIN_PARALLAX_LAYERS) {
        const placements = trainSceneryPlacementsForChunk(layer.name, chunk);
        if (!chunk.setPiece.reservedLayers.includes(layer.name)) continue;
        expect(placements.length).toBeLessThanOrEqual(1);
        expect(
          placements.every(
            (placement) => placement.setPiece?.id === chunk.setPiece?.id,
          ),
        ).toBe(true);
      }
    }
  });

  it("deweights recently used variants throughout every region", () => {
    for (const layer of TRAIN_PARALLAX_LAYERS) {
      for (let regionIndex = -80; regionIndex <= 80; regionIndex++) {
        const ids: string[] = [];
        for (let offset = 0; offset < TRAIN_REGION_CHUNK_LENGTH; offset++) {
          const chunk = generateRouteChunk(
            "cooldown-line",
            regionIndex * TRAIN_REGION_CHUNK_LENGTH + offset,
          );
          ids.push(
            ...trainSceneryPlacementsForChunk(layer.name, chunk).map(
              (placement) =>
                placement.setPiece ? "" : placement.asset.id,
            ),
          );
        }
        const normalIDs = ids.filter(Boolean);
        for (let index = 1; index < normalIDs.length; index++) {
          const profile = TRAIN_REGION_SCENERY_PROFILES[
            generateRouteChunk(
              "cooldown-line",
              regionIndex * TRAIN_REGION_CHUNK_LENGTH,
            ).region
          ] as TrainRegionSceneryProfile;
          const rule = profile.layers[layer.name];
          const pool = rule?.assetIds ?? [];
          if ((rule?.cooldownChunks ?? 0) > 0 && new Set(pool).size > 1) {
            expect(normalIDs[index]).not.toBe(normalIDs[index - 1]);
          }
        }
      }
    }
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
