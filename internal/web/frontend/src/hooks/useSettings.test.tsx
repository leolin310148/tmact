import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { useSettings } from "./useSettings";

const SETTINGS_KEY = "tmact.settings";

afterEach(() => {
  localStorage.clear();
  delete document.documentElement.dataset.paneSwitcherLayout;
});

describe("useSettings pane switcher layout", () => {
  it("applies and persists the selected pane switcher layout", () => {
    const { result } = renderHook(() => useSettings());

    act(() => result.current.onPaneSwitcherLayoutChange("office"));

    expect(document.documentElement.dataset.paneSwitcherLayout).toBe("office");
    expect(JSON.parse(localStorage.getItem(SETTINGS_KEY)!)).toMatchObject({
      paneSwitcherLayout: "office",
    });
  });

  it("falls back to the default for an invalid saved pane switcher layout", () => {
    localStorage.setItem(
      SETTINGS_KEY,
      JSON.stringify({ paneSwitcherLayout: "floating" }),
    );
    const { result } = renderHook(() => useSettings());

    act(() => result.current.loadClientSettings());

    expect(document.documentElement.dataset.paneSwitcherLayout).toBe("bottom");
    expect(JSON.parse(localStorage.getItem(SETTINGS_KEY)!)).toMatchObject({
      paneSwitcherLayout: "bottom",
    });
  });
});
