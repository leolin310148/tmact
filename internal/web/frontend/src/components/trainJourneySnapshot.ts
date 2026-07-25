import type { TrainStationJourney } from "./trainStation";

export const TRAIN_JOURNEY_SNAPSHOT_VERSION = 1;
export const TRAIN_JOURNEY_STORAGE_KEY = "tmact.trainJourney";
export const TRAIN_JOURNEY_PERSIST_INTERVAL_MS = 5_000;
export const TRAIN_JOURNEY_MAX_ROUTE_POSITION = 1_000_000_000;

export interface TrainJourneySnapshot {
  version: typeof TRAIN_JOURNEY_SNAPSHOT_VERSION;
  seedVersion: string;
  routeSeed: string;
  routePosition: number;
}

type JourneyStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;

const SNAPSHOT_KEYS = new Set([
  "version",
  "seedVersion",
  "routeSeed",
  "routePosition",
]);

function validSeed(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    value.length <= 64 &&
    value.trim() === value &&
    !/[\u0000-\u001f\u007f]/.test(value)
  );
}

export function isTrainJourneySnapshot(
  value: unknown,
  expectedSeedVersion: string,
): value is TrainJourneySnapshot {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const record = value as Record<string, unknown>;
  const keys = Object.keys(record);
  return (
    keys.length === SNAPSHOT_KEYS.size &&
    keys.every((key) => SNAPSHOT_KEYS.has(key)) &&
    record.version === TRAIN_JOURNEY_SNAPSHOT_VERSION &&
    record.seedVersion === expectedSeedVersion &&
    validSeed(record.routeSeed) &&
    typeof record.routePosition === "number" &&
    Number.isFinite(record.routePosition) &&
    record.routePosition >= 0 &&
    record.routePosition <= TRAIN_JOURNEY_MAX_ROUTE_POSITION
  );
}

export function trainJourneyStorage(): JourneyStorage | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

export function loadTrainJourneySnapshot(
  storage: JourneyStorage | null,
  expectedSeedVersion: string,
): TrainJourneySnapshot | null {
  if (!storage) return null;
  try {
    const serialized = storage.getItem(TRAIN_JOURNEY_STORAGE_KEY);
    if (!serialized) return null;
    const parsed: unknown = JSON.parse(serialized);
    return isTrainJourneySnapshot(parsed, expectedSeedVersion) ? parsed : null;
  } catch {
    return null;
  }
}

export function writeTrainJourneySnapshot(
  storage: JourneyStorage | null,
  snapshot: TrainJourneySnapshot,
): boolean {
  if (!storage || !isTrainJourneySnapshot(snapshot, snapshot.seedVersion)) {
    return false;
  }
  try {
    storage.setItem(TRAIN_JOURNEY_STORAGE_KEY, JSON.stringify(snapshot));
    return true;
  } catch {
    return false;
  }
}

export function resetTrainJourneySnapshot(
  storage: JourneyStorage | null,
): boolean {
  if (!storage) return false;
  try {
    storage.removeItem(TRAIN_JOURNEY_STORAGE_KEY);
    return true;
  } catch {
    return false;
  }
}

export function trainJourneyCheckpoint(
  journey: TrainStationJourney,
  seedVersion: string,
): TrainJourneySnapshot | null {
  if (journey.state !== "cruise") return null;
  const snapshot: TrainJourneySnapshot = {
    version: TRAIN_JOURNEY_SNAPSHOT_VERSION,
    seedVersion,
    routeSeed: journey.seed,
    routePosition: journey.routePosition,
  };
  return isTrainJourneySnapshot(snapshot, seedVersion) ? snapshot : null;
}

export function trainJourneyPersistenceDue(
  lastAttemptAtMs: number,
  nowMs: number,
): boolean {
  return (
    Number.isFinite(lastAttemptAtMs) &&
    Number.isFinite(nowMs) &&
    nowMs >= lastAttemptAtMs + TRAIN_JOURNEY_PERSIST_INTERVAL_MS
  );
}
