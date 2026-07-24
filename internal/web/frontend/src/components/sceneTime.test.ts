import { describe, expect, it } from "vitest";

import {
  clockSceneMode,
  nextClockSceneModeBoundary,
  nextSceneMode,
} from "./sceneTime";

describe("shared scene time", () => {
  it("selects day, sunset, and night at the office-compatible local boundaries", () => {
    expect(clockSceneMode(new Date(2026, 0, 1, 5, 59))).toBe("night");
    expect(clockSceneMode(new Date(2026, 0, 1, 6, 0))).toBe("day");
    expect(clockSceneMode(new Date(2026, 0, 1, 16, 59))).toBe("day");
    expect(clockSceneMode(new Date(2026, 0, 1, 17, 0))).toBe("sunset");
    expect(clockSceneMode(new Date(2026, 0, 1, 18, 29))).toBe("sunset");
    expect(clockSceneMode(new Date(2026, 0, 1, 18, 30))).toBe("night");
  });

  it("cycles the shared manual override in a stable order", () => {
    expect(nextSceneMode("day")).toBe("sunset");
    expect(nextSceneMode("sunset")).toBe("night");
    expect(nextSceneMode("night")).toBe("day");
  });

  it("finds the next meaningful local-time boundary", () => {
    expect(nextClockSceneModeBoundary(new Date(2026, 0, 1, 16, 59, 30))).toEqual(
      new Date(2026, 0, 1, 17, 0),
    );
    expect(nextClockSceneModeBoundary(new Date(2026, 0, 1, 17, 0))).toEqual(
      new Date(2026, 0, 1, 18, 30),
    );
    expect(nextClockSceneModeBoundary(new Date(2026, 0, 1, 19, 0))).toEqual(
      new Date(2026, 0, 2, 6, 0),
    );
  });
});
