export const TRAIN_ROUTE_SEED_VERSION = "tmact-train-route-v1";
export const DEFAULT_TRAIN_ROUTE_SEED = "infinite-journey";
export const TRAIN_ROUTE_CHUNK_WIDTH = 320;
export const TRAIN_ROUTE_OVERSCAN_CHUNKS = 2;

export interface RouteChunk {
  index: number;
  seedKey: string;
  variant: number;
  terrainHeight: number;
  ridgeHeight: number;
  featureOffset: number;
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

function hashString(value: string): number {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
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

  return {
    index,
    seedKey,
    variant: Math.floor(random() * 5),
    terrainHeight: 34 + Math.floor(random() * 25),
    ridgeHeight: 48 + Math.floor(random() * 33),
    featureOffset: 12 + Math.floor(random() * 76),
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
