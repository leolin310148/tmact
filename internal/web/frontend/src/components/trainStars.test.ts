import { describe, expect, it } from "vitest";

import {
  generateTrainNightSkyCatalogue,
  TRAIN_NIGHT_SKY_MAX_ELEMENTS,
  TRAIN_STAR_MAX_COUNT,
  TRAIN_STAR_MIN_COUNT,
  TRAIN_STAR_SKY_BOTTOM_PERCENT,
  TRAIN_STAR_SKY_TOP_PERCENT,
  trainStarIsInsideMoonHalo,
  trainStarTargetCount,
} from "./trainStars";

describe("deterministic train night-sky catalogue", () => {
  it("reproduces one composition for the same seed and viewport", () => {
    const first = generateTrainNightSkyCatalogue("night-line", 1_440);
    const repeat = generateTrainNightSkyCatalogue("night-line", 1_440);
    const otherSeed = generateTrainNightSkyCatalogue("other-night-line", 1_440);

    expect(repeat).toEqual(first);
    expect(otherSeed).not.toEqual(first);
    expect(new Set(first.stars.map((star) => star.xPercent.toFixed(4))).size).toBe(
      first.stars.length,
    );
    expect(new Set(first.stars.map((star) => star.yPercent.toFixed(4))).size).toBe(
      first.stars.length,
    );
  });

  it("varies moon phase, altitude, and horizontal position across journey seeds", () => {
    const catalogues = Array.from({ length: 48 }, (_, index) =>
      generateTrainNightSkyCatalogue(`moon-variety-${index}`, 1_440),
    );
    const moons = catalogues.map((catalogue) => catalogue.moon);

    expect(new Set(moons.map((moon) => moon.phase))).toEqual(
      new Set(["crescent", "quarter", "gibbous", "full"]),
    );
    expect(new Set(moons.map((moon) => moon.direction))).toEqual(
      new Set(["waxing", "waning"]),
    );
    expect(Math.min(...moons.map((moon) => moon.xPercent))).toBeLessThan(24);
    expect(Math.max(...moons.map((moon) => moon.xPercent))).toBeGreaterThan(76);
    expect(Math.min(...moons.map((moon) => moon.yPercent))).toBeLessThan(11);
    expect(Math.max(...moons.map((moon) => moon.yPercent))).toBeGreaterThan(21);
    expect(new Set(catalogues.map((catalogue) => JSON.stringify(catalogue))).size)
      .toBe(catalogues.length);
  });

  it("keeps a responsive bounded count and deterministically regenerates on resize", () => {
    const compact = generateTrainNightSkyCatalogue("responsive-night", 390);
    const desktop = generateTrainNightSkyCatalogue("responsive-night", 1_280);
    const ultrawide = generateTrainNightSkyCatalogue("responsive-night", 2_560);

    expect(compact.stars).toHaveLength(trainStarTargetCount(390));
    expect(desktop.stars).toHaveLength(trainStarTargetCount(1_280));
    expect(ultrawide.stars).toHaveLength(trainStarTargetCount(2_560));
    expect(compact.stars.length).toBeGreaterThanOrEqual(TRAIN_STAR_MIN_COUNT);
    expect(ultrawide.stars.length).toBeLessThanOrEqual(TRAIN_STAR_MAX_COUNT);
    expect(compact.stars.length).toBeLessThan(desktop.stars.length);
    expect(desktop.stars.length).toBeLessThan(ultrawide.stars.length);
    expect(generateTrainNightSkyCatalogue("responsive-night", 2_560)).toEqual(
      ultrawide,
    );
    expect(compact.moon.phase).toBe(ultrawide.moon.phase);
    expect(compact.moon.xPercent).toBe(ultrawide.moon.xPercent);
    expect(compact.moon.exclusionRadiusXPercent).toBeGreaterThan(
      ultrawide.moon.exclusionRadiusXPercent,
    );
  });

  it("mixes sparse bright stars, dim stars, tints, loose groups, and open sky", () => {
    const catalogues = ["field-a", "field-b", "field-c"].map((seed) =>
      generateTrainNightSkyCatalogue(seed, 1_920),
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

  it("adapts moon exclusion and rejects terrain, rows, and diagonal lattice rhythms", () => {
    for (const seed of ["lattice-a", "lattice-b", "lattice-c", "lattice-d"]) {
      const catalogue = generateTrainNightSkyCatalogue(seed, 1_920);
      const { stars, moon } = catalogue;
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
          trainStarIsInsideMoonHalo(star.xPercent, star.yPercent, moon),
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
      expect(Math.max(...repeatedGaps.values())).toBeLessThanOrEqual(3);
    }
  });

  it("keeps atmospheric bands occasional, accents rare, and every catalogue bounded", () => {
    const catalogues = Array.from({ length: 80 }, (_, index) =>
      generateTrainNightSkyCatalogue(`rarity-${index}`, 2_560),
    );
    const bands = catalogues.flatMap((catalogue) =>
      catalogue.band ? [catalogue.band] : [],
    );
    const accents = catalogues.flatMap((catalogue) =>
      catalogue.accent ? [catalogue.accent] : [],
    );

    expect(bands.length).toBeGreaterThan(8);
    expect(bands.length).toBeLessThan(38);
    expect(bands.every((band) => band.opacity <= 0.11)).toBe(true);
    expect(accents.length).toBeGreaterThan(4);
    expect(accents.length).toBeLessThan(24);
    expect(new Set(accents.map((accent) => accent.kind))).toEqual(
      new Set(["meteor", "planet"]),
    );
    expect(
      catalogues.every(
        (catalogue) =>
          catalogue.elementCount <= TRAIN_NIGHT_SKY_MAX_ELEMENTS &&
          catalogue.elementCount ===
            catalogue.stars.length +
              1 +
              Number(catalogue.band !== null) +
              Number(catalogue.accent !== null),
      ),
    ).toBe(true);
  });

  it("keeps multi-seed stars sparse, naturally grouped, and vertically distributed", () => {
    const catalogues = Array.from({ length: 64 }, (_, index) =>
      generateTrainNightSkyCatalogue(`distribution-${index}`, 1_280),
    );
    const stars = catalogues.flatMap((catalogue) => catalogue.stars);
    const altitudeSpan =
      TRAIN_STAR_SKY_BOTTOM_PERCENT - TRAIN_STAR_SKY_TOP_PERCENT;
    const altitudeBands = [0, 0, 0, 0];

    for (const star of stars) {
      const band = Math.min(
        3,
        Math.floor(
          ((star.yPercent - TRAIN_STAR_SKY_TOP_PERCENT) / altitudeSpan) * 4,
        ),
      );
      altitudeBands[band]!++;
    }

    const brightRate =
      stars.filter((star) => star.intensity === "bright").length / stars.length;
    expect(brightRate).toBeGreaterThan(0.1);
    expect(brightRate).toBeLessThan(0.26);
    expect(Math.min(...altitudeBands)).toBeGreaterThan(stars.length * 0.18);
    for (const catalogue of catalogues) {
      const groupSizes = new Map<string, number>();
      for (const star of catalogue.stars) {
        if (star.group === null) continue;
        groupSizes.set(star.group, (groupSizes.get(star.group) ?? 0) + 1);
      }
      expect(Math.max(...groupSizes.values())).toBeLessThanOrEqual(3);
    }
  });
});
