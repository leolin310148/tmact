import { describe, expect, it } from "vitest";

import {
  generateTrainDaySkyCatalogue,
  TRAIN_DAY_SKY_MAX_WISPS,
} from "./trainSky";

describe("day and sunset sky catalogue", () => {
  it("reproduces geometry for the same seed and viewport", () => {
    const first = generateTrainDaySkyCatalogue("sky-repeat", 1440);
    const repeated = generateTrainDaySkyCatalogue("sky-repeat", 1440);

    expect(repeated).toEqual(first);
    expect(generateTrainDaySkyCatalogue("sky-other", 1440)).not.toEqual(first);
    expect(generateTrainDaySkyCatalogue("sky-repeat", 390)).not.toEqual(first);
  });

  it("distributes high anchors and occasional weather across seeds", () => {
    const catalogues = Array.from({ length: 80 }, (_, index) =>
      generateTrainDaySkyCatalogue(`sky-distribution-${index}`, 1440),
    );
    const weather = new Set(catalogues.map((catalogue) => catalogue.weather));
    const sunPositions = new Set(
      catalogues.map((catalogue) => catalogue.sun.xPercent.toFixed(1)),
    );

    expect(weather).toEqual(
      new Set(["clear", "fair", "breezy", "showery"]),
    );
    expect(
      catalogues.filter((catalogue) => catalogue.weather === "showery").length,
    ).toBeGreaterThanOrEqual(4);
    expect(
      catalogues.filter((catalogue) => catalogue.weather === "showery").length,
    ).toBeLessThanOrEqual(18);
    expect(sunPositions.size).toBeGreaterThan(40);
    for (const catalogue of catalogues) {
      expect(catalogue.sun.yPercent).toBeGreaterThanOrEqual(10);
      expect(catalogue.sun.yPercent).toBeLessThanOrEqual(22);
      for (const wisp of catalogue.wisps) {
        expect(wisp.yPercent).toBeGreaterThanOrEqual(7);
        expect(wisp.yPercent).toBeLessThanOrEqual(31);
      }
    }
  });

  it("reserves believable open sky and keeps every catalogue bounded", () => {
    for (const width of [390, 1024, 2560]) {
      for (let index = 0; index < 40; index++) {
        const catalogue = generateTrainDaySkyCatalogue(
          `sky-open-${index}`,
          width,
        );
        const openWidth =
          catalogue.negativeSpaceEndPercent -
          catalogue.negativeSpaceStartPercent;

        expect(openWidth).toBeGreaterThanOrEqual(22);
        expect(openWidth).toBeLessThanOrEqual(32);
        expect(catalogue.wisps.length).toBeLessThanOrEqual(
          TRAIN_DAY_SKY_MAX_WISPS,
        );
        expect(catalogue.elementCount).toBe(1 + catalogue.wisps.length);
        expect(catalogue.elementCount).toBeLessThanOrEqual(5);
        expect(
          catalogue.sun.xPercent < catalogue.negativeSpaceStartPercent ||
            catalogue.sun.xPercent > catalogue.negativeSpaceEndPercent,
        ).toBe(true);
        for (const wisp of catalogue.wisps) {
          expect(
            wisp.xPercent < catalogue.negativeSpaceStartPercent ||
              wisp.xPercent > catalogue.negativeSpaceEndPercent,
          ).toBe(true);
        }
      }
    }
  });

  it("adds restrained richness by viewport without unbounded growth", () => {
    const compact = generateTrainDaySkyCatalogue("responsive-sky", 390);
    const desktop = generateTrainDaySkyCatalogue("responsive-sky", 1024);
    const ultrawide = generateTrainDaySkyCatalogue("responsive-sky", 2560);

    expect(compact.wisps.length).toBeGreaterThanOrEqual(1);
    expect(desktop.wisps.length).toBeGreaterThanOrEqual(2);
    expect(ultrawide.wisps.length).toBeGreaterThanOrEqual(3);
    expect(ultrawide.wisps.length).toBeLessThanOrEqual(
      TRAIN_DAY_SKY_MAX_WISPS,
    );
  });
});
