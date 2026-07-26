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
  generateRouteChunk,
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
  groundInsetPx: number;
  dayNightTreatment: TrainSceneryDayNightTreatment;
  emissive?: TrainSceneryEmissiveAsset;
}

function asset(
  definition: Omit<
    TrainSceneryAsset,
    "collisionWidth" | "safeScale" | "groundInsetPx"
  > & {
    collisionWidth?: number;
    safeScale?: TrainSceneryAsset["safeScale"];
    groundInsetPx?: number;
  },
): TrainSceneryAsset {
  return {
    collisionWidth: definition.width,
    safeScale: [0.75, 1.2],
    groundInsetPx: definition.anchor === "bottom-center" ? 2 : 0,
    ...definition,
  };
}

export interface TrainSceneryDepthGrammar {
  scaleMultiplier: number;
  saturation: number;
  brightness: number;
  contrast: number;
  detailBudget: number;
  anchorToContourTolerancePx: number;
  maximumGroundInsetPx: number;
  maximumCollisionOverlapRatio: number;
}

export const TRAIN_SCENERY_DEPTH_GRAMMAR = {
  sky: {
    scaleMultiplier: 1,
    saturation: 0.76,
    brightness: 1.08,
    contrast: 0.66,
    detailBudget: 1,
    anchorToContourTolerancePx: 0,
    maximumGroundInsetPx: 0,
    maximumCollisionOverlapRatio: 0.24,
  },
  "ultra-far": {
    scaleMultiplier: 0.58,
    saturation: 0.7,
    brightness: 1.1,
    contrast: 0.68,
    detailBudget: 1,
    anchorToContourTolerancePx: 0.001,
    maximumGroundInsetPx: 3,
    maximumCollisionOverlapRatio: 0.32,
  },
  far: {
    scaleMultiplier: 0.78,
    saturation: 0.79,
    brightness: 1.03,
    contrast: 0.8,
    detailBudget: 2,
    anchorToContourTolerancePx: 0.001,
    maximumGroundInsetPx: 3,
    maximumCollisionOverlapRatio: 0.22,
  },
  midground: {
    scaleMultiplier: 0.96,
    saturation: 0.9,
    brightness: 0.97,
    contrast: 0.94,
    detailBudget: 3,
    anchorToContourTolerancePx: 0.001,
    maximumGroundInsetPx: 3,
    maximumCollisionOverlapRatio: 0.12,
  },
  near: {
    scaleMultiplier: 1.2,
    saturation: 1,
    brightness: 0.9,
    contrast: 1.1,
    detailBudget: 4,
    anchorToContourTolerancePx: 0.001,
    maximumGroundInsetPx: 3,
    maximumCollisionOverlapRatio: 0,
  },
} as const satisfies Record<
  TrainParallaxLayerName,
  TrainSceneryDepthGrammar
>;

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
    groundInsetPx: 3,
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
    groundInsetPx: 3,
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
    groundInsetPx: 0,
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
    groundInsetPx: 0,
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
    groundInsetPx: 0,
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
    groundInsetPx: 0,
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
    groundInsetPx: 0,
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
    groundInsetPx: 3,
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
    groundInsetPx: 0,
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
    groundInsetPx: 0,
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
    groundInsetPx: 0,
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
    groundInsetPx: 0,
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
    groundInsetPx: 0,
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
    "prop-warning-sign",
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

export type TrainForestMountainRegion = "forest" | "mountain";
export const TRAIN_FOREST_MOUNTAIN_MIN_REPEAT_DISTANCE_PX =
  TRAIN_ROUTE_CHUNK_WIDTH * 0.8;
export const TRAIN_TOWN_INDUSTRIAL_MIN_REPEAT_DISTANCE_PX =
  TRAIN_ROUTE_CHUNK_WIDTH * 0.68;

export type TrainForestMountainSceneryRole =
  | "forest-transition-grove"
  | "forest-canopy-cluster"
  | "forest-undergrowth"
  | "forest-stream"
  | "forest-clearing"
  | "forest-fence-line"
  | "forest-landmark-approach"
  | "forest-landmark"
  | "mountain-transition-pines"
  | "mountain-layered-ridge"
  | "mountain-cliff"
  | "mountain-rock-field"
  | "mountain-alpine-scrub"
  | "mountain-open-vista"
  | "mountain-lookout-approach"
  | "mountain-landmark";

export type TrainForestMountainSilhouetteFamily =
  | "mixed-grove"
  | "high-canopy"
  | "low-understory"
  | "stream-cleft"
  | "open-meadow"
  | "human-scale-edge"
  | "layered-alpine"
  | "sheer-cliff"
  | "broken-rock"
  | "alpine-scrub"
  | "open-ridge"
  | "lookout-perch";

export interface TrainForestMountainSceneryBeat {
  region: TrainForestMountainRegion;
  role: TrainForestMountainSceneryRole;
  silhouetteFamily: TrainForestMountainSilhouetteFamily;
  templateVariant: number;
  densityClass: "dense" | "medium" | "sparse" | "gap";
  transition: "entry" | "interior" | "exit";
  transitionNeighbor: TrainRegionName | null;
  humanScaleLandmarkEligible: boolean;
}

const TRAIN_FOREST_INTERIOR_RHYTHMS = [
  [
    "forest-canopy-cluster",
    "forest-undergrowth",
    "forest-stream",
    "forest-clearing",
    "forest-canopy-cluster",
    "forest-fence-line",
    "forest-landmark-approach",
  ],
  [
    "forest-undergrowth",
    "forest-canopy-cluster",
    "forest-fence-line",
    "forest-clearing",
    "forest-stream",
    "forest-canopy-cluster",
    "forest-landmark-approach",
  ],
  [
    "forest-canopy-cluster",
    "forest-stream",
    "forest-undergrowth",
    "forest-clearing",
    "forest-landmark-approach",
    "forest-fence-line",
    "forest-canopy-cluster",
  ],
] as const satisfies readonly (readonly TrainForestMountainSceneryRole[])[];

const TRAIN_MOUNTAIN_INTERIOR_RHYTHMS = [
  [
    "mountain-layered-ridge",
    "mountain-rock-field",
    "mountain-open-vista",
    "mountain-cliff",
    "mountain-alpine-scrub",
    "mountain-lookout-approach",
    "mountain-open-vista",
  ],
  [
    "mountain-alpine-scrub",
    "mountain-layered-ridge",
    "mountain-cliff",
    "mountain-open-vista",
    "mountain-rock-field",
    "mountain-lookout-approach",
    "mountain-layered-ridge",
  ],
  [
    "mountain-layered-ridge",
    "mountain-open-vista",
    "mountain-rock-field",
    "mountain-alpine-scrub",
    "mountain-cliff",
    "mountain-open-vista",
    "mountain-lookout-approach",
  ],
] as const satisfies readonly (readonly TrainForestMountainSceneryRole[])[];

const TRAIN_FOREST_MOUNTAIN_ROLE_GRAMMAR = {
  "forest-transition-grove": ["mixed-grove", "medium", false],
  "forest-canopy-cluster": ["high-canopy", "dense", false],
  "forest-undergrowth": ["low-understory", "medium", false],
  "forest-stream": ["stream-cleft", "sparse", false],
  "forest-clearing": ["open-meadow", "gap", false],
  "forest-fence-line": ["human-scale-edge", "medium", true],
  "forest-landmark-approach": ["mixed-grove", "sparse", true],
  "forest-landmark": ["human-scale-edge", "sparse", true],
  "mountain-transition-pines": ["mixed-grove", "medium", false],
  "mountain-layered-ridge": ["layered-alpine", "dense", false],
  "mountain-cliff": ["sheer-cliff", "medium", false],
  "mountain-rock-field": ["broken-rock", "sparse", true],
  "mountain-alpine-scrub": ["alpine-scrub", "medium", false],
  "mountain-open-vista": ["open-ridge", "gap", false],
  "mountain-lookout-approach": ["lookout-perch", "sparse", true],
  "mountain-landmark": ["lookout-perch", "sparse", true],
} as const satisfies Record<
  TrainForestMountainSceneryRole,
  readonly [
    TrainForestMountainSilhouetteFamily,
    TrainForestMountainSceneryBeat["densityClass"],
    boolean,
  ]
>;

export function trainForestMountainSceneryBeatForChunk(
  chunk: RouteChunk,
): TrainForestMountainSceneryBeat | null {
  if (chunk.region !== "forest" && chunk.region !== "mountain") return null;
  const templateVariant = Math.floor(
    trainRouteRandomUnit(
      `${chunk.seedVersion}:${chunk.routeSeed}:ordinary-rhythm:${chunk.regionIndex}`,
    ) * 3,
  );
  const transition =
    chunk.regionChunkOffset === 0
      ? "entry"
      : chunk.regionChunkOffset === TRAIN_REGION_CHUNK_LENGTH - 1
        ? "exit"
        : "interior";
  const transitionNeighbor =
    transition === "entry"
      ? trainRegionAtIndex(
          chunk.routeSeed,
          chunk.regionIndex - 1,
          chunk.seedVersion,
        )
      : transition === "exit"
        ? trainRegionAtIndex(
            chunk.routeSeed,
            chunk.regionIndex + 1,
            chunk.seedVersion,
          )
        : null;
  const role =
    transition !== "interior"
      ? chunk.region === "forest"
        ? "forest-transition-grove"
        : "mountain-transition-pines"
      : chunk.region === "forest"
        ? TRAIN_FOREST_INTERIOR_RHYTHMS[templateVariant]![
            chunk.regionChunkOffset - 1
          ]!
        : TRAIN_MOUNTAIN_INTERIOR_RHYTHMS[templateVariant]![
            chunk.regionChunkOffset - 1
          ]!;
  const [silhouetteFamily, densityClass, humanScaleLandmarkEligible] =
    TRAIN_FOREST_MOUNTAIN_ROLE_GRAMMAR[role];
  return {
    region: chunk.region,
    role,
    silhouetteFamily,
    templateVariant,
    densityClass,
    transition,
    transitionNeighbor,
    humanScaleLandmarkEligible,
  };
}

export type TrainTownIndustrialRegion = "town" | "industrial";

export type TrainTownIndustrialSceneryRole =
  | "town-transition-lane"
  | "town-residential-block"
  | "town-commercial-main-street"
  | "town-yard-cluster"
  | "town-civic-square"
  | "town-tree-lined-street"
  | "town-open-lot"
  | "town-landmark-approach"
  | "town-landmark"
  | "industrial-transition-road"
  | "industrial-shed-district"
  | "industrial-tank-yard"
  | "industrial-stack-line"
  | "industrial-crane-yard"
  | "industrial-utility-corridor"
  | "industrial-service-gap"
  | "industrial-landmark-approach"
  | "industrial-landmark";

export type TrainTownIndustrialCompositionFamily =
  | "settlement-edge"
  | "residential-block"
  | "commercial-street"
  | "fenced-yard"
  | "civic-square"
  | "tree-lined-street"
  | "open-lot"
  | "industrial-edge"
  | "shed-district"
  | "tank-farm"
  | "stack-works"
  | "crane-yard"
  | "utility-corridor"
  | "service-gap";

export type TrainTownIndustrialScaleFamily =
  | "small"
  | "medium"
  | "tall"
  | "mixed";

export type TrainTownIndustrialGroundKind =
  | "lane"
  | "residential-street"
  | "main-street"
  | "yard"
  | "civic-square"
  | "tree-lined-street"
  | "open-lot"
  | "service-road"
  | "shed-yard"
  | "tank-pad"
  | "stack-yard"
  | "crane-pad"
  | "utility-corridor"
  | "service-gap";

export type TrainTownIndustrialFixtureKind =
  | "fence"
  | "street-tree"
  | "townhouse-block"
  | "shop-awning"
  | "civic-clock"
  | "yard-gate"
  | "utility-pole"
  | "industrial-shed"
  | "vent-stack"
  | "storage-tank"
  | "furnace-stack"
  | "gantry-crane"
  | "service-pipe";

export interface TrainTownIndustrialSceneryBeat {
  region: TrainTownIndustrialRegion;
  role: TrainTownIndustrialSceneryRole;
  compositionFamily: TrainTownIndustrialCompositionFamily;
  scaleFamily: TrainTownIndustrialScaleFamily;
  groundKind: TrainTownIndustrialGroundKind;
  fixtures: readonly TrainTownIndustrialFixtureKind[];
  templateVariant: number;
  densityClass: "dense" | "medium" | "sparse" | "gap";
  transition: "entry" | "interior" | "exit";
  transitionNeighbor: TrainRegionName | null;
}

interface TrainTownIndustrialRoleGrammar {
  compositionFamily: TrainTownIndustrialCompositionFamily;
  scaleFamily: TrainTownIndustrialScaleFamily;
  groundKind: TrainTownIndustrialGroundKind;
  fixtures: readonly TrainTownIndustrialFixtureKind[];
  densityClass: TrainTownIndustrialSceneryBeat["densityClass"];
}

const TRAIN_TOWN_INTERIOR_RHYTHMS = [
  [
    "town-residential-block",
    "town-tree-lined-street",
    "town-commercial-main-street",
    "town-yard-cluster",
    "town-civic-square",
    "town-open-lot",
    "town-landmark-approach",
  ],
  [
    "town-yard-cluster",
    "town-residential-block",
    "town-commercial-main-street",
    "town-tree-lined-street",
    "town-open-lot",
    "town-civic-square",
    "town-landmark-approach",
  ],
  [
    "town-tree-lined-street",
    "town-yard-cluster",
    "town-residential-block",
    "town-civic-square",
    "town-commercial-main-street",
    "town-open-lot",
    "town-landmark-approach",
  ],
] as const satisfies readonly (readonly TrainTownIndustrialSceneryRole[])[];

const TRAIN_INDUSTRIAL_INTERIOR_RHYTHMS = [
  [
    "industrial-shed-district",
    "industrial-utility-corridor",
    "industrial-tank-yard",
    "industrial-service-gap",
    "industrial-stack-line",
    "industrial-crane-yard",
    "industrial-landmark-approach",
  ],
  [
    "industrial-tank-yard",
    "industrial-shed-district",
    "industrial-service-gap",
    "industrial-utility-corridor",
    "industrial-crane-yard",
    "industrial-stack-line",
    "industrial-landmark-approach",
  ],
  [
    "industrial-utility-corridor",
    "industrial-stack-line",
    "industrial-shed-district",
    "industrial-service-gap",
    "industrial-tank-yard",
    "industrial-crane-yard",
    "industrial-landmark-approach",
  ],
] as const satisfies readonly (readonly TrainTownIndustrialSceneryRole[])[];

const TRAIN_TOWN_INDUSTRIAL_ROLE_GRAMMAR = {
  "town-transition-lane": {
    compositionFamily: "settlement-edge",
    scaleFamily: "small",
    groundKind: "lane",
    fixtures: ["fence", "street-tree"],
    densityClass: "sparse",
  },
  "town-residential-block": {
    compositionFamily: "residential-block",
    scaleFamily: "mixed",
    groundKind: "residential-street",
    fixtures: ["townhouse-block", "street-tree"],
    densityClass: "dense",
  },
  "town-commercial-main-street": {
    compositionFamily: "commercial-street",
    scaleFamily: "medium",
    groundKind: "main-street",
    fixtures: ["shop-awning", "civic-clock"],
    densityClass: "dense",
  },
  "town-yard-cluster": {
    compositionFamily: "fenced-yard",
    scaleFamily: "small",
    groundKind: "yard",
    fixtures: ["yard-gate", "townhouse-block"],
    densityClass: "medium",
  },
  "town-civic-square": {
    compositionFamily: "civic-square",
    scaleFamily: "tall",
    groundKind: "civic-square",
    fixtures: ["civic-clock", "street-tree"],
    densityClass: "sparse",
  },
  "town-tree-lined-street": {
    compositionFamily: "tree-lined-street",
    scaleFamily: "mixed",
    groundKind: "tree-lined-street",
    fixtures: ["street-tree", "townhouse-block"],
    densityClass: "medium",
  },
  "town-open-lot": {
    compositionFamily: "open-lot",
    scaleFamily: "small",
    groundKind: "open-lot",
    fixtures: ["fence"],
    densityClass: "gap",
  },
  "town-landmark-approach": {
    compositionFamily: "settlement-edge",
    scaleFamily: "medium",
    groundKind: "lane",
    fixtures: ["townhouse-block", "civic-clock"],
    densityClass: "sparse",
  },
  "town-landmark": {
    compositionFamily: "civic-square",
    scaleFamily: "tall",
    groundKind: "civic-square",
    fixtures: ["civic-clock", "street-tree"],
    densityClass: "sparse",
  },
  "industrial-transition-road": {
    compositionFamily: "industrial-edge",
    scaleFamily: "small",
    groundKind: "service-road",
    fixtures: ["industrial-shed", "utility-pole"],
    densityClass: "sparse",
  },
  "industrial-shed-district": {
    compositionFamily: "shed-district",
    scaleFamily: "medium",
    groundKind: "shed-yard",
    fixtures: ["industrial-shed", "vent-stack"],
    densityClass: "dense",
  },
  "industrial-tank-yard": {
    compositionFamily: "tank-farm",
    scaleFamily: "mixed",
    groundKind: "tank-pad",
    fixtures: ["storage-tank", "storage-tank"],
    densityClass: "medium",
  },
  "industrial-stack-line": {
    compositionFamily: "stack-works",
    scaleFamily: "tall",
    groundKind: "stack-yard",
    fixtures: ["furnace-stack", "vent-stack"],
    densityClass: "dense",
  },
  "industrial-crane-yard": {
    compositionFamily: "crane-yard",
    scaleFamily: "tall",
    groundKind: "crane-pad",
    fixtures: ["gantry-crane", "service-pipe"],
    densityClass: "medium",
  },
  "industrial-utility-corridor": {
    compositionFamily: "utility-corridor",
    scaleFamily: "small",
    groundKind: "utility-corridor",
    fixtures: ["utility-pole", "service-pipe"],
    densityClass: "medium",
  },
  "industrial-service-gap": {
    compositionFamily: "service-gap",
    scaleFamily: "small",
    groundKind: "service-gap",
    fixtures: ["utility-pole"],
    densityClass: "gap",
  },
  "industrial-landmark-approach": {
    compositionFamily: "industrial-edge",
    scaleFamily: "medium",
    groundKind: "service-road",
    fixtures: ["storage-tank", "utility-pole"],
    densityClass: "sparse",
  },
  "industrial-landmark": {
    compositionFamily: "crane-yard",
    scaleFamily: "tall",
    groundKind: "crane-pad",
    fixtures: ["gantry-crane", "utility-pole"],
    densityClass: "sparse",
  },
} as const satisfies Record<
  TrainTownIndustrialSceneryRole,
  TrainTownIndustrialRoleGrammar
>;

export function trainTownIndustrialSceneryBeatForChunk(
  chunk: RouteChunk,
): TrainTownIndustrialSceneryBeat | null {
  if (chunk.region !== "town" && chunk.region !== "industrial") return null;
  const templateVariant = Math.floor(
    trainRouteRandomUnit(
      `${chunk.seedVersion}:${chunk.routeSeed}:built-rhythm:${chunk.regionIndex}`,
    ) * 3,
  );
  const transition =
    chunk.regionChunkOffset === 0
      ? "entry"
      : chunk.regionChunkOffset === TRAIN_REGION_CHUNK_LENGTH - 1
        ? "exit"
        : "interior";
  const transitionNeighbor =
    transition === "entry"
      ? trainRegionAtIndex(
          chunk.routeSeed,
          chunk.regionIndex - 1,
          chunk.seedVersion,
        )
      : transition === "exit"
        ? trainRegionAtIndex(
            chunk.routeSeed,
            chunk.regionIndex + 1,
            chunk.seedVersion,
          )
        : null;
  const role =
    transition !== "interior"
      ? chunk.region === "town"
        ? "town-transition-lane"
        : "industrial-transition-road"
      : chunk.region === "town"
        ? TRAIN_TOWN_INTERIOR_RHYTHMS[templateVariant]![
            chunk.regionChunkOffset - 1
          ]!
        : TRAIN_INDUSTRIAL_INTERIOR_RHYTHMS[templateVariant]![
            chunk.regionChunkOffset - 1
          ]!;
  const grammar = TRAIN_TOWN_INDUSTRIAL_ROLE_GRAMMAR[role];
  return {
    region: chunk.region,
    role,
    ...grammar,
    templateVariant,
    transition,
    transitionNeighbor,
  };
}

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
        assetIds: ["terrain-foothills"],
        density: 1,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      far: {
        assetIds: ["terrain-foothills"],
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
          "vegetation-reeds",
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
        assetIds: ["terrain-alpine"],
        density: 1,
        maxPerChunk: 1,
        minimumSpacingPx: 0,
        cooldownChunks: 0,
      },
      far: {
        assetIds: ["terrain-foothills"],
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
        cooldownChunks: 3,
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
        cooldownChunks: 3,
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
  assetScale: number;
  scale: number;
  depthScaleMultiplier: number;
  detailBudget: number;
  groundInsetPx: number;
  maximumCollisionOverlapRatio: number;
  collisionWidth: number;
  minimumSpacingPx: number;
  landmark: boolean;
  setPiece: TrainSetPieceSegment | null;
  altitudePercent?: number;
  cloudPattern?: TrainCloudPattern;
  cloudGroup?: string;
  routePositionPx?: number;
  regionalRole?:
    | TrainForestMountainSceneryRole
    | TrainTownIndustrialSceneryRole;
  silhouetteFamily?:
    | TrainForestMountainSilhouetteFamily
    | TrainTownIndustrialCompositionFamily;
  regionalScaleFamily?: TrainTownIndustrialScaleFamily;
  regionalTemplateVariant?: number;
  regionalTransition?:
    | TrainForestMountainSceneryBeat["transition"]
    | TrainTownIndustrialSceneryBeat["transition"];
}

type TrainSceneryPlacementDefinition = Omit<
  TrainSceneryPlacement,
  | "assetScale"
  | "scale"
  | "depthScaleMultiplier"
  | "detailBudget"
  | "groundInsetPx"
  | "maximumCollisionOverlapRatio"
  | "collisionWidth"
>;

function sceneryPlacement(
  layer: TrainParallaxLayerName,
  assetScale: number,
  definition: TrainSceneryPlacementDefinition,
  scaleMultiplier = TRAIN_SCENERY_DEPTH_GRAMMAR[layer].scaleMultiplier,
): TrainSceneryPlacement {
  const grammar = TRAIN_SCENERY_DEPTH_GRAMMAR[layer];
  const scale = assetScale * scaleMultiplier;
  return {
    ...definition,
    assetScale,
    scale,
    depthScaleMultiplier: scaleMultiplier,
    detailBudget: grammar.detailBudget,
    groundInsetPx: definition.asset.groundInsetPx * scale,
    maximumCollisionOverlapRatio: grammar.maximumCollisionOverlapRatio,
    collisionWidth: definition.asset.collisionWidth * scale,
  };
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

function placementOffsets(
  count: number,
  randomValue: number,
  secondaryRandomValue?: number,
): number[] {
  if (count <= 0) return [];
  if (count === 1) {
    return [
      secondaryRandomValue === undefined
        ? 25 + randomValue * 50
        : 32 + randomValue * 36,
    ];
  }
  if (secondaryRandomValue !== undefined) {
    return [
      10 + randomValue * 10,
      70 + secondaryRandomValue * 20,
    ];
  }
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
    const assetScale =
      minimumScale + candidate.scaleUnit * (maximumScale - minimumScale);
    const chunkIndex = Math.floor(
      candidate.routePositionPx / TRAIN_ROUTE_CHUNK_WIDTH,
    );
    return sceneryPlacement("sky", assetScale, {
      asset: resolvedAsset,
      offsetPercent:
        ((candidate.routePositionPx - chunkIndex * TRAIN_ROUTE_CHUNK_WIDTH) /
          TRAIN_ROUTE_CHUNK_WIDTH) *
        100,
      minimumSpacingPx: TRAIN_CLOUD_MIN_SPACING_PX,
      landmark: false,
      setPiece: null,
      altitudePercent: candidate.altitudePercent,
      cloudPattern: pattern,
      cloudGroup: candidate.group,
      routePositionPx: candidate.routePositionPx,
    });
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
  const assetScale = resolvedAsset.safeScale[1];
  const variantOffset =
    setPiece.visualVariant === 0
      ? 50
      : setPiece.role === "entry"
        ? 62
        : setPiece.role === "exit"
          ? 38
          : 50;
  return sceneryPlacement(
    layer,
    assetScale,
    {
      asset: resolvedAsset,
      offsetPercent: variantOffset,
      minimumSpacingPx: 0,
      landmark: false,
      setPiece,
    },
    1,
  );
}

function nearTrackCandidateCount(
  routeSeed: string,
  chunkIndex: number,
  seedVersion: string,
): number {
  const chunk = generateRouteChunk(routeSeed, chunkIndex, seedVersion);
  const { regionIndex, region } = chunk;
  const rule = TRAIN_REGION_SCENERY_PROFILES[region].layers.near;
  if (!rule) return 0;
  const regionOffset = positiveModulo(chunkIndex, TRAIN_REGION_CHUNK_LENGTH);
  const chunkKey =
    `${seedVersion}:${routeSeed}:region-plan:` +
    `${regionIndex}:near:chunk:${regionOffset}`;
  const defaultCount = objectCount(
    rule.density,
    rule.maxPerChunk,
    trainRouteRandomUnit(`${chunkKey}:density`),
  );
  const forestMountainCount = forestMountainCountForLayer(
    trainForestMountainSceneryBeatForChunk(chunk),
    "near",
    defaultCount,
  );
  return townIndustrialCountForLayer(
    trainTownIndustrialSceneryBeatForChunk(chunk),
    "near",
    forestMountainCount,
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

function forestMountainCountForLayer(
  beat: TrainForestMountainSceneryBeat | null,
  layer: TrainParallaxLayerName,
  fallback: number,
): number {
  if (!beat || layer === "sky") return fallback;
  const counts: Partial<Record<TrainParallaxLayerName, number>> =
    beat.role === "forest-transition-grove"
      ? { "ultra-far": 1, far: 1, midground: 1, near: 0 }
      : beat.role === "forest-canopy-cluster"
        ? { "ultra-far": 1, far: 1, midground: 2, near: 0 }
        : beat.role === "forest-undergrowth"
          ? { "ultra-far": 0, far: 1, midground: 2, near: 0 }
          : beat.role === "forest-stream"
            ? { "ultra-far": 0, far: 0, midground: 1, near: 0 }
            : beat.role === "forest-clearing"
              ? { "ultra-far": 1, far: 0, midground: 0, near: 1 }
              : beat.role === "forest-fence-line"
                ? { "ultra-far": 0, far: 1, midground: 2, near: 1 }
                : beat.role === "forest-landmark-approach"
                  ? { "ultra-far": 1, far: 0, midground: 1, near: 1 }
                  : beat.role === "mountain-transition-pines"
                    ? { "ultra-far": 0, far: 1, midground: 1, near: 0 }
                    : beat.role === "mountain-layered-ridge"
                      ? { "ultra-far": 1, far: 1, midground: 1, near: 0 }
                      : beat.role === "mountain-cliff"
                        ? { "ultra-far": 1, far: 0, midground: 1, near: 0 }
                        : beat.role === "mountain-rock-field"
                          ? { "ultra-far": 0, far: 1, midground: 0, near: 1 }
                          : beat.role === "mountain-alpine-scrub"
                            ? {
                                "ultra-far": 1,
                                far: 0,
                                midground: 1,
                                near: 0,
                              }
                            : beat.role === "mountain-open-vista"
                              ? {
                                  "ultra-far": 1,
                                  far: 0,
                                  midground: 0,
                                  near: 0,
                                }
                              : beat.role === "mountain-lookout-approach"
                                ? {
                                    "ultra-far": 1,
                                    far: 1,
                                    midground: 1,
                                    near: 1,
                                  }
                                : {};
  return counts[layer] ?? fallback;
}

function forestMountainAssetPool(
  beat: TrainForestMountainSceneryBeat | null,
  layer: TrainParallaxLayerName,
  fallback: readonly string[],
): readonly string[] {
  if (!beat) return fallback;
  if (layer === "midground") {
    switch (beat.role) {
      case "forest-transition-grove":
        return ["vegetation-conifer-tall", "vegetation-conifer-squat"];
      case "forest-canopy-cluster":
        return [
          "vegetation-conifer-tall",
          "vegetation-conifer-squat",
          "vegetation-deciduous",
        ];
      case "forest-undergrowth":
        return ["vegetation-hedgerow", "vegetation-deciduous"];
      case "forest-stream":
        return ["vegetation-reeds", "vegetation-hedgerow"];
      case "forest-fence-line":
        return ["vegetation-hedgerow", "vegetation-conifer-tall"];
      case "forest-landmark-approach":
        return ["vegetation-deciduous", "vegetation-conifer-tall"];
      case "mountain-transition-pines":
        return ["vegetation-conifer-tall", "vegetation-conifer-squat"];
      case "mountain-layered-ridge":
        return ["vegetation-coastal-pine", "vegetation-conifer-tall"];
      case "mountain-cliff":
        return ["vegetation-conifer-squat", "vegetation-coastal-pine"];
      case "mountain-alpine-scrub":
        return ["vegetation-conifer-squat", "vegetation-coastal-pine"];
      case "mountain-lookout-approach":
        return ["vegetation-conifer-tall", "vegetation-conifer-squat"];
      default:
        return fallback;
    }
  }
  if (layer === "near") {
    if (beat.role === "forest-clearing") {
      return ["prop-milepost", "prop-telegraph-pole"];
    }
    if (beat.role === "forest-fence-line") return ["prop-fence"];
    if (beat.role === "forest-landmark-approach") {
      return ["prop-maintenance-equipment", "prop-telegraph-pole"];
    }
    if (beat.role === "mountain-rock-field") {
      return ["prop-warning-sign", "prop-crossing-marker"];
    }
    if (beat.role === "mountain-lookout-approach") {
      return [
        "prop-milepost",
        "prop-warning-sign",
        "prop-maintenance-equipment",
      ];
    }
  }
  return fallback;
}

function forestMountainPlacementMetadata(
  beat: TrainForestMountainSceneryBeat | null,
  roleOverride?: TrainForestMountainSceneryRole,
): Pick<
  TrainSceneryPlacement,
  | "regionalRole"
  | "silhouetteFamily"
  | "regionalTemplateVariant"
  | "regionalTransition"
> {
  if (!beat) return {};
  const role = roleOverride ?? beat.role;
  return {
    regionalRole: role,
    silhouetteFamily:
      TRAIN_FOREST_MOUNTAIN_ROLE_GRAMMAR[role][0],
    regionalTemplateVariant: beat.templateVariant,
    regionalTransition: beat.transition,
  };
}

function townIndustrialCountForLayer(
  beat: TrainTownIndustrialSceneryBeat | null,
  layer: TrainParallaxLayerName,
  fallback: number,
): number {
  if (!beat || layer === "sky") return fallback;
  if (layer === "ultra-far" || layer === "far") return fallback;
  const counts: Partial<Record<TrainParallaxLayerName, number>> =
    beat.role === "town-transition-lane"
      ? { "ultra-far": 1, far: 1, midground: 1, near: 1 }
      : beat.role === "town-residential-block"
        ? { "ultra-far": 0, far: 1, midground: 2, near: 1 }
        : beat.role === "town-commercial-main-street"
          ? { "ultra-far": 0, far: 0, midground: 2, near: 1 }
          : beat.role === "town-yard-cluster"
            ? { "ultra-far": 0, far: 1, midground: 2, near: 1 }
            : beat.role === "town-civic-square"
              ? { "ultra-far": 1, far: 0, midground: 1, near: 0 }
              : beat.role === "town-tree-lined-street"
                ? { "ultra-far": 0, far: 1, midground: 2, near: 1 }
                : beat.role === "town-open-lot"
                  ? { "ultra-far": 1, far: 0, midground: 0, near: 1 }
                  : beat.role === "town-landmark-approach"
                    ? { "ultra-far": 0, far: 1, midground: 1, near: 1 }
                    : beat.role === "industrial-transition-road"
                      ? { "ultra-far": 1, far: 1, midground: 1, near: 1 }
                      : beat.role === "industrial-shed-district"
                        ? {
                            "ultra-far": 0,
                            far: 1,
                            midground: 2,
                            near: 1,
                          }
                        : beat.role === "industrial-tank-yard"
                          ? {
                              "ultra-far": 0,
                              far: 1,
                              midground: 2,
                              near: 0,
                            }
                          : beat.role === "industrial-stack-line"
                            ? {
                                "ultra-far": 1,
                                far: 0,
                                midground: 2,
                                near: 1,
                              }
                            : beat.role === "industrial-crane-yard"
                              ? {
                                  "ultra-far": 0,
                                  far: 1,
                                  midground: 1,
                                  near: 0,
                                }
                              : beat.role ===
                                  "industrial-utility-corridor"
                                ? {
                                    "ultra-far": 0,
                                    far: 0,
                                    midground: 1,
                                    near: 1,
                                  }
                                : beat.role === "industrial-service-gap"
                                  ? {
                                      "ultra-far": 1,
                                      far: 0,
                                      midground: 0,
                                      near: 1,
                                    }
                                  : beat.role ===
                                      "industrial-landmark-approach"
                                    ? {
                                        "ultra-far": 0,
                                        far: 1,
                                        midground: 1,
                                        near: 1,
                                      }
                                    : {};
  return counts[layer] ?? fallback;
}

function townIndustrialAssetPool(
  beat: TrainTownIndustrialSceneryBeat | null,
  layer: TrainParallaxLayerName,
  fallback: readonly string[],
): readonly string[] {
  if (!beat) return fallback;
  if (layer === "midground") {
    switch (beat.role) {
      case "town-transition-lane":
        return ["building-cottage", "vegetation-hedgerow"];
      case "town-residential-block":
        return [
          "building-rowhouse",
          "building-apartments",
          "building-cottage",
        ];
      case "town-commercial-main-street":
        return ["building-rowhouse", "building-apartments"];
      case "town-yard-cluster":
        return [
          "building-cottage",
          "vegetation-hedgerow",
          "vegetation-deciduous",
        ];
      case "town-civic-square":
        return ["building-apartments", "building-rowhouse"];
      case "town-tree-lined-street":
        return [
          "vegetation-deciduous",
          "building-rowhouse",
          "vegetation-hedgerow",
        ];
      case "town-landmark-approach":
        return [
          "building-rowhouse",
          "building-cottage",
          "vegetation-deciduous",
        ];
      case "industrial-transition-road":
        return ["building-workshop", "building-warehouse"];
      case "industrial-shed-district":
        return ["building-workshop", "building-warehouse"];
      case "industrial-tank-yard":
        return ["building-water-tower", "building-workshop"];
      case "industrial-stack-line":
        return ["building-warehouse", "building-workshop"];
      case "industrial-crane-yard":
        return ["building-warehouse", "building-water-tower"];
      case "industrial-utility-corridor":
        return ["building-workshop", "building-water-tower"];
      case "industrial-landmark-approach":
        return ["building-warehouse", "building-water-tower"];
      default:
        return fallback;
    }
  }
  if (layer === "near") {
    if (beat.region === "industrial") return fallback;
    switch (beat.role) {
      case "town-transition-lane":
        return ["prop-fence", "prop-telegraph-pole"];
      case "town-residential-block":
        return ["prop-fence", "prop-lamp-post", "prop-crossing-marker"];
      case "town-commercial-main-street":
        return ["prop-lamp-post", "prop-signal-cabinet"];
      case "town-yard-cluster":
        return ["prop-fence", "prop-telegraph-pole"];
      case "town-tree-lined-street":
        return ["prop-fence", "prop-lamp-post"];
      case "town-open-lot":
        return ["prop-fence", "prop-telegraph-pole"];
      case "town-landmark-approach":
        return ["prop-lamp-post", "prop-signal-cabinet"];
      case "industrial-transition-road":
        return [
          "prop-telegraph-pole",
          "prop-crossing-marker",
          "prop-lamp-post",
        ];
      case "industrial-shed-district":
        return [
          "prop-signal-cabinet",
          "prop-maintenance-equipment",
          "prop-telegraph-pole",
          "prop-crossing-marker",
          "prop-lamp-post",
        ];
      case "industrial-stack-line":
        return [
          "prop-warning-sign",
          "prop-signal-cabinet",
          "prop-telegraph-pole",
          "prop-crossing-marker",
          "prop-lamp-post",
        ];
      case "industrial-utility-corridor":
        return [
          "prop-telegraph-pole",
          "prop-signal-cabinet",
          "prop-crossing-marker",
          "prop-lamp-post",
          "prop-maintenance-equipment",
        ];
      case "industrial-service-gap":
        return ["prop-maintenance-equipment", "prop-crossing-marker"];
      case "industrial-landmark-approach":
        return ["prop-signal-cabinet", "prop-maintenance-equipment"];
      default:
        return fallback;
    }
  }
  return fallback;
}

function townIndustrialAssetScale(
  asset: TrainSceneryAsset,
  variant: number,
  beat: TrainTownIndustrialSceneryBeat,
): number {
  const normalizedVariant = positiveModulo(variant, 5) / 4;
  const [minimum, maximum] = asset.safeScale;
  const scaleUnit =
    beat.scaleFamily === "small"
      ? normalizedVariant * 0.35
      : beat.scaleFamily === "medium"
        ? 0.28 + normalizedVariant * 0.42
        : beat.scaleFamily === "tall"
          ? 0.62 + normalizedVariant * 0.38
          : 0.12 + normalizedVariant * 0.76;
  return minimum + (maximum - minimum) * scaleUnit;
}

function townIndustrialPlacementMetadata(
  beat: TrainTownIndustrialSceneryBeat | null,
  roleOverride?: TrainTownIndustrialSceneryRole,
): Pick<
  TrainSceneryPlacement,
  | "regionalRole"
  | "silhouetteFamily"
  | "regionalScaleFamily"
  | "regionalTemplateVariant"
  | "regionalTransition"
> {
  if (!beat) return {};
  const role = roleOverride ?? beat.role;
  const grammar = TRAIN_TOWN_INDUSTRIAL_ROLE_GRAMMAR[role];
  return {
    regionalRole: role,
    silhouetteFamily: grammar.compositionFamily,
    regionalScaleFamily: grammar.scaleFamily,
    regionalTemplateVariant: beat.templateVariant,
    regionalTransition: beat.transition,
  };
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
    const localChunk =
      localOffset === chunk.regionChunkOffset
        ? chunk
        : generateRouteChunk(
            chunk.routeSeed,
            chunk.regionIndex * TRAIN_REGION_CHUNK_LENGTH + localOffset,
            chunk.seedVersion,
          );
    const regionalBeat = trainForestMountainSceneryBeatForChunk(localChunk);
    const builtEnvironmentBeat =
      trainTownIndustrialSceneryBeatForChunk(localChunk);
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
      const landmarkScale = builtEnvironmentBeat
        ? townIndustrialAssetScale(
            landmarkAsset,
            landmarkVariant,
            builtEnvironmentBeat,
          )
        : trainSceneryAssetScale(landmarkAsset, landmarkVariant);
      plan.push([
        sceneryPlacement(layer, landmarkScale, {
          asset: landmarkAsset,
          offsetPercent: 75,
          minimumSpacingPx: 0,
          landmark: true,
          setPiece: null,
          ...forestMountainPlacementMetadata(
            regionalBeat,
            chunk.region === "forest"
              ? "forest-landmark"
              : chunk.region === "mountain"
                ? "mountain-landmark"
                : undefined,
          ),
          ...townIndustrialPlacementMetadata(
            builtEnvironmentBeat,
            chunk.region === "town"
              ? "town-landmark"
              : chunk.region === "industrial"
                ? "industrial-landmark"
                : undefined,
          ),
        }),
      ]);
      continue;
    }
    if (composition === "open" && layer === "midground") {
      plan.push([]);
      continue;
    }
    const defaultCandidateCount = objectCount(
      rule.density,
      rule.maxPerChunk,
      trainRouteRandomUnit(`${chunkKey}:density`),
    );
    const forestMountainCount = forestMountainCountForLayer(
      regionalBeat,
      layer,
      defaultCandidateCount,
    );
    const candidateCount = Math.min(
      rule.maxPerChunk,
      townIndustrialCountForLayer(
        builtEnvironmentBeat,
        layer,
        forestMountainCount,
      ),
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
      regionalBeat || builtEnvironmentBeat
        ? trainRouteRandomUnit(`${chunkKey}:offset-secondary`)
        : undefined,
    );
    const forestMountainPool = forestMountainAssetPool(
      regionalBeat,
      layer,
      rule.assetIds,
    );
    const assetPool = townIndustrialAssetPool(
      builtEnvironmentBeat,
      layer,
      forestMountainPool,
    );
    const placements = offsets.map((offsetPercent, ordinal) => {
      const asset = chooseAsset(
        assetPool,
        trainRouteRandomUnit(`${chunkKey}:asset:${ordinal}`),
        recentIDs,
      );
      const variant = Math.floor(
        trainRouteRandomUnit(`${chunkKey}:variant:${ordinal}`) * 5,
      );
      const assetScale = builtEnvironmentBeat
        ? townIndustrialAssetScale(asset, variant, builtEnvironmentBeat)
        : trainSceneryAssetScale(asset, variant);
      recentIDs.push(asset.id);
      recentIDs.splice(0, Math.max(0, recentIDs.length - rule.cooldownChunks));
      return sceneryPlacement(layer, assetScale, {
        asset,
        offsetPercent,
        minimumSpacingPx: rule.minimumSpacingPx,
        landmark: false,
        setPiece: null,
        ...forestMountainPlacementMetadata(regionalBeat),
        ...townIndustrialPlacementMetadata(builtEnvironmentBeat),
      });
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
  layer: TrainParallaxLayerName = asset.layer,
): number {
  return (
    trainSceneryAssetScale(asset, variant) *
    TRAIN_SCENERY_DEPTH_GRAMMAR[layer].scaleMultiplier
  );
}

export function trainSceneryAssetScale(
  asset: TrainSceneryAsset,
  variant: number,
): number {
  const normalizedVariant = positiveModulo(variant, 5) / 4;
  const [minimum, maximum] = asset.safeScale;
  return minimum + (maximum - minimum) * normalizedVariant;
}
