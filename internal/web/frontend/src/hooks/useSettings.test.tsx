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
      paneFontFace: "maple-mono",
    });
  });

  it("persists an explicit system pick across loads", () => {
    const { result } = renderHook(() => useSettings());

    act(() => result.current.onFontFamilyChange("system"));
    act(() => result.current.loadClientSettings());

    expect(document.documentElement.dataset.paneFontFamily).toBe("system");
  });

  it("defaults to Maple Mono CN without persisting the default", () => {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify({ paneFontFace: "comic-sans" }));
    const { result } = renderHook(() => useSettings());

    act(() => result.current.loadClientSettings());

    expect(document.documentElement.dataset.paneFontFamily).toBe("maple-mono");
    expect(JSON.parse(localStorage.getItem(SETTINGS_KEY)!).paneFontFace).toBe("comic-sans");
  });

  it("ignores the legacy auto-saved paneFontFamily value", () => {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify({ paneFontFamily: "system" }));
    const { result } = renderHook(() => useSettings());

    act(() => result.current.loadClientSettings());

    expect(document.documentElement.dataset.paneFontFamily).toBe("maple-mono");
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
