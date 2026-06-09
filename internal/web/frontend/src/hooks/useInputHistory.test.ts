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
      result.current.record("%1", "a");
      result.current.record("%1", "a"); // consecutive dup → skipped
      result.current.record("%1", "b");
    });
    expect(JSON.parse(localStorage.getItem(KEY)!)).toEqual({ panes: { "%1": ["a", "b"] } });

    act(() => {
      for (let i = 0; i < 25; i++) result.current.record("%1", `m${i}`);
    });
    const stored = JSON.parse(localStorage.getItem(KEY)!).panes["%1"];
    expect(stored).toHaveLength(20);
    expect(stored[0]).toBe("m5");
    expect(stored[19]).toBe("m24");
  });

  it("ignores empty / whitespace-only messages", () => {
    const { result } = renderHook(() => useInputHistory());
    act(() => {
      result.current.record("%1", "   ");
      result.current.record("%1", "");
      result.current.record(null, "ignored");
    });
    expect(localStorage.getItem(KEY)).toBeNull();
  });

  it("loads existing history from localStorage", () => {
    localStorage.setItem(KEY, JSON.stringify({ panes: { "%1": ["one", "two"] } }));
    const { result } = renderHook(() => useInputHistory());
    expect(result.current.recallPrev("%1", "draft")).toBe("two");
    expect(result.current.recallPrev("%1", "draft")).toBe("one");
  });

  it("recalls older→newer and restores the stashed draft", () => {
    const { result } = renderHook(() => useInputHistory());
    act(() => {
      result.current.record("%1", "first");
      result.current.record("%1", "second");
    });

    // ArrowUp from a live draft: stash "live", walk to newest then oldest.
    expect(result.current.navigating("%1")).toBe(false);
    expect(result.current.recallPrev("%1", "live")).toBe("second");
    expect(result.current.navigating("%1")).toBe(true);
    expect(result.current.recallPrev("%1", "live")).toBe("first");
    expect(result.current.recallPrev("%1", "live")).toBeNull(); // already oldest

    // ArrowDown walks back and restores the stash past the newest entry.
    expect(result.current.recallNext("%1", "x")).toBe("second");
    expect(result.current.recallNext("%1", "x")).toBe("live"); // restored stash
    expect(result.current.navigating("%1")).toBe(false);
    expect(result.current.recallNext("%1", "x")).toBeNull(); // already live
  });

  it("keeps recall history separate per pane", () => {
    const { result } = renderHook(() => useInputHistory());
    act(() => {
      result.current.record("%1", "pane one");
      result.current.record("%2", "pane two");
    });

    expect(result.current.recallPrev("%1", "draft 1")).toBe("pane one");
    expect(result.current.recallPrev("%2", "draft 2")).toBe("pane two");
    expect(result.current.recallNext("%1", "x")).toBe("draft 1");
    expect(result.current.recallNext("%2", "x")).toBe("draft 2");
  });

  it("reset leaves navigation mode", () => {
    const { result } = renderHook(() => useInputHistory());
    act(() => result.current.record("%1", "only"));
    expect(result.current.recallPrev("%1", "live")).toBe("only");
    expect(result.current.navigating("%1")).toBe(true);
    act(() => result.current.reset("%1"));
    expect(result.current.navigating("%1")).toBe(false);
  });
});
