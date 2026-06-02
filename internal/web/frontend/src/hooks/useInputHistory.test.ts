// Coverage for the frontend-only sent-message history (useInputHistory).
//
// Pins the small contract: record caps at 20 + dedups consecutive + persists,
// and ArrowUp/ArrowDown recall walks older→newer with a stash for the live
// draft. recall* return null when there is nothing to change.

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useInputHistory } from "./useInputHistory";

const KEY = "tmact.inputHistory";

beforeEach(() => localStorage.clear());
afterEach(() => localStorage.clear());

describe("useInputHistory", () => {
  it("records, dedups consecutive, and persists the last 20", () => {
    const { result } = renderHook(() => useInputHistory());
    act(() => {
      result.current.record("a");
      result.current.record("a"); // consecutive dup → skipped
      result.current.record("b");
    });
    expect(JSON.parse(localStorage.getItem(KEY)!)).toEqual(["a", "b"]);

    act(() => {
      for (let i = 0; i < 25; i++) result.current.record(`m${i}`);
    });
    const stored = JSON.parse(localStorage.getItem(KEY)!);
    expect(stored).toHaveLength(20);
    expect(stored[0]).toBe("m5");
    expect(stored[19]).toBe("m24");
  });

  it("ignores empty / whitespace-only messages", () => {
    const { result } = renderHook(() => useInputHistory());
    act(() => {
      result.current.record("   ");
      result.current.record("");
    });
    expect(localStorage.getItem(KEY)).toBeNull();
  });

  it("loads existing history from localStorage", () => {
    localStorage.setItem(KEY, JSON.stringify(["one", "two"]));
    const { result } = renderHook(() => useInputHistory());
    expect(result.current.recallPrev("draft")).toBe("two");
    expect(result.current.recallPrev("draft")).toBe("one");
  });

  it("recalls older→newer and restores the stashed draft", () => {
    const { result } = renderHook(() => useInputHistory());
    act(() => {
      result.current.record("first");
      result.current.record("second");
    });

    // ArrowUp from a live draft: stash "live", walk to newest then oldest.
    expect(result.current.navigating()).toBe(false);
    expect(result.current.recallPrev("live")).toBe("second");
    expect(result.current.navigating()).toBe(true);
    expect(result.current.recallPrev("live")).toBe("first");
    expect(result.current.recallPrev("live")).toBeNull(); // already oldest

    // ArrowDown walks back and restores the stash past the newest entry.
    expect(result.current.recallNext("x")).toBe("second");
    expect(result.current.recallNext("x")).toBe("live"); // restored stash
    expect(result.current.navigating()).toBe(false);
    expect(result.current.recallNext("x")).toBeNull(); // already live
  });

  it("reset leaves navigation mode", () => {
    const { result } = renderHook(() => useInputHistory());
    act(() => result.current.record("only"));
    expect(result.current.recallPrev("live")).toBe("only");
    expect(result.current.navigating()).toBe(true);
    act(() => result.current.reset());
    expect(result.current.navigating()).toBe(false);
  });
});
