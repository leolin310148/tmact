import { describe, expect, it, vi } from "vitest";
import { FrontendLogger, type FrontendLoggerOptions } from "./frontendLog";

const now = () => new Date("2026-06-09T10:00:00.000Z");

function makeLogger(options: Partial<FrontendLoggerOptions> = {}) {
  return new FrontendLogger({
    now,
    sessionID: "browser-test",
    setIntervalImpl: vi.fn() as unknown as typeof setInterval,
    clearIntervalImpl: vi.fn() as unknown as typeof clearInterval,
    navigatorRef: {
      platform: "MacIntel",
      userAgent: "Mozilla/5.0 test browser",
      onLine: true,
    },
    windowRef: {
      innerWidth: 1440,
      innerHeight: 900,
      screen: { width: 1440, height: 900, orientation: { type: "landscape-primary", angle: 0 } },
      devicePixelRatio: 2,
    },
    documentRef: { visibilityState: "visible" } as Document,
    ...options,
  });
}

describe("FrontendLogger", () => {
  it("builds a safe device summary", () => {
    const logger = makeLogger();
    const device = logger.deviceSummary();

    expect(device.platform).toBe("macOS");
    expect(device.user_agent_summary).toBe("Mozilla/5.0 test browser");
    expect(device.viewport).toEqual({ width: 1440, height: 900 });
    expect(device.screen).toEqual({ width: 1440, height: 900 });
    expect(device.device_pixel_ratio).toBe(2);
    expect(device.orientation).toBe("landscape@0");
    expect(device.visibility_state).toBe("visible");
    expect(device.online).toBe(true);
  });

  it("keeps only the newest 500 entries", () => {
    const logger = makeLogger();
    for (let i = 0; i < 501; i++) {
      logger.log("info", "app_lifecycle", String(i));
    }

    const entries = logger.snapshotEntries();
    expect(entries).toHaveLength(500);
    expect(entries[0]?.message).toBe("1");
    expect(entries[499]?.message).toBe("500");
  });

  it("clears sent entries after a successful flush", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(`{"ok":true}`, { status: 200 }));
    const logger = makeLogger({ fetchImpl: fetchMock as unknown as typeof fetch });
    logger.log("warn", "api_error", "HTTP request failed", { status: 500 });

    await expect(logger.flush()).resolves.toBe(true);

    expect(logger.size).toBe(0);
    const [, init] = fetchMock.mock.calls[0] ?? [];
    const body = JSON.parse(String((init as RequestInit).body));
    expect(body).toMatchObject({
      session_id: "browser-test",
      device: { platform: "macOS" },
    });
    expect(body.entries[0]).toMatchObject({
      level: "warn",
      event: "api_error",
      message: "HTTP request failed",
    });
  });

  it("keeps entries after a failed flush without throwing", async () => {
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response("nope", { status: 500 }));
    const logger = makeLogger({ fetchImpl: fetchMock as unknown as typeof fetch });
    logger.log("error", "api_error", "network request failed");

    await expect(logger.flush()).resolves.toBe(false);

    expect(logger.size).toBe(1);
  });

  it("sends the same payload shape through sendBeacon", async () => {
    let blob: BodyInit | null = null;
    const sendBeacon = vi.fn((url: string, data: BodyInit | null) => {
      expect(url).toBe("/api/frontend-logs");
      blob = data;
      return true;
    });
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) => new Response(`{"ok":true}`, { status: 200 }));
    const logger = makeLogger({
      fetchImpl: fetchMock as unknown as typeof fetch,
      navigatorRef: {
        platform: "MacIntel",
        userAgent: "Mozilla/5.0 test browser",
        onLine: true,
        sendBeacon,
      },
    });
    logger.log("info", "app_lifecycle", "pagehide");

    await expect(logger.flush({ beacon: true, keepalive: true })).resolves.toBe(true);

    expect(fetchMock).not.toHaveBeenCalled();
    expect(logger.size).toBe(0);
    expect(blob).toBeInstanceOf(Blob);
    const payload = JSON.parse(await (blob as unknown as Blob).text());
    expect(payload).toMatchObject({
      session_id: "browser-test",
      entries: [{ event: "app_lifecycle", message: "pagehide" }],
    });
  });
});
