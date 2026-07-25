import bridgeTrussUrl from "../assets/train-theme/sprites/scenery/bridge-truss.png";
import buildingApartmentsUrl from "../assets/train-theme/sprites/scenery/building-apartments.png";
import buildingApartmentsEmissiveUrl from "../assets/train-theme/sprites/scenery/building-apartments-emissive.png";
import buildingCottageUrl from "../assets/train-theme/sprites/scenery/building-cottage.png";
import buildingCottageEmissiveUrl from "../assets/train-theme/sprites/scenery/building-cottage-emissive.png";
import buildingRowhouseUrl from "../assets/train-theme/sprites/scenery/building-rowhouse.png";
import buildingRowhouseEmissiveUrl from "../assets/train-theme/sprites/scenery/building-rowhouse-emissive.png";
import buildingWarehouseUrl from "../assets/train-theme/sprites/scenery/building-warehouse.png";
import buildingWarehouseEmissiveUrl from "../assets/train-theme/sprites/scenery/building-warehouse-emissive.png";
import buildingWaterTowerUrl from "../assets/train-theme/sprites/scenery/building-water-tower.png";
import buildingWaterTowerEmissiveUrl from "../assets/train-theme/sprites/scenery/building-water-tower-emissive.png";
import buildingWorkshopUrl from "../assets/train-theme/sprites/scenery/building-workshop.png";
import buildingWorkshopEmissiveUrl from "../assets/train-theme/sprites/scenery/building-workshop-emissive.png";
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
  TRAIN_ROUTE_CHUNK_WIDTH,
  trainRegionAtIndex,
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

export interface TrainSceneryEmissiveAsset {
  kind: "windows";
  fileName: string;
  src: string;
  width: number;
  height: number;
}

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
  emissive?: TrainSceneryEmissiveAsset;
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
    emissive: {
      kind: "windows",
      fileName: "building-rowhouse-emissive.png",
      src: buildingRowhouseEmissiveUrl,
      width: 46,
      height: 92,
    },
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
    emissive: {
      kind: "windows",
      fileName: "building-workshop-emissive.png",
      src: buildingWorkshopEmissiveUrl,
      width: 100,
      height: 28,
    },
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
    emissive: {
      kind: "windows",
      fileName: "building-apartments-emissive.png",
      src: buildingApartmentsEmissiveUrl,
      width: 91,
      height: 92,
    },
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
    emissive: {
      kind: "windows",
      fileName: "building-cottage-emissive.png",
      src: buildingCottageEmissiveUrl,
      width: 100,
      height: 79,
    },
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
    emissive: {
      kind: "windows",
      fileName: "building-warehouse-emissive.png",
      src: buildingWarehouseEmissiveUrl,
      width: 99,
      height: 84,
    },
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
    emissive: {
      kind: "windows",
      fileName: "building-water-tower-emissive.png",
      src: buildingWaterTowerEmissiveUrl,
      width: 56,
      height: 100,
    },
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
export const TRAIN_CLOUD_MIN_ALTITUDE_PERCENT = 10;
export const TRAIN_CLOUD_MAX_ALTITUDE_PERCENT = 42;
export const TRAIN_CLOUD_MIN_SPACING_PX = 168;
const TRAIN_CLOUD_REGION_WIDTH =
  TRAIN_REGION_CHUNK_LENGTH * TRAIN_ROUTE_CHUNK_WIDTH;
const TRAIN_CLOUD_BOUNDARY_CLEARANCE_PX = 184;

export const TRAIN_REGION_SCENERY_PROFILES = {
  forest: {
    name: "forest",
    layers: {
      sky: {
        assetIds: CLOUD_IDS,
        density: 0.55,
        maxPerChunk: 2,
        minimumSpacingPx: TRAIN_CLOUD_MIN_SPACING_PX,
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
        maxPerChunk: 2,
        minimumSpacingPx: TRAIN_CLOUD_MIN_SPACING_PX,
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
        assetIds: CLOUD_IDS,
        density: 0.38,
        maxPerChunk: 2,
        minimumSpacingPx: TRAIN_CLOUD_MIN_SPACING_PX,
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
  },
  coast: {
    name: "coast",
    layers: {
      sky: {
        assetIds: CLOUD_IDS,
        density: 0.62,
        maxPerChunk: 2,
        minimumSpacingPx: TRAIN_CLOUD_MIN_SPACING_PX,
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
        assetIds: CLOUD_IDS,
        density: 0.48,
        maxPerChunk: 2,
        minimumSpacingPx: TRAIN_CLOUD_MIN_SPACING_PX,
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
  altitudePercent?: number;
  cloudPattern?: TrainCloudPattern;
  cloudGroup?: string;
  routePositionPx?: number;
}

export type TrainCloudPattern = "open" | "grouped" | "scattered";

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

interface TrainCloudCandidate {
  routePositionPx: number;
  altitudePercent: number;
  scaleUnit: number;
  group: string;
}

function cloudRuleForRegion(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
): TrainRegionLayerRule {
  const region = trainRegionAtIndex(routeSeed, regionIndex, seedVersion);
  return TRAIN_REGION_SCENERY_PROFILES[region].layers.sky;
}

function cloudBoundaryRightClearance(
  routeSeed: string,
  boundaryRegionIndex: number,
  seedVersion: string,
): number {
  return (
    64 +
    trainRouteRandomUnit(
      `${seedVersion}:${routeSeed}:cloud-boundary:${boundaryRegionIndex}`,
    ) *
      56
  );
}

function cloudRegionBounds(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
): readonly [minimum: number, maximum: number] {
  const regionStart = regionIndex * TRAIN_CLOUD_REGION_WIDTH;
  const previousRight = cloudBoundaryRightClearance(
    routeSeed,
    regionIndex - 1,
    seedVersion,
  );
  const leftClearance = TRAIN_CLOUD_BOUNDARY_CLEARANCE_PX - previousRight;
  const rightClearance = cloudBoundaryRightClearance(
    routeSeed,
    regionIndex,
    seedVersion,
  );
  return [
    regionStart + leftClearance,
    regionStart + TRAIN_CLOUD_REGION_WIDTH - rightClearance,
  ];
}

function cloudPatternForRegion(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
): TrainCloudPattern {
  const value = trainRouteRandomUnit(
    `${seedVersion}:${routeSeed}:cloud-plan:${regionIndex}:pattern`,
  );
  if (value < 0.27) return "open";
  if (value < 0.61) return "grouped";
  return "scattered";
}

function cloudCountForRegion(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
  pattern: TrainCloudPattern,
): number {
  const previousDensity = cloudRuleForRegion(
    routeSeed,
    regionIndex - 1,
    seedVersion,
  ).density;
  const density = cloudRuleForRegion(
    routeSeed,
    regionIndex,
    seedVersion,
  ).density;
  const nextDensity = cloudRuleForRegion(
    routeSeed,
    regionIndex + 1,
    seedVersion,
  ).density;
  const blendedDensity =
    previousDensity * 0.2 + density * 0.6 + nextDensity * 0.2;
  const weatherFactor =
    0.82 +
    trainRouteRandomUnit(
      `${seedVersion}:${routeSeed}:cloud-plan:${regionIndex}:density`,
    ) *
      0.36;
  const patternFactor =
    pattern === "open" ? 0.62 : pattern === "grouped" ? 1.08 : 0.9;
  return Math.max(
    2,
    Math.min(
      7,
      Math.round(
        blendedDensity *
          TRAIN_REGION_CHUNK_LENGTH *
          weatherFactor *
          patternFactor,
      ),
    ),
  );
}

function cloudPositionOutsideGap(
  randomValue: number,
  minimum: number,
  maximum: number,
  gapStart: number,
  gapEnd: number,
): number {
  const leftWidth = Math.max(0, gapStart - minimum);
  const rightWidth = Math.max(0, maximum - gapEnd);
  const availableWidth = leftWidth + rightWidth;
  const distance = randomValue * availableWidth;
  return distance <= leftWidth
    ? minimum + distance
    : gapEnd + (distance - leftWidth);
}

function cloudCandidateFits(
  routePositionPx: number,
  candidates: readonly TrainCloudCandidate[],
): boolean {
  const chunkIndex = Math.floor(routePositionPx / TRAIN_ROUTE_CHUNK_WIDTH);
  if (
    candidates.filter(
      (candidate) =>
        Math.floor(candidate.routePositionPx / TRAIN_ROUTE_CHUNK_WIDTH) ===
        chunkIndex,
    ).length >= 2
  ) {
    return false;
  }
  return candidates.every(
    (candidate) =>
      Math.abs(candidate.routePositionPx - routePositionPx) >=
      TRAIN_CLOUD_MIN_SPACING_PX,
  );
}

function cloudCandidate(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
  ordinal: number,
  routePositionPx: number,
  group: string,
): TrainCloudCandidate {
  const key =
    `${seedVersion}:${routeSeed}:cloud-plan:${regionIndex}:` +
    `candidate:${ordinal}`;
  return {
    routePositionPx,
    altitudePercent:
      TRAIN_CLOUD_MIN_ALTITUDE_PERCENT +
      trainRouteRandomUnit(`${key}:altitude`) *
        (TRAIN_CLOUD_MAX_ALTITUDE_PERCENT -
          TRAIN_CLOUD_MIN_ALTITUDE_PERCENT),
    scaleUnit: trainRouteRandomUnit(`${key}:scale`),
    group,
  };
}

function cloudCandidatesForRegion(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
): {
  candidates: readonly TrainCloudCandidate[];
  pattern: TrainCloudPattern;
} {
  const key = `${seedVersion}:${routeSeed}:cloud-plan:${regionIndex}`;
  const pattern = cloudPatternForRegion(routeSeed, regionIndex, seedVersion);
  const count = cloudCountForRegion(
    routeSeed,
    regionIndex,
    seedVersion,
    pattern,
  );
  const [minimum, maximum] = cloudRegionBounds(
    routeSeed,
    regionIndex,
    seedVersion,
  );
  const width = maximum - minimum;
  const gapWidth =
    width *
    (0.18 + trainRouteRandomUnit(`${key}:gap-width`) * 0.15);
  const groupCenter =
    minimum +
    width *
      (0.2 + trainRouteRandomUnit(`${key}:group-center`) * 0.6);
  const gapCenter =
    pattern === "grouped"
      ? groupCenter < minimum + width / 2
        ? maximum - gapWidth / 2
        : minimum + gapWidth / 2
      : minimum +
        gapWidth / 2 +
        trainRouteRandomUnit(`${key}:gap-center`) * (width - gapWidth);
  const gapStart = gapCenter - gapWidth / 2;
  const gapEnd = gapCenter + gapWidth / 2;
  const candidates: TrainCloudCandidate[] = [];
  let ordinal = 0;

  if (pattern === "grouped") {
    const groupSize = Math.min(
      count,
      2 + Math.floor(trainRouteRandomUnit(`${key}:group-size`) * 2),
    );
    const groupSpacing =
      TRAIN_CLOUD_MIN_SPACING_PX +
      20 +
      trainRouteRandomUnit(`${key}:group-spacing`) * 44;
    for (let groupOffset = 0; groupOffset < groupSize; groupOffset++) {
      const routePositionPx =
        groupCenter +
        (groupOffset - (groupSize - 1) / 2) * groupSpacing;
      if (
        routePositionPx >= minimum &&
        routePositionPx <= maximum &&
        cloudCandidateFits(routePositionPx, candidates)
      ) {
        candidates.push(
          cloudCandidate(
            routeSeed,
            regionIndex,
            seedVersion,
            ordinal++,
            routePositionPx,
            `${key}:loose-group`,
          ),
        );
      }
    }
  }

  for (
    let attempt = 0;
    candidates.length < count && attempt < count * 32;
    attempt++
  ) {
    const routePositionPx = cloudPositionOutsideGap(
      trainRouteRandomUnit(`${key}:position:${attempt}`),
      minimum,
      maximum,
      gapStart,
      gapEnd,
    );
    if (!cloudCandidateFits(routePositionPx, candidates)) continue;
    candidates.push(
      cloudCandidate(
        routeSeed,
        regionIndex,
        seedVersion,
        ordinal++,
        routePositionPx,
        "",
      ),
    );
  }

  candidates.sort(
    (left, right) => left.routePositionPx - right.routePositionPx,
  );
  return { candidates, pattern };
}

function firstCloudAssetForRegion(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
): TrainSceneryAsset {
  const start = Math.floor(
    trainRouteRandomUnit(
      `${seedVersion}:${routeSeed}:cloud-plan:${regionIndex}:asset-start`,
    ) * CLOUD_IDS.length,
  );
  return assetForID(CLOUD_IDS[start]!);
}

function cloudPlacementsForRegion(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
): readonly TrainSceneryPlacement[] {
  const { candidates, pattern } = cloudCandidatesForRegion(
    routeSeed,
    regionIndex,
    seedVersion,
  );
  const nextFirstAsset = firstCloudAssetForRegion(
    routeSeed,
    regionIndex + 1,
    seedVersion,
  );
  const firstAsset = firstCloudAssetForRegion(
    routeSeed,
    regionIndex,
    seedVersion,
  );
  let previousAsset: TrainSceneryAsset | null = null;

  return candidates.map((candidate, ordinal) => {
    const blockedIDs = [
      previousAsset?.id,
      ordinal === candidates.length - 1 ? nextFirstAsset.id : undefined,
    ].filter((id): id is string => Boolean(id));
    const start = positiveModulo(
      CLOUD_IDS.indexOf(firstAsset.id) + ordinal,
      CLOUD_IDS.length,
    );
    const assetID =
      Array.from({ length: CLOUD_IDS.length }, (_, offset) =>
        CLOUD_IDS[positiveModulo(start + offset, CLOUD_IDS.length)]!,
      ).find((id) => !blockedIDs.includes(id)) ?? CLOUD_IDS[start]!;
    const resolvedAsset = assetForID(assetID);
    previousAsset = resolvedAsset;
    const [minimumScale, maximumScale] = resolvedAsset.safeScale;
    const scale =
      minimumScale +
      candidate.scaleUnit * (maximumScale - minimumScale);
    const chunkIndex = Math.floor(
      candidate.routePositionPx / TRAIN_ROUTE_CHUNK_WIDTH,
    );
    return {
      asset: resolvedAsset,
      offsetPercent:
        ((candidate.routePositionPx -
          chunkIndex * TRAIN_ROUTE_CHUNK_WIDTH) /
          TRAIN_ROUTE_CHUNK_WIDTH) *
        100,
      scale,
      collisionWidth: resolvedAsset.width * scale,
      minimumSpacingPx: TRAIN_CLOUD_MIN_SPACING_PX,
      landmark: false,
      setPiece: null,
      altitudePercent: candidate.altitudePercent,
      cloudPattern: pattern,
      cloudGroup: candidate.group,
      routePositionPx: candidate.routePositionPx,
    };
  });
}

export function trainCloudPlacementsForChunk(
  chunk: RouteChunk,
): readonly TrainSceneryPlacement[] {
  return cloudPlacementsForRegion(
    chunk.routeSeed,
    chunk.regionIndex,
    chunk.seedVersion,
  ).filter(
    (placement) =>
      Math.floor(placement.routePositionPx! / TRAIN_ROUTE_CHUNK_WIDTH) ===
      chunk.index,
  );
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
  if (layer === "sky") return trainCloudPlacementsForChunk(chunk);
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
