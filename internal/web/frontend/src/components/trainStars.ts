import {
  DEFAULT_TRAIN_ROUTE_SEED,
  TRAIN_ROUTE_SEED_VERSION,
  trainRouteRandomUnit,
} from "./trainRoute";

export const TRAIN_NIGHT_SKY_CATALOGUE_VERSION = "night-sky-v2";
export const TRAIN_STAR_MIN_COUNT = 12;
export const TRAIN_STAR_MAX_COUNT = 38;
export const TRAIN_STAR_SKY_TOP_PERCENT = 5;
export const TRAIN_STAR_SKY_BOTTOM_PERCENT = 42;
export const TRAIN_NIGHT_SKY_MAX_ELEMENTS = TRAIN_STAR_MAX_COUNT + 3;

export type TrainStarTint = "cool" | "neutral" | "warm";
export type TrainStarIntensity = "dim" | "bright";
export type TrainMoonPhase = "crescent" | "quarter" | "gibbous" | "full";
export type TrainMoonDirection = "waxing" | "waning";
export type TrainCelestialAccentKind = "meteor" | "planet";

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

export interface TrainMoon {
  id: string;
  phase: TrainMoonPhase;
  direction: TrainMoonDirection;
  xPercent: number;
  yPercent: number;
  diameterPx: number;
  illumination: number;
  exclusionRadiusXPercent: number;
  exclusionRadiusYPercent: number;
}

export interface TrainCelestialBand {
  id: string;
  yPercent: number;
  heightPx: number;
  rotationDeg: number;
  opacity: number;
}

export interface TrainCelestialAccent {
  id: string;
  kind: TrainCelestialAccentKind;
  xPercent: number;
  yPercent: number;
  widthPx: number;
  heightPx: number;
  rotationDeg: number;
  opacity: number;
}

export interface TrainNightSkyCatalogue {
  seed: string;
  version: string;
  viewportWidth: number;
  targetCount: number;
  negativeSpaceStartPercent: number;
  negativeSpaceEndPercent: number;
  moon: TrainMoon;
  band: TrainCelestialBand | null;
  accent: TrainCelestialAccent | null;
  stars: readonly TrainStar[];
  elementCount: number;
}

interface StarCandidate {
  xPercent: number;
  yPercent: number;
  group: string | null;
}

const MOON_PHASES: readonly TrainMoonPhase[] = [
  "crescent",
  "quarter",
  "gibbous",
  "full",
];

const MOON_ILLUMINATION: Readonly<Record<TrainMoonPhase, number>> = {
  crescent: 0.24,
  quarter: 0.5,
  gibbous: 0.76,
  full: 1,
};

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}

function boundedViewportWidth(viewportWidth: number): number {
  if (!Number.isFinite(viewportWidth)) return 1;
  return Math.max(1, Math.min(7680, Math.round(viewportWidth)));
}

function resolvedSeed(seed: string): string {
  return seed.trim() || DEFAULT_TRAIN_ROUTE_SEED;
}

function celestialRandom(seed: string, key: string): number {
  return trainRouteRandomUnit(
    `${TRAIN_ROUTE_SEED_VERSION}:${TRAIN_NIGHT_SKY_CATALOGUE_VERSION}:${seed}:${key}`,
  );
}

function starRandom(seed: string, viewportWidth: number, key: string): number {
  return celestialRandom(seed, `stars:${viewportWidth}:${key}`);
}

export function trainStarTargetCount(viewportWidth: number): number {
  const safeWidth = boundedViewportWidth(viewportWidth);
  return clamp(
    Math.round(safeWidth / 62),
    TRAIN_STAR_MIN_COUNT,
    TRAIN_STAR_MAX_COUNT,
  );
}

function trainMoonForCatalogue(
  seed: string,
  viewportWidth: number,
): TrainMoon {
  const phase =
    MOON_PHASES[
      Math.floor(celestialRandom(seed, "moon:phase") * MOON_PHASES.length)
    ] ?? "full";
  const direction: TrainMoonDirection =
    celestialRandom(seed, "moon:direction") < 0.5 ? "waxing" : "waning";
  const diameterPx = 18 + celestialRandom(seed, "moon:size") * 9;
  const haloPx =
    diameterPx *
    (phase === "full" ? 2.8 : phase === "gibbous" ? 2.55 : 2.3);

  return {
    id: `${TRAIN_NIGHT_SKY_CATALOGUE_VERSION}-moon`,
    phase,
    direction,
    xPercent: 8 + celestialRandom(seed, "moon:x") * 84,
    yPercent: 7 + celestialRandom(seed, "moon:y") * 18,
    diameterPx,
    illumination: MOON_ILLUMINATION[phase],
    exclusionRadiusXPercent: clamp(
      (haloPx / viewportWidth) * 100,
      2.8,
      16,
    ),
    exclusionRadiusYPercent: 7.5 + diameterPx * 0.12,
  };
}

export function trainStarIsInsideMoonHalo(
  xPercent: number,
  yPercent: number,
  moon: TrainMoon,
): boolean {
  const dx = (xPercent - moon.xPercent) / moon.exclusionRadiusXPercent;
  const dy = (yPercent - moon.yPercent) / moon.exclusionRadiusYPercent;
  return dx * dx + dy * dy <= 1;
}

function candidateCreatesLattice(
  candidate: StarCandidate,
  stars: readonly StarCandidate[],
  viewportWidth: number,
): boolean {
  const ordered = [...stars, candidate].sort(
    (left, right) => left.xPercent - right.xPercent,
  );
  const candidateIndex = ordered.indexOf(candidate);
  const firstTriple = Math.max(0, candidateIndex - 2);
  const lastTriple = Math.min(candidateIndex, ordered.length - 3);

  for (let start = firstTriple; start <= lastTriple; start++) {
    const left = ordered[start]!;
    const middle = ordered[start + 1]!;
    const right = ordered[start + 2]!;
    const ax = ((middle.xPercent - left.xPercent) / 100) * viewportWidth;
    const ay = (middle.yPercent - left.yPercent) * 2.4;
    const bx = ((right.xPercent - middle.xPercent) / 100) * viewportWidth;
    const by = (right.yPercent - middle.yPercent) * 2.4;
    const firstLength = Math.hypot(ax, ay);
    const secondLength = Math.hypot(bx, by);
    if (firstLength < 22 || secondLength < 22) continue;

    const cross = Math.abs(ax * by - ay * bx);
    const alignment = cross / (firstLength * secondLength);
    const repeatedStep =
      Math.abs(firstLength - secondLength) /
      Math.max(firstLength, secondLength);
    if (alignment < 0.035 && repeatedStep < 0.18) return true;
  }

  const gapCounts = new Map<number, number>();
  for (let index = 1; index < ordered.length; index++) {
    const gap = ordered[index]!.xPercent - ordered[index - 1]!.xPercent;
    const bucket = Math.round(gap * 10);
    const count = (gapCounts.get(bucket) ?? 0) + 1;
    if (count > 3) return true;
    gapCounts.set(bucket, count);
  }
  return false;
}

function candidateFits(
  candidate: StarCandidate,
  stars: readonly StarCandidate[],
  moon: TrainMoon,
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
  if (trainStarIsInsideMoonHalo(candidate.xPercent, candidate.yPercent, moon)) {
    return false;
  }

  const separated = stars.every((star) => {
    if (candidate.group !== null && candidate.group === star.group) return true;
    const horizontalPx =
      (Math.abs(candidate.xPercent - star.xPercent) / 100) * viewportWidth;
    const verticalPx = Math.abs(candidate.yPercent - star.yPercent) * 2.4;
    const distance = Math.hypot(horizontalPx, verticalPx);
    if (distance < 13) return false;
    if (horizontalPx < 150 && Math.abs(candidate.yPercent - star.yPercent) < 0.7) {
      return false;
    }
    return true;
  });

  return (
    separated &&
    (candidate.group !== null ||
      !candidateCreatesLattice(candidate, stars, viewportWidth))
  );
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

function celestialBandForSeed(seed: string): TrainCelestialBand | null {
  if (celestialRandom(seed, "band:present") >= 0.3) return null;
  return {
    id: `${TRAIN_NIGHT_SKY_CATALOGUE_VERSION}-airglow`,
    yPercent: 13 + celestialRandom(seed, "band:y") * 17,
    heightPx: 24 + celestialRandom(seed, "band:height") * 16,
    rotationDeg: -11 + celestialRandom(seed, "band:rotation") * 22,
    opacity: 0.055 + celestialRandom(seed, "band:opacity") * 0.055,
  };
}

function celestialAccentForSeed(
  seed: string,
  moon: TrainMoon,
): TrainCelestialAccent | null {
  if (celestialRandom(seed, "accent:present") >= 0.17) return null;
  const kind: TrainCelestialAccentKind =
    celestialRandom(seed, "accent:kind") < 0.58 ? "meteor" : "planet";

  for (let attempt = 0; attempt < 16; attempt++) {
    const xPercent =
      5 + celestialRandom(seed, `accent:x:${attempt}`) * 90;
    const yPercent =
      6 + celestialRandom(seed, `accent:y:${attempt}`) * 26;
    if (trainStarIsInsideMoonHalo(xPercent, yPercent, moon)) continue;

    return {
      id: `${TRAIN_NIGHT_SKY_CATALOGUE_VERSION}-${kind}`,
      kind,
      xPercent,
      yPercent,
      widthPx:
        kind === "meteor"
          ? 13 + celestialRandom(seed, "accent:width") * 8
          : 2 + celestialRandom(seed, "accent:width") * 1.4,
      heightPx:
        kind === "meteor"
          ? 1
          : 2 + celestialRandom(seed, "accent:height") * 1.4,
      rotationDeg:
        kind === "meteor"
          ? -32 + celestialRandom(seed, "accent:rotation") * 18
          : 0,
      opacity: 0.48 + celestialRandom(seed, "accent:opacity") * 0.26,
    };
  }
  return null;
}

export function generateTrainNightSkyCatalogue(
  seed: string,
  viewportWidth: number,
): TrainNightSkyCatalogue {
  const routeSeed = resolvedSeed(seed);
  const width = boundedViewportWidth(viewportWidth);
  const moon = trainMoonForCatalogue(routeSeed, width);
  const targetCount = trainStarTargetCount(width);
  const negativeSpaceWidth =
    12 + starRandom(routeSeed, width, "negative-space-width") * 7;
  const negativeSpaceStart =
    28 +
    starRandom(routeSeed, width, "negative-space-start") *
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
          starRandom(routeSeed, width, `${group}:center-x:${attempt}`) * 92,
        yPercent:
          TRAIN_STAR_SKY_TOP_PERCENT +
          starRandom(routeSeed, width, `${group}:center-y:${attempt}`) *
            (TRAIN_STAR_SKY_BOTTOM_PERCENT - TRAIN_STAR_SKY_TOP_PERCENT),
        group,
      };
      if (
        candidateFits(
          candidate,
          candidates,
          moon,
          width,
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
      (starRandom(routeSeed, width, `${group}:size`) > 0.66 ? 1 : 0);
    for (let member = 1; member < groupSize; member++) {
      const angle =
        starRandom(routeSeed, width, `${group}:angle:${member}`) * Math.PI * 2;
      const radiusPx =
        7 + starRandom(routeSeed, width, `${group}:radius:${member}`) * 8;
      const groupedCandidate = {
        xPercent:
          center.xPercent + (Math.cos(angle) * radiusPx * 100) / width,
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
          moon,
          width,
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
    candidates.length < targetCount && attempt < targetCount * 128;
    attempt++
  ) {
    const candidate = {
      xPercent:
        2 + starRandom(routeSeed, width, `single-x:${attempt}`) * 96,
      yPercent:
        TRAIN_STAR_SKY_TOP_PERCENT +
        starRandom(routeSeed, width, `single-y:${attempt}`) *
          (TRAIN_STAR_SKY_BOTTOM_PERCENT - TRAIN_STAR_SKY_TOP_PERCENT),
      group: null,
    };
    if (
      candidateFits(
        candidate,
        candidates,
        moon,
        width,
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
      starAppearance(routeSeed, width, ordinal, candidate),
    );
  const band = celestialBandForSeed(routeSeed);
  const accent = celestialAccentForSeed(routeSeed, moon);

  return {
    seed: routeSeed,
    version: TRAIN_NIGHT_SKY_CATALOGUE_VERSION,
    viewportWidth: width,
    targetCount,
    negativeSpaceStartPercent: negativeSpaceStart,
    negativeSpaceEndPercent: negativeSpaceEnd,
    moon,
    band,
    accent,
    stars,
    elementCount: stars.length + 1 + Number(band !== null) + Number(accent !== null),
  };
}
