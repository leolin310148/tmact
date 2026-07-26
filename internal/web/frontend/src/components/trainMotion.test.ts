import { describe, expect, it } from "vitest";

import {
  advanceTrainWorldRoutePosition,
  clampTrainWorldElapsedMs,
  trainSkyAnchorPositionPx,
  trainWheelRotationDegrees,
  TRAIN_SKY_CLOUD_SPEED_RATIO,
  TRAIN_SKY_SUN_SPEED_RATIO,
  TRAIN_SKY_WISP_SPEED_RATIO,
  TRAIN_SKY_WRAP_OVERSCAN_PX,
  TRAIN_WHEEL_CIRCUMFERENCE_PX,
  TRAIN_WHEEL_DIAMETER_PX,
  TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND,
  TRAIN_WORLD_MAX_FRAME_ELAPSED_MS,
  TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS,
  TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS,
} from "./trainMotion";

describe("train world motion", () => {
  it("advances from elapsed time at the configured cruise speed", () => {
    expect(advanceTrainWorldRoutePosition(10, 200)).toBe(14.8);
    expect(advanceTrainWorldRoutePosition(10, 200, 30)).toBe(16);
    expect(TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND).toBe(24);
  });

  it("clamps throttled frames and rejects invalid elapsed time", () => {
    expect(clampTrainWorldElapsedMs(10_000)).toBe(
      TRAIN_WORLD_MAX_FRAME_ELAPSED_MS,
    );
    expect(advanceTrainWorldRoutePosition(10, 10_000)).toBe(16);
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
    ).toBeLessThan(3);
  });

  it("orders bounded sky motion behind far and near scenery", () => {
    expect(TRAIN_SKY_SUN_SPEED_RATIO).toBeGreaterThan(0);
    expect(TRAIN_SKY_WISP_SPEED_RATIO).toBeGreaterThan(
      TRAIN_SKY_SUN_SPEED_RATIO,
    );
    expect(TRAIN_SKY_CLOUD_SPEED_RATIO).toBeGreaterThan(
      TRAIN_SKY_WISP_SPEED_RATIO,
    );
    expect(TRAIN_SKY_CLOUD_SPEED_RATIO).toBeLessThan(0.1);

    const routePosition = 960;
    const initial = trainSkyAnchorPositionPx(0, 0, 1_280, 50);
    const sun = trainSkyAnchorPositionPx(
      routePosition,
      TRAIN_SKY_SUN_SPEED_RATIO,
      1_280,
      50,
    );
    const wisp = trainSkyAnchorPositionPx(
      routePosition,
      TRAIN_SKY_WISP_SPEED_RATIO,
      1_280,
      50,
    );
    expect(sun - initial).toBeCloseTo(3.84);
    expect(wisp - initial).toBeCloseTo(19.2);
  });

  it("wraps sky anchors deterministically and freezes them for reduced motion", () => {
    const viewportWidth = 800;
    const initialXPercent = 70;
    const period = viewportWidth + TRAIN_SKY_WRAP_OVERSCAN_PX * 2;
    const onePeriodRoute = period / TRAIN_SKY_WISP_SPEED_RATIO;
    const initial = trainSkyAnchorPositionPx(
      0,
      TRAIN_SKY_WISP_SPEED_RATIO,
      viewportWidth,
      initialXPercent,
    );

    expect(
      trainSkyAnchorPositionPx(
        onePeriodRoute,
        TRAIN_SKY_WISP_SPEED_RATIO,
        viewportWidth,
        initialXPercent,
      ),
    ).toBeCloseTo(initial);
    expect(
      trainSkyAnchorPositionPx(
        onePeriodRoute,
        TRAIN_SKY_WISP_SPEED_RATIO,
        viewportWidth,
        initialXPercent,
      ),
    ).toBe(
      trainSkyAnchorPositionPx(
        onePeriodRoute,
        TRAIN_SKY_WISP_SPEED_RATIO,
        viewportWidth,
        initialXPercent,
      ),
    );
    expect(
      trainSkyAnchorPositionPx(
        onePeriodRoute,
        TRAIN_SKY_WISP_SPEED_RATIO,
        viewportWidth,
        initialXPercent,
        true,
      ),
    ).toBeCloseTo((viewportWidth * initialXPercent) / 100);
  });

  it("derives one bounded counter-clockwise wheel angle from route distance", () => {
    expect(TRAIN_WHEEL_DIAMETER_PX).toBe(18);
    expect(TRAIN_WHEEL_CIRCUMFERENCE_PX).toBeCloseTo(18 * Math.PI);
    expect(trainWheelRotationDegrees(0)).toBe(0);
    expect(
      trainWheelRotationDegrees(TRAIN_WHEEL_CIRCUMFERENCE_PX / 4),
    ).toBeCloseTo(-90);
    expect(
      trainWheelRotationDegrees(TRAIN_WHEEL_CIRCUMFERENCE_PX / 2),
    ).toBeCloseTo(-180);
    expect(trainWheelRotationDegrees(TRAIN_WHEEL_CIRCUMFERENCE_PX)).toBe(0);
    expect(
      trainWheelRotationDegrees(
        TRAIN_WHEEL_CIRCUMFERENCE_PX * 10 + TRAIN_WHEEL_CIRCUMFERENCE_PX / 4,
      ),
    ).toBeCloseTo(-90);
    expect(trainWheelRotationDegrees(Number.NaN)).toBe(0);
    expect(trainWheelRotationDegrees(10, 0)).toBe(0);
  });
});
