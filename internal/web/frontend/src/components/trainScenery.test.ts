/// <reference types="node" />

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import {
  generateRouteChunk,
  TRAIN_PARALLAX_LAYERS,
  TRAIN_REGION_CHUNK_LENGTH,
  TRAIN_ROUTE_CHUNK_WIDTH,
  type TrainRegionName,
} from "./trainRoute";
import {
  TRAIN_CLOUD_MAX_ALTITUDE_PERCENT,
  TRAIN_CLOUD_MIN_ALTITUDE_PERCENT,
  TRAIN_CLOUD_MIN_SPACING_PX,
  TRAIN_REGION_NIGHT_LIFE,
  TRAIN_REGION_OPEN_VIEW_TARGET,
  TRAIN_REGION_SCENERY_PROFILES,
  TRAIN_SCENERY_ASSETS,
  TRAIN_SCENERY_BRIDGES,
  TRAIN_SCENERY_BUILDINGS,
  TRAIN_SCENERY_CLOUDS,
  TRAIN_SCENERY_COASTS,
  TRAIN_SCENERY_LANDMARKS,
  TRAIN_SCENERY_PROPS,
  TRAIN_SCENERY_TERRAIN,
  TRAIN_SCENERY_VEGETATION,
  trainCloudPlacementsForChunk,
  trainNightLifeForPlacement,
  trainRegionCompositionForChunk,
  trainSceneryPlacementsForChunk,
  trainSceneryScale,
  type TrainRegionSceneryProfile,
} from "./trainScenery";

function cloudLine(seed: string, firstIndex: number, lastIndex: number) {
  return Array.from({ length: lastIndex - firstIndex + 1 }, (_, offset) =>
    generateRouteChunk(seed, firstIndex + offset),
  )
    .flatMap((chunk) =>
      trainCloudPlacementsForChunk(chunk).map((placement) => ({
        chunkIndex: chunk.index,
        regionIndex: chunk.regionIndex,
        ...placement,
      })),
    )
    .sort((left, right) => left.routePositionPx! - right.routePositionPx!);
}

const REGION_LANDMARK_IDS = {
  forest: "landmark-forest-clearing",
  mountain: "landmark-mountain-lookout",
  town: "landmark-town-church",
  coast: "landmark-coast-lighthouse",
  industrial: "landmark-industrial-gantry",
} as const satisfies Record<TrainRegionName, string>;

function regionChunks(seed: string, regionIndex: number) {
  return Array.from({ length: TRAIN_REGION_CHUNK_LENGTH }, (_, offset) =>
    generateRouteChunk(seed, regionIndex * TRAIN_REGION_CHUNK_LENGTH + offset),
  );
}

function regionSignature(seed: string, firstIndex: number, lastIndex: number) {
  return Array.from({ length: lastIndex - firstIndex + 1 }, (_, offset) =>
    generateRouteChunk(seed, firstIndex + offset),
  ).map((chunk) => ({
    index: chunk.index,
    region: chunk.region,
    composition: trainRegionCompositionForChunk(chunk),
    layers: TRAIN_PARALLAX_LAYERS.map((layer) =>
      trainSceneryPlacementsForChunk(layer.name, chunk).map(
        (placement) =>
          `${placement.asset.id}:${placement.landmark}:${placement.offsetPercent.toFixed(3)}`,
      ),
    ),
  }));
}

describe("train scenery asset kit", () => {
  it("records the complete reusable kit and rendering metadata", () => {
    expect(TRAIN_SCENERY_CLOUDS).toHaveLength(3);
    expect(TRAIN_SCENERY_TERRAIN).toHaveLength(3);
    expect(TRAIN_SCENERY_VEGETATION).toHaveLength(6);
    expect(TRAIN_SCENERY_BUILDINGS).toHaveLength(6);
    expect(TRAIN_SCENERY_LANDMARKS).toHaveLength(5);
    expect(TRAIN_SCENERY_BRIDGES).toHaveLength(1);
    expect(TRAIN_SCENERY_COASTS).toHaveLength(1);
    expect(TRAIN_SCENERY_PROPS).toHaveLength(8);
    expect(TRAIN_SCENERY_ASSETS).toHaveLength(33);
    expect(new Set(TRAIN_SCENERY_ASSETS.map((asset) => asset.id)).size).toBe(
      TRAIN_SCENERY_ASSETS.length,
    );

    for (const asset of TRAIN_SCENERY_ASSETS) {
      expect(asset.src).toBeTruthy();
      expect(asset.anchor).toMatch(/^(center|bottom-center)$/);
      expect(asset.width).toBeGreaterThan(0);
      expect(asset.height).toBeGreaterThan(0);
      expect(asset.collisionWidth).toBeGreaterThan(0);
      expect(asset.collisionWidth).toBeLessThanOrEqual(asset.width);
      expect(asset.safeScale[0]).toBeGreaterThan(0);
      expect(asset.safeScale[1]).toBeGreaterThanOrEqual(asset.safeScale[0]);
      expect(asset.dayNightTreatment).toMatch(
        /^(atmospheric-filter|emissive-windows|solid-palette-grade|water-reflection)$/,
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
            isAllowedNormal || isAllowedLandmark || placement.setPiece !== null,
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
              (placement) => (placement.setPiece ? "" : placement.asset.id),
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

  it("gives every region a distinct daytime profile and landmark vocabulary", () => {
    expect(
      TRAIN_REGION_SCENERY_PROFILES.forest.layers.midground.assetIds,
    ).toEqual([
      "vegetation-conifer-tall",
      "vegetation-conifer-squat",
      "vegetation-deciduous",
      "vegetation-hedgerow",
    ]);
    expect(
      TRAIN_REGION_SCENERY_PROFILES.mountain.layers.midground.assetIds,
    ).toEqual([
      "vegetation-conifer-tall",
      "vegetation-conifer-squat",
      "vegetation-coastal-pine",
    ]);
    expect(
      TRAIN_REGION_SCENERY_PROFILES.town.layers.midground.assetIds,
    ).toEqual([
      "building-rowhouse",
      "building-apartments",
      "building-cottage",
      "vegetation-deciduous",
      "vegetation-hedgerow",
    ]);
    expect(
      TRAIN_REGION_SCENERY_PROFILES.coast.layers.midground.assetIds,
    ).toEqual([
      "vegetation-coastal-pine",
      "vegetation-reeds",
      "vegetation-hedgerow",
      "building-cottage",
    ]);
    expect(
      TRAIN_REGION_SCENERY_PROFILES.industrial.layers.midground.assetIds,
    ).toEqual([
      "building-workshop",
      "building-warehouse",
      "building-water-tower",
    ]);

    for (const [region, landmarkID] of Object.entries(REGION_LANDMARK_IDS) as [
      TrainRegionName,
      string,
    ][]) {
      const profile = TRAIN_REGION_SCENERY_PROFILES[region];
      expect(profile.landmark).toMatchObject({
        layer: "midground",
        assetIds: [landmarkID],
        maxPerRegion: 1,
        spanChunks: 2,
        edgeClearanceChunks: 1,
      });
      expect(profile.landmark.probability).toBeGreaterThan(0.45);
      expect(profile.landmark.probability).toBeLessThan(0.65);
    }
  });

  it("keeps landmarks rare, region-owned, and capped at one major asset per region", () => {
    const regionCounts = new Map<TrainRegionName, number>();
    const landmarkCounts = new Map<TrainRegionName, number>();

    for (const seed of [
      "regional-day-a",
      "regional-day-b",
      "regional-day-c",
      "regional-day-d",
      "regional-day-e",
      "regional-day-f",
    ]) {
      for (let regionIndex = -48; regionIndex <= 48; regionIndex++) {
        const chunks = regionChunks(seed, regionIndex);
        const region = chunks[0]!.region;
        regionCounts.set(region, (regionCounts.get(region) ?? 0) + 1);
        const landmarks = chunks.flatMap((chunk) =>
          trainSceneryPlacementsForChunk("midground", chunk).filter(
            (placement) => placement.landmark,
          ),
        );
        expect(
          landmarks.length,
          `${seed}/${regionIndex}/${region}`,
        ).toBeLessThanOrEqual(1);
        for (const placement of landmarks) {
          expect(placement.asset.id).toBe(REGION_LANDMARK_IDS[region]);
        }
        if (landmarks.length === 1) {
          landmarkCounts.set(region, (landmarkCounts.get(region) ?? 0) + 1);
        }
      }
    }

    for (const region of Object.keys(
      REGION_LANDMARK_IDS,
    ) as TrainRegionName[]) {
      const regions = regionCounts.get(region) ?? 0;
      const landmarks = landmarkCounts.get(region) ?? 0;
      expect(regions, region).toBeGreaterThan(40);
      expect(landmarks, region).toBeGreaterThan(regions * 0.12);
      expect(landmarks, region).toBeLessThan(regions * 0.75);
    }
  });

  it("reserves landmark spans before filler and alternates them with open views", () => {
    for (const seed of ["composition-a", "composition-b", "composition-c"]) {
      for (let regionIndex = -60; regionIndex <= 60; regionIndex++) {
        const chunks = regionChunks(seed, regionIndex);
        const compositions = chunks.map((chunk) =>
          trainRegionCompositionForChunk(chunk),
        );
        const openOffsets = compositions.flatMap((composition, offset) =>
          composition === "open" ? [offset] : [],
        );
        expect(
          openOffsets.length,
          `${seed}/${regionIndex}`,
        ).toBeGreaterThanOrEqual(1);
        expect(
          openOffsets.length,
          `${seed}/${regionIndex}`,
        ).toBeLessThanOrEqual(TRAIN_REGION_OPEN_VIEW_TARGET);
        for (let index = 1; index < openOffsets.length; index++) {
          expect(
            openOffsets[index]! - openOffsets[index - 1]!,
          ).toBeGreaterThanOrEqual(2);
        }

        for (const chunk of chunks) {
          const composition = trainRegionCompositionForChunk(chunk);
          const midground = trainSceneryPlacementsForChunk("midground", chunk);
          const near = trainSceneryPlacementsForChunk("near", chunk);
          if (composition === "open") {
            expect(midground, `${seed}/${chunk.index}/midground`).toEqual([]);
          }
          if (composition === "landmark") {
            expect(chunk.setPiece).toBeNull();
            expect(near, `${seed}/${chunk.index}/near`).toEqual([]);
            expect(midground.length).toBeLessThanOrEqual(1);
            expect(midground.every((placement) => placement.landmark)).toBe(
              true,
            );
          }
        }

        const landmarkOffsets = compositions.flatMap((composition, offset) =>
          composition === "landmark" ? [offset] : [],
        );
        if (landmarkOffsets.length > 0) {
          expect(landmarkOffsets).toHaveLength(2);
          const first = landmarkOffsets[0]!;
          const last = landmarkOffsets.at(-1)!;
          for (
            let offset = Math.max(0, first - 1);
            offset <= Math.min(chunks.length - 1, last + 1);
            offset++
          ) {
            expect(chunks[offset]!.setPiece).toBeNull();
          }
        }
      }
    }
  });

  it("keeps composition deterministic across calls and varied across seeds", () => {
    const first = regionSignature("day-composition-a", -240, 240);
    expect(regionSignature("day-composition-a", -240, 240)).toEqual(first);
    expect(regionSignature("day-composition-b", -240, 240)).not.toEqual(first);
  });

  it("builds sparse deterministic region-owned nighttime life catalogues", () => {
    const signature = (seed: string) =>
      Array.from({ length: 2401 }, (_, offset) =>
        generateRouteChunk(seed, offset - 1200),
      ).flatMap((chunk) =>
        trainSceneryPlacementsForChunk("midground", chunk).flatMap(
          (placement, ordinal) => {
            const plan = trainNightLifeForPlacement(
              chunk,
              placement,
              ordinal,
            );
            return plan
              ? [
                  {
                    chunk: chunk.index,
                    asset: placement.asset.id,
                    ...plan,
                  },
                ]
              : [];
          },
        ),
      );

    const first = signature("night-life-catalogue-a");
    expect(signature("night-life-catalogue-a")).toEqual(first);
    expect(signature("night-life-catalogue-b")).not.toEqual(first);
    expect(new Set(first.map((plan) => plan.region))).toEqual(
      new Set<TrainRegionName>([
        "forest",
        "mountain",
        "town",
        "coast",
        "industrial",
      ]),
    );

    for (const plan of first) {
      const rule = TRAIN_REGION_NIGHT_LIFE[plan.region];
      expect(plan.kind).toBe(rule.kind);
      expect(
        rule.owners.some((owner) => owner.assetId === plan.ownerAssetId),
      ).toBe(true);
      expect(plan.asset).toBe(plan.ownerAssetId);
      expect(plan.intensity).toBeGreaterThanOrEqual(0.62);
      expect(plan.intensity).toBeLessThan(0.88);
      expect(plan.points.length).toBeLessThanOrEqual(6);
      expect(plan.pairedReflection).toBe(plan.region === "coast");
      if (plan.kind === "forest-fireflies") {
        expect(plan.points.length).toBeGreaterThanOrEqual(4);
        expect(
          plan.points.every(
            (point) =>
              point.xPercent >= 20 &&
              point.xPercent <= 82 &&
              point.yPercent >= 24 &&
              point.yPercent <= 66,
          ),
        ).toBe(true);
      } else {
        expect(plan.points).toEqual([]);
      }
    }
  });

  it("keeps occupied windows varied, rare, and night-life DOM strictly bounded", () => {
    const eligibleByRegion = new Map<TrainRegionName, number>();
    const activeByRegion = new Map<TrainRegionName, number>();
    const occupancies = new Set<string>();
    let maximumPlansPerChunk = 0;
    let maximumDetailNodesPerChunk = 0;

    for (const seed of [
      "night-life-bounds-a",
      "night-life-bounds-b",
      "night-life-bounds-c",
    ]) {
      for (let index = -1500; index <= 1500; index++) {
        const chunk = generateRouteChunk(seed, index);
        const placements = trainSceneryPlacementsForChunk("midground", chunk);
        const ownerIDs = new Set<string>(
          TRAIN_REGION_NIGHT_LIFE[chunk.region].owners.map(
            (owner) => owner.assetId,
          ),
        );
        const eligible = placements.filter((placement) =>
          ownerIDs.has(placement.asset.id),
        );
        eligibleByRegion.set(
          chunk.region,
          (eligibleByRegion.get(chunk.region) ?? 0) + eligible.length,
        );
        const plans = placements.flatMap((placement, ordinal) => {
          const plan = trainNightLifeForPlacement(chunk, placement, ordinal);
          return plan ? [plan] : [];
        });
        activeByRegion.set(
          chunk.region,
          (activeByRegion.get(chunk.region) ?? 0) + plans.length,
        );
        plans
          .filter(
            (plan) =>
              plan.region === "town" || plan.region === "industrial",
          )
          .forEach((plan) => occupancies.add(plan.occupancy));
        maximumPlansPerChunk = Math.max(maximumPlansPerChunk, plans.length);
        maximumDetailNodesPerChunk = Math.max(
          maximumDetailNodesPerChunk,
          plans.reduce(
            (total, plan) =>
              total +
              1 +
              (plan.kind === "forest-fireflies"
                ? plan.points.length
                : plan.kind === "coast-lighthouse-beacon"
                  ? 3
                  : 2),
            0,
          ),
        );
      }
    }

    for (const region of Object.keys(
      TRAIN_REGION_NIGHT_LIFE,
    ) as TrainRegionName[]) {
      const eligible = eligibleByRegion.get(region) ?? 0;
      const active = activeByRegion.get(region) ?? 0;
      expect(eligible, region).toBeGreaterThan(10);
      expect(active, region).toBeGreaterThan(0);
      expect(active, region).toBeLessThan(eligible);
    }
    expect(occupancies).toEqual(new Set(["left", "center", "right"]));
    expect(maximumPlansPerChunk).toBeLessThanOrEqual(2);
    expect(maximumDetailNodesPerChunk).toBeLessThanOrEqual(8);
  });

  it("does not increase the established long-route scenery node bound", () => {
    let maximumNodes = 0;
    for (const seed of ["bounded-day-a", "bounded-day-b", "bounded-day-c"]) {
      for (let index = -900; index <= 900; index++) {
        const chunk = generateRouteChunk(seed, index);
        const nodeCount = TRAIN_PARALLAX_LAYERS.reduce(
          (total, layer) =>
            total +
            trainSceneryPlacementsForChunk(layer.name, chunk).reduce(
              (layerTotal, placement) =>
                layerTotal + 1 + (placement.asset.emissive ? 1 : 0),
              0,
            ),
          0,
        );
        maximumNodes = Math.max(maximumNodes, nodeCount);
        expect(nodeCount, `${seed}/${index}`).toBeLessThanOrEqual(9);
      }
    }
    expect(maximumNodes).toBeGreaterThanOrEqual(7);
  });

  it("builds deterministic region-scale cloud plans that vary by seed", () => {
    const first = cloudLine("natural-clouds-a", -90, 90);
    const repeated = cloudLine("natural-clouds-a", -90, 90);
    const secondSeed = cloudLine("natural-clouds-b", -90, 90);

    expect(repeated).toEqual(first);
    expect(secondSeed).not.toEqual(first);
    expect(new Set(first.map((placement) => placement.asset.id))).toEqual(
      new Set(["cloud-cumulus", "cloud-wisp", "cloud-storm"]),
    );
    expect(new Set(first.map((placement) => placement.cloudPattern))).toEqual(
      new Set(["open", "grouped", "scattered"]),
    );
  });

  it("varies cloud altitude, scale, spacing, density, gaps, and loose groups across seeds", () => {
    const lines = [
      "cirrus-line",
      "harbor-weather",
      "highland-front",
      "summer-local",
      "winter-express",
    ].map((seed) => cloudLine(seed, -360, 360));
    const samples = lines.flat();
    const altitudes = samples.map((placement) => placement.altitudePercent!);
    const scales = samples.map((placement) => placement.scale);
    const offsets = samples.map((placement) => placement.offsetPercent);
    const spacings = lines.flatMap((line) =>
      line
        .slice(1)
        .map(
          (placement, index) =>
            placement.routePositionPx! - line[index]!.routePositionPx!,
        ),
    );
    const grouped = samples.filter((placement) => placement.cloudGroup);

    expect(Math.min(...altitudes)).toBeGreaterThanOrEqual(
      TRAIN_CLOUD_MIN_ALTITUDE_PERCENT,
    );
    expect(Math.max(...altitudes)).toBeLessThanOrEqual(
      TRAIN_CLOUD_MAX_ALTITUDE_PERCENT,
    );
    expect(Math.max(...altitudes) - Math.min(...altitudes)).toBeGreaterThan(28);
    expect(
      new Set(altitudes.map((value) => value.toFixed(1))).size,
    ).toBeGreaterThan(100);
    expect(Math.max(...scales) - Math.min(...scales)).toBeGreaterThan(0.3);
    expect(new Set(offsets.map((value) => Math.floor(value / 10))).size).toBe(
      10,
    );
    expect(Math.min(...spacings)).toBeGreaterThanOrEqual(
      TRAIN_CLOUD_MIN_SPACING_PX,
    );
    expect(Math.max(...spacings)).toBeGreaterThan(700);
    expect(grouped.length).toBeGreaterThan(100);
    expect(
      new Set(grouped.map((placement) => placement.cloudGroup)).size,
    ).toBeGreaterThan(30);
  });

  it("keeps clouds collision-free and variant-cooled across chunks and regions", () => {
    for (const seed of ["boundary-a", "boundary-b", "boundary-c"]) {
      const samples = cloudLine(seed, -540, 540);
      for (let index = 1; index < samples.length; index++) {
        const previous = samples[index - 1]!;
        const current = samples[index]!;
        const spacing = current.routePositionPx! - previous.routePositionPx!;
        expect(spacing).toBeGreaterThanOrEqual(TRAIN_CLOUD_MIN_SPACING_PX);
        expect(spacing).toBeGreaterThanOrEqual(
          previous.collisionWidth / 2 + current.collisionWidth / 2,
        );
        expect(current.asset.id).not.toBe(previous.asset.id);
      }

      for (const sample of samples) {
        expect(
          Math.floor(sample.routePositionPx! / TRAIN_ROUTE_CHUNK_WIDTH),
        ).toBe(sample.chunkIndex);
        expect(sample.offsetPercent).toBeGreaterThanOrEqual(0);
        expect(sample.offsetPercent).toBeLessThan(100);
      }
    }
  });
});
