export const TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND = 24;
export const TRAIN_WORLD_MAX_FRAME_ELAPSED_MS = 250;
export const TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS = 30_000;
export const TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS = 100;
export const TRAIN_WHEEL_DIAMETER_PX = 18;
export const TRAIN_WHEEL_CIRCUMFERENCE_PX =
  Math.PI * TRAIN_WHEEL_DIAMETER_PX;
export const TRAIN_SKY_SUN_SPEED_RATIO = 0.004;
export const TRAIN_SKY_WISP_SPEED_RATIO = 0.02;
export const TRAIN_SKY_CLOUD_SPEED_RATIO = 0.06;
export const TRAIN_SKY_WRAP_OVERSCAN_PX = 192;

function positiveModulo(value: number, divisor: number): number {
  return ((value % divisor) + divisor) % divisor;
}

export function trainSkyAnchorPositionPx(
  routePosition: number,
  speedRatio: number,
  viewportWidth: number,
  initialXPercent: number,
  reducedMotion = false,
): number {
  const safeWidth =
    Number.isFinite(viewportWidth) && viewportWidth > 0 ? viewportWidth : 1;
  const safeXPercent = Number.isFinite(initialXPercent)
    ? Math.min(100, Math.max(0, initialXPercent))
    : 0;
  const initialPosition = (safeWidth * safeXPercent) / 100;
  if (reducedMotion) return initialPosition;

  const safeRoutePosition =
    Number.isFinite(routePosition) && routePosition > 0 ? routePosition : 0;
  const safeSpeedRatio =
    Number.isFinite(speedRatio) && speedRatio > 0 ? speedRatio : 0;
  const period = safeWidth + TRAIN_SKY_WRAP_OVERSCAN_PX * 2;
  return (
    positiveModulo(
      initialPosition +
        safeRoutePosition * safeSpeedRatio +
        TRAIN_SKY_WRAP_OVERSCAN_PX,
      period,
    ) - TRAIN_SKY_WRAP_OVERSCAN_PX
  );
}

export function clampTrainWorldElapsedMs(
  elapsedMs: number,
  maximumElapsedMs = TRAIN_WORLD_MAX_FRAME_ELAPSED_MS,
): number {
  if (!Number.isFinite(elapsedMs) || elapsedMs <= 0) return 0;
  if (!Number.isFinite(maximumElapsedMs) || maximumElapsedMs <= 0) return 0;
  return Math.min(elapsedMs, maximumElapsedMs);
}

export function advanceTrainWorldRoutePosition(
  routePosition: number,
  elapsedMs: number,
  speedPxPerSecond = TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND,
): number {
  const safePosition =
    Number.isFinite(routePosition) && routePosition > 0 ? routePosition : 0;
  const safeSpeed =
    Number.isFinite(speedPxPerSecond) && speedPxPerSecond > 0
      ? speedPxPerSecond
      : 0;
  return (
    safePosition +
    (clampTrainWorldElapsedMs(elapsedMs) * safeSpeed) / 1000
  );
}

export function trainWheelRotationDegrees(
  routeDistance: number,
  wheelCircumference = TRAIN_WHEEL_CIRCUMFERENCE_PX,
): number {
  const safeDistance =
    Number.isFinite(routeDistance) && routeDistance > 0 ? routeDistance : 0;
  if (
    !Number.isFinite(wheelCircumference) ||
    wheelCircumference <= 0 ||
    safeDistance === 0
  ) {
    return 0;
  }

  const travelledWithinTurn = safeDistance % wheelCircumference;
  if (travelledWithinTurn === 0) return 0;
  // The fixed locomotive faces left while the world moves right, so rolling
  // towards the left is counter-clockwise in CSS's clockwise-positive space.
  return -(travelledWithinTurn / wheelCircumference) * 360;
}
