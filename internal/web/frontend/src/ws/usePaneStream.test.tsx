import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { logFrontend } from "../lib/frontendLog";
import { usePaneStream, type PaneStreamCallbacks } from "./usePaneStream";

vi.mock("../lib/frontendLog", () => ({
  logFrontend: vi.fn(),
}));

class FakeWebSocket {
  static instances: FakeWebSocket[] = [];
  static OPEN = 1;

  readyState = FakeWebSocket.OPEN;
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  sent: string[] = [];

  constructor(readonly url: string) {
    FakeWebSocket.instances.push(this);
  }

  send(data: string): void {
    this.sent.push(data);
  }

  close(): void {}
}

function callbacks(overrides: Partial<PaneStreamCallbacks> = {}): PaneStreamCallbacks {
  return {
    getSelectedPane: vi.fn(() => "%12"),
    onPatch: vi.fn(),
    onQuestion: vi.fn(),
    onError: vi.fn(),
    onStatus: vi.fn(),
    ...overrides,
  };
}

describe("usePaneStream logging", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    FakeWebSocket.instances = [];
    vi.stubGlobal("WebSocket", FakeWebSocket);
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("logs lifecycle, parse errors, server error frames, and socket errors", () => {
    const onError = vi.fn();
    const { result } = renderHook(() => usePaneStream(callbacks({ onError })));

    act(() => {
      result.current.open("%12");
    });
    const ws = FakeWebSocket.instances[0];
    expect(ws?.url).toContain("/ws/pane?pane=%2512");
    expect(logFrontend).toHaveBeenCalledWith("info", "pane_ws", "connecting", { pane: "%12" });

    act(() => {
      ws?.onopen?.();
      ws?.onmessage?.({ data: "not json" } as MessageEvent);
      ws?.onmessage?.({ data: `{"t":"error","s":"capture failed"}` } as MessageEvent);
      ws?.onerror?.();
    });

    expect(logFrontend).toHaveBeenCalledWith("info", "pane_ws", "open", { pane: "%12" });
    expect(logFrontend).toHaveBeenCalledWith("warn", "pane_ws", "message parse failed", {
      pane: "%12",
    });
    expect(logFrontend).toHaveBeenCalledWith("error", "pane_ws", "server error frame", {
      pane: "%12",
      error: "capture failed",
    });
    expect(logFrontend).toHaveBeenCalledWith("error", "pane_ws", "socket error", { pane: "%12" });
    expect(onError).toHaveBeenCalledWith("capture failed");
  });

  it("ignores queued events from a replaced socket", () => {
    const onPatch = vi.fn();
    const onStatus = vi.fn();
    const { result } = renderHook(() =>
      usePaneStream(callbacks({ onPatch, onStatus })),
    );

    act(() => {
      result.current.open("%12");
      result.current.open("%13");
    });
    const oldSocket = FakeWebSocket.instances[0];
    const currentSocket = FakeWebSocket.instances[1];

    act(() => {
      oldSocket?.onopen?.();
      oldSocket?.onmessage?.({
        data: `{"t":"patch","from":0,"lines":["stale"]}`,
      } as MessageEvent);
      oldSocket?.onerror?.();
    });

    expect(onPatch).not.toHaveBeenCalled();
    expect(onStatus).not.toHaveBeenCalledWith("open");
    expect(logFrontend).not.toHaveBeenCalledWith(
      "error",
      "pane_ws",
      "socket error",
      { pane: "%12" },
    );

    act(() => currentSocket?.onopen?.());
    expect(onStatus).toHaveBeenLastCalledWith("open");
  });
});
