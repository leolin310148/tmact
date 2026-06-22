import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { useSettings } from "./useSettings";

const SETTINGS_KEY = "tmact.settings";

afterEach(() => {
  localStorage.clear();
  delete document.documentElement.dataset.paneSwitcherLayout;
  delete document.documentElement.dataset.officeScale;
  document.documentElement.style.removeProperty("--office-scale");
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

describe("useSettings office scale", () => {
  it("applies and persists a custom virtual office scale", () => {
    const { result } = renderHook(() => useSettings());

    act(() => result.current.onOfficeScaleInput("80"));

    expect(document.documentElement.dataset.officeScale).toBe("custom");
    expect(document.documentElement.style.getPropertyValue("--office-scale")).toBe("0.8");
    expect(JSON.parse(localStorage.getItem(SETTINGS_KEY)!)).toMatchObject({
      officeScale: 80,
    });
  });

  it("resets virtual office scale to auto", () => {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify({ officeScale: 90 }));
    const { result } = renderHook(() => useSettings());

    act(() => result.current.loadClientSettings());
    act(() => result.current.onOfficeScaleAuto());

    expect(document.documentElement.dataset.officeScale).toBe("auto");
    expect(document.documentElement.style.getPropertyValue("--office-scale")).toBe("");
    expect(JSON.parse(localStorage.getItem(SETTINGS_KEY)!)).toMatchObject({
      officeScale: "auto",
    });
  });
});
