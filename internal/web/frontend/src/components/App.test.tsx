import { describe, expect, it } from "vitest";
import { streamRenderLineLimit, switchSelectedPane } from "./App";

describe("App pane interaction lifecycle", () => {
  it("does not carry selection mode from the old pane into a pane switch", () => {
    const state = {
      selected: "%1",
      selectionMode: true,
    };

    switchSelectedPane(state, "%2");

    expect(state.selected).toBe("%2");
    expect(state.selectionMode).toBe(false);
  });
});

describe("App pane stream render window", () => {
  it("halves the initial rendered tail in split slots without shrinking single view", () => {
    expect(streamRenderLineLimit(false)).toBe(500);
    expect(streamRenderLineLimit(true)).toBe(250);
  });
});
