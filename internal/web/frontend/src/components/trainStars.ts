import {
  DEFAULT_TRAIN_ROUTE_SEED,
  TRAIN_ROUTE_SEED_VERSION,
  trainRouteRandomUnit,
} from "./trainRoute";

export const TRAIN_STAR_MIN_COUNT = 12;
export const TRAIN_STAR_MAX_COUNT = 38;
export const TRAIN_STAR_SKY_TOP_PERCENT = 5;
export const TRAIN_STAR_SKY_BOTTOM_PERCENT = 42;

export type TrainStarTint = "cool" | "neutral" | "warm";
export type TrainStarIntensity = "dim" | "bright";

export interface TrainStar {
  id: string;
  xPercent: number;
  yPercent: number;
  sizePx: number;
  brightness: number;
  tint: TrainStarTint;
  intensity: TrainStarIntensity;
  group: string | null;
}

export interface TrainStarCatalogue {
  seed: string;
  viewportWidth: number;
  targetCount: number;
  negativeSpaceStartPercent: number;
  negativeSpaceEndPercent: number;
  stars: readonly TrainStar[];
}

interface StarCandidate {
  xPercent: number;
  yPercent: number;
  group: string | null;
}

const MOON_HALO = {
  leftPercent: 2,
  rightPercent: 19,
  topPercent: 0,
  bottomPercent: 31,
} as const;

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}

function starRandom(seed: string, viewportWidth: number, key: string): number {
  return trainRouteRandomUnit(
    `${TRAIN_ROUTE_SEED_VERSION}:${seed}:stars:${viewportWidth}:${key}`,
  );
}

export function trainStarTargetCount(viewportWidth: number): number {
  const safeWidth =
    Number.isFinite(viewportWidth) && viewportWidth > 0
      ? Math.round(viewportWidth)
      : 1;
  return clamp(
    Math.round(safeWidth / 62),
    TRAIN_STAR_MIN_COUNT,
    TRAIN_STAR_MAX_COUNT,
  );
}

export function trainStarIsInsideMoonHalo(
  xPercent: number,
  yPercent: number,
): boolean {
  return (
    xPercent >= MOON_HALO.leftPercent &&
    xPercent <= MOON_HALO.rightPercent &&
    yPercent >= MOON_HALO.topPercent &&
    yPercent <= MOON_HALO.bottomPercent
  );
}

function candidateFits(
  candidate: StarCandidate,
  stars: readonly StarCandidate[],
  viewportWidth: number,
  negativeSpaceStart: number,
  negativeSpaceEnd: number,
): boolean {
  if (
    candidate.xPercent >= negativeSpaceStart &&
    candidate.xPercent <= negativeSpaceEnd
  ) {
    return false;
  }
  if (
    trainStarIsInsideMoonHalo(candidate.xPercent, candidate.yPercent)
  ) {
    return false;
  }

  return stars.every((star) => {
    if (candidate.group !== null && candidate.group === star.group) return true;
    const horizontalPx =
      (Math.abs(candidate.xPercent - star.xPercent) / 100) * viewportWidth;
    const verticalPx = Math.abs(candidate.yPercent - star.yPercent) * 2.4;
    const distance = Math.hypot(horizontalPx, verticalPx);
    if (distance < 13) return false;
    // Reject near-horizontal rows even when their points are far apart.
    if (horizontalPx < 150 && Math.abs(candidate.yPercent - star.yPercent) < 0.7) {
      return false;
    }
    return true;
  });
}

function starAppearance(
  seed: string,
  viewportWidth: number,
  ordinal: number,
  candidate: StarCandidate,
): TrainStar {
  const key = `appearance:${ordinal}`;
  const intensity: TrainStarIntensity =
    starRandom(seed, viewportWidth, `${key}:bright`) < 0.18 ? "bright" : "dim";
  const tintUnit = starRandom(seed, viewportWidth, `${key}:tint`);
  const tint: TrainStarTint =
    tintUnit < 0.34 ? "cool" : tintUnit < 0.78 ? "neutral" : "warm";
  const sizeUnit = starRandom(seed, viewportWidth, `${key}:size`);
  const sizePx =
    intensity === "bright"
      ? 1.8 + sizeUnit * 1.1
      : 0.8 + sizeUnit * 0.9;
  const brightness =
    intensity === "bright"
      ? 0.82 + starRandom(seed, viewportWidth, `${key}:opacity`) * 0.18
      : 0.38 + starRandom(seed, viewportWidth, `${key}:opacity`) * 0.34;

  return {
    id: `star-${ordinal}`,
    xPercent: candidate.xPercent,
    yPercent: candidate.yPercent,
    sizePx,
    brightness,
    tint,
    intensity,
    group: candidate.group,
  };
}

export function generateTrainStarCatalogue(
  seed: string,
  viewportWidth: number,
): TrainStarCatalogue {
  const resolvedSeed = seed.trim() || DEFAULT_TRAIN_ROUTE_SEED;
  const resolvedWidth = Math.max(1, Math.round(viewportWidth));
  const targetCount = trainStarTargetCount(resolvedWidth);
  const negativeSpaceWidth =
    12 + starRandom(resolvedSeed, resolvedWidth, "negative-space-width") * 7;
  const negativeSpaceStart =
    28 +
    starRandom(resolvedSeed, resolvedWidth, "negative-space-start") *
      (66 - negativeSpaceWidth - 28);
  const negativeSpaceEnd = negativeSpaceStart + negativeSpaceWidth;
  const candidates: StarCandidate[] = [];
  const groupCount = Math.max(1, Math.min(3, Math.floor(targetCount / 13)));

  for (let groupIndex = 0; groupIndex < groupCount; groupIndex++) {
    const group = `group-${groupIndex}`;
    let center: StarCandidate | null = null;
    for (let attempt = 0; attempt < 64 && center === null; attempt++) {
      const candidate = {
        xPercent:
          4 +
          starRandom(
            resolvedSeed,
            resolvedWidth,
            `${group}:center-x:${attempt}`,
          ) *
            92,
        yPercent:
          TRAIN_STAR_SKY_TOP_PERCENT +
          starRandom(
            resolvedSeed,
            resolvedWidth,
            `${group}:center-y:${attempt}`,
          ) *
            (TRAIN_STAR_SKY_BOTTOM_PERCENT - TRAIN_STAR_SKY_TOP_PERCENT),
        group,
      };
      if (
        candidateFits(
          candidate,
          candidates,
          resolvedWidth,
          negativeSpaceStart,
          negativeSpaceEnd,
        )
      ) {
        center = candidate;
      }
    }
    if (!center) continue;
    candidates.push(center);

    const groupSize =
      2 +
      (starRandom(resolvedSeed, resolvedWidth, `${group}:size`) > 0.66 ? 1 : 0);
    for (let member = 1; member < groupSize; member++) {
      const angle =
        starRandom(resolvedSeed, resolvedWidth, `${group}:angle:${member}`) *
        Math.PI *
        2;
      const radiusPx =
        7 +
        starRandom(resolvedSeed, resolvedWidth, `${group}:radius:${member}`) * 8;
      const groupedCandidate = {
        xPercent:
          center.xPercent + (Math.cos(angle) * radiusPx * 100) / resolvedWidth,
        yPercent: center.yPercent + (Math.sin(angle) * radiusPx) / 2.4,
        group,
      };
      if (
        groupedCandidate.xPercent >= 2 &&
        groupedCandidate.xPercent <= 98 &&
        groupedCandidate.yPercent >= TRAIN_STAR_SKY_TOP_PERCENT &&
        groupedCandidate.yPercent <= TRAIN_STAR_SKY_BOTTOM_PERCENT &&
        candidateFits(
          groupedCandidate,
          candidates,
          resolvedWidth,
          negativeSpaceStart,
          negativeSpaceEnd,
        )
      ) {
        candidates.push(groupedCandidate);
      }
    }
  }

  for (
    let attempt = 0;
    candidates.length < targetCount && attempt < targetCount * 96;
    attempt++
  ) {
    const candidate = {
      xPercent:
        2 +
        starRandom(resolvedSeed, resolvedWidth, `single-x:${attempt}`) * 96,
      yPercent:
        TRAIN_STAR_SKY_TOP_PERCENT +
        starRandom(resolvedSeed, resolvedWidth, `single-y:${attempt}`) *
          (TRAIN_STAR_SKY_BOTTOM_PERCENT - TRAIN_STAR_SKY_TOP_PERCENT),
      group: null,
    };
    if (
      candidateFits(
        candidate,
        candidates,
        resolvedWidth,
        negativeSpaceStart,
        negativeSpaceEnd,
      )
    ) {
      candidates.push(candidate);
    }
  }

  const stars = candidates
    .slice(0, targetCount)
    .sort((left, right) => left.xPercent - right.xPercent)
    .map((candidate, ordinal) =>
      starAppearance(resolvedSeed, resolvedWidth, ordinal, candidate),
    );

  return {
    seed: resolvedSeed,
    viewportWidth: resolvedWidth,
    targetCount,
    negativeSpaceStartPercent: negativeSpaceStart,
    negativeSpaceEndPercent: negativeSpaceEnd,
    stars,
  };
}
