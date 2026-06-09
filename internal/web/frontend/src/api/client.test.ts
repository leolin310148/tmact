import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { logFrontend } from "../lib/frontendLog";
import { fetchSnapshot, subscribeSnapshot } from "./client";

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
});
