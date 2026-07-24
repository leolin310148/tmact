export const TRAIN_ROUTE_SEED_VERSION = "tmact-train-route-v1";
export const DEFAULT_TRAIN_ROUTE_SEED = "infinite-journey";
export const TRAIN_ROUTE_CHUNK_WIDTH = 320;
export const TRAIN_ROUTE_OVERSCAN_CHUNKS = 2;
export const TRAIN_PARALLAX_SEAM_OVERLAP = 2;
export const TRAIN_REGION_CHUNK_LENGTH = 9;
const TRAIN_REGION_MACRO_LENGTH = 32;

export const TRAIN_PARALLAX_LAYERS = [
  { name: "sky", speedRatio: 0 },
  { name: "ultra-far", speedRatio: 0.1 },
  { name: "far", speedRatio: 0.25 },
  { name: "midground", speedRatio: 0.55 },
  { name: "near", speedRatio: 1 },
] as const;

export type TrainParallaxLayer = (typeof TRAIN_PARALLAX_LAYERS)[number];
export type TrainParallaxLayerName = TrainParallaxLayer["name"];

export type TrainRegionName =
  | "forest"
  | "mountain"
  | "town"
  | "coast"
  | "industrial";

export type TrainSetPieceType =
  | "bridge"
  | "tunnel"
  | "coast-reveal"
  | "town-edge";

export type TrainSetPieceRole = "entry" | "body" | "exit";

export interface TrainSetPieceDefinition {
  type: TrainSetPieceType;
  span: number;
  renderLayer: TrainParallaxLayerName;
  reservedLayers: readonly TrainParallaxLayerName[];
  incompatibleWith: readonly TrainSetPieceType[];
}

export interface TrainSetPieceSegment {
  id: string;
  type: TrainSetPieceType;
  role: TrainSetPieceRole;
  startIndex: number;
  endIndex: number;
  span: number;
  segmentOffset: number;
  renderLayer: TrainParallaxLayerName;
  reservedLayers: readonly TrainParallaxLayerName[];
  incompatibleWith: readonly TrainSetPieceType[];
}

export const TRAIN_SET_PIECE_DEFINITIONS = {
  bridge: {
    type: "bridge",
    span: 4,
    renderLayer: "midground",
    reservedLayers: ["midground", "near"],
    incompatibleWith: ["tunnel", "coast-reveal", "town-edge"],
  },
  tunnel: {
    type: "tunnel",
    span: 3,
    renderLayer: "midground",
    reservedLayers: ["midground", "near"],
    incompatibleWith: ["bridge", "coast-reveal", "town-edge"],
  },
  "coast-reveal": {
    type: "coast-reveal",
    span: 4,
    renderLayer: "far",
    reservedLayers: ["far", "midground", "near"],
    incompatibleWith: ["bridge", "tunnel", "town-edge"],
  },
  "town-edge": {
    type: "town-edge",
    span: 3,
    renderLayer: "midground",
    reservedLayers: ["midground", "near"],
    incompatibleWith: ["bridge", "tunnel", "coast-reveal"],
  },
} as const satisfies Record<TrainSetPieceType, TrainSetPieceDefinition>;

export interface TrainRegionProfile {
  name: TrainRegionName;
  label: string;
  transitionWeights: Readonly<Partial<Record<TrainRegionName, number>>>;
}

export const TRAIN_REGION_PROFILES = {
  forest: {
    name: "forest",
    label: "Forest",
    transitionWeights: { mountain: 8, town: 2 },
  },
  mountain: {
    name: "mountain",
    label: "Mountain foothills",
    transitionWeights: { forest: 4, town: 7, coast: 1 },
  },
  town: {
    name: "town",
    label: "Town",
    transitionWeights: { forest: 1, mountain: 2, coast: 5, industrial: 4 },
  },
  coast: {
    name: "coast",
    label: "Coast",
    transitionWeights: { mountain: 2, town: 8 },
  },
  industrial: {
    name: "industrial",
    label: "Industrial outskirts",
    transitionWeights: { forest: 1, mountain: 2, town: 8 },
  },
} as const satisfies Record<TrainRegionName, TrainRegionProfile>;

export interface RouteChunk {
  index: number;
  seedKey: string;
  routeSeed: string;
  seedVersion: string;
  variant: number;
  terrainHeight: number;
  ridgeHeight: number;
  featureOffset: number;
  region: TrainRegionName;
  regionIndex: number;
  regionChunkOffset: number;
  regionChunkLength: number;
  setPiece: TrainSetPieceSegment | null;
}

export interface RouteChunkWindowSnapshot {
  routePosition: number;
  viewportWidth: number;
  firstVisibleIndex: number;
  lastVisibleIndex: number;
  firstIndex: number;
  lastIndex: number;
  chunks: readonly RouteChunk[];
}

export function trainParallaxLayerPosition(
  routePosition: number,
  speedRatio: number,
  reducedMotion = false,
): number {
  if (reducedMotion) return 0;
  const safeRoutePosition =
    Number.isFinite(routePosition) && routePosition > 0 ? routePosition : 0;
  const safeSpeedRatio =
    Number.isFinite(speedRatio) && speedRatio > 0 ? speedRatio : 0;
  return safeRoutePosition * safeSpeedRatio;
}

export function trainParallaxLayerTransform(
  routePosition: number,
  speedRatio: number,
  reducedMotion = false,
): string {
  if (reducedMotion) return "none";
  return `translate3d(${trainParallaxLayerPosition(
    routePosition,
    speedRatio,
  ).toFixed(3)}px, 0, 0)`;
}

function hashString(value: string): number {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}

export function trainRouteRandomUnit(value: string): number {
  return createSeededRandom(hashString(value))();
}

function createSeededRandom(seed: number): () => number {
  let state = seed;
  return () => {
    state = (state + 0x6d2b79f5) | 0;
    let value = Math.imul(state ^ (state >>> 15), 1 | state);
    value ^= value + Math.imul(value ^ (value >>> 7), 61 | value);
    return ((value ^ (value >>> 14)) >>> 0) / 4294967296;
  };
}

function positiveModulo(value: number, divisor: number): number {
  return ((value % divisor) + divisor) % divisor;
}

function weightedTransition(
  current: TrainRegionName,
  randomValue: number,
  excluded?: TrainRegionName,
): TrainRegionName {
  const choices = (
    Object.entries(TRAIN_REGION_PROFILES[current].transitionWeights) as [
      TrainRegionName,
      number,
    ][]
  ).filter(([name, weight]) => name !== excluded && weight > 0);
  const totalWeight = choices.reduce((total, [, weight]) => total + weight, 0);
  let cursor = randomValue * totalWeight;

  for (const [name, weight] of choices) {
    cursor -= weight;
    if (cursor < 0) return name;
  }
  return choices.at(-1)?.[0] ?? "town";
}

export function trainRegionAtIndex(
  seed: string,
  regionIndex: number,
  seedVersion = TRAIN_ROUTE_SEED_VERSION,
): TrainRegionName {
  assertInteger(regionIndex, "train region index");
  const resolvedSeed = seed || DEFAULT_TRAIN_ROUTE_SEED;
  const macroIndex = Math.floor(regionIndex / TRAIN_REGION_MACRO_LENGTH);
  const macroOffset = positiveModulo(regionIndex, TRAIN_REGION_MACRO_LENGTH);
  let region: TrainRegionName = "town";

  for (let offset = 1; offset <= macroOffset; offset++) {
    if (offset === TRAIN_REGION_MACRO_LENGTH - 1) {
      region = "mountain";
      continue;
    }
    const exclude =
      offset === TRAIN_REGION_MACRO_LENGTH - 2 ? "mountain" : undefined;
    region = weightedTransition(
      region,
      trainRouteRandomUnit(
        `${seedVersion}:${resolvedSeed}:region:${macroIndex}:${offset}`,
      ),
      exclude,
    );
  }
  return region;
}

export function trainRegionForChunk(
  seed: string,
  chunkIndex: number,
  seedVersion = TRAIN_ROUTE_SEED_VERSION,
): {
  name: TrainRegionName;
  index: number;
  chunkOffset: number;
  chunkLength: number;
} {
  assertInteger(chunkIndex, "route chunk index");
  const regionIndex = Math.floor(chunkIndex / TRAIN_REGION_CHUNK_LENGTH);
  return {
    name: trainRegionAtIndex(seed, regionIndex, seedVersion),
    index: regionIndex,
    chunkOffset: positiveModulo(chunkIndex, TRAIN_REGION_CHUNK_LENGTH),
    chunkLength: TRAIN_REGION_CHUNK_LENGTH,
  };
}

function trainSetPieceTypeForRegion(
  seed: string,
  region: TrainRegionName,
  regionIndex: number,
  seedVersion: string,
): TrainSetPieceType {
  if (region === "coast") return "coast-reveal";
  if (region === "town" || region === "industrial") return "town-edge";
  if (region === "forest") return "bridge";
  return trainRouteRandomUnit(
    `${seedVersion}:${seed}:set-piece:${regionIndex}:mountain-type`,
  ) < 0.5
    ? "bridge"
    : "tunnel";
}

export function trainRouteSetPieceForChunk(
  seed: string,
  chunkIndex: number,
  seedVersion = TRAIN_ROUTE_SEED_VERSION,
): TrainSetPieceSegment | null {
  assertInteger(chunkIndex, "route chunk index");
  const resolvedSeed = seed || DEFAULT_TRAIN_ROUTE_SEED;
  const region = trainRegionForChunk(resolvedSeed, chunkIndex, seedVersion);
  const type = trainSetPieceTypeForRegion(
    resolvedSeed,
    region.name,
    region.index,
    seedVersion,
  );
  const definition = TRAIN_SET_PIECE_DEFINITIONS[type];
  const maximumStartOffset =
    TRAIN_REGION_CHUNK_LENGTH - definition.span - 1;
  const startOffset =
    type === "coast-reveal" || type === "town-edge"
      ? 0
      : 1 +
        Math.floor(
          trainRouteRandomUnit(
            `${seedVersion}:${resolvedSeed}:set-piece:${region.index}:start`,
          ) * maximumStartOffset,
        );
  const segmentOffset = region.chunkOffset - startOffset;
  if (segmentOffset < 0 || segmentOffset >= definition.span) return null;

  const startIndex =
    region.index * TRAIN_REGION_CHUNK_LENGTH + startOffset;
  const role: TrainSetPieceRole =
    segmentOffset === 0
      ? "entry"
      : segmentOffset === definition.span - 1
        ? "exit"
        : "body";

  return {
    id: `${seedVersion}:${resolvedSeed}:set-piece:${region.index}:${type}`,
    type,
    role,
    startIndex,
    endIndex: startIndex + definition.span - 1,
    span: definition.span,
    segmentOffset,
    renderLayer: definition.renderLayer,
    reservedLayers: definition.reservedLayers,
    incompatibleWith: definition.incompatibleWith,
  };
}

export function trainSetPiecesAreIncompatible(
  left: TrainSetPieceType,
  right: TrainSetPieceType,
): boolean {
  const incompatibleWith: readonly TrainSetPieceType[] =
    TRAIN_SET_PIECE_DEFINITIONS[left].incompatibleWith;
  return incompatibleWith.includes(right);
}

function assertInteger(value: number, name: string): void {
  if (!Number.isInteger(value)) {
    throw new Error(`${name} must be an integer`);
  }
}

export function generateRouteChunk(
  seed: string,
  index: number,
  seedVersion = TRAIN_ROUTE_SEED_VERSION,
): RouteChunk {
  assertInteger(index, "route chunk index");
  const resolvedSeed = seed || DEFAULT_TRAIN_ROUTE_SEED;
  const seedKey = `${seedVersion}:${resolvedSeed}:${index}`;
  const random = createSeededRandom(hashString(seedKey));
  const region = trainRegionForChunk(resolvedSeed, index, seedVersion);

  return {
    index,
    seedKey,
    routeSeed: resolvedSeed,
    seedVersion,
    variant: Math.floor(random() * 5),
    terrainHeight: 34 + Math.floor(random() * 25),
    ridgeHeight: 48 + Math.floor(random() * 33),
    featureOffset: 12 + Math.floor(random() * 76),
    region: region.name,
    regionIndex: region.index,
    regionChunkOffset: region.chunkOffset,
    regionChunkLength: region.chunkLength,
    setPiece: trainRouteSetPieceForChunk(resolvedSeed, index, seedVersion),
  };
}

export function routeChunkWindowRange(
  routePosition: number,
  viewportWidth: number,
  chunkWidth = TRAIN_ROUTE_CHUNK_WIDTH,
  overscan = TRAIN_ROUTE_OVERSCAN_CHUNKS,
): Omit<RouteChunkWindowSnapshot, "chunks"> {
  if (!Number.isFinite(chunkWidth) || chunkWidth <= 0) {
    throw new Error("route chunk width must be positive");
  }
  assertInteger(overscan, "route chunk overscan");
  if (overscan < 0) throw new Error("route chunk overscan must not be negative");

  const safeRoutePosition =
    Number.isFinite(routePosition) && routePosition > 0 ? routePosition : 0;
  const safeViewportWidth =
    Number.isFinite(viewportWidth) && viewportWidth > 0 ? viewportWidth : 1;
  const firstVisibleIndex =
    Math.floor((safeRoutePosition - safeViewportWidth) / chunkWidth) + 1;
  const lastVisibleIndex = Math.ceil(safeRoutePosition / chunkWidth);

  return {
    routePosition: safeRoutePosition,
    viewportWidth: safeViewportWidth,
    firstVisibleIndex,
    lastVisibleIndex,
    firstIndex: firstVisibleIndex - overscan,
    lastIndex: lastVisibleIndex + overscan,
  };
}

export class RouteChunkWindow {
  private mounted = new Map<number, RouteChunk>();
  readonly seed: string;
  readonly chunkWidth: number;
  readonly overscan: number;

  constructor(
    seed = DEFAULT_TRAIN_ROUTE_SEED,
    chunkWidth = TRAIN_ROUTE_CHUNK_WIDTH,
    overscan = TRAIN_ROUTE_OVERSCAN_CHUNKS,
  ) {
    this.seed = seed || DEFAULT_TRAIN_ROUTE_SEED;
    this.chunkWidth = chunkWidth;
    this.overscan = overscan;
    routeChunkWindowRange(0, 1, this.chunkWidth, this.overscan);
  }

  update(routePosition: number, viewportWidth: number): RouteChunkWindowSnapshot {
    const range = routeChunkWindowRange(
      routePosition,
      viewportWidth,
      this.chunkWidth,
      this.overscan,
    );
    const nextMounted = new Map<number, RouteChunk>();

    for (let index = range.firstIndex; index <= range.lastIndex; index++) {
      nextMounted.set(
        index,
        this.mounted.get(index) ?? generateRouteChunk(this.seed, index),
      );
    }

    this.mounted = nextMounted;
    return {
      ...range,
      chunks: [...nextMounted.values()],
    };
  }
}
