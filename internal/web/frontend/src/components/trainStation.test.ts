import { describe, expect, it } from "vitest";

import {
  advanceTrainStationJourney,
  advanceTrainStationJourneyOnClock,
  createTrainStationJourney,
  DEFAULT_TRAIN_STATION_JOURNEY_OPTIONS,
  TRAIN_STATION_DEFAULT_INITIAL_JOURNEY_DISTANCE,
  TRAIN_STATION_DEFAULT_DWELL_MS,
  TRAIN_STATION_DEFAULT_MINIMUM_JOURNEY_DISTANCE,
  TRAIN_STATION_REGION_ALIGNMENT_CHUNKS,
  TRAIN_STATION_SPAN_CHUNKS,
  trainStationDevelopmentTrigger,
  trainStationEventByIndex,
  trainStationEventForChunk,
  trainStationMinimumGap,
  type TrainStationJourney,
  type TrainStationState,
} from "./trainStation";

function advanceUntil(
  journey: TrainStationJourney,
  expected: TrainStationState,
  limit = 20_000,
): TrainStationJourney {
  let current = journey;
  for (let step = 0; step < limit && current.state !== expected; step++) {
    current = advanceTrainStationJourney(current, 250);
  }
  expect(current.state).toBe(expected);
  return current;
}

describe("train station journey", () => {
  it("discovers the default-seed station after an exact normal-speed lead-in", () => {
    const first = trainStationEventByIndex("infinite-journey", 0);
    const approachPosition =
      first.stopPosition -
      DEFAULT_TRAIN_STATION_JOURNEY_OPTIONS.approachDistance;
    const approachSeconds =
      approachPosition / DEFAULT_TRAIN_STATION_JOURNEY_OPTIONS.cruiseSpeed;

    expect(first.startChunk).toBe(9);
    expect(first.startChunk * 320).toBe(
      TRAIN_STATION_DEFAULT_INITIAL_JOURNEY_DISTANCE,
    );
    expect(first.stopPosition).toBe(3_680);
    expect(approachPosition).toBe(3_536);
    expect(approachSeconds).toBeCloseTo(147.333_333_333, 9);
    expect(first.startChunk % TRAIN_STATION_REGION_ALIGNMENT_CHUNKS).toBe(0);
    expect(
      Math.floor(first.startChunk / TRAIN_STATION_REGION_ALIGNMENT_CHUNKS),
    ).toBe(
      Math.floor(first.endChunk / TRAIN_STATION_REGION_ALIGNMENT_CHUNKS),
    );
  });

  it("keeps first approach and repeat cadence in bounds across seeds", () => {
    const seeds = [
      "",
      "infinite-journey",
      "forest-local",
      "coast-local",
      "industrial-local",
      "night-express",
    ];

    for (const seed of seeds) {
      const first = trainStationEventByIndex(seed, 0);
      const second = trainStationEventByIndex(seed, 1);
      const approachSeconds =
        (first.stopPosition -
          DEFAULT_TRAIN_STATION_JOURNEY_OPTIONS.approachDistance) /
        DEFAULT_TRAIN_STATION_JOURNEY_OPTIONS.cruiseSpeed;
      const repeatSeconds =
        (second.stopPosition - first.stopPosition) /
        DEFAULT_TRAIN_STATION_JOURNEY_OPTIONS.cruiseSpeed;

      expect(approachSeconds, seed || "fallback seed").toBeGreaterThanOrEqual(
        2 * 60,
      );
      expect(approachSeconds, seed || "fallback seed").toBeLessThanOrEqual(
        3 * 60,
      );
      expect(repeatSeconds, seed || "fallback seed").toBeGreaterThanOrEqual(
        3 * 60,
      );
      expect(repeatSeconds, seed || "fallback seed").toBeLessThanOrEqual(
        5 * 60,
      );
      expect(second.startChunk % TRAIN_STATION_REGION_ALIGNMENT_CHUNKS).toBe(0);
      expect(second.startChunk).toBeGreaterThan(first.endChunk + 1);
    }
  });

  it("reserves deterministic continuous station spans without violating cooldown", () => {
    const first = trainStationEventByIndex("station-line", 0);
    const repeated = trainStationEventByIndex("station-line", 0);
    const next = trainStationEventByIndex("station-line", 1);

    expect(repeated).toEqual(first);
    expect(first.spanChunks).toBe(TRAIN_STATION_SPAN_CHUNKS);
    expect(next.startChunk).toBeGreaterThan(first.endChunk);
    expect(trainStationMinimumGap("station-line", 0)).toBeGreaterThanOrEqual(
      TRAIN_STATION_DEFAULT_MINIMUM_JOURNEY_DISTANCE,
    );
    expect(trainStationMinimumGap("station-line", 0)).toBe(3_840);
    expect(
      Array.from({ length: first.spanChunks }, (_, offset) =>
        trainStationEventForChunk("station-line", first.startChunk + offset),
      ).map((event) => event?.id),
    ).toEqual(Array.from({ length: first.spanChunks }, () => first.id));
    expect(
      trainStationEventForChunk("station-line", first.endChunk + 1),
    ).toBeNull();
  });

  it("supports configurable longer initial journeys and repeat distances", () => {
    const initialJourneyDistance = 14_000;
    const minimumJourneyDistance = 14_000;
    const first = trainStationEventByIndex("long-local", 0, {
      initialJourneyDistance,
      minimumJourneyDistance,
    });

    expect(first.startChunk * 320).toBeGreaterThanOrEqual(
      initialJourneyDistance,
    );
    expect(first.startChunk % TRAIN_STATION_REGION_ALIGNMENT_CHUNKS).toBe(0);
    expect(
      trainStationMinimumGap("long-local", 2, {
        initialJourneyDistance,
        minimumJourneyDistance,
      }),
    ).toBeGreaterThanOrEqual(minimumJourneyDistance);
  });

  it("preserves continuous progress from departure to the next approach", () => {
    let journey = createTrainStationJourney(
      "continuity-line",
      0,
      {},
      "depart",
    );
    const completedStation = journey.station;
    let previousPosition = journey.routePosition;
    let maximumStep = 0;

    for (let step = 0; step < 20_000; step++) {
      journey = advanceTrainStationJourney(journey, 250);
      const routeStep = journey.routePosition - previousPosition;
      expect(routeStep).toBeGreaterThanOrEqual(0);
      maximumStep = Math.max(maximumStep, routeStep);
      previousPosition = journey.routePosition;
      if (
        journey.state === "approach" &&
        journey.station.eventIndex === completedStation.eventIndex + 1
      ) {
        break;
      }
    }

    expect(journey.state).toBe("approach");
    expect(journey.station).toEqual(
      trainStationEventByIndex(
        "continuity-line",
        completedStation.eventIndex + 1,
      ),
    );
    expect(journey.routePosition).toBeGreaterThan(
      completedStation.stopPosition,
    );
    expect(maximumStep).toBeLessThanOrEqual(
      (DEFAULT_TRAIN_STATION_JOURNEY_OPTIONS.cruiseSpeed * 250) / 1_000,
    );
  });

  it("passes through every approach, stop, dwell, departure, and cruise state", () => {
    let journey = createTrainStationJourney(
      "state-line",
      0,
      {},
      "approach",
    );
    const states = new Set<TrainStationState>([journey.state]);
    const completedStation = journey.station;

    for (let step = 0; step < 20_000; step++) {
      journey = advanceTrainStationJourney(journey, 250);
      states.add(journey.state);
      if (
        journey.state === "cruise" &&
        journey.station.eventIndex === completedStation.eventIndex + 1
      ) {
        break;
      }
    }

    expect(states).toEqual(
      new Set([
        "approach",
        "decelerate",
        "platform",
        "dwell",
        "depart",
        "cruise",
      ]),
    );
    expect(journey.routePosition).toBeGreaterThan(
      completedStation.stopPosition,
    );
    expect(journey.station.eventIndex).toBe(completedStation.eventIndex + 1);
  });

  it("keeps speed curves bounded and positional scenery still throughout dwell", () => {
    let journey = createTrainStationJourney(
      "curve-line",
      0,
      {},
      "approach",
    );
    const deceleratingSpeeds: number[] = [];
    const departingSpeeds: number[] = [];

    for (let step = 0; step < 20_000 && journey.state !== "dwell"; step++) {
      journey = advanceTrainStationJourney(journey, 250);
      if (journey.state === "decelerate") {
        deceleratingSpeeds.push(journey.currentSpeed);
      }
    }
    expect(journey.state).toBe("dwell");
    expect(journey.currentSpeed).toBe(0);
    expect(
      deceleratingSpeeds.every(
        (speed, index) => index === 0 || speed <= deceleratingSpeeds[index - 1]!,
      ),
    ).toBe(true);

    const dwellPosition = journey.routePosition;
    for (
      let elapsed = 0;
      elapsed < TRAIN_STATION_DEFAULT_DWELL_MS - 250;
      elapsed += 250
    ) {
      journey = advanceTrainStationJourney(journey, 250, {
        dwellMs: TRAIN_STATION_DEFAULT_DWELL_MS,
      });
    }
    journey = advanceTrainStationJourney(journey, 249, {
      dwellMs: TRAIN_STATION_DEFAULT_DWELL_MS,
    });
    expect(journey.state).toBe("dwell");
    expect(journey.routePosition).toBe(dwellPosition);
    journey = advanceTrainStationJourney(journey, 1, {
      dwellMs: TRAIN_STATION_DEFAULT_DWELL_MS,
    });
    expect(journey.state).toBe("depart");
    expect(journey.routePosition).toBe(dwellPosition);

    for (let step = 0; step < 100 && journey.state === "depart"; step++) {
      journey = advanceTrainStationJourney(journey, 250);
      departingSpeeds.push(journey.currentSpeed);
    }
    expect(
      departingSpeeds.every(
        (speed, index) => index === 0 || speed >= departingSpeeds[index - 1]!,
      ),
    ).toBe(true);
    expect(Math.max(...departingSpeeds)).toBeLessThanOrEqual(24);
  });

  it("offers deterministic development triggers for arriving and leaving", () => {
    expect(trainStationDevelopmentTrigger("?train-station-trigger=approach")).toBe(
      "approach",
    );
    expect(trainStationDevelopmentTrigger("?train-station-trigger=depart")).toBe(
      "depart",
    );
    expect(trainStationDevelopmentTrigger("?train-station-trigger=other")).toBe(
      null,
    );

    const arriving = createTrainStationJourney(
      "debug-line",
      0,
      {},
      "approach",
    );
    const leaving = createTrainStationJourney(
      "debug-line",
      0,
      {},
      "depart",
    );
    expect(arriving.routePosition).toBe(
      arriving.station.stopPosition - 144,
    );
    expect(leaving.routePosition).toBe(leaving.station.stopPosition);
    expect(leaving.currentSpeed).toBe(0);
  });

  it("expands the braking envelope for high development cruise speeds", () => {
    const options = { cruiseSpeed: 96 };
    let journey = createTrainStationJourney(
      "fast-line",
      0,
      options,
      "approach",
    );
    const speedChanges: number[] = [];

    for (let step = 0; step < 2_000 && journey.state !== "platform"; step++) {
      const previousSpeed = journey.currentSpeed;
      journey = advanceTrainStationJourney(journey, 250, options);
      speedChanges.push(Math.abs(journey.currentSpeed - previousSpeed));
    }

    expect(journey.state).toBe("platform");
    expect(journey.routePosition).toBe(journey.station.stopPosition);
    expect(Math.max(...speedChanges)).toBeLessThanOrEqual(2.5);
  });

  it("does not advance state or position without elapsed time", () => {
    const journey = advanceUntil(
      createTrainStationJourney("pause-line", 0, {}, "approach"),
      "dwell",
    );
    expect(advanceTrainStationJourney(journey, 0)).toEqual(journey);
    expect(advanceTrainStationJourney(journey, Number.NaN)).toEqual(journey);
  });

  it("advances non-positional station phases from a separate wall clock", () => {
    let journey = advanceUntil(
      createTrainStationJourney("wall-clock-line", 0, {}, "approach"),
      "platform",
    );
    const stopPosition = journey.routePosition;

    journey = advanceTrainStationJourneyOnClock(journey, 0, 249);
    expect(journey.state).toBe("platform");
    expect(journey.routePosition).toBe(stopPosition);
    journey = advanceTrainStationJourneyOnClock(journey, 0, 1);
    expect(journey.state).toBe("dwell");

    journey = advanceTrainStationJourneyOnClock(
      journey,
      0,
      TRAIN_STATION_DEFAULT_DWELL_MS - 1,
    );
    expect(journey.state).toBe("dwell");
    expect(journey.routePosition).toBe(stopPosition);
    journey = advanceTrainStationJourneyOnClock(journey, 0, 1);
    expect(journey.state).toBe("depart");
    expect(journey.routePosition).toBe(stopPosition);
  });
});
