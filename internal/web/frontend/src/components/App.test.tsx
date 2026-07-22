import { describe, expect, it } from "vitest";
import { switchSelectedPane } from "./App";

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
