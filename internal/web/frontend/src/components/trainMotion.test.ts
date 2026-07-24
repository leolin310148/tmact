import { describe, expect, it } from "vitest";

import {
  advanceTrainWorldRoutePosition,
  clampTrainWorldElapsedMs,
  TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND,
  TRAIN_WORLD_MAX_FRAME_ELAPSED_MS,
  TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS,
  TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS,
} from "./trainMotion";

describe("train world motion", () => {
  it("advances from elapsed time at the configured cruise speed", () => {
    expect(advanceTrainWorldRoutePosition(10, 200)).toBe(12.4);
    expect(advanceTrainWorldRoutePosition(10, 200, 30)).toBe(16);
    expect(TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND).toBe(12);
  });

  it("clamps throttled frames and rejects invalid elapsed time", () => {
    expect(clampTrainWorldElapsedMs(10_000)).toBe(
      TRAIN_WORLD_MAX_FRAME_ELAPSED_MS,
    );
    expect(advanceTrainWorldRoutePosition(10, 10_000)).toBe(13);
    expect(advanceTrainWorldRoutePosition(10, -1)).toBe(10);
    expect(advanceTrainWorldRoutePosition(10, Number.NaN)).toBe(10);
  });

  it("defines a restrained reduced-motion cadence", () => {
    expect(TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS).toBeGreaterThanOrEqual(10_000);
    expect(TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS).toBeLessThanOrEqual(
      TRAIN_WORLD_MAX_FRAME_ELAPSED_MS,
    );
    expect(
      advanceTrainWorldRoutePosition(0, TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS),
    ).toBeLessThan(2);
  });
});
