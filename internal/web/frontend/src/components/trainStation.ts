import { TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND } from "./trainMotion";

export const TRAIN_STATION_SPAN_CHUNKS = 6;
export const TRAIN_STATION_REGION_ALIGNMENT_CHUNKS = 9;
export const TRAIN_STATION_DEFAULT_CHUNK_WIDTH = 320;
export const TRAIN_STATION_DEFAULT_INITIAL_JOURNEY_DISTANCE = 2_880;
export const TRAIN_STATION_DEFAULT_MINIMUM_JOURNEY_DISTANCE = 3_840;
export const TRAIN_STATION_DEFAULT_APPROACH_DISTANCE = 144;
export const TRAIN_STATION_DEFAULT_DECELERATE_DISTANCE = 48;
export const TRAIN_STATION_DEFAULT_DWELL_MS = 4_000;
export const TRAIN_STATION_PLATFORM_SETTLE_MS = 250;
export const TRAIN_STATION_MAX_ELAPSED_MS = 250;

export type TrainStationState =
  | "cruise"
  | "approach"
  | "decelerate"
  | "platform"
  | "dwell"
  | "depart";

export type TrainStationDevelopmentTrigger = "approach" | "depart" | null;

export interface TrainStationScheduleOptions {
  chunkWidth: number;
  initialJourneyDistance: number;
  minimumJourneyDistance: number;
  alignmentChunks: number;
  spanChunks: number;
}

export interface TrainStationEvent {
  id: string;
  eventIndex: number;
  startChunk: number;
  endChunk: number;
  spanChunks: number;
  stopPosition: number;
}

export interface TrainStationJourneyOptions
  extends TrainStationScheduleOptions {
  cruiseSpeed: number;
  approachDistance: number;
  decelerateDistance: number;
  dwellMs: number;
  platformSettleMs: number;
  accelerationPxPerSecondSquared: number;
  decelerationPxPerSecondSquared: number;
  departureClearDistance: number;
}

export interface TrainStationJourney {
  seed: string;
  state: TrainStationState;
  routePosition: number;
  currentSpeed: number;
  targetSpeed: number;
  stateElapsedMs: number;
  station: TrainStationEvent;
}

const DEFAULT_SCHEDULE_OPTIONS: TrainStationScheduleOptions = {
  chunkWidth: TRAIN_STATION_DEFAULT_CHUNK_WIDTH,
  initialJourneyDistance: TRAIN_STATION_DEFAULT_INITIAL_JOURNEY_DISTANCE,
  minimumJourneyDistance: TRAIN_STATION_DEFAULT_MINIMUM_JOURNEY_DISTANCE,
  alignmentChunks: TRAIN_STATION_REGION_ALIGNMENT_CHUNKS,
  spanChunks: TRAIN_STATION_SPAN_CHUNKS,
};

export const DEFAULT_TRAIN_STATION_JOURNEY_OPTIONS: TrainStationJourneyOptions =
  {
    ...DEFAULT_SCHEDULE_OPTIONS,
    cruiseSpeed: TRAIN_WORLD_DEFAULT_SPEED_PX_PER_SECOND,
    approachDistance: TRAIN_STATION_DEFAULT_APPROACH_DISTANCE,
    decelerateDistance: TRAIN_STATION_DEFAULT_DECELERATE_DISTANCE,
    dwellMs: TRAIN_STATION_DEFAULT_DWELL_MS,
    platformSettleMs: TRAIN_STATION_PLATFORM_SETTLE_MS,
    accelerationPxPerSecondSquared: 12,
    decelerationPxPerSecondSquared: 10,
    departureClearDistance: 72,
  };

function resolvedScheduleOptions(
  options: Partial<TrainStationScheduleOptions> = {},
): TrainStationScheduleOptions {
  const resolved = { ...DEFAULT_SCHEDULE_OPTIONS, ...options };
  if (
    !Number.isFinite(resolved.chunkWidth) ||
    resolved.chunkWidth <= 0 ||
    !Number.isInteger(resolved.alignmentChunks) ||
    resolved.alignmentChunks <= 0 ||
    !Number.isInteger(resolved.spanChunks) ||
    resolved.spanChunks <= 0 ||
    !Number.isFinite(resolved.initialJourneyDistance) ||
    resolved.initialJourneyDistance < 0 ||
    !Number.isFinite(resolved.minimumJourneyDistance) ||
    resolved.minimumJourneyDistance < 0
  ) {
    throw new Error("invalid train station schedule options");
  }
  return resolved;
}

function stationCycleChunks(options: TrainStationScheduleOptions): number {
  const minimumGapChunks = Math.ceil(
    options.minimumJourneyDistance / options.chunkWidth,
  );
  return (
    Math.ceil(
      (options.spanChunks + minimumGapChunks) / options.alignmentChunks,
    ) * options.alignmentChunks
  );
}

function stationPhaseChunks(options: TrainStationScheduleOptions): number {
  const alignmentDistance = options.alignmentChunks * options.chunkWidth;
  return (
    Math.ceil(options.initialJourneyDistance / alignmentDistance) *
    options.alignmentChunks
  );
}

export function trainStationEventByIndex(
  seed: string,
  eventIndex: number,
  options: Partial<TrainStationScheduleOptions> = {},
): TrainStationEvent {
  if (!Number.isInteger(eventIndex)) {
    throw new Error("train station event index must be an integer");
  }
  const resolved = resolvedScheduleOptions(options);
  const routeSeed = seed || "infinite-journey";
  const startChunk =
    stationPhaseChunks(resolved) +
    eventIndex * stationCycleChunks(resolved);
  return {
    id: `station:${routeSeed}:${eventIndex}`,
    eventIndex,
    startChunk,
    endChunk: startChunk + resolved.spanChunks - 1,
    spanChunks: resolved.spanChunks,
    stopPosition:
      (startChunk + (resolved.spanChunks - 1) / 2) * resolved.chunkWidth,
  };
}

export function trainStationEventForChunk(
  seed: string,
  chunkIndex: number,
  options: Partial<TrainStationScheduleOptions> = {},
): TrainStationEvent | null {
  if (!Number.isInteger(chunkIndex)) {
    throw new Error("train station chunk index must be an integer");
  }
  const resolved = resolvedScheduleOptions(options);
  const phase = stationPhaseChunks(resolved);
  const cycle = stationCycleChunks(resolved);
  const eventIndex = Math.floor((chunkIndex - phase) / cycle);
  if (eventIndex < 0) return null;
  const event = trainStationEventByIndex(seed, eventIndex, resolved);
  return chunkIndex >= event.startChunk && chunkIndex <= event.endChunk
    ? event
    : null;
}

export function trainStationEventAtOrAfter(
  seed: string,
  routePosition: number,
  options: Partial<TrainStationScheduleOptions> = {},
): TrainStationEvent {
  const resolved = resolvedScheduleOptions(options);
  const safePosition =
    Number.isFinite(routePosition) && routePosition > 0 ? routePosition : 0;
  const phase = stationPhaseChunks(resolved);
  const cycle = stationCycleChunks(resolved);
  const firstStop =
    (phase + (resolved.spanChunks - 1) / 2) * resolved.chunkWidth;
  const eventIndex = Math.max(
    0,
    Math.ceil((safePosition - firstStop) / (cycle * resolved.chunkWidth)),
  );
  return trainStationEventByIndex(seed, eventIndex, resolved);
}

export function trainStationDevelopmentTrigger(
  search: string,
): TrainStationDevelopmentTrigger {
  if (!import.meta.env.DEV) return null;
  const value = new URLSearchParams(search).get("train-station-trigger");
  return value === "approach" || value === "depart" ? value : null;
}

function resolvedJourneyOptions(
  options: Partial<TrainStationJourneyOptions> = {},
): TrainStationJourneyOptions {
  const resolved = {
    ...DEFAULT_TRAIN_STATION_JOURNEY_OPTIONS,
    ...options,
  };
  resolvedScheduleOptions(resolved);
  return resolved;
}

function effectiveStationDistances(
  options: TrainStationJourneyOptions,
): { approach: number; decelerate: number } {
  const approachSpeed = trainStationTargetSpeed(
    "approach",
    options.cruiseSpeed,
  );
  const deceleration = Math.max(
    Number.EPSILON,
    options.decelerationPxPerSecondSquared,
  );
  const stoppingDistance = (approachSpeed ** 2) / (2 * deceleration);
  const slowdownDistance =
    Math.max(0, options.cruiseSpeed ** 2 - approachSpeed ** 2) /
    (2 * deceleration);
  const decelerate = Math.max(
    options.decelerateDistance,
    stoppingDistance,
  );
  return {
    decelerate,
    approach: Math.max(
      options.approachDistance,
      decelerate + slowdownDistance + options.cruiseSpeed * 0.25,
    ),
  };
}

export function trainStationTargetSpeed(
  state: TrainStationState,
  cruiseSpeed: number,
): number {
  const safeCruise =
    Number.isFinite(cruiseSpeed) && cruiseSpeed > 0 ? cruiseSpeed : 0;
  if (state === "cruise" || state === "depart") return safeCruise;
  if (state === "approach") return safeCruise * 0.7;
  return 0;
}

export function createTrainStationJourney(
  seed: string,
  routePosition = 0,
  options: Partial<TrainStationJourneyOptions> = {},
  trigger: TrainStationDevelopmentTrigger = null,
): TrainStationJourney {
  const resolved = resolvedJourneyOptions(options);
  const station = trainStationEventAtOrAfter(seed, routePosition, resolved);
  const distances = effectiveStationDistances(resolved);
  let position =
    Number.isFinite(routePosition) && routePosition > 0 ? routePosition : 0;
  let state: TrainStationState = "cruise";
  let speed = resolved.cruiseSpeed;

  if (trigger === "approach") {
    position = station.stopPosition - distances.approach;
    state = "approach";
  } else if (trigger === "depart") {
    position = station.stopPosition;
    state = "depart";
    speed = 0;
  } else if (position >= station.stopPosition - distances.approach) {
    state = "approach";
  }

  return {
    seed: seed || "infinite-journey",
    state,
    routePosition: position,
    currentSpeed: speed,
    targetSpeed: trainStationTargetSpeed(state, resolved.cruiseSpeed),
    stateElapsedMs: 0,
    station,
  };
}

function moveToward(current: number, target: number, maximumDelta: number): number {
  if (current < target) return Math.min(target, current + maximumDelta);
  return Math.max(target, current - maximumDelta);
}

function transition(
  journey: TrainStationJourney,
  state: TrainStationState,
  options: TrainStationJourneyOptions,
): TrainStationJourney {
  return {
    ...journey,
    state,
    stateElapsedMs: 0,
    targetSpeed: trainStationTargetSpeed(state, options.cruiseSpeed),
  };
}

export function advanceTrainStationJourney(
  journey: TrainStationJourney,
  elapsedMs: number,
  options: Partial<TrainStationJourneyOptions> = {},
): TrainStationJourney {
  const boundedElapsed = boundedTrainStationRouteElapsed(elapsedMs);
  return advanceTrainStationJourneyOnClock(
    journey,
    boundedElapsed,
    boundedElapsed,
    options,
  );
}

function boundedTrainStationRouteElapsed(elapsedMs: number): number {
  return Math.min(
    TRAIN_STATION_MAX_ELAPSED_MS,
    Number.isFinite(elapsedMs) && elapsedMs > 0 ? elapsedMs : 0,
  );
}

export function advanceTrainStationJourneyOnClock(
  journey: TrainStationJourney,
  routeElapsedMs: number,
  stationElapsedMs: number,
  options: Partial<TrainStationJourneyOptions> = {},
): TrainStationJourney {
  const resolved = resolvedJourneyOptions(options);
  const boundedRouteElapsed = boundedTrainStationRouteElapsed(routeElapsedMs);
  const wallClockElapsed =
    Number.isFinite(stationElapsedMs) && stationElapsedMs > 0
      ? stationElapsedMs
      : 0;
  if (boundedRouteElapsed === 0 && wallClockElapsed === 0) return journey;

  const elapsedSeconds = boundedRouteElapsed / 1_000;
  let next = { ...journey };
  const distances = effectiveStationDistances(resolved);
  const approachStart =
    next.station.stopPosition - distances.approach;
  const decelerateStart =
    next.station.stopPosition - distances.decelerate;

  if (next.state === "cruise" && next.routePosition >= approachStart) {
    next = transition(next, "approach", resolved);
  }
  if (next.state === "approach" && next.routePosition >= decelerateStart) {
    next = transition(next, "decelerate", resolved);
  }

  if (next.state === "platform") {
    next.stateElapsedMs += wallClockElapsed;
    if (next.stateElapsedMs >= resolved.platformSettleMs) {
      next = transition(next, "dwell", resolved);
    }
    return next;
  }

  if (next.state === "dwell") {
    next.stateElapsedMs += wallClockElapsed;
    if (next.stateElapsedMs >= resolved.dwellMs) {
      next = transition(next, "depart", resolved);
    }
    return next;
  }

  if (boundedRouteElapsed === 0) return next;

  let desiredSpeed = trainStationTargetSpeed(next.state, resolved.cruiseSpeed);
  let rate = resolved.accelerationPxPerSecondSquared;
  if (next.state === "decelerate") {
    const remaining = Math.max(
      Number.EPSILON,
      next.station.stopPosition - next.routePosition,
    );
    desiredSpeed = 0;
    rate = Math.min(
      resolved.decelerationPxPerSecondSquared,
      next.currentSpeed ** 2 / (2 * remaining),
    );
  } else if (desiredSpeed < next.currentSpeed) {
    rate = resolved.decelerationPxPerSecondSquared;
  }

  const previousSpeed = next.currentSpeed;
  next.currentSpeed = moveToward(
    next.currentSpeed,
    desiredSpeed,
    rate * elapsedSeconds,
  );
  next.targetSpeed = trainStationTargetSpeed(next.state, resolved.cruiseSpeed);
  next.routePosition +=
    ((previousSpeed + next.currentSpeed) / 2) * elapsedSeconds;
  next.stateElapsedMs += boundedRouteElapsed;

  if (
    next.state === "decelerate" &&
    next.routePosition >= next.station.stopPosition
  ) {
    next.routePosition = next.station.stopPosition;
    if (next.currentSpeed <= 0.05) {
      next.currentSpeed = 0;
      return transition(next, "platform", resolved);
    }
    return next;
  }

  if (
    next.state === "depart" &&
    next.currentSpeed >= resolved.cruiseSpeed &&
    next.routePosition >=
      next.station.stopPosition + resolved.departureClearDistance
  ) {
    const completedStation = next.station;
    next = transition(next, "cruise", resolved);
    next.station = trainStationEventByIndex(
      next.seed,
      completedStation.eventIndex + 1,
      resolved,
    );
  }

  const nextApproachStart =
    next.station.stopPosition - distances.approach;
  const nextDecelerateStart =
    next.station.stopPosition - distances.decelerate;
  if (next.state === "cruise" && next.routePosition >= nextApproachStart) {
    next = transition(next, "approach", resolved);
  } else if (
    next.state === "approach" &&
    next.routePosition >= nextDecelerateStart
  ) {
    next = transition(next, "decelerate", resolved);
  }

  return next;
}

export function trainStationMinimumGap(
  seed: string,
  eventIndex: number,
  options: Partial<TrainStationScheduleOptions> = {},
): number {
  const resolved = resolvedScheduleOptions(options);
  const current = trainStationEventByIndex(seed, eventIndex, resolved);
  const next = trainStationEventByIndex(seed, eventIndex + 1, resolved);
  return (next.startChunk - current.endChunk - 1) * resolved.chunkWidth;
}
