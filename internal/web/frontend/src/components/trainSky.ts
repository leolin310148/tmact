import { trainRouteRandomUnit } from "./trainRoute";

export const TRAIN_DAY_SKY_CATALOGUE_VERSION = "day-sky-v2";
export const TRAIN_DAY_SKY_MAX_WISPS = 4;

export type TrainDaySkyWeather = "clear" | "fair" | "breezy" | "showery";

export interface TrainDaySkyAnchor {
  id: string;
  kind: "sun" | "wisp";
  xPercent: number;
  yPercent: number;
  sunsetYPercent: number;
  widthPx: number;
  heightPx: number;
  opacity: number;
}

export interface TrainDaySkyCatalogue {
  seed: string;
  viewportWidth: number;
  weather: TrainDaySkyWeather;
  negativeSpaceStartPercent: number;
  negativeSpaceEndPercent: number;
  sun: TrainDaySkyAnchor;
  wisps: readonly TrainDaySkyAnchor[];
  elementCount: number;
}

function boundedViewportWidth(viewportWidth: number): number {
  if (!Number.isFinite(viewportWidth)) return 1;
  return Math.max(1, Math.min(7680, Math.round(viewportWidth)));
}

function random(key: string, part: string): number {
  return trainRouteRandomUnit(
    `${TRAIN_DAY_SKY_CATALOGUE_VERSION}:${key}:${part}`,
  );
}

function weatherForKey(key: string): TrainDaySkyWeather {
  const value = random(key, "weather");
  if (value < 0.14) return "showery";
  if (value < 0.36) return "breezy";
  if (value < 0.72) return "fair";
  return "clear";
}

function positionOutsideGap(
  unit: number,
  minimum: number,
  maximum: number,
  gapStart: number,
  gapEnd: number,
): number {
  const leftWidth = Math.max(0, gapStart - minimum);
  const rightWidth = Math.max(0, maximum - gapEnd);
  const availableWidth = leftWidth + rightWidth;
  const distance = unit * availableWidth;
  return distance <= leftWidth
    ? minimum + distance
    : gapEnd + (distance - leftWidth);
}

export function generateTrainDaySkyCatalogue(
  routeSeed: string,
  viewportWidth: number,
): TrainDaySkyCatalogue {
  const width = boundedViewportWidth(viewportWidth);
  const key = `${routeSeed}:${width}`;
  const weather = weatherForKey(routeSeed);
  const negativeSpaceWidth = 22 + random(key, "open-width") * 10;
  const negativeSpaceStartPercent =
    34 + random(key, "open-start") * (36 - negativeSpaceWidth);
  const negativeSpaceEndPercent =
    negativeSpaceStartPercent + negativeSpaceWidth;
  const sunOnLeft = random(key, "sun-side") < 0.5;
  const sunMinimum = sunOnLeft ? 14 : negativeSpaceEndPercent + 7;
  const sunMaximum = sunOnLeft ? negativeSpaceStartPercent - 7 : 88;
  const sunX =
    sunMinimum + random(key, "sun-x") * Math.max(1, sunMaximum - sunMinimum);
  const sunSize = 18 + random(key, "sun-size") * 8;
  const sun: TrainDaySkyAnchor = {
    id: `${TRAIN_DAY_SKY_CATALOGUE_VERSION}-sun`,
    kind: "sun",
    xPercent: sunX,
    yPercent: 10 + random(key, "sun-y") * 12,
    sunsetYPercent: 58 + random(key, "sunset-sun-y") * 10,
    widthPx: sunSize,
    heightPx: sunSize,
    opacity: 0.88 + random(key, "sun-opacity") * 0.1,
  };

  const baseWispCount = width < 720 ? 1 : width < 1600 ? 2 : 3;
  const weatherWispCount =
    weather === "breezy" || weather === "showery" ? 1 : 0;
  const wispCount = Math.min(
    TRAIN_DAY_SKY_MAX_WISPS,
    baseWispCount + weatherWispCount,
  );
  const wisps: TrainDaySkyAnchor[] = [];

  for (let ordinal = 0; ordinal < wispCount; ordinal++) {
    let xPercent = 0;
    for (let attempt = 0; attempt < 12; attempt++) {
      xPercent = positionOutsideGap(
        random(key, `wisp-${ordinal}-x-${attempt}`),
        6,
        94,
        negativeSpaceStartPercent,
        negativeSpaceEndPercent,
      );
      if (
        Math.abs(xPercent - sun.xPercent) >= 10 &&
        wisps.every((wisp) => Math.abs(xPercent - wisp.xPercent) >= 11)
      ) {
        break;
      }
    }
    const wispWidth =
      54 +
      random(key, `wisp-${ordinal}-width`) *
        (width < 720 ? 42 : 72);
    wisps.push({
      id: `${TRAIN_DAY_SKY_CATALOGUE_VERSION}-wisp-${ordinal}`,
      kind: "wisp",
      xPercent,
      yPercent: 7 + random(key, `wisp-${ordinal}-y`) * 24,
      sunsetYPercent:
        11 + random(key, `wisp-${ordinal}-sunset-y`) * 30,
      widthPx: wispWidth,
      heightPx: 7 + random(key, `wisp-${ordinal}-height`) * 7,
      opacity: 0.42 + random(key, `wisp-${ordinal}-opacity`) * 0.24,
    });
  }

  return {
    seed: `${TRAIN_DAY_SKY_CATALOGUE_VERSION}:${key}`,
    viewportWidth: width,
    weather,
    negativeSpaceStartPercent,
    negativeSpaceEndPercent,
    sun,
    wisps,
    elementCount: 1 + wisps.length,
  };
}
