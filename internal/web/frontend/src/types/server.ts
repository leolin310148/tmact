// Server contract types — the frozen boundary between the React app and the Go
// `tmact statusd` web server (internal/web/*.go). The Go server is immutable;
// these types MUST match the JSON it emits byte-for-name. Field names are
// snake_case exactly as the Go struct json tags produce them.
//
// Authoritative sources:
//   - MIGRATION_SPEC.md §2 (frozen server contract)
//   - internal/statusd/snapshot.go        (Snapshot / SessionStatus / PaneStatus / Summary / SnapshotError)
//   - internal/prompt/question.go         (Question / Choice — the `prompt`/`q` payload)
//   - internal/agentusage/types.go + pace.go (AgentUsage / ProviderUsage / RateWindow / CostWindow / Pace)
//   - internal/web/stt_handlers.go        (STT settings GET/PUT)
//   - internal/web/server.go              (/api/version)
//   - internal/web/pane_ws.go             (WS inputMsg / outMsg)
//
// Notes on optionality: a Go field tagged `,omitempty` is absent from the JSON
// when it holds its zero value, so it is modeled here as an optional `?` field.
// A pointer field (`*time.Time`, `*prompt.Question`, `*Pace`, `*CostWindow`)
// with `,omitempty` may be absent; where the spec or client logic treats it as
// an explicit `null`, the type is widened to `T | null` per the spec.

// ---------------------------------------------------------------------------
// Snapshot (GET /api/snapshot, SSE /api/snapshot/stream) — internal/statusd
// ---------------------------------------------------------------------------

/**
 * Top-level statusd snapshot. `panes` is keyed by target; `sessions` by session
 * name. Both maps include peer-prefixed keys ("<peer>@...") for merged remotes.
 */
export interface Snapshot {
  /** Always 1 (current schema version). */
  version: number;
  /** Generation time, ISO8601 (Go json tag is "ts"). */
  ts: string;
  /** Producer identifier, e.g. "tmact statusd". */
  generated_by: string;
  /** Poll interval in milliseconds. */
  interval_ms: number;
  /** Staleness threshold in milliseconds. */
  stale_after_ms: number;
  summary: Summary;
  /** Keyed by session name. */
  sessions: Record<string, SessionStatus>;
  /** Keyed by target. */
  panes: Record<string, PaneStatus>;
  /** Up to 32 collection errors; absent (omitempty) when none. */
  errors?: SnapshotError[];
}

export interface Summary {
  sessions: number;
  panes: number;
  working: number;
  asking: number;
  errors: number;
}

export interface SessionStatus {
  session: string;
  /** Go: `session_id,omitempty` (spec §2.3 labels this "id"). */
  session_id?: string;
  /** Active pane target; absent when none. */
  active_target?: string;
  tag: string;
  runtime: string;
  /** "working" | "idle" | "unknown" | "waiting_permission" (free-form string). */
  state: string;
  running: boolean;
  asking: boolean;
  stale: boolean;
  /** 0..2 visual row hint, NOT semantic. */
  row_bucket: number;
  updated_at: string;
  /** Empty for local sessions; the peer name for merged remotes (omitempty). */
  peer?: string;
}

export interface PaneStatus {
  target: string;
  /** Go: `pane_id,omitempty`. */
  pane_id?: string;
  session: string;
  /** Go: `session_id,omitempty`. */
  session_id?: string;
  window_index: number;
  /** Go: `window,omitempty` (window name). */
  window?: string;
  pane_index: number;
  /** Go: `cwd,omitempty`. */
  cwd?: string;
  /** Go: `current_command,omitempty`. */
  current_command?: string;
  runtime: string;
  tag: string;
  /** "working" | "idle" | "unknown" | "waiting_permission" (free-form string). */
  state: string;
  idle: boolean;
  input_ready: boolean;
  running: boolean;
  asking: boolean;
  /** Go: `stale,omitempty` — true only for stale merged-peer panes. */
  stale?: boolean;
  /**
   * Go: `confidence,omitempty` — emitted as a STRING ("", "low", "medium",
   * "high"), NOT a number. (MIGRATION_SPEC §2.3 mislabels it `number`; the
   * server is authoritative — see deviation note.)
   */
  confidence?: string;
  /** Go: `signals,omitempty`. */
  signals?: string[];
  /**
   * Interactive menu detected in pane output, or null/absent when none.
   * Go: `*prompt.Question` with `,omitempty` → may be absent or null.
   */
  prompt?: Question | null;
  /** Go: `last_line,omitempty`. */
  last_line?: string;
  /**
   * Go: `*time.Time` with `,omitempty` → RFC3339 string once a change has been
   * observed, otherwise absent/null (null until first run). Spec §2.3 models
   * this explicitly as `string | null`.
   */
  last_changed_at?: string | null;
  updated_at: string;
  /** Go: `error,omitempty`. */
  error?: string;
  /** Empty for local panes; the peer name for merged remotes (omitempty). */
  peer?: string;
}

export interface SnapshotError {
  scope: string;
  /** Go: `target,omitempty`. */
  target?: string;
  error: string;
}

// ---------------------------------------------------------------------------
// Question / Choice — internal/prompt/question.go
// Rides on PaneStatus.prompt and on every WS "patch" message as `q`.
// ---------------------------------------------------------------------------

export interface Question {
  /** Go: `prompt,omitempty` — the menu's question text. */
  prompt?: string;
  /** Numbered choices; always present (Go tag has no omitempty). */
  choices: Choice[];
}

export interface Choice {
  number: number;
  /** Go: `label,omitempty`. */
  label?: string;
}

// ---------------------------------------------------------------------------
// Agent usage (GET /api/agent-usage) — internal/agentusage
// ---------------------------------------------------------------------------

export interface AgentUsage {
  generated_at: string;
  providers: ProviderUsage[];
}

export interface ProviderUsage {
  /** "claude" | "codex" | (other provider keys). */
  provider: string;
  /** Go: `account,omitempty`. */
  account?: string;
  /** Go: `plan,omitempty` — subscription tier ("max" | "pro" | "plus" | "team" | ...). */
  plan?: string;
  /** Go: `windows,omitempty` — rolling rate-limit windows, most-significant first. */
  windows?: RateWindow[];
  /**
   * Go: `*CostWindow` with `,omitempty` — present only when the plan exposes a
   * metered spend window; absent/null otherwise.
   */
  cost?: CostWindow | null;
  /**
   * Go: `*SpendWindow` with `,omitempty` — locally-computed dollar-equivalent
   * token spend (LiteLLM-priced, like codeburn) for the current calendar week
   * and month. Absent/null when the provider has no local session logs.
   */
  spend?: SpendWindow | null;
  /** Go: `error,omitempty` — per-provider failure reason. */
  error?: string;
  /**
   * Go: `stale,omitempty` — true when `windows` is a last-known reading kept
   * after the latest refresh failed (e.g. an expired agent token). `error`
   * carries the failure reason. Render the block dimmed rather than as an error.
   */
  stale?: boolean;
  /** Go: `*time.Time` `stale_since,omitempty` — when refreshes began failing. */
  stale_since?: string | null;
}

export interface SpendWindow {
  /** Week-to-date dollar-equivalent token spend (USD). */
  week_usd: number;
  /** Month-to-date dollar-equivalent token spend (USD). */
  month_usd: number;
}

export interface RateWindow {
  /** Stable window key: "session" | "weekly" | "weekly_opus" | "weekly_sonnet" | ... */
  name: string;
  /** 0..100 (may exceed 100 when over allowance). */
  used_percent: number;
  /** Go: `window_minutes,omitempty` (300 = 5h, 10080 = 7d). */
  window_minutes?: number;
  /**
   * Go: `*time.Time` with `,omitempty` — RFC3339 reset time when known,
   * otherwise absent. Spec §2.5 models it as `string | null`.
   */
  resets_at?: string | null;
  /**
   * Go: `*Pace` with `,omitempty` — present only when ResetsAt and
   * WindowMinutes are known; absent/null otherwise.
   */
  pace?: Pace | null;
}

export type PaceStage =
  | "on_track"
  | "slightly_ahead"
  | "ahead"
  | "far_ahead"
  | "slightly_behind"
  | "behind"
  | "far_behind";

export interface Pace {
  stage: PaceStage;
  /** actual − expected: positive = ahead of pace (deficit), negative = behind (reserve). */
  delta_percent: number;
  expected_percent: number;
  actual_percent: number;
  /**
   * Go: `*float64` with `,omitempty` — projected seconds until usage hits 100%
   * at the current rate; set only when the window is expected to exhaust before
   * reset. Spec §2.5 models it as `number | null`.
   */
  eta_seconds?: number | null;
  /** True when the current rate would not exhaust the window before reset. */
  lasts_until_reset: boolean;
}

export interface CostWindow {
  enabled: boolean;
  /** Major currency units (dollars), already converted from any cents source. */
  used: number;
  limit: number;
  /** Go: `currency,omitempty`. */
  currency?: string;
  /** Go: `unlimited,omitempty` — plans with no spend cap. */
  unlimited?: boolean;
}

// ---------------------------------------------------------------------------
// STT settings (GET / PUT /api/settings/stt) — internal/web/stt_handlers.go
// ---------------------------------------------------------------------------

/** Response shape for both GET and PUT (PUT returns the post-validation state). */
export interface STTSettings {
  model: string;
  endpoint: string;
  /** APIKey != "" — the key itself is never returned. */
  configured: boolean;
}

/**
 * PUT request body. Blank `api_key` keeps the existing key (merge, not erase).
 * Model/endpoint are trimmed server-side.
 */
export interface STTSettingsInput {
  model: string;
  endpoint: string;
  api_key: string;
}

// ---------------------------------------------------------------------------
// Version (GET /api/version) — internal/web/server.go
// ---------------------------------------------------------------------------

export interface VersionInfo {
  /** VCS build timestamp (ISO8601), or "" when unknown. */
  build_time: string;
  /** First 12 hex chars of the embedded-static SHA256. */
  asset_hash: string;
}

// ---------------------------------------------------------------------------
// WebSocket protocol (/ws/pane?pane=<id|peer@id>) — internal/web/pane_ws.go
// ---------------------------------------------------------------------------

/**
 * Client → server. `inputMsg`. Empty `s` for "text"/"send" is ignored by the
 * server; "resize" is a legacy no-op.
 */
export type InputMsg =
  | { t: "text"; s: string } // paste literal text, NO Enter
  | { t: "send"; s: string } // paste text + Enter
  | { t: "key"; k: string } // single allowlisted tmux key
  | { t: "clear" } // clear pane + scrollback
  | { t: "resize" }; // legacy, silently ignored

/**
 * Server → client. `outMsg`.
 * - "patch": initial connect is `from:0` with the full lines array (even
 *   when empty). Subsequent patches are an LCP line diff: `from = prefixCount`,
 *   `lines` = diverging tail. `q` rides on every patch (Question when an
 *   interactive menu is detected, else omitted/null).
 * - "error": shown to the user but does NOT close the socket.
 *
 * Go json tags use `,omitempty` on `from`/`lines`/`s`/`q`, so a "patch" with
 * `from:0` omits the `from` field entirely (decoded as 0) and an empty `lines`
 * tail omits `lines` (decoded as []). Optional markers reflect that.
 */
export type OutMsg =
  | { t: "patch"; from?: number; lines?: string[]; q?: Question | null }
  | { t: "error"; s: string };
