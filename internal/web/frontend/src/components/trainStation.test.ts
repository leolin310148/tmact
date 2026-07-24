import { describe, expect, it } from "vitest";

import {
  advanceTrainStationJourney,
  advanceTrainStationJourneyOnClock,
  createTrainStationJourney,
  TRAIN_STATION_DEFAULT_DWELL_MS,
  TRAIN_STATION_DEFAULT_MINIMUM_JOURNEY_DISTANCE,
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
    expect(
      Array.from({ length: first.spanChunks }, (_, offset) =>
        trainStationEventForChunk("station-line", first.startChunk + offset),
      ).map((event) => event?.id),
    ).toEqual(Array.from({ length: first.spanChunks }, () => first.id));
    expect(
      trainStationEventForChunk("station-line", first.endChunk + 1),
    ).toBeNull();
  });

  it("supports a configurable minimum journey distance", () => {
    const minimumJourneyDistance = 14_000;
    expect(
      trainStationMinimumGap("long-local", 2, { minimumJourneyDistance }),
    ).toBeGreaterThanOrEqual(minimumJourneyDistance);
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
    expect(Math.max(...departingSpeeds)).toBeLessThanOrEqual(12);
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
