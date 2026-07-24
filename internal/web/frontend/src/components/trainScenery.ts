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
import type { TrainParallaxLayerName } from "./trainRoute";

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

function positiveModulo(value: number, divisor: number): number {
  return ((value % divisor) + divisor) % divisor;
}

function selectAsset(
  assets: readonly TrainSceneryAsset[],
  chunkIndex: number,
  variant: number,
): TrainSceneryAsset {
  return assets[positiveModulo(chunkIndex + variant, assets.length)]!;
}

export function trainSceneryAssetsForChunk(
  layer: TrainParallaxLayerName,
  chunkIndex: number,
  variant: number,
): readonly TrainSceneryAsset[] {
  switch (layer) {
    case "sky":
      return [selectAsset(TRAIN_SCENERY_CLOUDS, chunkIndex, variant)];
    case "ultra-far":
      return [selectAsset(TRAIN_SCENERY_TERRAIN, chunkIndex, variant)];
    case "far":
      return positiveModulo(chunkIndex, 7) === 0
        ? TRAIN_SCENERY_COASTS
        : [selectAsset(TRAIN_SCENERY_TERRAIN, chunkIndex + 1, variant)];
    case "midground":
      if (positiveModulo(chunkIndex, 11) === 0) return TRAIN_SCENERY_BRIDGES;
      return positiveModulo(chunkIndex, 2) === 0
        ? [selectAsset(TRAIN_SCENERY_BUILDINGS, chunkIndex / 2, variant)]
        : [
            selectAsset(
              TRAIN_SCENERY_VEGETATION,
              Math.floor(chunkIndex / 2),
              variant,
            ),
          ];
    case "near":
      return [selectAsset(TRAIN_SCENERY_PROPS, chunkIndex, variant)];
  }
}

export function trainSceneryScale(
  asset: TrainSceneryAsset,
  variant: number,
): number {
  const normalizedVariant = positiveModulo(variant, 5) / 4;
  const [minimum, maximum] = asset.safeScale;
  return minimum + (maximum - minimum) * normalizedVariant;
}
