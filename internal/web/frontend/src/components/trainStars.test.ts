import { describe, expect, it } from "vitest";

import {
  generateTrainStarCatalogue,
  TRAIN_STAR_MAX_COUNT,
  TRAIN_STAR_MIN_COUNT,
  TRAIN_STAR_SKY_BOTTOM_PERCENT,
  TRAIN_STAR_SKY_TOP_PERCENT,
  trainStarIsInsideMoonHalo,
  trainStarTargetCount,
} from "./trainStars";

describe("deterministic train star catalogue", () => {
  it("reproduces one field for the same seed and viewport without tiled coordinates", () => {
    const first = generateTrainStarCatalogue("night-line", 1_440);
    const repeat = generateTrainStarCatalogue("night-line", 1_440);
    const otherSeed = generateTrainStarCatalogue("other-night-line", 1_440);

    expect(repeat).toEqual(first);
    expect(otherSeed.stars).not.toEqual(first.stars);
    expect(new Set(first.stars.map((star) => star.xPercent.toFixed(4))).size).toBe(
      first.stars.length,
    );
    expect(new Set(first.stars.map((star) => star.yPercent.toFixed(4))).size).toBe(
      first.stars.length,
    );
  });

  it("keeps a responsive bounded count and deterministically regenerates on resize", () => {
    const compact = generateTrainStarCatalogue("responsive-night", 390);
    const desktop = generateTrainStarCatalogue("responsive-night", 1_280);
    const ultrawide = generateTrainStarCatalogue("responsive-night", 2_560);

    expect(compact.stars).toHaveLength(trainStarTargetCount(390));
    expect(desktop.stars).toHaveLength(trainStarTargetCount(1_280));
    expect(ultrawide.stars).toHaveLength(trainStarTargetCount(2_560));
    expect(compact.stars.length).toBeGreaterThanOrEqual(TRAIN_STAR_MIN_COUNT);
    expect(ultrawide.stars.length).toBeLessThanOrEqual(TRAIN_STAR_MAX_COUNT);
    expect(compact.stars.length).toBeLessThan(desktop.stars.length);
    expect(desktop.stars.length).toBeLessThan(ultrawide.stars.length);
    expect(generateTrainStarCatalogue("responsive-night", 2_560)).toEqual(
      ultrawide,
    );
  });

  it("mixes sparse bright stars, dim stars, tints, loose groups, and open sky", () => {
    const catalogues = ["field-a", "field-b", "field-c"].map((seed) =>
      generateTrainStarCatalogue(seed, 1_920),
    );
    const stars = catalogues.flatMap((catalogue) => catalogue.stars);

    expect(stars.filter((star) => star.intensity === "bright").length).toBeGreaterThan(
      2,
    );
    expect(stars.filter((star) => star.intensity === "dim").length).toBeGreaterThan(
      stars.length / 2,
    );
    expect(new Set(stars.map((star) => star.tint))).toEqual(
      new Set(["cool", "neutral", "warm"]),
    );
    expect(stars.some((star) => star.group !== null)).toBe(true);
    expect(new Set(stars.map((star) => star.sizePx.toFixed(2))).size).toBeGreaterThan(
      stars.length / 2,
    );
    expect(
      new Set(stars.map((star) => star.brightness.toFixed(2))).size,
    ).toBeGreaterThan(stars.length / 3);

    for (const catalogue of catalogues) {
      expect(
        catalogue.negativeSpaceEndPercent -
          catalogue.negativeSpaceStartPercent,
      ).toBeGreaterThanOrEqual(12);
      expect(
        catalogue.stars.some(
          (star) =>
            star.xPercent >= catalogue.negativeSpaceStartPercent &&
            star.xPercent <= catalogue.negativeSpaceEndPercent,
        ),
      ).toBe(false);
    }
  });

  it("rejects moon overlap, terrain overlap, rows, and diagonal lattice rhythms", () => {
    for (const seed of ["lattice-a", "lattice-b", "lattice-c", "lattice-d"]) {
      const { stars } = generateTrainStarCatalogue(seed, 1_920);
      const ordered = [...stars].sort(
        (left, right) => left.xPercent - right.xPercent,
      );

      expect(
        stars.every(
          (star) =>
            star.yPercent >= TRAIN_STAR_SKY_TOP_PERCENT &&
            star.yPercent <= TRAIN_STAR_SKY_BOTTOM_PERCENT,
        ),
      ).toBe(true);
      expect(
        stars.some((star) =>
          trainStarIsInsideMoonHalo(star.xPercent, star.yPercent),
        ),
      ).toBe(false);

      const altitudeBands = new Map<number, number>();
      for (const star of stars) {
        const band = Math.round(star.yPercent);
        altitudeBands.set(band, (altitudeBands.get(band) ?? 0) + 1);
      }
      expect(Math.max(...altitudeBands.values())).toBeLessThanOrEqual(3);

      const slopes = ordered.slice(1).map((star, index) => {
        const previous = ordered[index]!;
        return (
          (star.yPercent - previous.yPercent) /
          (star.xPercent - previous.xPercent)
        );
      });
      expect(new Set(slopes.map((slope) => slope.toFixed(1))).size).toBeGreaterThan(
        slopes.length * 0.55,
      );

      const horizontalGaps = ordered
        .slice(1)
        .map((star, index) => star.xPercent - ordered[index]!.xPercent);
      const repeatedGaps = new Map<number, number>();
      for (const gap of horizontalGaps) {
        const bucket = Math.round(gap * 10);
        repeatedGaps.set(bucket, (repeatedGaps.get(bucket) ?? 0) + 1);
      }
      expect(Math.max(...repeatedGaps.values())).toBeLessThanOrEqual(
        3,
      );
    }
  });
});
