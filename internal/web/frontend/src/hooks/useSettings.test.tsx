import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { useSettings } from "./useSettings";

const SETTINGS_KEY = "tmact.settings";

afterEach(() => {
  localStorage.clear();
  delete document.documentElement.dataset.paneSwitcherLayout;
  delete document.documentElement.dataset.paneFontFamily;
});

describe("useSettings pane font family", () => {
  it("applies and persists the selected pane font", () => {
    const { result } = renderHook(() => useSettings());

    act(() => result.current.onFontFamilyChange("maple-mono"));

    expect(document.documentElement.dataset.paneFontFamily).toBe("maple-mono");
    expect(JSON.parse(localStorage.getItem(SETTINGS_KEY)!)).toMatchObject({
      paneFontFamily: "maple-mono",
    });
  });

  it("falls back to the system font for an unknown saved value", () => {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify({ paneFontFamily: "comic-sans" }));
    const { result } = renderHook(() => useSettings());

    act(() => result.current.loadClientSettings());

    expect(document.documentElement.dataset.paneFontFamily).toBe("system");
    expect(JSON.parse(localStorage.getItem(SETTINGS_KEY)!)).toMatchObject({
      paneFontFamily: "system",
    });
  });
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

  it("falls back to the default for the removed train pane switcher layout", () => {
    localStorage.setItem(
      SETTINGS_KEY,
      JSON.stringify({ paneSwitcherLayout: "train" }),
    );
    const { result } = renderHook(() => useSettings());

    act(() => result.current.loadClientSettings());

    expect(document.documentElement.dataset.paneSwitcherLayout).toBe("bottom");
    expect(JSON.parse(localStorage.getItem(SETTINGS_KEY)!)).toMatchObject({
      paneSwitcherLayout: "bottom",
    });
  });
});
