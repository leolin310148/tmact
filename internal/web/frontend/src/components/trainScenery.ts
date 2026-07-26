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
import landmarkCoastLighthouseUrl from "../assets/train-theme/sprites/scenery/landmark-coast-lighthouse.png";
import landmarkForestClearingUrl from "../assets/train-theme/sprites/scenery/landmark-forest-clearing.png";
import landmarkIndustrialGantryUrl from "../assets/train-theme/sprites/scenery/landmark-industrial-gantry.png";
import landmarkMountainLookoutUrl from "../assets/train-theme/sprites/scenery/landmark-mountain-lookout.png";
import landmarkTownChurchUrl from "../assets/train-theme/sprites/scenery/landmark-town-church.png";
import propCrossingMarkerUrl from "../assets/train-theme/sprites/scenery/prop-crossing-marker.png";
import propFenceUrl from "../assets/train-theme/sprites/scenery/prop-fence.png";
import propLampPostUrl from "../assets/train-theme/sprites/scenery/prop-lamp-post.png";
import propMaintenanceEquipmentUrl from "../assets/train-theme/sprites/scenery/prop-maintenance-equipment.png";
import propMilepostUrl from "../assets/train-theme/sprites/scenery/prop-milepost.png";
import propSignalCabinetUrl from "../assets/train-theme/sprites/scenery/prop-signal-cabinet.png";
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
  "cloud" | "terrain" | "vegetation" | "building" | "bridge" | "coast" | "prop";

export type TrainSceneryAnchor = "center" | "bottom-center";

export type TrainSceneryDayNightTreatment =
  | "atmospheric-filter"
  | "emissive-windows"
  | "solid-palette-grade"
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
  collisionWidth: number;
  safeScale: readonly [minimum: number, maximum: number];
  dayNightTreatment: TrainSceneryDayNightTreatment;
  emissive?: TrainSceneryEmissiveAsset;
}

function asset(
  definition: Omit<TrainSceneryAsset, "collisionWidth" | "safeScale"> & {
    collisionWidth?: number;
    safeScale?: TrainSceneryAsset["safeScale"];
  },
): TrainSceneryAsset {
  return {
    collisionWidth: definition.width,
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
    dayNightTreatment: "solid-palette-grade",
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
    dayNightTreatment: "solid-palette-grade",
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
    dayNightTreatment: "solid-palette-grade",
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
    dayNightTreatment: "solid-palette-grade",
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
    dayNightTreatment: "solid-palette-grade",
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
    dayNightTreatment: "solid-palette-grade",
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

export const TRAIN_SCENERY_LANDMARKS = [
  asset({
    id: "landmark-forest-clearing",
    fileName: "landmark-forest-clearing.png",
    src: landmarkForestClearingUrl,
    category: "vegetation",
    layer: "midground",
    anchor: "bottom-center",
    width: 192,
    height: 96,
    collisionWidth: 176,
    safeScale: [0.62, 0.82],
    dayNightTreatment: "solid-palette-grade",
  }),
  asset({
    id: "landmark-mountain-lookout",
    fileName: "landmark-mountain-lookout.png",
    src: landmarkMountainLookoutUrl,
    category: "building",
    layer: "midground",
    anchor: "bottom-center",
    width: 176,
    height: 104,
    collisionWidth: 160,
    safeScale: [0.58, 0.78],
    dayNightTreatment: "solid-palette-grade",
  }),
  asset({
    id: "landmark-town-church",
    fileName: "landmark-town-church.png",
    src: landmarkTownChurchUrl,
    category: "building",
    layer: "midground",
    anchor: "bottom-center",
    width: 168,
    height: 112,
    collisionWidth: 150,
    safeScale: [0.58, 0.78],
    dayNightTreatment: "solid-palette-grade",
  }),
  asset({
    id: "landmark-coast-lighthouse",
    fileName: "landmark-coast-lighthouse.png",
    src: landmarkCoastLighthouseUrl,
    category: "building",
    layer: "midground",
    anchor: "bottom-center",
    width: 184,
    height: 112,
    collisionWidth: 160,
    safeScale: [0.58, 0.78],
    dayNightTreatment: "solid-palette-grade",
  }),
  asset({
    id: "landmark-industrial-gantry",
    fileName: "landmark-industrial-gantry.png",
    src: landmarkIndustrialGantryUrl,
    category: "building",
    layer: "midground",
    anchor: "bottom-center",
    width: 200,
    height: 96,
    collisionWidth: 184,
    safeScale: [0.6, 0.82],
    dayNightTreatment: "solid-palette-grade",
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
    dayNightTreatment: "solid-palette-grade",
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
    dayNightTreatment: "solid-palette-grade",
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
    dayNightTreatment: "solid-palette-grade",
  }),
  asset({
    id: "prop-milepost",
    fileName: "prop-milepost.png",
    src: propMilepostUrl,
    category: "prop",
    layer: "near",
    anchor: "bottom-center",
    width: 32,
    height: 68,
    collisionWidth: 24,
    safeScale: [0.7, 1],
    dayNightTreatment: "solid-palette-grade",
  }),
  asset({
    id: "prop-signal-cabinet",
    fileName: "prop-signal-cabinet.png",
    src: propSignalCabinetUrl,
    category: "prop",
    layer: "near",
    anchor: "bottom-center",
    width: 68,
    height: 52,
    collisionWidth: 58,
    safeScale: [0.65, 0.95],
    dayNightTreatment: "solid-palette-grade",
  }),
  asset({
    id: "prop-crossing-marker",
    fileName: "prop-crossing-marker.png",
    src: propCrossingMarkerUrl,
    category: "prop",
    layer: "near",
    anchor: "bottom-center",
    width: 54,
    height: 84,
    collisionWidth: 42,
    safeScale: [0.55, 0.85],
    dayNightTreatment: "solid-palette-grade",
  }),
  asset({
    id: "prop-lamp-post",
    fileName: "prop-lamp-post.png",
    src: propLampPostUrl,
    category: "prop",
    layer: "near",
    anchor: "bottom-center",
    width: 40,
    height: 92,
    collisionWidth: 24,
    safeScale: [0.55, 0.85],
    dayNightTreatment: "solid-palette-grade",
  }),
  asset({
    id: "prop-maintenance-equipment",
    fileName: "prop-maintenance-equipment.png",
    src: propMaintenanceEquipmentUrl,
    category: "prop",
    layer: "near",
    anchor: "bottom-center",
    width: 96,
    height: 48,
    collisionWidth: 82,
    safeScale: [0.65, 0.95],
    dayNightTreatment: "solid-palette-grade",
  }),
] as const satisfies readonly TrainSceneryAsset[];

export const TRAIN_NEAR_TRACK_PROP_POOLS = {
  forest: [
    "prop-fence",
    "prop-telegraph-pole",
    "prop-milepost",
    "prop-maintenance-equipment",
  ],
  mountain: [
    "prop-warning-sign",
    "prop-milepost",
    "prop-crossing-marker",
    "prop-maintenance-equipment",
  ],
  town: [
    "prop-telegraph-pole",
    "prop-fence",
    "prop-crossing-marker",
    "prop-lamp-post",
    "prop-signal-cabinet",
  ],
  coast: [
    "prop-fence",
    "prop-milepost",
    "prop-lamp-post",
    "prop-maintenance-equipment",
  ],
  industrial: [
    "prop-telegraph-pole",
    "prop-crossing-marker",
    "prop-lamp-post",
    "prop-signal-cabinet",
    "prop-maintenance-equipment",
  ],
} as const satisfies Record<TrainRegionName, readonly string[]>;

export const TRAIN_NEAR_TRACK_MIN_SPACING_PX = 320;
export const TRAIN_NEAR_TRACK_COOLDOWN_CHUNKS = 3;

export const TRAIN_SCENERY_ASSETS = [
  ...TRAIN_SCENERY_CLOUDS,
  ...TRAIN_SCENERY_TERRAIN,
  ...TRAIN_SCENERY_VEGETATION,
  ...TRAIN_SCENERY_BUILDINGS,
  ...TRAIN_SCENERY_LANDMARKS,
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
  landmark: {
    layer: TrainParallaxLayerName;
    assetIds: readonly string[];
    probability: number;
    maxPerRegion: 1;
    spanChunks: 2;
    edgeClearanceChunks: 1;
  };
}

export type TrainRegionComposition =
  "dense" | "open" | "landmark" | "set-piece";

export const TRAIN_REGION_OPEN_VIEW_TARGET = 2;

const CLOUD_IDS = TRAIN_SCENERY_CLOUDS.map((asset) => asset.id);
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
        density: 1.65,
        maxPerChunk: 2,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
      near: {
        assetIds: TRAIN_NEAR_TRACK_PROP_POOLS.forest,
        density: 0.34,
        maxPerChunk: 1,
        minimumSpacingPx: TRAIN_NEAR_TRACK_MIN_SPACING_PX,
        cooldownChunks: TRAIN_NEAR_TRACK_COOLDOWN_CHUNKS,
      },
    },
    landmark: {
      layer: "midground",
      assetIds: ["landmark-forest-clearing"],
      probability: 0.54,
      maxPerRegion: 1,
      spanChunks: 2,
      edgeClearanceChunks: 1,
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
        density: 0.64,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
      near: {
        assetIds: TRAIN_NEAR_TRACK_PROP_POOLS.mountain,
        density: 0.26,
        maxPerChunk: 1,
        minimumSpacingPx: TRAIN_NEAR_TRACK_MIN_SPACING_PX,
        cooldownChunks: TRAIN_NEAR_TRACK_COOLDOWN_CHUNKS,
      },
    },
    landmark: {
      layer: "midground",
      assetIds: ["landmark-mountain-lookout"],
      probability: 0.52,
      maxPerRegion: 1,
      spanChunks: 2,
      edgeClearanceChunks: 1,
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
        density: 1.7,
        maxPerChunk: 2,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
      near: {
        assetIds: TRAIN_NEAR_TRACK_PROP_POOLS.town,
        density: 0.42,
        maxPerChunk: 1,
        minimumSpacingPx: TRAIN_NEAR_TRACK_MIN_SPACING_PX,
        cooldownChunks: TRAIN_NEAR_TRACK_COOLDOWN_CHUNKS,
      },
    },
    landmark: {
      layer: "midground",
      assetIds: ["landmark-town-church"],
      probability: 0.6,
      maxPerRegion: 1,
      spanChunks: 2,
      edgeClearanceChunks: 1,
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
        density: 0.62,
        maxPerChunk: 1,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
      near: {
        assetIds: TRAIN_NEAR_TRACK_PROP_POOLS.coast,
        density: 0.28,
        maxPerChunk: 1,
        minimumSpacingPx: TRAIN_NEAR_TRACK_MIN_SPACING_PX,
        cooldownChunks: TRAIN_NEAR_TRACK_COOLDOWN_CHUNKS,
      },
    },
    landmark: {
      layer: "midground",
      assetIds: ["landmark-coast-lighthouse"],
      probability: 0.56,
      maxPerRegion: 1,
      spanChunks: 2,
      edgeClearanceChunks: 1,
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
          "building-water-tower",
        ],
        density: 1.52,
        maxPerChunk: 2,
        minimumSpacingPx: 144,
        cooldownChunks: 2,
      },
      near: {
        assetIds: TRAIN_NEAR_TRACK_PROP_POOLS.industrial,
        density: 0.46,
        maxPerChunk: 1,
        minimumSpacingPx: TRAIN_NEAR_TRACK_MIN_SPACING_PX,
        cooldownChunks: TRAIN_NEAR_TRACK_COOLDOWN_CHUNKS,
      },
    },
    landmark: {
      layer: "midground",
      assetIds: ["landmark-industrial-gantry"],
      probability: 0.55,
      maxPerRegion: 1,
      spanChunks: 2,
      edgeClearanceChunks: 1,
    },
  },
} as const satisfies Record<TrainRegionName, TrainRegionSceneryProfile>;

export type TrainNightLifeKind =
  | "forest-fireflies"
  | "mountain-lookout-glow"
  | "town-settlement-glow"
  | "coast-lighthouse-beacon"
  | "industrial-beacons";

export type TrainNightLifeOccupancy = "left" | "center" | "right";

interface TrainNightLifeOwnerRule {
  assetId: string;
  probability: number;
}

export interface TrainRegionNightLifeRule {
  kind: TrainNightLifeKind;
  owners: readonly TrainNightLifeOwnerRule[];
}

export const TRAIN_REGION_NIGHT_LIFE = {
  forest: {
    kind: "forest-fireflies",
    owners: [{ assetId: "landmark-forest-clearing", probability: 0.78 }],
  },
  mountain: {
    kind: "mountain-lookout-glow",
    owners: [{ assetId: "landmark-mountain-lookout", probability: 0.76 }],
  },
  town: {
    kind: "town-settlement-glow",
    owners: [
      { assetId: "landmark-town-church", probability: 0.74 },
      { assetId: "building-rowhouse", probability: 0.34 },
      { assetId: "building-apartments", probability: 0.3 },
      { assetId: "building-cottage", probability: 0.36 },
    ],
  },
  coast: {
    kind: "coast-lighthouse-beacon",
    owners: [{ assetId: "landmark-coast-lighthouse", probability: 0.82 }],
  },
  industrial: {
    kind: "industrial-beacons",
    owners: [
      { assetId: "landmark-industrial-gantry", probability: 0.62 },
      { assetId: "building-workshop", probability: 0.2 },
      { assetId: "building-warehouse", probability: 0.17 },
      { assetId: "building-water-tower", probability: 0.23 },
    ],
  },
} as const satisfies Record<TrainRegionName, TrainRegionNightLifeRule>;

export const TRAIN_NIGHT_LIFE_MIN_INTENSITY = 0.56;
export const TRAIN_NIGHT_LIFE_MAX_INTENSITY = 0.8;

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

export interface TrainNightLifePoint {
  xPercent: number;
  yPercent: number;
  delayMs: number;
}

export interface TrainNightLifePlan {
  kind: TrainNightLifeKind;
  region: TrainRegionName;
  ownerAssetId: string;
  intensity: number;
  variant: number;
  occupancy: TrainNightLifeOccupancy;
  points: readonly TrainNightLifePoint[];
  pairedReflection: boolean;
}

export function trainNightLifeForPlacement(
  chunk: RouteChunk,
  placement: TrainSceneryPlacement,
  ordinal: number,
): TrainNightLifePlan | null {
  const regionRule: TrainRegionNightLifeRule =
    TRAIN_REGION_NIGHT_LIFE[chunk.region];
  const ownerRule = regionRule.owners.find(
    (candidate) => candidate.assetId === placement.asset.id,
  );
  if (!ownerRule || placement.asset.layer !== "midground") return null;

  const key =
    `${chunk.seedVersion}:${chunk.routeSeed}:night-life:` +
    `${chunk.regionIndex}:${chunk.index}:${placement.asset.id}:${ordinal}`;
  if (trainRouteRandomUnit(`${key}:rarity`) >= ownerRule.probability) {
    return null;
  }

  const variant = Math.floor(trainRouteRandomUnit(`${key}:variant`) * 3);
  const pointCount = regionRule.kind === "forest-fireflies" ? 4 + variant : 0;
  const points = Array.from({ length: pointCount }, (_, pointIndex) => ({
    xPercent:
      20 + trainRouteRandomUnit(`${key}:point:${pointIndex}:x`) * 62,
    yPercent:
      24 + trainRouteRandomUnit(`${key}:point:${pointIndex}:y`) * 42,
    delayMs: Math.round(
      trainRouteRandomUnit(`${key}:point:${pointIndex}:delay`) * 2200,
    ),
  }));

  return {
    kind: regionRule.kind,
    region: chunk.region,
    ownerAssetId: placement.asset.id,
    intensity:
      TRAIN_NIGHT_LIFE_MIN_INTENSITY +
      trainRouteRandomUnit(`${key}:intensity`) *
        (TRAIN_NIGHT_LIFE_MAX_INTENSITY - TRAIN_NIGHT_LIFE_MIN_INTENSITY),
    variant,
    occupancy: (["left", "center", "right"] as const)[variant]!,
    points,
    pairedReflection: regionRule.kind === "coast-lighthouse-beacon",
  };
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
        (TRAIN_CLOUD_MAX_ALTITUDE_PERCENT - TRAIN_CLOUD_MIN_ALTITUDE_PERCENT),
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
    width * (0.18 + trainRouteRandomUnit(`${key}:gap-width`) * 0.15);
  const groupCenter =
    minimum + width * (0.2 + trainRouteRandomUnit(`${key}:group-center`) * 0.6);
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
        groupCenter + (groupOffset - (groupSize - 1) / 2) * groupSpacing;
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
      Array.from(
        { length: CLOUD_IDS.length },
        (_, offset) =>
          CLOUD_IDS[positiveModulo(start + offset, CLOUD_IDS.length)]!,
      ).find((id) => !blockedIDs.includes(id)) ?? CLOUD_IDS[start]!;
    const resolvedAsset = assetForID(assetID);
    previousAsset = resolvedAsset;
    const [minimumScale, maximumScale] = resolvedAsset.safeScale;
    const scale =
      minimumScale + candidate.scaleUnit * (maximumScale - minimumScale);
    const chunkIndex = Math.floor(
      candidate.routePositionPx / TRAIN_ROUTE_CHUNK_WIDTH,
    );
    return {
      asset: resolvedAsset,
      offsetPercent:
        ((candidate.routePositionPx - chunkIndex * TRAIN_ROUTE_CHUNK_WIDTH) /
          TRAIN_ROUTE_CHUNK_WIDTH) *
        100,
      scale,
      collisionWidth: resolvedAsset.collisionWidth * scale,
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
  const variantOffset =
    setPiece.visualVariant === 0
      ? 50
      : setPiece.role === "entry"
        ? 62
        : setPiece.role === "exit"
          ? 38
          : 50;
  return {
    asset: resolvedAsset,
    offsetPercent: variantOffset,
    scale,
    collisionWidth: resolvedAsset.collisionWidth * scale,
    minimumSpacingPx: 0,
    landmark: false,
    setPiece,
  };
}

function nearTrackCandidateCount(
  routeSeed: string,
  chunkIndex: number,
  seedVersion: string,
): number {
  const regionIndex = Math.floor(chunkIndex / TRAIN_REGION_CHUNK_LENGTH);
  const region = trainRegionAtIndex(routeSeed, regionIndex, seedVersion);
  const rule = TRAIN_REGION_SCENERY_PROFILES[region].layers.near;
  if (!rule) return 0;
  const regionOffset = positiveModulo(chunkIndex, TRAIN_REGION_CHUNK_LENGTH);
  const chunkKey =
    `${seedVersion}:${routeSeed}:region-plan:` +
    `${regionIndex}:near:chunk:${regionOffset}`;
  return objectCount(
    rule.density,
    rule.maxPerChunk,
    trainRouteRandomUnit(`${chunkKey}:density`),
  );
}

interface TrainRegionLandmarkPlan {
  startOffset: number;
  endOffset: number;
  placementOffset: number;
  assetIds: readonly string[];
}

interface TrainRegionCompositionPlan {
  landmark: TrainRegionLandmarkPlan | null;
  openViewOffsets: readonly number[];
  setPieces: readonly (TrainSetPieceSegment | null)[];
}

const TRAIN_REGION_COMPOSITION_CACHE_LIMIT = 64;
const trainRegionCompositionCache = new Map<
  string,
  TrainRegionCompositionPlan
>();

function regionSetPieceAtOffset(
  routeSeed: string,
  regionIndex: number,
  regionOffset: number,
  seedVersion: string,
): TrainSetPieceSegment | null {
  return trainRouteSetPieceForChunk(
    routeSeed,
    regionIndex * TRAIN_REGION_CHUNK_LENGTH + regionOffset,
    seedVersion,
  );
}

function regionLandmarkPlan(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
  profile: TrainRegionSceneryProfile,
  setPieces: readonly (TrainSetPieceSegment | null)[],
): TrainRegionLandmarkPlan | null {
  const { landmark } = profile;
  const key =
    `${seedVersion}:${routeSeed}:region-plan:` + `${regionIndex}:landmark`;
  if (trainRouteRandomUnit(`${key}:enabled`) >= landmark.probability) {
    return null;
  }

  const firstStart = landmark.edgeClearanceChunks;
  const lastStart =
    TRAIN_REGION_CHUNK_LENGTH -
    landmark.edgeClearanceChunks -
    landmark.spanChunks;
  const candidates = Array.from(
    { length: Math.max(0, lastStart - firstStart + 1) },
    (_, index) => firstStart + index,
  )
    .filter((startOffset) => {
      const endOffset = startOffset + landmark.spanChunks - 1;
      const clearanceStart = Math.max(0, startOffset - 1);
      const clearanceEnd = Math.min(
        TRAIN_REGION_CHUNK_LENGTH - 1,
        endOffset + 1,
      );
      for (
        let regionOffset = clearanceStart;
        regionOffset <= clearanceEnd;
        regionOffset++
      ) {
        if (setPieces[regionOffset]) {
          return false;
        }
      }
      return true;
    })
    .sort(
      (left, right) =>
        trainRouteRandomUnit(`${key}:candidate:${left}`) -
        trainRouteRandomUnit(`${key}:candidate:${right}`),
    );

  const startOffset = candidates[0];
  if (startOffset === undefined) return null;
  return {
    startOffset,
    endOffset: startOffset + landmark.spanChunks - 1,
    placementOffset: startOffset + Math.floor((landmark.spanChunks - 1) / 2),
    assetIds: landmark.assetIds,
  };
}

function regionOpenViewOffsets(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
  landmark: TrainRegionLandmarkPlan | null,
  setPieces: readonly (TrainSetPieceSegment | null)[],
): readonly number[] {
  const key =
    `${seedVersion}:${routeSeed}:region-plan:` + `${regionIndex}:open-view`;
  const candidates = Array.from(
    { length: TRAIN_REGION_CHUNK_LENGTH - 2 },
    (_, index) => index + 1,
  )
    .filter(
      (regionOffset) =>
        !setPieces[regionOffset] &&
        !(
          landmark &&
          regionOffset >= landmark.startOffset &&
          regionOffset <= landmark.endOffset
        ),
    )
    .sort(
      (left, right) =>
        trainRouteRandomUnit(`${key}:candidate:${left}`) -
        trainRouteRandomUnit(`${key}:candidate:${right}`),
    );
  const selected: number[] = [];
  for (const candidate of candidates) {
    if (selected.every((offset) => Math.abs(offset - candidate) >= 2)) {
      selected.push(candidate);
    }
    if (selected.length === TRAIN_REGION_OPEN_VIEW_TARGET) break;
  }
  return selected.sort((left, right) => left - right);
}

function regionCompositionPlan(
  routeSeed: string,
  regionIndex: number,
  seedVersion: string,
  profile: TrainRegionSceneryProfile,
  includeSetPieces = true,
): TrainRegionCompositionPlan {
  const key =
    `${seedVersion}:${routeSeed}:${regionIndex}:` +
    `${includeSetPieces ? "set-pieces" : "ordinary"}`;
  const cached = trainRegionCompositionCache.get(key);
  if (cached) {
    trainRegionCompositionCache.delete(key);
    trainRegionCompositionCache.set(key, cached);
    return cached;
  }

  const setPieces = includeSetPieces
    ? Array.from(
        { length: TRAIN_REGION_CHUNK_LENGTH },
        (_, regionOffset) =>
          regionSetPieceAtOffset(
            routeSeed,
            regionIndex,
            regionOffset,
            seedVersion,
          ),
      )
    : Array.from(
        { length: TRAIN_REGION_CHUNK_LENGTH },
        () => null as TrainSetPieceSegment | null,
      );
  const landmark = regionLandmarkPlan(
    routeSeed,
    regionIndex,
    seedVersion,
    profile,
    setPieces,
  );
  const plan = {
    landmark,
    openViewOffsets: regionOpenViewOffsets(
      routeSeed,
      regionIndex,
      seedVersion,
      landmark,
      setPieces,
    ),
    setPieces,
  };
  trainRegionCompositionCache.set(key, plan);
  if (trainRegionCompositionCache.size > TRAIN_REGION_COMPOSITION_CACHE_LIMIT) {
    const oldestKey = trainRegionCompositionCache.keys().next().value;
    if (oldestKey !== undefined) trainRegionCompositionCache.delete(oldestKey);
  }
  return plan;
}

function regionCompositionAtOffset(
  regionOffset: number,
  plan: TrainRegionCompositionPlan,
): TrainRegionComposition {
  if (plan.setPieces[regionOffset]) return "set-piece";
  if (
    plan.landmark &&
    regionOffset >= plan.landmark.startOffset &&
    regionOffset <= plan.landmark.endOffset
  ) {
    return "landmark";
  }
  return plan.openViewOffsets.includes(regionOffset) ? "open" : "dense";
}

export function trainRegionCompositionForChunk(
  chunk: RouteChunk,
): TrainRegionComposition {
  const profile: TrainRegionSceneryProfile =
    TRAIN_REGION_SCENERY_PROFILES[chunk.region];
  const plan = regionCompositionPlan(
    chunk.routeSeed,
    chunk.regionIndex,
    chunk.seedVersion,
    profile,
  );
  return regionCompositionAtOffset(chunk.regionChunkOffset, plan);
}

function regionLayerPlan(
  chunk: RouteChunk,
  layer: TrainParallaxLayerName,
  includeSetPieces = true,
): readonly (readonly TrainSceneryPlacement[])[] {
  const profile: TrainRegionSceneryProfile =
    TRAIN_REGION_SCENERY_PROFILES[chunk.region];
  const rule = profile.layers[layer];
  if (!rule) return Array.from({ length: TRAIN_REGION_CHUNK_LENGTH }, () => []);

  const regionKey =
    `${chunk.seedVersion}:${chunk.routeSeed}:region-plan:` +
    `${chunk.regionIndex}:${layer}`;
  const compositionPlan = regionCompositionPlan(
    chunk.routeSeed,
    chunk.regionIndex,
    chunk.seedVersion,
    profile,
    includeSetPieces,
  );
  const { landmark } = compositionPlan;
  const previousRegion =
    layer === "near"
      ? trainRegionAtIndex(
          chunk.routeSeed,
          chunk.regionIndex - 1,
          chunk.seedVersion,
        )
      : null;
  const previousRule = previousRegion
    ? TRAIN_REGION_SCENERY_PROFILES[previousRegion].layers.near
    : null;
  const recentIDs: string[] =
    layer === "near" && previousRule
      ? previousRule.assetIds.filter((id) => rule.assetIds.includes(id))
      : [];
  const plan: TrainSceneryPlacement[][] = [];

  for (
    let localOffset = 0;
    localOffset < TRAIN_REGION_CHUNK_LENGTH;
    localOffset++
  ) {
    const chunkKey = `${regionKey}:chunk:${localOffset}`;
    const setPiece = compositionPlan.setPieces[localOffset] ?? null;
    if (setPiece?.reservedLayers.includes(layer)) {
      const placement = setPiecePlacement(setPiece, layer);
      plan.push(placement ? [placement] : []);
      continue;
    }
    const composition = regionCompositionAtOffset(localOffset, compositionPlan);
    if (
      composition === "landmark" &&
      landmark &&
      (layer === profile.landmark.layer || layer === "near")
    ) {
      if (
        layer !== profile.landmark.layer ||
        localOffset !== landmark.placementOffset
      ) {
        plan.push([]);
        continue;
      }
      const landmarkAsset = chooseAsset(
        landmark.assetIds,
        trainRouteRandomUnit(`${chunkKey}:asset:0`),
        [],
      );
      const landmarkVariant = Math.floor(
        trainRouteRandomUnit(`${chunkKey}:variant:0`) * 5,
      );
      const landmarkScale = trainSceneryScale(landmarkAsset, landmarkVariant);
      plan.push([
        {
          asset: landmarkAsset,
          offsetPercent: 75,
          scale: landmarkScale,
          collisionWidth: landmarkAsset.collisionWidth * landmarkScale,
          minimumSpacingPx: 0,
          landmark: true,
          setPiece: null,
        },
      ]);
      continue;
    }
    if (composition === "open" && layer === "midground") {
      plan.push([]);
      continue;
    }
    const candidateCount = objectCount(
      rule.density,
      rule.maxPerChunk,
      trainRouteRandomUnit(`${chunkKey}:density`),
    );
    const absoluteChunkIndex =
      chunk.regionIndex * TRAIN_REGION_CHUNK_LENGTH + localOffset;
    const count =
      layer === "near" &&
      nearTrackCandidateCount(
        chunk.routeSeed,
        absoluteChunkIndex - 1,
        chunk.seedVersion,
      ) > 0
        ? 0
        : candidateCount;
    const offsets = placementOffsets(
      count,
      trainRouteRandomUnit(`${chunkKey}:offset`),
    );
    const placements = offsets.map((offsetPercent, ordinal) => {
      const asset = chooseAsset(
        rule.assetIds,
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
        collisionWidth: asset.collisionWidth * scale,
        minimumSpacingPx: rule.minimumSpacingPx,
        landmark: false,
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
  options: { includeSetPieces?: boolean } = {},
): readonly TrainSceneryPlacement[] {
  if (layer === "sky") return trainCloudPlacementsForChunk(chunk);
  return regionLayerPlan(
    chunk,
    layer,
    options.includeSetPieces ?? true,
  )[chunk.regionChunkOffset] ?? [];
}

export function trainSceneryScale(
  asset: TrainSceneryAsset,
  variant: number,
): number {
  const normalizedVariant = positiveModulo(variant, 5) / 4;
  const [minimum, maximum] = asset.safeScale;
  return minimum + (maximum - minimum) * normalizedVariant;
}
