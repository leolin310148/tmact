import bridgeTrussUrl from "../assets/train-theme/sprites/scenery/bridge-truss.png";
import buildingApartmentsUrl from "../assets/train-theme/sprites/scenery/building-apartments.png";
import buildingCottageUrl from "../assets/train-theme/sprites/scenery/building-cottage.png";
import buildingRowhouseUrl from "../assets/train-theme/sprites/scenery/building-rowhouse.png";
import buildingWarehouseUrl from "../assets/train-theme/sprites/scenery/building-warehouse.png";
import buildingWaterTowerUrl from "../assets/train-theme/sprites/scenery/building-water-tower.png";
import buildingWorkshopUrl from "../assets/train-theme/sprites/scenery/building-workshop.png";
import cloudCumulusUrl from "../assets/train-theme/sprites/scenery/cloud-cumulus.png";
import cloudStormUrl from "../assets/train-theme/sprites/scenery/cloud-storm.png";
import cloudWispUrl from "../assets/train-theme/sprites/scenery/cloud-wisp.png";
import coastShoreUrl from "../assets/train-theme/sprites/scenery/coast-shore.png";
import propFenceUrl from "../assets/train-theme/sprites/scenery/prop-fence.png";
import propTelegraphPoleUrl from "../assets/train-theme/sprites/scenery/prop-telegraph-pole.png";
import propWarningSignUrl from "../assets/train-theme/sprites/scenery/prop-warning-sign.png";
import terrainAlpineUrl from "../assets/train-theme/sprites/scenery/terrain-alpine.png";
import terrainFoothillsUrl from "../assets/train-theme/sprites/scenery/terrain-foothills.png";
import terrainMesaUrl from "../assets/train-theme/sprites/scenery/terrain-mesa.png";
import vegetationCoastalPineUrl from "../assets/train-theme/sprites/scenery/vegetation-coastal-pine.png";
import vegetationConiferSquatUrl from "../assets/train-theme/sprites/scenery/vegetation-conifer-squat.png";
import vegetationConiferTallUrl from "../assets/train-theme/sprites/scenery/vegetation-conifer-tall.png";
import vegetationDeciduousUrl from "../assets/train-theme/sprites/scenery/vegetation-deciduous.png";
import vegetationHedgerowUrl from "../assets/train-theme/sprites/scenery/vegetation-hedgerow.png";
import vegetationReedsUrl from "../assets/train-theme/sprites/scenery/vegetation-reeds.png";
import {
  TRAIN_REGION_CHUNK_LENGTH,
  trainRouteSetPieceForChunk,
  trainRouteRandomUnit,
  type RouteChunk,
  type TrainSetPieceSegment,
  type TrainParallaxLayerName,
  type TrainRegionName,
} from "./trainRoute";

export type TrainSceneryCategory =
  | "cloud"
  | "terrain"
  | "vegetation"
  | "building"
  | "bridge"
  | "coast"
  | "prop";

export type TrainSceneryAnchor = "center" | "bottom-center";

export type TrainSceneryDayNightTreatment =
  | "atmospheric-filter"
  | "emissive-windows"
  | "water-reflection";

export interface TrainSceneryAsset {
  id: string;
  fileName: string;
  src: string;
  category: TrainSceneryCategory;
  layer: TrainParallaxLayerName;
  anchor: TrainSceneryAnchor;
  width: number;
  height: number;
  safeScale: readonly [minimum: number, maximum: number];
  dayNightTreatment: TrainSceneryDayNightTreatment;
}

function asset(
  definition: Omit<TrainSceneryAsset, "safeScale"> & {
    safeScale?: TrainSceneryAsset["safeScale"];
  },
): TrainSceneryAsset {
  return {
    safeScale: [0.75, 1.2],
    ...definition,
  };
}

export const TRAIN_SCENERY_CLOUDS = [
  asset({
    id: "cloud-cumulus",
    fileName: "cloud-cumulus.png",
    src: cloudCumulusUrl,
    category: "cloud",
    layer: "sky",
    anchor: "center",
    width: 148,
    height: 42,
    safeScale: [0.65, 1],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "cloud-wisp",
    fileName: "cloud-wisp.png",
    src: cloudWispUrl,
    category: "cloud",
    layer: "sky",
    anchor: "center",
    width: 148,
    height: 29,
    safeScale: [0.6, 0.95],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "cloud-storm",
    fileName: "cloud-storm.png",
    src: cloudStormUrl,
    category: "cloud",
    layer: "sky",
    anchor: "center",
    width: 135,
    height: 60,
    safeScale: [0.6, 0.9],
    dayNightTreatment: "atmospheric-filter",
  }),
] as const satisfies readonly TrainSceneryAsset[];

export const TRAIN_SCENERY_TERRAIN = [
  asset({
    id: "terrain-foothills",
    fileName: "terrain-foothills.png",
    src: terrainFoothillsUrl,
    category: "terrain",
    layer: "ultra-far",
    anchor: "bottom-center",
    width: 244,
    height: 61,
    safeScale: [0.8, 1.25],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "terrain-alpine",
    fileName: "terrain-alpine.png",
    src: terrainAlpineUrl,
    category: "terrain",
    layer: "ultra-far",
    anchor: "bottom-center",
    width: 244,
    height: 71,
    safeScale: [0.75, 1.15],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "terrain-mesa",
    fileName: "terrain-mesa.png",
    src: terrainMesaUrl,
    category: "terrain",
    layer: "ultra-far",
    anchor: "bottom-center",
    width: 244,
    height: 45,
    safeScale: [0.85, 1.3],
    dayNightTreatment: "atmospheric-filter",
  }),
] as const satisfies readonly TrainSceneryAsset[];

export const TRAIN_SCENERY_VEGETATION = [
  asset({
    id: "vegetation-conifer-tall",
    fileName: "vegetation-conifer-tall.png",
    src: vegetationConiferTallUrl,
    category: "vegetation",
    layer: "midground",
    anchor: "bottom-center",
    width: 35,
    height: 84,
    safeScale: [0.65, 1.05],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "vegetation-conifer-squat",
    fileName: "vegetation-conifer-squat.png",
    src: vegetationConiferSquatUrl,
    category: "vegetation",
    layer: "midground",
    anchor: "bottom-center",
    width: 73,
    height: 84,
    safeScale: [0.6, 1],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "vegetation-deciduous",
    fileName: "vegetation-deciduous.png",
    src: vegetationDeciduousUrl,
    category: "vegetation",
    layer: "midground",
    anchor: "bottom-center",
    width: 70,
    height: 84,
    safeScale: [0.6, 1],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "vegetation-coastal-pine",
    fileName: "vegetation-coastal-pine.png",
    src: vegetationCoastalPineUrl,
    category: "vegetation",
    layer: "midground",
    anchor: "bottom-center",
    width: 43,
    height: 84,
    safeScale: [0.65, 1.05],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "vegetation-hedgerow",
    fileName: "vegetation-hedgerow.png",
    src: vegetationHedgerowUrl,
    category: "vegetation",
    layer: "midground",
    anchor: "bottom-center",
    width: 112,
    height: 52,
    safeScale: [0.7, 1.1],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "vegetation-reeds",
    fileName: "vegetation-reeds.png",
    src: vegetationReedsUrl,
    category: "vegetation",
    layer: "midground",
    anchor: "bottom-center",
    width: 27,
    height: 68,
    safeScale: [0.7, 1.15],
    dayNightTreatment: "atmospheric-filter",
  }),
] as const satisfies readonly TrainSceneryAsset[];

export const TRAIN_SCENERY_BUILDINGS = [
  asset({
    id: "building-rowhouse",
    fileName: "building-rowhouse.png",
    src: buildingRowhouseUrl,
    category: "building",
    layer: "midground",
    anchor: "bottom-center",
    width: 46,
    height: 92,
    safeScale: [0.65, 1],
    dayNightTreatment: "emissive-windows",
  }),
  asset({
    id: "building-workshop",
    fileName: "building-workshop.png",
    src: buildingWorkshopUrl,
    category: "building",
    layer: "midground",
    anchor: "bottom-center",
    width: 100,
    height: 28,
    safeScale: [0.75, 1.2],
    dayNightTreatment: "emissive-windows",
  }),
  asset({
    id: "building-apartments",
    fileName: "building-apartments.png",
    src: buildingApartmentsUrl,
    category: "building",
    layer: "midground",
    anchor: "bottom-center",
    width: 91,
    height: 92,
    safeScale: [0.6, 0.95],
    dayNightTreatment: "emissive-windows",
  }),
  asset({
    id: "building-cottage",
    fileName: "building-cottage.png",
    src: buildingCottageUrl,
    category: "building",
    layer: "midground",
    anchor: "bottom-center",
    width: 100,
    height: 79,
    safeScale: [0.65, 1],
    dayNightTreatment: "emissive-windows",
  }),
  asset({
    id: "building-warehouse",
    fileName: "building-warehouse.png",
    src: buildingWarehouseUrl,
    category: "building",
    layer: "midground",
    anchor: "bottom-center",
    width: 99,
    height: 84,
    safeScale: [0.65, 1.05],
    dayNightTreatment: "emissive-windows",
  }),
  asset({
    id: "building-water-tower",
    fileName: "building-water-tower.png",
    src: buildingWaterTowerUrl,
    category: "building",
    layer: "midground",
    anchor: "bottom-center",
    width: 56,
    height: 100,
    safeScale: [0.55, 0.9],
    dayNightTreatment: "emissive-windows",
  }),
] as const satisfies readonly TrainSceneryAsset[];

export const TRAIN_SCENERY_BRIDGES = [
  asset({
    id: "bridge-truss",
    fileName: "bridge-truss.png",
    src: bridgeTrussUrl,
    category: "bridge",
    layer: "midground",
    anchor: "bottom-center",
    width: 224,
    height: 67,
    safeScale: [0.75, 1],
    dayNightTreatment: "atmospheric-filter",
  }),
] as const satisfies readonly TrainSceneryAsset[];

export const TRAIN_SCENERY_COASTS = [
  asset({
    id: "coast-shore",
    fileName: "coast-shore.png",
    src: coastShoreUrl,
    category: "coast",
    layer: "far",
    anchor: "bottom-center",
    width: 224,
    height: 33,
    safeScale: [0.8, 1.15],
    dayNightTreatment: "water-reflection",
  }),
] as const satisfies readonly TrainSceneryAsset[];

export const TRAIN_SCENERY_PROPS = [
  asset({
    id: "prop-telegraph-pole",
    fileName: "prop-telegraph-pole.png",
    src: propTelegraphPoleUrl,
    category: "prop",
    layer: "near",
    anchor: "bottom-center",
    width: 34,
    height: 84,
    safeScale: [0.65, 1],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "prop-warning-sign",
    fileName: "prop-warning-sign.png",
    src: propWarningSignUrl,
    category: "prop",
    layer: "near",
    anchor: "bottom-center",
    width: 51,
    height: 84,
    safeScale: [0.55, 0.85],
    dayNightTreatment: "atmospheric-filter",
  }),
  asset({
    id: "prop-fence",
    fileName: "prop-fence.png",
    src: propFenceUrl,
    category: "prop",
    layer: "near",
    anchor: "bottom-center",
    width: 124,
    height: 34,
    safeScale: [0.7, 1],
    dayNightTreatment: "atmospheric-filter",
  }),
] as const satisfies readonly TrainSceneryAsset[];

export const TRAIN_SCENERY_ASSETS = [
  ...TRAIN_SCENERY_CLOUDS,
  ...TRAIN_SCENERY_TERRAIN,
  ...TRAIN_SCENERY_VEGETATION,
  ...TRAIN_SCENERY_BUILDINGS,
  ...TRAIN_SCENERY_BRIDGES,
  ...TRAIN_SCENERY_COASTS,
  ...TRAIN_SCENERY_PROPS,
] as const satisfies readonly TrainSceneryAsset[];

export interface TrainRegionLayerRule {
  assetIds: readonly string[];
  density: number;
  maxPerChunk: number;
  minimumSpacingPx: number;
  cooldownChunks: number;
}

export interface TrainRegionSceneryProfile {
  name: TrainRegionName;
  layers: Readonly<
    Partial<Record<TrainParallaxLayerName, TrainRegionLayerRule>>
  >;
  landmark?: {
    layer: TrainParallaxLayerName;
    assetIds: readonly string[];
    probability: number;
    maxPerRegion: 1;
  };
}

const CLOUD_IDS = TRAIN_SCENERY_CLOUDS.map((asset) => asset.id);
const PROP_IDS = TRAIN_SCENERY_PROPS.map((asset) => asset.id);

export const TRAIN_REGION_SCENERY_PROFILES = {
  forest: {
    name: "forest",
    layers: {
      sky: {
        assetIds: CLOUD_IDS,
        density: 0.55,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 1,
      },
      "ultra-far": {
        assetIds: ["terrain-foothills", "terrain-mesa"],
        density: 1,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      far: {
        assetIds: ["terrain-foothills", "terrain-mesa"],
        density: 0.85,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      midground: {
        assetIds: [
          "vegetation-conifer-tall",
          "vegetation-conifer-squat",
          "vegetation-deciduous",
          "vegetation-hedgerow",
        ],
        density: 1.35,
        maxPerChunk: 2,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
      near: {
        assetIds: ["prop-fence", "prop-telegraph-pole"],
        density: 0.42,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
    },
  },
  mountain: {
    name: "mountain",
    layers: {
      sky: {
        assetIds: CLOUD_IDS,
        density: 0.72,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 1,
      },
      "ultra-far": {
        assetIds: ["terrain-alpine", "terrain-foothills"],
        density: 1,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      far: {
        assetIds: ["terrain-alpine", "terrain-foothills"],
        density: 0.9,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      midground: {
        assetIds: [
          "vegetation-conifer-tall",
          "vegetation-conifer-squat",
          "vegetation-coastal-pine",
        ],
        density: 0.72,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
      near: {
        assetIds: ["prop-warning-sign", "prop-telegraph-pole"],
        density: 0.28,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
    },
  },
  town: {
    name: "town",
    layers: {
      sky: {
        assetIds: ["cloud-wisp", "cloud-cumulus"],
        density: 0.38,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 1,
      },
      "ultra-far": {
        assetIds: ["terrain-foothills", "terrain-mesa"],
        density: 1,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      far: {
        assetIds: ["terrain-foothills", "terrain-mesa"],
        density: 0.72,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      midground: {
        assetIds: [
          "building-rowhouse",
          "building-apartments",
          "building-cottage",
          "vegetation-deciduous",
          "vegetation-hedgerow",
        ],
        density: 1.5,
        maxPerChunk: 2,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
      near: {
        assetIds: PROP_IDS,
        density: 0.52,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
    },
    landmark: {
      layer: "midground",
      assetIds: ["building-water-tower"],
      probability: 0.42,
      maxPerRegion: 1,
    },
  },
  coast: {
    name: "coast",
    layers: {
      sky: {
        assetIds: ["cloud-wisp", "cloud-storm", "cloud-cumulus"],
        density: 0.62,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 1,
      },
      "ultra-far": {
        assetIds: ["terrain-mesa", "terrain-foothills"],
        density: 0.78,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      far: {
        assetIds: ["coast-shore"],
        density: 1,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      midground: {
        assetIds: [
          "vegetation-coastal-pine",
          "vegetation-reeds",
          "vegetation-hedgerow",
          "building-cottage",
        ],
        density: 0.78,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
      near: {
        assetIds: ["prop-fence", "prop-warning-sign"],
        density: 0.3,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
    },
  },
  industrial: {
    name: "industrial",
    layers: {
      sky: {
        assetIds: ["cloud-storm", "cloud-wisp"],
        density: 0.48,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 1,
      },
      "ultra-far": {
        assetIds: ["terrain-mesa", "terrain-foothills"],
        density: 1,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      far: {
        assetIds: ["terrain-mesa", "terrain-foothills"],
        density: 0.68,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      midground: {
        assetIds: [
          "building-workshop",
          "building-warehouse",
          "building-apartments",
        ],
        density: 1.42,
        maxPerChunk: 2,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
      near: {
        assetIds: PROP_IDS,
        density: 0.62,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
    },
    landmark: {
      layer: "midground",
      assetIds: ["building-water-tower"],
      probability: 0.58,
      maxPerRegion: 1,
    },
  },
} as const satisfies Record<TrainRegionName, TrainRegionSceneryProfile>;

export interface TrainSceneryPlacement {
  asset: TrainSceneryAsset;
  offsetPercent: number;
  scale: number;
  collisionWidth: number;
  minimumSpacingPx: number;
  landmark: boolean;
  setPiece: TrainSetPieceSegment | null;
}

const TRAIN_SCENERY_ASSET_BY_ID = new Map(
  TRAIN_SCENERY_ASSETS.map((asset) => [asset.id, asset]),
);

function positiveModulo(value: number, divisor: number): number {
  return ((value % divisor) + divisor) % divisor;
}

function assetForID(id: string): TrainSceneryAsset {
  const resolved = TRAIN_SCENERY_ASSET_BY_ID.get(id);
  if (!resolved) throw new Error(`unknown train scenery asset: ${id}`);
  return resolved;
}

function objectCount(
  density: number,
  maximum: number,
  randomValue: number,
): number {
  const base = Math.floor(density);
  const fractional = density - base;
  return Math.min(maximum, base + (randomValue < fractional ? 1 : 0));
}

function placementOffsets(count: number, randomValue: number): number[] {
  if (count <= 0) return [];
  if (count === 1) return [25 + randomValue * 50];
  const jitter = (randomValue - 0.5) * 2;
  return [25 + jitter, 75 - jitter];
}

function chooseAsset(
  assetIds: readonly string[],
  randomValue: number,
  recentIDs: readonly string[],
): TrainSceneryAsset {
  const start = Math.floor(randomValue * assetIds.length);
  for (let offset = 0; offset < assetIds.length; offset++) {
    const id = assetIds[positiveModulo(start + offset, assetIds.length)]!;
    if (!recentIDs.includes(id)) return assetForID(id);
  }
  const leastRecentID = [...assetIds].sort(
    (left, right) => recentIDs.lastIndexOf(left) - recentIDs.lastIndexOf(right),
  )[0]!;
  return assetForID(leastRecentID);
}

function setPiecePlacement(
  setPiece: TrainSetPieceSegment,
  layer: TrainParallaxLayerName,
): TrainSceneryPlacement | null {
  const assetID =
    setPiece.type === "bridge" && layer === "midground"
      ? "bridge-truss"
      : setPiece.type === "coast-reveal" && layer === "far"
        ? "coast-shore"
        : null;
  if (!assetID) return null;
  const resolvedAsset = assetForID(assetID);
  const scale = resolvedAsset.safeScale[1];
  return {
    asset: resolvedAsset,
    offsetPercent: 50,
    scale,
    collisionWidth: resolvedAsset.width * scale,
    minimumSpacingPx: 0,
    landmark: false,
    setPiece,
  };
}

function regionLayerPlan(
  chunk: RouteChunk,
  layer: TrainParallaxLayerName,
): readonly (readonly TrainSceneryPlacement[])[] {
  const profile: TrainRegionSceneryProfile =
    TRAIN_REGION_SCENERY_PROFILES[chunk.region];
  const rule = profile.layers[layer];
  if (!rule) return Array.from({ length: TRAIN_REGION_CHUNK_LENGTH }, () => []);

  const regionKey =
    `${chunk.seedVersion}:${chunk.routeSeed}:region-plan:` +
    `${chunk.regionIndex}:${layer}`;
  const landmark =
    profile.landmark?.layer === layer &&
    trainRouteRandomUnit(`${regionKey}:landmark:enabled`) <
      profile.landmark.probability
      ? {
          offset:
            2 +
            Math.floor(
              trainRouteRandomUnit(`${regionKey}:landmark:offset`) *
                (TRAIN_REGION_CHUNK_LENGTH - 4),
            ),
          assetIds: profile.landmark.assetIds,
        }
      : null;
  const recentIDs: string[] = [];
  const plan: TrainSceneryPlacement[][] = [];

  for (let localOffset = 0; localOffset < TRAIN_REGION_CHUNK_LENGTH; localOffset++) {
    const chunkKey = `${regionKey}:chunk:${localOffset}`;
    const setPiece = trainRouteSetPieceForChunk(
      chunk.routeSeed,
      chunk.regionIndex * TRAIN_REGION_CHUNK_LENGTH + localOffset,
      chunk.seedVersion,
    );
    if (setPiece?.reservedLayers.includes(layer)) {
      const placement = setPiecePlacement(setPiece, layer);
      plan.push(placement ? [placement] : []);
      continue;
    }
    const isLandmark = landmark?.offset === localOffset;
    const assetIds = isLandmark ? landmark.assetIds : rule.assetIds;
    const count = isLandmark
      ? 1
      : objectCount(
          rule.density,
          rule.maxPerChunk,
          trainRouteRandomUnit(`${chunkKey}:density`),
        );
    const offsets = isLandmark
      ? [50]
      : placementOffsets(
          count,
          trainRouteRandomUnit(`${chunkKey}:offset`),
        );
    const placements = offsets.map((offsetPercent, ordinal) => {
      const asset = chooseAsset(
        assetIds,
        trainRouteRandomUnit(`${chunkKey}:asset:${ordinal}`),
        recentIDs,
      );
      const variant = Math.floor(
        trainRouteRandomUnit(`${chunkKey}:variant:${ordinal}`) * 5,
      );
      const scale = trainSceneryScale(asset, variant);
      recentIDs.push(asset.id);
      recentIDs.splice(0, Math.max(0, recentIDs.length - rule.cooldownChunks));
      return {
        asset,
        offsetPercent,
        scale,
        collisionWidth: asset.width * scale,
        minimumSpacingPx: rule.minimumSpacingPx,
        landmark: isLandmark,
        setPiece: null,
      };
    });
    plan.push(placements);
  }
  return plan;
}

export function trainSceneryPlacementsForChunk(
  layer: TrainParallaxLayerName,
  chunk: RouteChunk,
): readonly TrainSceneryPlacement[] {
  return regionLayerPlan(chunk, layer)[chunk.regionChunkOffset] ?? [];
}

export function trainSceneryScale(
  asset: TrainSceneryAsset,
  variant: number,
): number {
  const normalizedVariant = positiveModulo(variant, 5) / 4;
  const [minimum, maximum] = asset.safeScale;
  return minimum + (maximum - minimum) * normalizedVariant;
}
