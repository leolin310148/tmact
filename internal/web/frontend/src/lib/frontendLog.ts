export type FrontendLogLevel = "info" | "warn" | "error";

export interface FrontendLogEntry {
  ts: string;
  level: FrontendLogLevel;
  event: string;
  message: string;
  data?: Record<string, unknown>;
}

export interface FrontendLogDevice {
  platform: string;
  user_agent_summary?: string;
  user_agent_brands?: Array<{ brand: string; version: string }>;
  viewport: { width: number; height: number };
  screen: { width: number; height: number };
  device_pixel_ratio: number;
  orientation: string;
  visibility_state: string;
  online: boolean;
}

export interface FrontendLogPayload {
  session_id: string;
  sent_at: string;
  device: FrontendLogDevice;
  entries: FrontendLogEntry[];
}

interface NavigatorUADataLike {
  platform?: string;
  brands?: Array<{ brand?: string; version?: string }>;
}

interface NavigatorLike {
  userAgent?: string;
  platform?: string;
  userAgentData?: NavigatorUADataLike;
  onLine?: boolean;
  sendBeacon?: (url: string, data: BodyInit | null) => boolean;
}

interface WindowLike {
  innerWidth?: number;
  innerHeight?: number;
  screen?: {
    width?: number;
    height?: number;
    orientation?: { type?: string; angle?: number };
  };
  devicePixelRatio?: number;
  crypto?: {
    randomUUID?: Crypto["randomUUID"];
    getRandomValues?: Crypto["getRandomValues"];
  };
  addEventListener?: Window["addEventListener"];
  removeEventListener?: Window["removeEventListener"];
}

export interface FrontendLoggerOptions {
  endpoint?: string;
  now?: () => Date;
  fetchImpl?: typeof fetch;
  navigatorRef?: NavigatorLike;
  windowRef?: WindowLike;
  documentRef?: Document;
  setIntervalImpl?: typeof setInterval;
  clearIntervalImpl?: typeof clearInterval;
  sessionID?: string;
}

const MAX_BUFFER = 500;
const MAX_BATCH = 100;
const FLUSH_MS = 10000;
const MAX_MESSAGE = 300;
const MAX_DATA_STRING = 300;

export class FrontendLogger {
  readonly endpoint: string;

  private entries: FrontendLogEntry[] = [];
  private timer: ReturnType<typeof setInterval> | null = null;
  private readonly now: () => Date;
  private readonly fetchImpl: typeof fetch | undefined;
  private readonly navigatorRef: NavigatorLike | undefined;
  private readonly windowRef: WindowLike | undefined;
  private readonly documentRef: Document | undefined;
  private readonly setIntervalImpl: typeof setInterval;
  private readonly clearIntervalImpl: typeof clearInterval;
  private readonly sessionID: string;

  constructor(options: FrontendLoggerOptions = {}) {
    this.endpoint = options.endpoint ?? "/api/frontend-logs";
    this.now = options.now ?? (() => new Date());
    this.fetchImpl = options.fetchImpl ?? globalThis.fetch?.bind(globalThis);
    this.navigatorRef = options.navigatorRef ?? globalThis.navigator;
    this.windowRef = options.windowRef ?? globalThis.window;
    this.documentRef = options.documentRef ?? globalThis.document;
    this.setIntervalImpl =
      options.setIntervalImpl ?? globalThis.setInterval.bind(globalThis);
    this.clearIntervalImpl =
      options.clearIntervalImpl ?? globalThis.clearInterval.bind(globalThis);
    this.sessionID = options.sessionID ?? createSessionID(this.windowRef);
  }

  get size(): number {
    return this.entries.length;
  }

  snapshotEntries(): FrontendLogEntry[] {
    return this.entries.map((entry) => ({
      ...entry,
      data: entry.data ? { ...entry.data } : undefined,
    }));
  }

  deviceSummary(): FrontendLogDevice {
    const nav = this.navigatorRef;
    const win = this.windowRef;
    const doc = this.documentRef;
    const width = finiteInt(win?.innerWidth, 0);
    const height = finiteInt(win?.innerHeight, 0);
    const rawPlatform = nav?.userAgentData?.platform || nav?.platform || "";
    const brands = Array.isArray(nav?.userAgentData?.brands)
      ? nav?.userAgentData?.brands
          ?.slice(0, 8)
          .map((brand) => ({
            brand: cleanString(brand.brand ?? "", 80),
            version: cleanString(brand.version ?? "", 40),
          }))
          .filter((brand) => brand.brand)
      : undefined;

    return {
      platform: summarizePlatform(rawPlatform, nav?.userAgent ?? ""),
      ...(brands && brands.length > 0
        ? { user_agent_brands: brands }
        : { user_agent_summary: cleanString(nav?.userAgent ?? "", 160) }),
      viewport: { width, height },
      screen: {
        width: finiteInt(win?.screen?.width, 0),
        height: finiteInt(win?.screen?.height, 0),
      },
      device_pixel_ratio: finiteNumber(win?.devicePixelRatio, 1),
      orientation: orientationSummary(win, width, height),
      visibility_state: cleanString(doc?.visibilityState ?? "unknown", 40),
      online: nav?.onLine !== false,
    };
  }

  log(
    level: FrontendLogLevel,
    event: string,
    message: string,
    data?: Record<string, unknown>,
  ): void {
    this.entries.push({
      ts: this.now().toISOString(),
      level,
      event: cleanString(event, 80),
      message: cleanString(message, MAX_MESSAGE),
      ...(data ? { data: sanitizeData(data) } : {}),
    });
    if (this.entries.length > MAX_BUFFER) {
      this.entries.splice(0, this.entries.length - MAX_BUFFER);
    }
  }

  start(): void {
    if (this.timer !== null) return;
    this.timer = this.setIntervalImpl(() => {
      void this.flush();
    }, FLUSH_MS);
  }

  stop(): void {
    if (this.timer === null) return;
    this.clearIntervalImpl(this.timer);
    this.timer = null;
  }

  async flush(
    options: { beacon?: boolean; keepalive?: boolean } = {},
  ): Promise<boolean> {
    if (this.entries.length === 0) return true;
    const batch = this.entries.slice(0, MAX_BATCH);
    const payload = this.buildPayload(batch);

    if (options.beacon && this.navigatorRef?.sendBeacon) {
      try {
        const ok = this.navigatorRef.sendBeacon(
          this.endpoint,
          new Blob([JSON.stringify(payload)], { type: "application/json" }),
        );
        if (ok) {
          this.removeSent(batch);
          return true;
        }
      } catch {
        // fall through to keepalive fetch
      }
    }

    const fetchImpl = this.fetchImpl;
    if (!fetchImpl) return false;
    try {
      const res = await fetchImpl(this.endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
        keepalive: options.keepalive === true,
      });
      if (!res.ok) return false;
      this.removeSent(batch);
      return true;
    } catch {
      return false;
    }
  }

  buildPayload(entries: FrontendLogEntry[]): FrontendLogPayload {
    return {
      session_id: this.sessionID,
      sent_at: this.now().toISOString(),
      device: this.deviceSummary(),
      entries,
    };
  }

  private removeSent(batch: FrontendLogEntry[]): void {
    const sent = new Set(batch);
    this.entries = this.entries.filter((entry) => !sent.has(entry));
  }
}

const defaultLogger = new FrontendLogger();
let defaultInitialized = false;

export function logFrontend(
  level: FrontendLogLevel,
  event: string,
  message: string,
  data?: Record<string, unknown>,
): void {
  try {
    defaultLogger.log(level, event, message, data);
  } catch {
    // Logging must never affect the UI path.
  }
}

export function initFrontendLogging(): () => void {
  if (defaultInitialized) return () => {};
  defaultInitialized = true;
  defaultLogger.start();

  const listeners: Array<() => void> = [];
  const onWindow = <K extends keyof WindowEventMap>(
    type: K,
    listener: (event: WindowEventMap[K]) => void,
  ) => {
    window.addEventListener(type, listener);
    listeners.push(() => window.removeEventListener(type, listener));
  };
  const onDocument = (type: "visibilitychange", listener: () => void) => {
    document.addEventListener(type, listener);
    listeners.push(() => document.removeEventListener(type, listener));
  };

  const logLoad = () => logFrontend("info", "app_lifecycle", "page load");
  if (document.readyState === "complete") {
    logLoad();
  } else {
    onWindow("load", logLoad);
  }
  onDocument("visibilitychange", () => {
    logFrontend("info", "app_lifecycle", "visibility changed", {
      visibility_state: document.visibilityState,
    });
    if (document.visibilityState === "hidden") {
      void defaultLogger.flush({ beacon: true, keepalive: true });
    }
  });
  onWindow("online", () => logFrontend("info", "app_lifecycle", "online"));
  onWindow("offline", () => logFrontend("warn", "app_lifecycle", "offline"));
  onWindow("pagehide", () => {
    logFrontend("info", "app_lifecycle", "pagehide");
    void defaultLogger.flush({ beacon: true, keepalive: true });
  });
  onWindow("error", (event) => {
    logFrontend("error", "global_error", errorMessage(event.message || event.error), {
      lineno: event.lineno,
      colno: event.colno,
    });
  });
  onWindow("unhandledrejection", (event) => {
    logFrontend("error", "unhandled_rejection", errorMessage(event.reason));
  });

  return () => {
    defaultLogger.stop();
    for (const remove of listeners) remove();
    defaultInitialized = false;
  };
}

export function flushFrontendLogs(options?: {
  beacon?: boolean;
  keepalive?: boolean;
}): Promise<boolean> {
  return defaultLogger.flush(options);
}

function createSessionID(win: WindowLike | undefined): string {
  try {
    const uuid = win?.crypto?.randomUUID?.();
    if (uuid) return "browser-" + uuid;
    const values = win?.crypto?.getRandomValues?.(new Uint32Array(2));
    if (values) {
      return `browser-${values[0]?.toString(16) ?? "0"}${values[1]?.toString(16) ?? "0"}`;
    }
  } catch {
    // fall back below
  }
  return "browser-" + Math.random().toString(36).slice(2, 12);
}

function summarizePlatform(platform: string, userAgent: string): string {
  const raw = `${platform} ${userAgent}`.toLowerCase();
  if (raw.includes("iphone") || raw.includes("ipad") || raw.includes("ios")) {
    return "iOS";
  }
  if (raw.includes("android")) return "Android";
  if (raw.includes("mac")) return "macOS";
  if (raw.includes("win")) return "Windows";
  if (raw.includes("linux")) return "Linux";
  return "unknown";
}

function orientationSummary(
  win: WindowLike | undefined,
  width: number,
  height: number,
): string {
  const type = win?.screen?.orientation?.type;
  const angle = win?.screen?.orientation?.angle;
  const base = type?.includes("portrait")
    ? "portrait"
    : type?.includes("landscape")
      ? "landscape"
      : width >= height
        ? "landscape"
        : "portrait";
  return typeof angle === "number" ? `${base}@${angle}` : base;
}

function sanitizeData(input: Record<string, unknown>): Record<string, unknown> {
  return sanitizeObject(input, 0);
}

function sanitizeObject(
  input: Record<string, unknown>,
  depth: number,
): Record<string, unknown> {
  if (depth >= 4) return {};
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(input).slice(0, 40)) {
    const cleanKey = cleanString(key, 80);
    if (!cleanKey) continue;
    const cleanValue = sanitizeValue(value, depth + 1);
    if (cleanValue !== undefined) out[cleanKey] = cleanValue;
  }
  return out;
}

function sanitizeValue(value: unknown, depth: number): unknown {
  if (value === null) return null;
  if (typeof value === "string") return cleanString(value, MAX_DATA_STRING);
  if (typeof value === "number") return Number.isFinite(value) ? value : undefined;
  if (typeof value === "boolean") return value;
  if (Array.isArray(value)) {
    if (depth >= 4) return undefined;
    return value
      .slice(0, 20)
      .map((item) => sanitizeValue(item, depth + 1))
      .filter((item) => item !== undefined);
  }
  if (typeof value === "object") {
    if (depth >= 4) return undefined;
    return sanitizeObject(value as Record<string, unknown>, depth);
  }
  return undefined;
}

function cleanString(value: string, max: number): string {
  const clean = value.replace(/[\u0000-\u001f\u007f-\u009f]/g, "");
  return Array.from(clean).slice(0, max).join("");
}

function finiteInt(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value)
    ? Math.max(0, Math.round(value))
    : fallback;
}

function finiteNumber(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value)
    ? Math.max(0, value)
    : fallback;
}

function errorMessage(value: unknown): string {
  if (value instanceof Error) return value.message;
  if (typeof value === "string") return value;
  try {
    return JSON.stringify(value);
  } catch {
    return String(value);
  }
}
