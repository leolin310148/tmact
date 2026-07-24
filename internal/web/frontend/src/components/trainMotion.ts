export const TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND = 12;
export const TRAIN_WORLD_MAX_FRAME_ELAPSED_MS = 250;
export const TRAIN_WORLD_REDUCED_STEP_INTERVAL_MS = 30_000;
export const TRAIN_WORLD_REDUCED_STEP_ELAPSED_MS = 100;

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
