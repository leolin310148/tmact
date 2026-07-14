import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { loadAgentUsage } from "../api/client";
import type { AgentUsage } from "../types/server";
import { useUsage } from "./useUsage";

vi.mock("../api/client", () => ({ loadAgentUsage: vi.fn() }));

const mockedLoadAgentUsage = vi.mocked(loadAgentUsage);
const POLL_MS = 60000;

type UsageResult = Awaited<ReturnType<typeof loadAgentUsage>>;
const EMPTY_USAGE: AgentUsage = {
  generated_at: "2026-07-15T00:00:00Z",
  providers: [],
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function result(status: number, data: AgentUsage = EMPTY_USAGE): UsageResult {
  return {
    res: { ok: status >= 200 && status < 300, status } as Response,
    data,
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  mockedLoadAgentUsage.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useUsage", () => {
  it("waits for a slow request to finish before scheduling the next poll", async () => {
    const first = deferred<UsageResult>();
    const second = deferred<UsageResult>();
    mockedLoadAgentUsage
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { result: hookResult } = renderHook(() => useUsage());
    expect(mockedLoadAgentUsage).toHaveBeenCalledTimes(1);

    await act(async () => vi.advanceTimersByTimeAsync(POLL_MS * 2));
    expect(mockedLoadAgentUsage).toHaveBeenCalledTimes(1);

    const firstSnap: AgentUsage = {
      generated_at: "2026-07-15T00:00:00Z",
      providers: [],
    };
    await act(async () => first.resolve(result(200, firstSnap)));
    expect(hookResult.current.snap).toBe(firstSnap);

    await act(async () => vi.advanceTimersByTimeAsync(POLL_MS - 1));
    expect(mockedLoadAgentUsage).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTimeAsync(1));
    expect(mockedLoadAgentUsage).toHaveBeenCalledTimes(2);

    await act(async () => second.resolve(result(500)));
  });

  it("retries one cadence after a failed request settles", async () => {
    mockedLoadAgentUsage
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(result(500));

    renderHook(() => useUsage());
    await act(async () => Promise.resolve());
    expect(mockedLoadAgentUsage).toHaveBeenCalledTimes(1);

    await act(async () => vi.advanceTimersByTimeAsync(POLL_MS));
    expect(mockedLoadAgentUsage).toHaveBeenCalledTimes(2);
  });

  it("stops permanently and reports disabled after a 404", async () => {
    mockedLoadAgentUsage.mockResolvedValue(result(404));

    const { result: hookResult } = renderHook(() => useUsage());
    await act(async () => Promise.resolve());

    expect(hookResult.current.disabled).toBe(true);
    await act(async () => vi.advanceTimersByTimeAsync(POLL_MS * 2));
    expect(mockedLoadAgentUsage).toHaveBeenCalledTimes(1);
  });

  it("does not update or schedule another poll when an in-flight request settles after unmount", async () => {
    const pending = deferred<UsageResult>();
    mockedLoadAgentUsage.mockReturnValueOnce(pending.promise);

    const { unmount } = renderHook(() => useUsage());
    expect(mockedLoadAgentUsage).toHaveBeenCalledTimes(1);
    unmount();

    await act(async () => pending.resolve(result(200, EMPTY_USAGE)));
    await act(async () => vi.advanceTimersByTimeAsync(POLL_MS * 2));
    expect(mockedLoadAgentUsage).toHaveBeenCalledTimes(1);
  });
});
