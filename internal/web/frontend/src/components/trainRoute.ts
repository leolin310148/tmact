import {
  TRAIN_STATION_SPAN_CHUNKS,
  trainStationEventForChunk,
} from "./trainStation";
import { TRAIN_SKY_CLOUD_SPEED_RATIO } from "./trainMotion";

export const TRAIN_ROUTE_SEED_VERSION = "tmact-train-route-v1";
export const DEFAULT_TRAIN_ROUTE_SEED = "infinite-journey";
export const TRAIN_ROUTE_CHUNK_WIDTH = 320;
export const TRAIN_ROUTE_OVERSCAN_CHUNKS = 2;
export const TRAIN_PARALLAX_SEAM_OVERLAP = 2;
export const TRAIN_REGION_CHUNK_LENGTH = 9;
export const TRAIN_SET_PIECE_FOCUS_SCAN_LIMIT_CHUNKS = 20_000;
const TRAIN_REGION_MACRO_LENGTH = 32;

export const TRAIN_PARALLAX_LAYERS = [
  { name: "sky", speedRatio: TRAIN_SKY_CLOUD_SPEED_RATIO },
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
  | "town-edge"
  | "station";

export type TrainSetPieceRole = "entry" | "body" | "exit";
export type TrainSetPieceVisualVariant = 0 | 1;

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
  visualVariant: TrainSetPieceVisualVariant;
  renderLayer: TrainParallaxLayerName;
  reservedLayers: readonly TrainParallaxLayerName[];
  incompatibleWith: readonly TrainSetPieceType[];
}

export interface TrainSetPieceFocus {
  id: string;
  type: TrainSetPieceType;
  occurrence: number;
  startIndex: number;
  endIndex: number;
  span: number;
  visualVariant: TrainSetPieceVisualVariant;
  renderLayer: TrainParallaxLayerName;
  reservedLayers: readonly TrainParallaxLayerName[];
  logicalStartPx: number;
  logicalEndPx: number;
  logicalCenterPx: number;
  viewportWidth: number;
  journeyPosition: number;
  expectedVisibleSegmentIDs: readonly string[];
}

export interface TrainSetPieceScreenGeometry {
  screenLeftPx: number;
  screenRightPx: number;
  screenCenterPx: number;
  unionWidthPx: number;
  visibleLeftPx: number;
  visibleRightPx: number;
  visibleWidthPx: number;
}

export const TRAIN_SET_PIECE_DEFINITIONS = {
  bridge: {
    type: "bridge",
    span: 4,
    renderLayer: "midground",
    reservedLayers: ["midground", "near"],
    incompatibleWith: ["tunnel", "coast-reveal", "town-edge", "station"],
  },
  tunnel: {
    type: "tunnel",
    span: 3,
    renderLayer: "midground",
    reservedLayers: ["midground", "near"],
    incompatibleWith: ["bridge", "coast-reveal", "town-edge", "station"],
  },
  "coast-reveal": {
    type: "coast-reveal",
    span: 4,
    renderLayer: "far",
    reservedLayers: ["far", "midground", "near"],
    incompatibleWith: ["bridge", "tunnel", "town-edge", "station"],
  },
  "town-edge": {
    type: "town-edge",
    span: 3,
    renderLayer: "midground",
    reservedLayers: ["midground", "near"],
    incompatibleWith: ["bridge", "tunnel", "coast-reveal", "station"],
  },
  station: {
    type: "station",
    span: TRAIN_STATION_SPAN_CHUNKS,
    renderLayer: "near",
    reservedLayers: ["midground", "near"],
    incompatibleWith: ["bridge", "tunnel", "coast-reveal", "town-edge"],
  },
} as const satisfies Record<TrainSetPieceType, TrainSetPieceDefinition>;

export const TRAIN_SET_PIECE_VISUAL_VARIANT_COUNT = {
  bridge: 2,
  tunnel: 2,
  "coast-reveal": 2,
  "town-edge": 2,
  station: 1,
} as const satisfies Record<TrainSetPieceType, number>;

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

export function trainSetPieceVisualVariant(
  seed: string,
  setPieceID: string,
  type: TrainSetPieceType,
  seedVersion = TRAIN_ROUTE_SEED_VERSION,
): TrainSetPieceVisualVariant {
  const count = TRAIN_SET_PIECE_VISUAL_VARIANT_COUNT[type];
  if (count === 1) return 0;
  const resolvedSeed = seed || DEFAULT_TRAIN_ROUTE_SEED;
  return Math.floor(
    trainRouteRandomUnit(
      `${seedVersion}:${resolvedSeed}:set-piece-visual:${setPieceID}`,
    ) * count,
  ) as TrainSetPieceVisualVariant;
}

export function trainRouteSetPieceForChunk(
  seed: string,
  chunkIndex: number,
  seedVersion = TRAIN_ROUTE_SEED_VERSION,
): TrainSetPieceSegment | null {
  assertInteger(chunkIndex, "route chunk index");
  const resolvedSeed = seed || DEFAULT_TRAIN_ROUTE_SEED;
  const region = trainRegionForChunk(resolvedSeed, chunkIndex, seedVersion);
  const station = trainStationEventForChunk(resolvedSeed, chunkIndex, {
    chunkWidth: TRAIN_ROUTE_CHUNK_WIDTH,
    alignmentChunks: TRAIN_REGION_CHUNK_LENGTH,
  });
  if (station) {
    const segmentOffset = chunkIndex - station.startChunk;
    const id = station.id;
    return {
      id,
      type: "station",
      role:
        segmentOffset === 0
          ? "entry"
          : segmentOffset === station.spanChunks - 1
            ? "exit"
            : "body",
      startIndex: station.startChunk,
      endIndex: station.endChunk,
      span: station.spanChunks,
      segmentOffset,
      visualVariant: trainSetPieceVisualVariant(
        resolvedSeed,
        id,
        "station",
        seedVersion,
      ),
      renderLayer: "near",
      reservedLayers: ["midground", "near"],
      incompatibleWith: ["bridge", "tunnel", "coast-reveal", "town-edge"],
    };
  }
  const regionStart = region.index * TRAIN_REGION_CHUNK_LENGTH;
  if (
    trainStationEventForChunk(resolvedSeed, regionStart, {
      chunkWidth: TRAIN_ROUTE_CHUNK_WIDTH,
      alignmentChunks: TRAIN_REGION_CHUNK_LENGTH,
    })
  ) {
    return null;
  }
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

  const id = `${seedVersion}:${resolvedSeed}:set-piece:${region.index}:${type}`;
  return {
    id,
    type,
    role,
    startIndex,
    endIndex: startIndex + definition.span - 1,
    span: definition.span,
    segmentOffset,
    visualVariant: trainSetPieceVisualVariant(
      resolvedSeed,
      id,
      type,
      seedVersion,
    ),
    renderLayer: definition.renderLayer,
    reservedLayers: definition.reservedLayers,
    incompatibleWith: definition.incompatibleWith,
  };
}

function safeViewportWidth(viewportWidth: number): number {
  return Number.isFinite(viewportWidth) && viewportWidth > 0
    ? viewportWidth
    : 1;
}

function safeLayerSpeedRatio(speedRatio: number): number {
  if (!Number.isFinite(speedRatio) || speedRatio <= 0) {
    throw new Error("train set-piece layer speed ratio must be positive");
  }
  return speedRatio;
}

export function trainSetPieceFocusFromSegment(
  segment: TrainSetPieceSegment,
  viewportWidth: number,
  occurrence = 0,
): TrainSetPieceFocus {
  if (!Number.isInteger(occurrence) || occurrence < 0) {
    throw new Error("train set-piece occurrence must be a non-negative integer");
  }
  const resolvedViewportWidth = safeViewportWidth(viewportWidth);
  // Chunk coordinates mark their screen-left edge while the 320px body
  // extends toward the screen-right (the world itself travels right). The
  // composition's covered route interval therefore begins one chunk before
  // its entry coordinate and ends at the exit coordinate.
  const logicalStartPx =
    (segment.startIndex - 1) * TRAIN_ROUTE_CHUNK_WIDTH;
  const logicalEndPx = segment.endIndex * TRAIN_ROUTE_CHUNK_WIDTH;
  const logicalCenterPx = (logicalStartPx + logicalEndPx) / 2;
  return {
    id: segment.id,
    type: segment.type,
    occurrence,
    startIndex: segment.startIndex,
    endIndex: segment.endIndex,
    span: segment.span,
    visualVariant: segment.visualVariant,
    renderLayer: segment.renderLayer,
    reservedLayers: segment.reservedLayers,
    logicalStartPx,
    logicalEndPx,
    logicalCenterPx,
    viewportWidth: resolvedViewportWidth,
    journeyPosition: logicalCenterPx + resolvedViewportWidth / 2,
    expectedVisibleSegmentIDs: Array.from(
      { length: segment.span },
      (_, segmentOffset) => `${segment.id}:${segmentOffset}`,
    ),
  };
}

export function trainSetPieceFocusForOccurrence(
  seed: string,
  type: TrainSetPieceType,
  viewportWidth: number,
  occurrence = 0,
  fromChunk = 0,
  seedVersion = TRAIN_ROUTE_SEED_VERSION,
): TrainSetPieceFocus | null {
  if (!Number.isInteger(occurrence) || occurrence < 0) {
    throw new Error("train set-piece occurrence must be a non-negative integer");
  }
  assertInteger(fromChunk, "train set-piece focus start chunk");
  let matchedOccurrence = 0;
  for (
    let chunkIndex = fromChunk;
    chunkIndex < fromChunk + TRAIN_SET_PIECE_FOCUS_SCAN_LIMIT_CHUNKS;
    chunkIndex++
  ) {
    const segment = trainRouteSetPieceForChunk(seed, chunkIndex, seedVersion);
    if (segment?.type !== type || segment.role !== "entry") continue;
    if (matchedOccurrence === occurrence) {
      return trainSetPieceFocusFromSegment(
        segment,
        viewportWidth,
        occurrence,
      );
    }
    matchedOccurrence++;
  }
  return null;
}

export function trainSetPieceProjectionOffset(
  focus: TrainSetPieceFocus,
  speedRatio: number,
): number {
  return (safeLayerSpeedRatio(speedRatio) - 1) * focus.journeyPosition;
}

export function trainSetPieceProjectedCoordinate(
  focus: TrainSetPieceFocus,
  chunkIndex: number,
  speedRatio: number,
): number {
  assertInteger(chunkIndex, "train set-piece projected chunk index");
  return (
    chunkIndex * TRAIN_ROUTE_CHUNK_WIDTH +
    trainSetPieceProjectionOffset(focus, speedRatio)
  );
}

export function trainSetPieceScreenGeometry(
  focus: TrainSetPieceFocus,
  speedRatio: number,
  routePosition = focus.journeyPosition,
): TrainSetPieceScreenGeometry {
  const resolvedSpeedRatio = safeLayerSpeedRatio(speedRatio);
  const safeRoutePosition =
    Number.isFinite(routePosition) && routePosition > 0 ? routePosition : 0;
  const projectionOffset = trainSetPieceProjectionOffset(
    focus,
    resolvedSpeedRatio,
  );
  const screenLeftPx =
    safeRoutePosition * resolvedSpeedRatio -
    (focus.logicalEndPx + projectionOffset);
  const screenRightPx =
    safeRoutePosition * resolvedSpeedRatio -
    (focus.logicalStartPx + projectionOffset);
  const visibleLeftPx = Math.max(0, screenLeftPx);
  const visibleRightPx = Math.min(focus.viewportWidth, screenRightPx);
  return {
    screenLeftPx,
    screenRightPx,
    screenCenterPx: (screenLeftPx + screenRightPx) / 2,
    unionWidthPx: screenRightPx - screenLeftPx,
    visibleLeftPx,
    visibleRightPx,
    visibleWidthPx: Math.max(0, visibleRightPx - visibleLeftPx),
  };
}

export function trainSetPieceReservationIntersectsChunk(
  focus: TrainSetPieceFocus,
  speedRatio: number,
  chunkIndex: number,
): boolean {
  assertInteger(chunkIndex, "train set-piece reservation chunk index");
  const reservationStart =
    focus.logicalStartPx + trainSetPieceProjectionOffset(focus, speedRatio);
  const reservationEnd =
    focus.logicalEndPx + trainSetPieceProjectionOffset(focus, speedRatio);
  const chunkStart = chunkIndex * TRAIN_ROUTE_CHUNK_WIDTH;
  const chunkEnd = chunkStart + TRAIN_ROUTE_CHUNK_WIDTH;
  return chunkStart < reservationEnd && chunkEnd > reservationStart;
}

export function trainJourneyPositionForProjectedSetPieceCenter(
  focus: TrainSetPieceFocus,
  speedRatio: number,
): number {
  const resolvedSpeedRatio = safeLayerSpeedRatio(speedRatio);
  const projectedCenter =
    focus.logicalCenterPx +
    trainSetPieceProjectionOffset(focus, resolvedSpeedRatio);
  return (projectedCenter + focus.viewportWidth / 2) / resolvedSpeedRatio;
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
