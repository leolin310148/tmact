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
  TRAIN_COAST_MIN_LANDMARK_SPACING_PX,
  TRAIN_FOREST_MOUNTAIN_MIN_REPEAT_DISTANCE_PX,
  TRAIN_NIGHT_LIFE_MAX_INTENSITY,
  TRAIN_NIGHT_LIFE_MIN_INTENSITY,
  TRAIN_REGION_NIGHT_LIFE,
  TRAIN_REGION_OPEN_VIEW_TARGET,
  TRAIN_REGION_SCENERY_PROFILES,
  TRAIN_SCENERY_ASSETS,
  TRAIN_SCENERY_BRIDGES,
  TRAIN_SCENERY_BUILDINGS,
  TRAIN_SCENERY_CLOUDS,
  TRAIN_SCENERY_COASTS,
  TRAIN_SCENERY_DEPTH_GRAMMAR,
  TRAIN_SCENERY_LANDMARKS,
  TRAIN_SCENERY_PROPS,
  TRAIN_SCENERY_TERRAIN,
  TRAIN_SCENERY_VEGETATION,
  TRAIN_TOWN_INDUSTRIAL_MIN_REPEAT_DISTANCE_PX,
  trainCloudPlacementsForChunk,
  trainCoastSceneryBeatForChunk,
  trainCoastTransitionFamilyForRegion,
  trainForestMountainSceneryBeatForChunk,
  trainNightLifeForPlacement,
  trainRegionCompositionForChunk,
  trainSceneryPlacementsForChunk,
  trainSceneryAssetScale,
  trainSceneryScale,
  trainTownIndustrialSceneryBeatForChunk,
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
      expect(asset.groundInsetPx).toBeGreaterThanOrEqual(0);
      expect(asset.groundInsetPx).toBeLessThanOrEqual(3);
      if (asset.anchor === "center") {
        expect(asset.groundInsetPx).toBe(0);
      }
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
      new Set(
        TRAIN_SCENERY_ASSETS.filter(
          (asset) => asset.id !== "bridge-truss",
        ).map((asset) => asset.id),
      ),
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

  it("keeps ordinary placement overlap inside each depth grammar bound", () => {
    for (const layer of TRAIN_PARALLAX_LAYERS) {
      const placements = [];
      for (let index = -600; index <= 600; index++) {
        const chunk = generateRouteChunk("overlap-grammar-line", index);
        placements.push(
          ...trainSceneryPlacementsForChunk(layer.name, chunk)
            .filter((placement) => placement.setPiece === null)
            .map((placement) => ({
              center:
                index * TRAIN_ROUTE_CHUNK_WIDTH +
                (placement.offsetPercent / 100) * TRAIN_ROUTE_CHUNK_WIDTH,
              ...placement,
            })),
        );
      }
      placements.sort((left, right) => left.center - right.center);

      for (let index = 1; index < placements.length; index++) {
        const previous = placements[index - 1]!;
        const current = placements[index]!;
        const overlapPx = Math.max(
          0,
          previous.collisionWidth / 2 +
            current.collisionWidth / 2 -
            (current.center - previous.center),
        );
        const overlapRatio =
          overlapPx /
          Math.max(
            1,
            Math.min(previous.collisionWidth, current.collisionWidth),
          );
        expect(overlapRatio, `${layer.name}/${index}`).toBeLessThanOrEqual(
          Math.max(
            previous.maximumCollisionOverlapRatio,
            current.maximumCollisionOverlapRatio,
          ),
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

  it("keeps bridges DOM-owned and composes coast raster placements from the shared variant", () => {
    const compositions = new Map<
      string,
      {
        variant: number;
        roles: string[];
        scales: number[];
        offsets: number[];
      }
    >();
    const bridgeVariants = new Set<number>();

    for (let index = -3_600; index <= 3_600; index++) {
      const chunk = generateRouteChunk("set-piece-compositions", index);
      const setPiece = chunk.setPiece;
      if (
        !setPiece ||
        (setPiece.type !== "bridge" && setPiece.type !== "coast-reveal")
      ) {
        continue;
      }
      const placements = trainSceneryPlacementsForChunk(
        setPiece.renderLayer,
        chunk,
      );
      if (setPiece.type === "bridge") {
        expect(placements).toHaveLength(0);
        bridgeVariants.add(setPiece.visualVariant);
        continue;
      }
      expect(placements).toHaveLength(1);
      const placement = placements[0]!;
      const composition = compositions.get(setPiece.id) ?? {
        variant: setPiece.visualVariant,
        roles: [],
        scales: [],
        offsets: [],
      };
      expect(setPiece.visualVariant).toBe(composition.variant);
      expect(placement.setPiece?.visualVariant).toBe(composition.variant);
      composition.roles.push(setPiece.role);
      composition.scales.push(placement.scale);
      composition.offsets.push(placement.offsetPercent);
      compositions.set(setPiece.id, composition);
    }

    expect(bridgeVariants).toEqual(new Set([0, 1]));
    const complete = [...compositions.values()].filter(
      (composition) => composition.roles.at(0) === "entry" &&
        composition.roles.at(-1) === "exit",
    );
    expect(new Set(complete.map((composition) => composition.variant))).toEqual(
      new Set([0, 1]),
    );
    for (const composition of complete) {
      expect(new Set(composition.scales)).toHaveProperty("size", 1);
      expect(composition.offsets).toEqual(
        composition.variant === 0
          ? composition.roles.map(() => 50)
          : composition.roles.map((role) =>
              role === "entry" ? 62 : role === "exit" ? 38 : 50,
            ),
      );
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

  it("combines safe asset scaling with deterministic monotonic depth scaling", () => {
    for (const asset of TRAIN_SCENERY_ASSETS) {
      const assetScales = [0, 1, 2, 3, 4].map((variant) =>
        trainSceneryAssetScale(asset, variant),
      );
      const scales = [0, 1, 2, 3, 4].map((variant) =>
        trainSceneryScale(asset, variant, asset.layer),
      );
      expect(assetScales[0]).toBe(asset.safeScale[0]);
      expect(assetScales[4]).toBe(asset.safeScale[1]);
      expect(assetScales).toEqual(
        [...assetScales].sort((left, right) => left - right),
      );
      expect(scales).toEqual(
        assetScales.map(
          (scale) =>
            scale * TRAIN_SCENERY_DEPTH_GRAMMAR[asset.layer].scaleMultiplier,
        ),
      );
    }

    for (const terrain of TRAIN_SCENERY_TERRAIN) {
      for (let variant = 0; variant < 5; variant++) {
        expect(trainSceneryScale(terrain, variant, "ultra-far")).toBeLessThan(
          trainSceneryScale(terrain, variant, "far"),
        );
      }
    }
  });

  it("applies measurable depth, detail, anchor, and overlap grammar to every placement", () => {
    const solidLayers = ["ultra-far", "far", "midground", "near"] as const;
    const multipliers = solidLayers.map(
      (layer) => TRAIN_SCENERY_DEPTH_GRAMMAR[layer].scaleMultiplier,
    );
    const detailBudgets = solidLayers.map(
      (layer) => TRAIN_SCENERY_DEPTH_GRAMMAR[layer].detailBudget,
    );
    const contrast = solidLayers.map(
      (layer) => TRAIN_SCENERY_DEPTH_GRAMMAR[layer].contrast,
    );

    expect(multipliers).toEqual(
      [...multipliers].sort((left, right) => left - right),
    );
    expect(detailBudgets).toEqual(
      [...detailBudgets].sort((left, right) => left - right),
    );
    expect(contrast).toEqual(
      [...contrast].sort((left, right) => left - right),
    );
    expect(new Set(multipliers).size).toBe(solidLayers.length);
    expect(new Set(detailBudgets).size).toBe(solidLayers.length);
    expect(new Set(contrast).size).toBe(solidLayers.length);

    let placementCount = 0;
    for (let index = -900; index <= 900; index++) {
      const chunk = generateRouteChunk("depth-grammar-line", index);
      for (const layer of TRAIN_PARALLAX_LAYERS) {
        const grammar = TRAIN_SCENERY_DEPTH_GRAMMAR[layer.name];
        for (const placement of trainSceneryPlacementsForChunk(
          layer.name,
          chunk,
        )) {
          placementCount++;
          expect(placement.assetScale).toBeGreaterThanOrEqual(
            placement.asset.safeScale[0],
          );
          expect(placement.assetScale).toBeLessThanOrEqual(
            placement.asset.safeScale[1],
          );
          expect(placement.depthScaleMultiplier).toBe(
            placement.setPiece ? 1 : grammar.scaleMultiplier,
          );
          expect(placement.scale).toBeCloseTo(
            placement.assetScale * placement.depthScaleMultiplier,
            10,
          );
          expect(placement.detailBudget).toBe(grammar.detailBudget);
          expect(placement.groundInsetPx).toBeCloseTo(
            placement.asset.groundInsetPx * placement.scale,
            10,
          );
          expect(placement.groundInsetPx).toBeLessThanOrEqual(
            grammar.maximumGroundInsetPx,
          );
          expect(placement.maximumCollisionOverlapRatio).toBe(
            grammar.maximumCollisionOverlapRatio,
          );
          expect(placement.collisionWidth).toBeCloseTo(
            placement.asset.collisionWidth * placement.scale,
            10,
          );
        }
      }
    }
    expect(placementCount).toBeGreaterThan(4_000);
    expect(TRAIN_SCENERY_DEPTH_GRAMMAR.near.scaleMultiplier).toBeGreaterThan(
      TRAIN_SCENERY_DEPTH_GRAMMAR.far.scaleMultiplier,
    );
    expect(TRAIN_SCENERY_DEPTH_GRAMMAR.near.brightness).toBeLessThan(
      TRAIN_SCENERY_DEPTH_GRAMMAR.midground.brightness,
    );
    expect(
      TRAIN_SCENERY_DEPTH_GRAMMAR.near.maximumCollisionOverlapRatio,
    ).toBe(0);
  });

  it("gives every region a distinct daytime profile and landmark vocabulary", () => {
    expect(
      TRAIN_REGION_SCENERY_PROFILES.forest.layers.midground.assetIds,
    ).toEqual([
      "vegetation-conifer-tall",
      "vegetation-conifer-squat",
      "vegetation-deciduous",
      "vegetation-hedgerow",
      "vegetation-reeds",
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

  it("builds distinct nine-beat forest and mountain identities across seeds", () => {
    const roles = {
      forest: new Set<string>(),
      mountain: new Set<string>(),
    };
    const silhouettes = {
      forest: new Set<string>(),
      mountain: new Set<string>(),
    };
    const variants = {
      forest: new Set<number>(),
      mountain: new Set<number>(),
    };
    let sampledRegions = 0;

    for (const seed of [
      "regional-rhythm-cedar",
      "regional-rhythm-granite",
      "regional-rhythm-river",
      "regional-rhythm-summit",
    ]) {
      for (let regionIndex = -90; regionIndex <= 90; regionIndex++) {
        const chunks = regionChunks(seed, regionIndex);
        const region = chunks[0]!.region;
        if (region !== "forest" && region !== "mountain") continue;
        sampledRegions++;
        const beats = chunks.map((chunk) =>
          trainForestMountainSceneryBeatForChunk(chunk),
        );
        expect(beats.every(Boolean)).toBe(true);
        expect(beats[0]).toMatchObject({
          region,
          transition: "entry",
          silhouetteFamily: "mixed-grove",
        });
        expect(beats.at(-1)).toMatchObject({
          region,
          transition: "exit",
          silhouetteFamily: "mixed-grove",
        });
        expect(beats.slice(1, -1).every((beat) => beat!.transition === "interior")).toBe(
          true,
        );
        for (const beat of beats) {
          roles[region].add(beat!.role);
          silhouettes[region].add(beat!.silhouetteFamily);
          variants[region].add(beat!.templateVariant);
        }
      }
    }

    expect(sampledRegions).toBeGreaterThan(200);
    expect(roles.forest).toEqual(
      new Set([
        "forest-transition-grove",
        "forest-canopy-cluster",
        "forest-undergrowth",
        "forest-stream",
        "forest-clearing",
        "forest-fence-line",
        "forest-landmark-approach",
      ]),
    );
    expect(roles.mountain).toEqual(
      new Set([
        "mountain-transition-pines",
        "mountain-layered-ridge",
        "mountain-cliff",
        "mountain-rock-field",
        "mountain-alpine-scrub",
        "mountain-open-vista",
        "mountain-lookout-approach",
      ]),
    );
    expect(silhouettes.forest.size).toBeGreaterThanOrEqual(6);
    expect(silhouettes.mountain.size).toBeGreaterThanOrEqual(6);
    expect(variants.forest).toEqual(new Set([0, 1, 2]));
    expect(variants.mountain).toEqual(new Set([0, 1, 2]));
  });

  it("turns regional rhythm into pools, density gaps, and non-mirrored clusters", () => {
    const countsByRole = new Map<string, number[]>();
    const idsByRole = new Map<string, Set<string>>();
    let asymmetricClusters = 0;

    for (const seed of [
      "ordinary-pools-a",
      "ordinary-pools-b",
      "ordinary-pools-c",
    ]) {
      for (let regionIndex = -80; regionIndex <= 80; regionIndex++) {
        for (const chunk of regionChunks(seed, regionIndex)) {
          if (chunk.region !== "forest" && chunk.region !== "mountain") {
            continue;
          }
          const beat = trainForestMountainSceneryBeatForChunk(chunk)!;
          const midground = trainSceneryPlacementsForChunk(
            "midground",
            chunk,
            { includeSetPieces: false },
          ).filter((placement) => !placement.landmark);
          const counts = countsByRole.get(beat.role) ?? [];
          if (
            trainRegionCompositionForChunk(chunk) === "dense" ||
            beat.densityClass === "gap"
          ) {
            counts.push(midground.length);
          }
          countsByRole.set(beat.role, counts);
          const ids = idsByRole.get(beat.role) ?? new Set<string>();
          midground.forEach((placement) => {
            ids.add(placement.asset.id);
            expect(placement.regionalRole).toBe(beat.role);
            expect(placement.silhouetteFamily).toBe(beat.silhouetteFamily);
            expect(placement.regionalTemplateVariant).toBe(
              beat.templateVariant,
            );
          });
          idsByRole.set(beat.role, ids);
          if (midground.length === 2) {
            asymmetricClusters++;
            expect(
              Math.abs(
                midground[0]!.offsetPercent +
                  midground[1]!.offsetPercent -
                  100,
              ),
            ).toBeGreaterThan(0.01);
          }
        }
      }
    }

    const average = (role: string) => {
      const values = countsByRole.get(role)!;
      return values.reduce((total, value) => total + value, 0) / values.length;
    };
    expect(average("forest-canopy-cluster")).toBeGreaterThan(1.1);
    expect(average("forest-undergrowth")).toBeGreaterThan(1.1);
    expect(average("forest-clearing")).toBe(0);
    expect(average("mountain-open-vista")).toBe(0);
    expect(average("mountain-layered-ridge")).toBeGreaterThan(0.5);
    expect(asymmetricClusters).toBeGreaterThan(100);
    expect(idsByRole.get("forest-stream")).toEqual(
      new Set(["vegetation-reeds", "vegetation-hedgerow"]),
    );
    expect(
      idsByRole.get("mountain-layered-ridge"),
    ).toEqual(
      new Set(["vegetation-coastal-pine", "vegetation-conifer-tall"]),
    );
  });

  it("spaces repeated silhouettes and preserves forest-mountain edge grammar", () => {
    let forestToMountain = 0;
    let mountainToForest = 0;
    const repeatedDistances: number[] = [];

    for (const seed of [
      "transition-forest-a",
      "transition-mountain-b",
      "transition-highland-c",
    ]) {
      for (let regionIndex = -160; regionIndex < 160; regionIndex++) {
        const leftChunks = regionChunks(seed, regionIndex);
        const rightChunks = regionChunks(seed, regionIndex + 1);
        const leftRegion = leftChunks[0]!.region;
        const rightRegion = rightChunks[0]!.region;
        if (
          (leftRegion === "forest" && rightRegion === "mountain") ||
          (leftRegion === "mountain" && rightRegion === "forest")
        ) {
          const exit = trainForestMountainSceneryBeatForChunk(
            leftChunks.at(-1)!,
          )!;
          const entry = trainForestMountainSceneryBeatForChunk(
            rightChunks[0]!,
          )!;
          expect(exit).toMatchObject({
            transition: "exit",
            transitionNeighbor: rightRegion,
            silhouetteFamily: "mixed-grove",
          });
          expect(entry).toMatchObject({
            transition: "entry",
            transitionNeighbor: leftRegion,
            silhouetteFamily: "mixed-grove",
          });
          if (leftRegion === "forest") forestToMountain++;
          else mountainToForest++;
        }

        if (leftRegion !== "forest" && leftRegion !== "mountain") continue;
        const lastCenterByID = new Map<string, number>();
        for (const chunk of leftChunks) {
          for (const placement of trainSceneryPlacementsForChunk(
            "midground",
            chunk,
            { includeSetPieces: false },
          ).filter((candidate) => !candidate.landmark)) {
            const center =
              chunk.index * TRAIN_ROUTE_CHUNK_WIDTH +
              (placement.offsetPercent / 100) * TRAIN_ROUTE_CHUNK_WIDTH;
            const previous = lastCenterByID.get(placement.asset.id);
            if (previous !== undefined) repeatedDistances.push(center - previous);
            lastCenterByID.set(placement.asset.id, center);
          }
        }
      }
    }

    expect(forestToMountain).toBeGreaterThan(20);
    expect(mountainToForest).toBeGreaterThan(20);
    expect(repeatedDistances.length).toBeGreaterThan(100);
    expect(Math.min(...repeatedDistances)).toBeGreaterThanOrEqual(
      TRAIN_FOREST_MOUNTAIN_MIN_REPEAT_DISTANCE_PX,
    );
  });

  it("keeps forest clearings and mountain lookouts occasional across many seeds", () => {
    const statistics = {
      forest: { regions: 0, landmarks: 0 },
      mountain: { regions: 0, landmarks: 0 },
    };

    for (const seed of [
      "landmark-cadence-a",
      "landmark-cadence-b",
      "landmark-cadence-c",
      "landmark-cadence-d",
    ]) {
      for (let regionIndex = -120; regionIndex <= 120; regionIndex++) {
        const chunks = regionChunks(seed, regionIndex);
        const region = chunks[0]!.region;
        if (region !== "forest" && region !== "mountain") continue;
        statistics[region].regions++;
        const landmarks = chunks.flatMap((chunk) =>
          trainSceneryPlacementsForChunk("midground", chunk, {
            includeSetPieces: false,
          }).filter((placement) => placement.landmark),
        );
        expect(landmarks.length).toBeLessThanOrEqual(1);
        if (landmarks.length === 1) {
          statistics[region].landmarks++;
          expect(landmarks[0]!.regionalRole).toBe(
            region === "forest" ? "forest-landmark" : "mountain-landmark",
          );
        }
      }
    }

    for (const region of ["forest", "mountain"] as const) {
      const rate =
        statistics[region].landmarks / statistics[region].regions;
      expect(rate, region).toBeGreaterThan(0.35);
      expect(rate, region).toBeLessThan(0.65);
    }
  });

  it("builds distinct town blocks and industrial districts across deterministic rhythms", () => {
    const roles = {
      town: new Set<string>(),
      industrial: new Set<string>(),
    };
    const families = {
      town: new Set<string>(),
      industrial: new Set<string>(),
    };
    const grounds = {
      town: new Set<string>(),
      industrial: new Set<string>(),
    };
    const fixtures = {
      town: new Set<string>(),
      industrial: new Set<string>(),
    };
    const variants = {
      town: new Set<number>(),
      industrial: new Set<number>(),
    };

    for (const seed of [
      "built-rhythm-market",
      "built-rhythm-foundry",
      "built-rhythm-civic",
      "built-rhythm-freight",
    ]) {
      for (let regionIndex = -90; regionIndex <= 90; regionIndex++) {
        const chunks = regionChunks(seed, regionIndex);
        const region = chunks[0]!.region;
        if (region !== "town" && region !== "industrial") continue;
        const beats = chunks.map((chunk) =>
          trainTownIndustrialSceneryBeatForChunk(chunk),
        );
        expect(beats.every(Boolean)).toBe(true);
        expect(beats[0]).toMatchObject({
          region,
          transition: "entry",
          role:
            region === "town"
              ? "town-transition-lane"
              : "industrial-transition-road",
        });
        expect(beats.at(-1)).toMatchObject({
          region,
          transition: "exit",
          role:
            region === "town"
              ? "town-transition-lane"
              : "industrial-transition-road",
        });
        expect(
          beats
            .slice(1, -1)
            .every((beat) => beat!.transition === "interior"),
        ).toBe(true);
        for (const beat of beats) {
          roles[region].add(beat!.role);
          families[region].add(beat!.compositionFamily);
          grounds[region].add(beat!.groundKind);
          beat!.fixtures.forEach((fixture) => fixtures[region].add(fixture));
          variants[region].add(beat!.templateVariant);
        }
      }
    }

    expect(roles.town).toEqual(
      new Set([
        "town-transition-lane",
        "town-residential-block",
        "town-commercial-main-street",
        "town-yard-cluster",
        "town-civic-square",
        "town-tree-lined-street",
        "town-open-lot",
        "town-landmark-approach",
      ]),
    );
    expect(roles.industrial).toEqual(
      new Set([
        "industrial-transition-road",
        "industrial-shed-district",
        "industrial-tank-yard",
        "industrial-stack-line",
        "industrial-crane-yard",
        "industrial-utility-corridor",
        "industrial-service-gap",
        "industrial-landmark-approach",
      ]),
    );
    expect(families.town.size).toBeGreaterThanOrEqual(7);
    expect(families.industrial.size).toBeGreaterThanOrEqual(7);
    expect(grounds.town.size).toBeGreaterThanOrEqual(7);
    expect(grounds.industrial.size).toBeGreaterThanOrEqual(7);
    expect(fixtures.town).toEqual(
      new Set([
        "fence",
        "street-tree",
        "townhouse-block",
        "shop-awning",
        "civic-clock",
        "yard-gate",
      ]),
    );
    expect(fixtures.industrial).toEqual(
      new Set([
        "utility-pole",
        "industrial-shed",
        "vent-stack",
        "service-pipe",
        "storage-tank",
        "furnace-stack",
        "gantry-crane",
      ]),
    );
    expect(variants.town).toEqual(new Set([0, 1, 2]));
    expect(variants.industrial).toEqual(new Set([0, 1, 2]));
  });

  it("turns built-environment roles into coherent pools, scale families, and negative space", () => {
    const countsByRole = new Map<string, number[]>();
    const idsByRole = new Map<string, Set<string>>();

    for (const seed of [
      "built-pools-residential",
      "built-pools-steel",
      "built-pools-service",
    ]) {
      for (let regionIndex = -90; regionIndex <= 90; regionIndex++) {
        for (const chunk of regionChunks(seed, regionIndex)) {
          if (chunk.region !== "town" && chunk.region !== "industrial") {
            continue;
          }
          const beat = trainTownIndustrialSceneryBeatForChunk(chunk)!;
          const midground = trainSceneryPlacementsForChunk(
            "midground",
            chunk,
            { includeSetPieces: false },
          ).filter((placement) => !placement.landmark);
          const counts = countsByRole.get(beat.role) ?? [];
          counts.push(midground.length);
          countsByRole.set(beat.role, counts);
          const ids = idsByRole.get(beat.role) ?? new Set<string>();
          for (const placement of midground) {
            ids.add(placement.asset.id);
            expect(placement.regionalRole).toBe(beat.role);
            expect(placement.silhouetteFamily).toBe(
              beat.compositionFamily,
            );
            expect(placement.regionalScaleFamily).toBe(beat.scaleFamily);
            expect(placement.regionalTemplateVariant).toBe(
              beat.templateVariant,
            );
            const [minimum, maximum] = placement.asset.safeScale;
            const scaleUnit =
              (placement.assetScale - minimum) / (maximum - minimum);
            const expectedRange =
              beat.scaleFamily === "small"
                ? [0, 0.35]
                : beat.scaleFamily === "medium"
                  ? [0.28, 0.7]
                  : beat.scaleFamily === "tall"
                    ? [0.62, 1]
                    : [0.12, 0.88];
            expect(scaleUnit).toBeGreaterThanOrEqual(expectedRange[0]! - 1e-9);
            expect(scaleUnit).toBeLessThanOrEqual(expectedRange[1]! + 1e-9);
          }
          idsByRole.set(beat.role, ids);
        }
      }
    }

    const average = (role: string) => {
      const values = countsByRole.get(role)!;
      return values.reduce((total, value) => total + value, 0) / values.length;
    };
    expect(average("town-residential-block")).toBeGreaterThan(1.1);
    expect(average("town-commercial-main-street")).toBeGreaterThan(1);
    expect(average("town-open-lot")).toBe(0);
    expect(average("industrial-shed-district")).toBeGreaterThan(1.1);
    expect(average("industrial-stack-line")).toBeGreaterThan(1);
    expect(average("industrial-service-gap")).toBe(0);
    expect(idsByRole.get("town-commercial-main-street")).toEqual(
      new Set(["building-rowhouse", "building-apartments"]),
    );
    expect(idsByRole.get("industrial-tank-yard")).toEqual(
      new Set(["building-water-tower", "building-workshop"]),
    );
    expect(idsByRole.get("industrial-crane-yard")).toEqual(
      new Set(["building-warehouse", "building-water-tower"]),
    );
  });

  it("spaces repeated town and industrial facades while preserving both transition directions", () => {
    let townToIndustrial = 0;
    let industrialToTown = 0;
    const repeatedDistances: number[] = [];

    for (const seed of [
      "built-transition-market",
      "built-transition-freight",
      "built-transition-works",
    ]) {
      for (let regionIndex = -180; regionIndex < 180; regionIndex++) {
        const leftChunks = regionChunks(seed, regionIndex);
        const rightChunks = regionChunks(seed, regionIndex + 1);
        const leftRegion = leftChunks[0]!.region;
        const rightRegion = rightChunks[0]!.region;
        if (
          (leftRegion === "town" && rightRegion === "industrial") ||
          (leftRegion === "industrial" && rightRegion === "town")
        ) {
          const exit = trainTownIndustrialSceneryBeatForChunk(
            leftChunks.at(-1)!,
          )!;
          const entry = trainTownIndustrialSceneryBeatForChunk(
            rightChunks[0]!,
          )!;
          expect(exit.transition).toBe("exit");
          expect(exit.transitionNeighbor).toBe(rightRegion);
          expect(entry.transition).toBe("entry");
          expect(entry.transitionNeighbor).toBe(leftRegion);
          if (leftRegion === "town") townToIndustrial++;
          else industrialToTown++;
        }

        if (leftRegion !== "town" && leftRegion !== "industrial") continue;
        const lastCenterByID = new Map<string, number>();
        for (const chunk of leftChunks) {
          for (const placement of trainSceneryPlacementsForChunk(
            "midground",
            chunk,
            { includeSetPieces: false },
          ).filter((candidate) => !candidate.landmark)) {
            const center =
              chunk.index * TRAIN_ROUTE_CHUNK_WIDTH +
              (placement.offsetPercent / 100) * TRAIN_ROUTE_CHUNK_WIDTH;
            const previous = lastCenterByID.get(placement.asset.id);
            if (previous !== undefined) repeatedDistances.push(center - previous);
            lastCenterByID.set(placement.asset.id, center);
          }
        }
      }
    }

    expect(townToIndustrial).toBeGreaterThan(20);
    expect(industrialToTown).toBeGreaterThan(20);
    expect(repeatedDistances.length).toBeGreaterThan(100);
    expect(Math.min(...repeatedDistances)).toBeGreaterThanOrEqual(
      TRAIN_TOWN_INDUSTRIAL_MIN_REPEAT_DISTANCE_PX,
    );
  });

  it("keeps built surfaces opaque and lights attached to compatible owners", () => {
    const townIDs = new Set(
      TRAIN_REGION_SCENERY_PROFILES.town.layers.midground.assetIds,
    );
    const industrialIDs = new Set(
      TRAIN_REGION_SCENERY_PROFILES.industrial.layers.midground.assetIds,
    );
    const buildingIDs = new Set(
      TRAIN_SCENERY_BUILDINGS.map((asset) => asset.id),
    );

    expect(
      [...townIDs].filter((assetID) => buildingIDs.has(assetID)),
    ).toEqual([
      "building-rowhouse",
      "building-apartments",
      "building-cottage",
    ]);
    expect([...industrialIDs]).toEqual([
      "building-workshop",
      "building-warehouse",
      "building-water-tower",
    ]);
    for (const asset of TRAIN_SCENERY_BUILDINGS) {
      expect(asset.anchor).toBe("bottom-center");
      expect(asset.dayNightTreatment).toBe("emissive-windows");
      expect(asset.emissive).toMatchObject({
        kind: "windows",
        width: asset.width,
        height: asset.height,
      });
      expect(asset.groundInsetPx).toBeLessThanOrEqual(3);
    }
  });

  it("builds a nine-beat coast from continuous-water, shore, and harbour roles", () => {
    const roles = new Set<string>();
    const shores = new Set<string>();
    const waters = new Set<string>();
    const fixtures = new Set<string>();
    const variants = new Set<number>();
    let sampledRegions = 0;

    for (const seed of [
      "coast-rhythm-tide",
      "coast-rhythm-harbour",
      "coast-rhythm-headland",
      "coast-rhythm-cove",
    ]) {
      for (let regionIndex = -100; regionIndex <= 100; regionIndex++) {
        const chunks = regionChunks(seed, regionIndex);
        if (chunks[0]!.region !== "coast") continue;
        sampledRegions++;
        const beats = chunks.map((chunk) =>
          trainCoastSceneryBeatForChunk(chunk),
        );
        expect(beats.every(Boolean)).toBe(true);
        expect(beats[0]).toMatchObject({
          region: "coast",
          role: "coast-transition-shore",
          transition: "entry",
          waterKind: "arrival-tide",
        });
        expect(beats.at(-1)).toMatchObject({
          region: "coast",
          role: "coast-transition-shore",
          transition: "exit",
          waterKind: "arrival-tide",
        });
        for (const beat of beats) {
          roles.add(beat!.role);
          shores.add(beat!.shoreFamily);
          waters.add(beat!.waterKind);
          beat!.fixtures.forEach((fixture) => fixtures.add(fixture));
          variants.add(beat!.templateVariant);
        }
      }
    }

    expect(sampledRegions).toBeGreaterThan(80);
    expect(roles).toEqual(
      new Set([
        "coast-transition-shore",
        "coast-open-water",
        "coast-beach-cove",
        "coast-rock-shelf",
        "coast-harbour-reach",
        "coast-dune-grass",
        "coast-navigation-channel",
        "coast-landmark-approach",
      ]),
    );
    expect(shores.size).toBeGreaterThanOrEqual(8);
    expect(waters.size).toBeGreaterThanOrEqual(7);
    expect(fixtures).toEqual(
      new Set([
        "beach",
        "rock-shelf",
        "pier",
        "boat",
        "buoy",
        "harbour-post",
        "dune-grass",
      ]),
    );
    expect(variants).toEqual(new Set([0, 1, 2]));
  });

  it("keeps ordinary coast pools sparse, water-led, and free of mountain wallpaper", () => {
    const midgroundIDs = new Set<string>();
    const countsByRole = new Map<string, number[]>();

    for (const seed of [
      "coast-pool-shore",
      "coast-pool-navigation",
      "coast-pool-port",
    ]) {
      for (let regionIndex = -100; regionIndex <= 100; regionIndex++) {
        for (const chunk of regionChunks(seed, regionIndex)) {
          if (chunk.region !== "coast") continue;
          const beat = trainCoastSceneryBeatForChunk(chunk)!;
          expect(
            trainSceneryPlacementsForChunk("ultra-far", chunk, {
              includeSetPieces: false,
            }),
          ).toEqual([]);
          expect(
            trainSceneryPlacementsForChunk("far", chunk, {
              includeSetPieces: false,
            }),
          ).toEqual([]);
          const placements = trainSceneryPlacementsForChunk(
            "midground",
            chunk,
            { includeSetPieces: false },
          ).filter((placement) => !placement.landmark);
          const counts = countsByRole.get(beat.role) ?? [];
          counts.push(placements.length);
          countsByRole.set(beat.role, counts);
          for (const placement of placements) {
            midgroundIDs.add(placement.asset.id);
            expect(placement.regionalRole).toBe(beat.role);
            expect(placement.silhouetteFamily).toBe(beat.shoreFamily);
            expect(placement.regionalWaterKind).toBe(beat.waterKind);
            expect(placement.asset.category).not.toBe("terrain");
          }
        }
      }
    }

    expect(countsByRole.get("coast-open-water")!.every((count) => count === 0))
      .toBe(true);
    expect(
      countsByRole
        .get("coast-harbour-reach")!
        .some((count) => count === 1),
    ).toBe(true);
    expect(midgroundIDs).toEqual(
      new Set([
        "vegetation-reeds",
        "vegetation-hedgerow",
        "vegetation-coastal-pine",
        "building-cottage",
      ]),
    );
  });

  it("maps coast boundaries for natural, town, and industrial transitions", () => {
    expect(trainCoastTransitionFamilyForRegion("forest")).toBe(
      "natural-bank",
    );
    expect(trainCoastTransitionFamilyForRegion("mountain")).toBe(
      "natural-bank",
    );
    expect(trainCoastTransitionFamilyForRegion("town")).toBe(
      "settlement-harbour",
    );
    expect(trainCoastTransitionFamilyForRegion("industrial")).toBe(
      "working-port",
    );
    expect(trainCoastTransitionFamilyForRegion(null)).toBe("open-horizon");

    let actualBoundaries = 0;
    for (const seed of ["coast-edge-town", "coast-edge-cliff"]) {
      for (let regionIndex = -200; regionIndex <= 200; regionIndex++) {
        const chunks = regionChunks(seed, regionIndex);
        if (chunks[0]!.region !== "coast") continue;
        const entry = trainCoastSceneryBeatForChunk(chunks[0]!)!;
        const exit = trainCoastSceneryBeatForChunk(chunks.at(-1)!)!;
        expect(entry.transitionNeighbor).toMatch(/^(mountain|town)$/);
        expect(exit.transitionNeighbor).toMatch(/^(mountain|town)$/);
        expect(entry.transitionFamily).not.toBe("open-horizon");
        expect(exit.transitionFamily).not.toBe("open-horizon");
        actualBoundaries++;
      }
    }
    expect(actualBoundaries).toBeGreaterThan(60);
  });

  it("caps coast landmarks and keeps lighthouse occurrences widely spaced", () => {
    let landmarks = 0;
    for (const seed of [
      "coast-landmark-beacon",
      "coast-landmark-channel",
      "coast-landmark-harbour",
    ]) {
      let previousCenter: number | null = null;
      for (let regionIndex = -160; regionIndex <= 160; regionIndex++) {
        const chunks = regionChunks(seed, regionIndex);
        if (chunks[0]!.region !== "coast") continue;
        const regionLandmarks = chunks.flatMap((chunk) =>
          trainSceneryPlacementsForChunk("midground", chunk, {
            includeSetPieces: false,
          })
            .filter((placement) => placement.landmark)
            .map(
              (placement) =>
                chunk.index * TRAIN_ROUTE_CHUNK_WIDTH +
                (placement.offsetPercent / 100) * TRAIN_ROUTE_CHUNK_WIDTH,
            ),
        );
        expect(regionLandmarks.length).toBeLessThanOrEqual(1);
        if (regionLandmarks[0] === undefined) continue;
        landmarks++;
        if (previousCenter !== null) {
          expect(regionLandmarks[0] - previousCenter).toBeGreaterThanOrEqual(
            TRAIN_COAST_MIN_LANDMARK_SPACING_PX,
          );
        }
        previousCenter = regionLandmarks[0];
      }
    }
    expect(landmarks).toBeGreaterThan(40);
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

  it("keeps multi-seed regional variety and rare-feature cadence statistically balanced", () => {
    const seeds = [
      "balance-alpine",
      "balance-harbor",
      "balance-local",
      "balance-night-mail",
      "balance-orchard",
      "balance-river",
      "balance-steel",
      "balance-winter",
    ];
    const statistics = Object.fromEntries(
      (Object.keys(REGION_LANDMARK_IDS) as TrainRegionName[]).map((region) => [
        region,
        {
          regions: 0,
          chunks: 0,
          openChunks: 0,
          denseChunks: 0,
          landmarks: 0,
          midgroundPlacements: 0,
          nearPlacements: 0,
          eligibleNightLife: 0,
          activeNightLife: 0,
          assetIDs: new Set<string>(),
          landmarkSequence: [] as boolean[],
        },
      ]),
    ) as Record<
      TrainRegionName,
      {
        regions: number;
        chunks: number;
        openChunks: number;
        denseChunks: number;
        landmarks: number;
        midgroundPlacements: number;
        nearPlacements: number;
        eligibleNightLife: number;
        activeNightLife: number;
        assetIDs: Set<string>;
        landmarkSequence: boolean[];
      }
    >;
    const setPieceTypes = new Set<string>();
    const setPieceVariants = new Map<string, Set<number>>();

    for (const seed of seeds) {
      for (let regionIndex = -120; regionIndex <= 120; regionIndex++) {
        const chunks = regionChunks(seed, regionIndex);
        const region = chunks[0]!.region;
        const sample = statistics[region];
        sample.regions++;
        let hasLandmark = false;

        for (const chunk of chunks) {
          sample.chunks++;
          const composition = trainRegionCompositionForChunk(chunk);
          if (composition === "open") sample.openChunks++;
          if (composition === "dense") sample.denseChunks++;
          if (chunk.setPiece) {
            setPieceTypes.add(chunk.setPiece.type);
            const variants =
              setPieceVariants.get(chunk.setPiece.type) ?? new Set<number>();
            variants.add(chunk.setPiece.visualVariant);
            setPieceVariants.set(chunk.setPiece.type, variants);
          }

          const midground = trainSceneryPlacementsForChunk(
            "midground",
            chunk,
          );
          const near = trainSceneryPlacementsForChunk("near", chunk);
          sample.midgroundPlacements += midground.length;
          sample.nearPlacements += near.length;
          for (const placement of [...midground, ...near]) {
            sample.assetIDs.add(placement.asset.id);
            if (placement.landmark) {
              hasLandmark = true;
              sample.landmarks++;
            }
          }

          const nightOwners = new Set<string>(
            TRAIN_REGION_NIGHT_LIFE[region].owners.map(
              (owner) => owner.assetId,
            ),
          );
          midground.forEach((placement, ordinal) => {
            if (nightOwners.has(placement.asset.id)) {
              sample.eligibleNightLife++;
            }
            if (trainNightLifeForPlacement(chunk, placement, ordinal)) {
              sample.activeNightLife++;
            }
          });
        }
        sample.landmarkSequence.push(hasLandmark);
      }
    }

    const longestRun = (values: readonly boolean[], target: boolean) => {
      let longest = 0;
      let current = 0;
      for (const value of values) {
        current = value === target ? current + 1 : 0;
        longest = Math.max(longest, current);
      }
      return longest;
    };

    for (const region of Object.keys(statistics) as TrainRegionName[]) {
      const sample = statistics[region];
      const profile = TRAIN_REGION_SCENERY_PROFILES[region];
      const expectedIDs = new Set([
        ...(profile.layers.midground?.assetIds ?? []),
        ...(profile.layers.near?.assetIds ?? []),
        ...profile.landmark.assetIds,
      ]);
      const landmarkRate = sample.landmarks / sample.regions;
      const openRate = sample.openChunks / sample.chunks;
      const midgroundRate = sample.midgroundPlacements / sample.chunks;
      const nearRate = sample.nearPlacements / sample.chunks;
      const nightLifeChunkRate = sample.activeNightLife / sample.chunks;

      expect(sample.regions, region).toBeGreaterThan(150);
      const observedExpectedIDs = [...expectedIDs].filter((assetID) =>
        sample.assetIDs.has(assetID),
      );
      expect(
        observedExpectedIDs.length / expectedIDs.size,
        `${region}: found=${observedExpectedIDs.join(",")} expected=${[...expectedIDs].join(",")}`,
      ).toBeGreaterThan(0.74);
      expect(landmarkRate, region).toBeGreaterThan(0.16);
      expect(landmarkRate, region).toBeLessThan(0.55);
      expect(openRate, region).toBeGreaterThan(0.12);
      expect(openRate, region).toBeLessThan(0.25);
      expect(sample.denseChunks / sample.chunks, region).toBeGreaterThan(0.2);
      expect(midgroundRate, region).toBeGreaterThan(0.15);
      expect(midgroundRate, region).toBeLessThan(1.5);
      expect(nearRate, region).toBeGreaterThan(
        region === "forest" ? 0.045 : 0.06,
      );
      expect(nearRate, region).toBeLessThan(0.24);
      expect(nightLifeChunkRate, region).toBeGreaterThan(0.01);
      expect(nightLifeChunkRate, region).toBeLessThan(0.14);
      expect(sample.activeNightLife, region).toBeLessThan(
        sample.eligibleNightLife,
      );
      expect(longestRun(sample.landmarkSequence, true), region).toBeLessThan(
        12,
      );
      expect(longestRun(sample.landmarkSequence, false), region).toBeLessThan(
        28,
      );
    }

    expect(setPieceTypes).toEqual(
      new Set(["bridge", "coast-reveal", "station", "town-edge", "tunnel"]),
    );
    for (const type of ["bridge", "coast-reveal", "town-edge", "tunnel"]) {
      expect(setPieceVariants.get(type), type).toEqual(new Set([0, 1]));
    }
    expect(setPieceVariants.get("station")).toEqual(new Set([0]));
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
      expect(plan.intensity).toBeGreaterThanOrEqual(
        TRAIN_NIGHT_LIFE_MIN_INTENSITY,
      );
      expect(plan.intensity).toBeLessThan(
        TRAIN_NIGHT_LIFE_MAX_INTENSITY,
      );
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
