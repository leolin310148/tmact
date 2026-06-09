// Typed API client — a faithful 1:1 port of static/js/api.js. Function names,
// request paths, headers, cache modes, error strings, and the shallow
// {res, data} envelope are preserved byte-for-behavior. No timeouts are added.
//
// snapshotEtag / lastSnapshot are module-scoped MUTABLE state (let), exactly as
// in api.js, so repeated polls can return the cached snapshot on a 304. The Go
// server currently emits no ETag (see MIGRATION_SPEC §2.3), so this is a
// no-op-safe defensive path: if 304 never comes back, the cache simply never
// serves stale.

import type {
  Snapshot,
  AgentUsage,
  STTSettings,
  STTSettingsInput,
  VersionInfo,
} from "../types/server";
import { logFrontend } from "../lib/frontendLog";

// Shallow envelope returned by every non-snapshot wrapper: the raw Response plus
// the parsed JSON body (or {} when the body is absent / not valid JSON).
export interface JsonResponse<T> {
  res: Response;
  data: T;
}

async function jsonResponse<T>(
  url: string,
  options?: RequestInit,
): Promise<JsonResponse<T>> {
  let res: Response;
  try {
    res = await fetch(url, options);
  } catch (e) {
    logFrontend("error", "api_error", "network request failed", {
      url,
      method: options?.method ?? "GET",
      error: errorSummary(e),
    });
    throw e;
  }
  if (!res.ok) {
    logFrontend("warn", "api_error", "HTTP request failed", {
      url,
      method: options?.method ?? "GET",
      status: res.status,
    });
  }
  let data = {} as T;
  try {
    data = (await res.json()) as T;
  } catch (e) {
    logFrontend("warn", "api_error", "JSON response parse failed", {
      url,
      method: options?.method ?? "GET",
      status: res.status,
      error: errorSummary(e),
    });
  }
  return { res, data };
}

let snapshotEtag = "";

// fetchSnapshot caches the snapshot ETag so repeated polls return 304 from
// the server until the daemon writes a new file. The cached snapshot is
// returned on 304 so callers always see a value.
let lastSnapshot: Snapshot | null = null;
export async function fetchSnapshot(): Promise<Snapshot> {
  const headers: Record<string, string> = {};
  if (snapshotEtag) headers["If-None-Match"] = snapshotEtag;
  let res: Response;
  try {
    res = await fetch("api/snapshot", { cache: "no-store", headers });
  } catch (e) {
    logFrontend("error", "api_error", "snapshot request failed", {
      url: "api/snapshot",
      error: errorSummary(e),
    });
    throw e;
  }
  if (res.status === 304) {
    if (!lastSnapshot) {
      logFrontend("warn", "api_error", "snapshot cache miss on 304", {
        url: "api/snapshot",
        status: res.status,
      });
      throw new Error("HTTP 304 without cached snapshot");
    }
    return lastSnapshot;
  }
  if (!res.ok) {
    logFrontend("warn", "api_error", "snapshot HTTP request failed", {
      url: "api/snapshot",
      status: res.status,
    });
    throw new Error("HTTP " + res.status);
  }
  const tag = res.headers.get("ETag");
  if (tag) snapshotEtag = tag;
  try {
    lastSnapshot = (await res.json()) as Snapshot;
  } catch (e) {
    logFrontend("warn", "api_error", "snapshot JSON parse failed", {
      url: "api/snapshot",
      status: res.status,
      error: errorSummary(e),
    });
    throw e;
  }
  return lastSnapshot;
}

// subscribeSnapshot opens an SSE stream; onSnapshot fires for every push,
// onError when the connection breaks. The caller is expected to fall back to
// fetchSnapshot polling on error. Returns a close() function.
export function subscribeSnapshot(
  onSnapshot: (snapshot: Snapshot) => void,
  onError: (err: Error) => void,
): () => void {
  if (typeof EventSource === "undefined") {
    logFrontend("warn", "snapshot_stream", "EventSource unavailable");
    onError(new Error("EventSource not supported"));
    return () => {};
  }
  const es = new EventSource("/api/snapshot/stream");
  es.onopen = () => {
    logFrontend("info", "snapshot_stream", "stream open");
  };
  es.addEventListener("snapshot", (ev: MessageEvent) => {
    try {
      onSnapshot(JSON.parse(ev.data) as Snapshot);
    } catch (e) {
      logFrontend("warn", "snapshot_stream", "snapshot message parse failed", {
        error: errorSummary(e),
      });
    }
  });
  es.onerror = () => {
    es.close();
    logFrontend("warn", "snapshot_stream", "stream closed");
    onError(new Error("snapshot stream closed"));
  };
  return () => es.close();
}

export function transcribeAudio(form: FormData): Promise<JsonResponse<{ text?: string }>> {
  return jsonResponse("/api/transcribe", { method: "POST", body: form });
}

function peerQuery(peer?: string): string {
  return peer ? "?peer=" + encodeURIComponent(peer) : "";
}

export function uploadClipboardImage(
  form: FormData,
  peer?: string,
): Promise<JsonResponse<{ path?: string }>> {
  return jsonResponse("/api/paste-image" + peerQuery(peer), { method: "POST", body: form });
}

export function uploadPaneFiles(
  form: FormData,
  peer?: string,
): Promise<JsonResponse<{ path?: string; paths?: string[] }>> {
  return jsonResponse("/api/upload-file" + peerQuery(peer), { method: "POST", body: form });
}

export function loadSTTConfig(): Promise<JsonResponse<STTSettings>> {
  return jsonResponse("/api/settings/stt", { cache: "no-store" });
}

export function loadVersion(): Promise<JsonResponse<VersionInfo>> {
  return jsonResponse("/api/version", { cache: "no-store" });
}

export function loadAgentUsage(): Promise<JsonResponse<AgentUsage>> {
  return jsonResponse("/api/agent-usage", { cache: "no-store" });
}

export function saveSTTConfig(payload: STTSettingsInput): Promise<JsonResponse<STTSettings>> {
  return jsonResponse("/api/settings/stt", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

function errorSummary(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "string") return err;
  try {
    return JSON.stringify(err);
  } catch {
    return String(err);
  }
}
