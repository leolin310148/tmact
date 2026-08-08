import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { logFrontend } from "../lib/frontendLog";
import {
  fetchSnapshot,
  loadCommandJob,
  startBackgroundCommand,
  startTmuxCommand,
  subscribeSnapshot,
} from "./client";

vi.mock("../lib/frontendLog", () => ({
  logFrontend: vi.fn(),
}));

class FakeEventSource {
  static instances: FakeEventSource[] = [];

  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  listeners: Record<string, Array<(event: MessageEvent) => void>> = {};

  constructor(readonly url: string) {
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void): void {
    this.listeners[type] = [...(this.listeners[type] ?? []), listener];
  }

  close(): void {}

  emit(type: string, data: string): void {
    for (const listener of this.listeners[type] ?? []) {
      listener({ data } as MessageEvent);
    }
  }
}

describe("api client logging", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    FakeEventSource.instances = [];
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("logs snapshot HTTP errors", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("nope", { status: 503 })));

    await expect(fetchSnapshot()).rejects.toThrow("HTTP 503");

    expect(logFrontend).toHaveBeenCalledWith(
      "warn",
      "api_error",
      "snapshot HTTP request failed",
      expect.objectContaining({ status: 503 }),
    );
  });

  it("logs SSE open, parse errors, and close", () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    const onSnapshot = vi.fn();
    const onError = vi.fn();

    subscribeSnapshot(onSnapshot, onError);
    const source = FakeEventSource.instances[0];
    expect(source?.url).toBe("/api/snapshot/stream?view=web");
    source?.onopen?.();
    source?.emit("snapshot", "{");
    source?.onerror?.();

    expect(logFrontend).toHaveBeenCalledWith("info", "snapshot_stream", "stream open");
    expect(logFrontend).toHaveBeenCalledWith(
      "warn",
      "snapshot_stream",
      "snapshot message parse failed",
      expect.any(Object),
    );
    expect(logFrontend).toHaveBeenCalledWith("warn", "snapshot_stream", "stream closed");
    expect(onSnapshot).not.toHaveBeenCalled();
    expect(onError).toHaveBeenCalled();
  });

  it("delivers compact snapshot heartbeat events without a full snapshot", () => {
    vi.stubGlobal("EventSource", FakeEventSource);
    const onSnapshot = vi.fn();
    const onError = vi.fn();
    const onHeartbeat = vi.fn();

    subscribeSnapshot(onSnapshot, onError, onHeartbeat);
    const source = FakeEventSource.instances[0];
    source?.emit(
      "heartbeat",
      JSON.stringify({
        ts: "2026-08-08T00:00:05.000Z",
        interval_ms: 500,
        stale_after_ms: 10000,
      }),
    );

    expect(onHeartbeat).toHaveBeenCalledWith({
      ts: "2026-08-08T00:00:05.000Z",
      interval_ms: 500,
      stale_after_ms: 10000,
    });
    expect(onSnapshot).not.toHaveBeenCalled();
    expect(onError).not.toHaveBeenCalled();
  });

  it("routes federated commands to the peer with a local pane id", async () => {
    const fetchMock = vi.fn(async () =>
      new Response(
        JSON.stringify({
          pane_id: "%12",
          session: "work-run",
        }),
        { status: 201, headers: { "Content-Type": "application/json" } },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await startTmuxCommand("mini@%7", "make test");
    await startBackgroundCommand("mini@%7", "make test");

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/command?peer=mini",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ pane: "%7", command: "make test", mode: "tmux" }),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/command?peer=mini",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ pane: "%7", command: "make test", mode: "background" }),
      }),
    );
  });

  it("polls a command job on its originating peer", async () => {
    const fetchMock = vi.fn(async () =>
      new Response(JSON.stringify({ job: { id: "abc", status: "running" } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await loadCommandJob("abc", "mini");

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/command?id=abc&peer=mini",
      expect.objectContaining({ cache: "no-store" }),
    );
  });
});
