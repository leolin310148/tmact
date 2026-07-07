// UsagePanel (#usage-panel) — 1:1 port of static/js/usage.js render()/appendWindow().
//
// Agent quota / rate-limit usage panel (top-right overlay). One runtime badge
// (cc / cx) per provider, then a line per rate window showing remaining % /
// pace reserve / reset countdown — session on top, weekly below. Data comes
// from /api/agent-usage via useUsage (statusd refreshes it on a slow ~5m
// ticker; the panel re-polls every 60 s and recomputes the reset countdown
// locally against the current wall clock on every render).
//
// WIRING DECISION (documented per task): UsagePanel calls useUsage() itself —
// App only mounts <UsagePanel/> inside #content-wrap (after #content / before
// the input bar, matching index.html order). This keeps the 60 s poll lifecycle
// co-located with the only consumer, matches ARCHITECTURE.md §3 ("useUsage owns
// the poll, renders into UsagePanel"), and means App does NOT call useUsage —
// the hook's public signature App would care about is effectively void.
//
// IMPERATIVE-DOM NOTE: the original built the grid with createElement (`h`).
// Here it is declarative JSX with IDENTICAL ids/classes/cell order so app.css
// (the verbatim grid in `.usage-panel`) applies. The `#usage-panel` `hidden`
// attribute is toggled exactly as usage.js toggled `panel.hidden`
// (true on 0 providers / no data / server-disabled, false otherwise). The
// static `aria-hidden="true"` from index.html is preserved unchanged (the
// original render() never touched it).

import { Fragment } from "react";
import { useUsage } from "../hooks/useUsage";
import type { AgentUsage, Pace, ProviderUsage, RateWindow } from "../types/server";

// fmtUSD renders a dollar-equivalent spend compactly: <$10 keeps cents
// ("$5.62"), <$1000 drops them ("$562"), ≥$1000 uses a k suffix ("$3.5k").
// Mirrors the spirit of codeburn's compact cost columns.
function fmtUSD(n: number): string {
  if (!(n > 0)) return "$0";
  if (n < 10) return "$" + n.toFixed(2);
  if (n < 1000) return "$" + Math.round(n);
  return "$" + (n / 1000).toFixed(1) + "k";
}

// RUNTIME_ICON mirrors usage.js exactly.
const RUNTIME_ICON: Record<string, string> = {
  claude: "cc",
  codex: "cx",
  copilot: "cp",
  gemini: "g",
};

// fmtCountdown — verbatim from usage.js. Full "resets in …" phrasing for the
// % cell's tooltip. Recomputed from Date.now() on every render.
function fmtCountdown(resetsAt: string | null | undefined): string {
  if (!resetsAt) return "";
  const ms = new Date(resetsAt).getTime() - Date.now();
  if (!(ms > 0)) return "resets now";
  const mins = Math.floor(ms / 60000);
  if (mins < 60) return "resets in " + mins + "m";
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return "resets in " + hrs + "h" + (mins % 60) + "m";
  return "resets in " + Math.floor(hrs / 24) + "d" + (hrs % 24) + "h";
}

// fmtShort — verbatim from usage.js. Compact form for the visible time column
// (no "resets" prefix): 59m→"0h59m" handled by the <60 branch, 23h→"0d23h" by
// the <24 branch, ≤0→"now", missing resets_at → "".
function fmtShort(resetsAt: string | null | undefined): string {
  if (!resetsAt) return "";
  const ms = new Date(resetsAt).getTime() - Date.now();
  if (!(ms > 0)) return "now";
  const mins = Math.floor(ms / 60000);
  if (mins < 60) return mins + "m";
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return hrs + "h" + (mins % 60) + "m";
  return Math.floor(hrs / 24) + "d" + (hrs % 24) + "h";
}

// pad2 — verbatim from usage.js.
function pad2(n: number): string {
  const s = String(n);
  return s.length < 2 ? "0" + s : s;
}

// paceInfo turns the pace into a signed reserve percentage. reserve =
// -delta_percent: how far UNDER the steady-burn line you are. reserve >= 0 is
// headroom (green, "+NN%"); reserve < 0 is over-pace / deficit (red, "-NN%").
// Verbatim from usage.js (null pace → null cell).
function paceInfo(pace: Pace | null | undefined): { cls: string; text: string } | null {
  if (!pace) return null;
  const reserve = Math.round(-pace.delta_percent);
  if (reserve >= 0) return { cls: "reserve", text: "+" + pad2(reserve) + "%" };
  return { cls: "deficit", text: "-" + pad2(-reserve) + "%" };
}

function windowLabel(name: string): string {
  const base = name.startsWith("weekly_") ? name.slice("weekly_".length) : name;
  return base.replace(/_/g, " ");
}

function windowMarker(name: string): string {
  const label = windowLabel(name).trim();
  return label.charAt(0).toUpperCase();
}

// WindowCells emits one window's three grid cells (remaining %, pace reserve,
// reset countdown) — i.e. one line of the panel. A missing window (`w`
// undefined) still emits empty cells so the columns stay aligned across rows.
// Mirrors usage.js appendWindow() byte-for-behavior. `keyBase` only namespaces
// the three React keys; it is NOT rendered.
function WindowCells({
  w,
  keyBase,
  stale,
  marker,
}: {
  w: RateWindow | undefined;
  keyBase: string;
  stale?: boolean;
  marker?: string;
}) {
  const remain = w ? Math.max(0, Math.round(100 - (w.used_percent || 0))) + "%" : "";
  const pace = w ? paceInfo(w.pace) : null;
  const t = w && w.resets_at ? fmtShort(w.resets_at) : "";
  const dim = stale ? " u-stale" : "";
  const title = w ? [marker ? windowLabel(w.name) : "", fmtCountdown(w.resets_at)].filter(Boolean).join(" · ") : "";
  return (
    <>
      <span key={keyBase + ":remain"} className={"u-remain" + dim} title={title}>
        {marker ? <span className="u-window-marker">{marker}</span> : null}
        {remain}
      </span>
      <span key={keyBase + ":pace"} className={"u-pace" + (pace ? " " + pace.cls : "") + dim}>
        {pace ? pace.text : ""}
      </span>
      <span key={keyBase + ":time"} className={"u-time" + dim}>
        {t}
      </span>
    </>
  );
}

// ProviderRows lays one provider into the grid: icon | % | reserve | time. The
// first line is the session window, the second is the all-models weekly window,
// and any model-family weekly windows follow with a compact marker. An errored
// provider spans the value columns (u-err, cols 2-4) with its message instead.
function ProviderRows({ p, idx }: { p: ProviderUsage; idx: number }) {
  const runtime = (p.provider || "").toLowerCase();
  const icon = RUNTIME_ICON[runtime] || runtime.slice(0, 2);
  const hasWindows = !!(p.windows && p.windows.length > 0);
  const stale = !!p.stale;
  let title = p.provider + (p.plan ? " · " + p.plan : "") + (p.account ? " · " + p.account : "");
  // A stale block keeps showing the last-known windows; surface the failure
  // reason on the badge tooltip so the dimming is explainable.
  if (stale && p.error) title += " · stale: " + p.error;
  const iconCls = "agent-icon u-icon runtime-" + runtime + (stale ? " stale" : "");
  const base = "p" + idx;
  // Error block only when there are no windows to fall back to. A stale provider
  // (error present but last-known windows kept) renders its windows dimmed.
  if (p.error && !hasWindows) {
    return (
      <>
        <span key={base + ":icon"} className={iconCls} title={title}>
          {icon}
        </span>
        <span key={base + ":err"} className="u-err" title={p.error}>
          {p.error}
        </span>
      </>
    );
  }
  const byName: Record<string, RateWindow> = {};
  for (const w of p.windows || []) byName[w.name] = w;
  const extraWindows = (p.windows || []).filter((w) => w.name !== "session" && w.name !== "weekly");
  // Dollar-equivalent month-to-date token spend (codeburn-style), tucked under
  // the runtime badge on the weekly line (column 1, left-aligned).
  const spend = p.spend ? fmtUSD(p.spend.month_usd) : "";
  return (
    <>
      <span key={base + ":icon"} className={iconCls} title={title}>
        {icon}
      </span>
      <WindowCells key={base + ":session"} keyBase={base + ":session"} w={byName.session} stale={stale} />
      <span
        key={base + ":spend"}
        className={"u-spend" + (stale ? " u-stale" : "")}
        title="month-to-date dollar-equivalent token spend (API rates)"
      >
        {spend}
      </span>
      <WindowCells key={base + ":weekly"} keyBase={base + ":weekly"} w={byName.weekly} stale={stale} />
      {extraWindows.map((w) => (
        <Fragment key={base + ":" + w.name}>
          <span className="u-window-spacer" aria-hidden="true" />
          <WindowCells keyBase={base + ":" + w.name} w={w} stale={stale} marker={windowMarker(w.name)} />
        </Fragment>
      ))}
    </>
  );
}

export default function UsagePanel() {
  const { snap, disabled } = useUsage();
  const providers: ProviderUsage[] = (snap && snap.providers) || [];
  // usage.js render(): 0 providers → panel.hidden = true. Before any data
  // (snap null) the original lastSnap is null → render(panel, null) → hidden.
  // A 404 disables the panel permanently. Otherwise hidden = false.
  const hidden = disabled || providers.length === 0;
  return (
    <div className="usage-panel" id="usage-panel" hidden={hidden} aria-hidden="true">
      {providers.map((p, i) => (
        <Fragment key={"p" + i}>
          {/* A thin full-width rule between provider blocks (not before the
              first), so each agent's two-row usage/spend block reads as its
              own group. */}
          {i > 0 ? <span className="u-sep" aria-hidden="true" /> : null}
          <ProviderRows p={p} idx={i} />
        </Fragment>
      ))}
    </div>
  );
}

// Re-export the data shape for callers that want to introspect (none currently).
export type { AgentUsage };
