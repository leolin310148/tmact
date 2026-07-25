import { describe, expect, it, vi } from "vitest";

import {
  isTrainJourneySnapshot,
  loadTrainJourneySnapshot,
  resetTrainJourneySnapshot,
  TRAIN_JOURNEY_MAX_ROUTE_POSITION,
  TRAIN_JOURNEY_PERSIST_INTERVAL_MS,
  TRAIN_JOURNEY_SNAPSHOT_VERSION,
  TRAIN_JOURNEY_STORAGE_KEY,
  trainJourneyCheckpoint,
  trainJourneyPersistenceDue,
  writeTrainJourneySnapshot,
  type TrainJourneySnapshot,
} from "./trainJourneySnapshot";
import { createTrainStationJourney } from "./trainStation";

const SEED_VERSION = "tmact-train-route-v1";

function snapshot(
  overrides: Partial<TrainJourneySnapshot> = {},
): TrainJourneySnapshot {
  return {
    version: TRAIN_JOURNEY_SNAPSHOT_VERSION,
    seedVersion: SEED_VERSION,
    routeSeed: "snapshot-seed",
    routePosition: 12_345.5,
    ...overrides,
  };
}

describe("train journey snapshots", () => {
  it("round-trips the bounded versioned route-only schema", () => {
    const storage = new Map<string, string>();
    const adapter = {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    };

    expect(writeTrainJourneySnapshot(adapter, snapshot())).toBe(true);
    expect(JSON.parse(storage.get(TRAIN_JOURNEY_STORAGE_KEY)!)).toEqual({
      version: 1,
      seedVersion: SEED_VERSION,
      routeSeed: "snapshot-seed",
      routePosition: 12_345.5,
    });
    expect(loadTrainJourneySnapshot(adapter, SEED_VERSION)).toEqual(snapshot());
    expect(resetTrainJourneySnapshot(adapter)).toBe(true);
    expect(storage.has(TRAIN_JOURNEY_STORAGE_KEY)).toBe(false);
  });

  it.each([
    ["malformed JSON", "{"],
    ["null", "null"],
    ["old snapshot schema", JSON.stringify({ ...snapshot(), version: 0 })],
    [
      "stale route seed version",
      JSON.stringify({ ...snapshot(), seedVersion: "old-route-v0" }),
    ],
    [
      "negative position",
      JSON.stringify({ ...snapshot(), routePosition: -1 }),
    ],
    [
      "unbounded position",
      JSON.stringify({
        ...snapshot(),
        routePosition: TRAIN_JOURNEY_MAX_ROUTE_POSITION + 1,
      }),
    ],
    [
      "non-finite position",
      JSON.stringify({ ...snapshot(), routePosition: "Infinity" }),
    ],
    [
      "blank seed",
      JSON.stringify({ ...snapshot(), routeSeed: " " }),
    ],
    [
      "unexpected history",
      JSON.stringify({ ...snapshot(), history: [1, 2, 3] }),
    ],
  ])("rejects %s without throwing", (_label, serialized) => {
    const storage = {
      getItem: vi.fn(() => serialized),
      setItem: vi.fn(),
      removeItem: vi.fn(),
    };

    expect(loadTrainJourneySnapshot(storage, SEED_VERSION)).toBeNull();
  });

  it("tolerates unavailable and failing storage for load, save, and reset", () => {
    const error = new DOMException("storage denied", "SecurityError");
    const storage = {
      getItem: vi.fn(() => {
        throw error;
      }),
      setItem: vi.fn(() => {
        throw error;
      }),
      removeItem: vi.fn(() => {
        throw error;
      }),
    };

    expect(loadTrainJourneySnapshot(null, SEED_VERSION)).toBeNull();
    expect(loadTrainJourneySnapshot(storage, SEED_VERSION)).toBeNull();
    expect(writeTrainJourneySnapshot(null, snapshot())).toBe(false);
    expect(writeTrainJourneySnapshot(storage, snapshot())).toBe(false);
    expect(resetTrainJourneySnapshot(null)).toBe(false);
    expect(resetTrainJourneySnapshot(storage)).toBe(false);
  });

  it("only creates station-safe cruise checkpoints", () => {
    const cruise = createTrainStationJourney("station-safe", 1_200);
    expect(trainJourneyCheckpoint(cruise, SEED_VERSION)).toEqual({
      version: 1,
      seedVersion: SEED_VERSION,
      routeSeed: "station-safe",
      routePosition: 1_200,
    });

    for (const state of [
      "approach",
      "decelerate",
      "platform",
      "dwell",
      "depart",
    ] as const) {
      expect(
        trainJourneyCheckpoint({ ...cruise, state }, SEED_VERSION),
      ).toBeNull();
    }
  });

  it("uses a restrained monotonic persistence cadence", () => {
    expect(trainJourneyPersistenceDue(1_000, 1_000)).toBe(false);
    expect(
      trainJourneyPersistenceDue(
        1_000,
        1_000 + TRAIN_JOURNEY_PERSIST_INTERVAL_MS - 1,
      ),
    ).toBe(false);
    expect(
      trainJourneyPersistenceDue(
        1_000,
        1_000 + TRAIN_JOURNEY_PERSIST_INTERVAL_MS,
      ),
    ).toBe(true);
    expect(trainJourneyPersistenceDue(10_000, 9_999)).toBe(false);
    expect(isTrainJourneySnapshot(snapshot(), SEED_VERSION)).toBe(true);
  });
});
