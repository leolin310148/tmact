// Agent quota / rate-limit usage panel (top-right overlay). Mirrors the
// statusline chip look: a per-provider runtime badge (cc / cx) followed by the
// remaining % and leading/lagging pace for each rate window. Data comes from
// /api/agent-usage, which statusd refreshes on a slow (~5m) ticker; the panel
// just re-polls and recomputes the reset countdown locally.
import { $, h } from "./dom.js";

const USAGE_POLL_MS = 60000;
const RUNTIME_ICON = { claude: "cc", codex: "cx", copilot: "cp", gemini: "g" };

function fmtCountdown(resetsAt) {
  if (!resetsAt) return "";
  const ms = new Date(resetsAt).getTime() - Date.now();
  if (!(ms > 0)) return "resets now";
  const mins = Math.floor(ms / 60000);
  if (mins < 60) return "resets in " + mins + "m";
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return "resets in " + hrs + "h" + (mins % 60) + "m";
  return "resets in " + Math.floor(hrs / 24) + "d" + (hrs % 24) + "h";
}

function pad2(n) {
  const s = String(n);
  return s.length < 2 ? "0" + s : s;
}

// paceInfo turns the pace into a signed reserve percentage. reserve =
// -delta_percent: how far UNDER the steady-burn line you are. reserve >= 0 is
// headroom (green, "+NN%"); reserve < 0 is over-pace / deficit (red, "-NN%").
function paceInfo(pace) {
  if (!pace) return null;
  const reserve = Math.round(-pace.delta_percent);
  if (reserve >= 0) return { cls: "reserve", text: "+" + pad2(reserve) + "%" };
  return { cls: "deficit", text: "-" + pad2(-reserve) + "%" };
}

// appendWindow pushes one window's two grid cells (remaining %, pace) onto the
// panel. A missing window still emits empty cells so columns stay aligned.
function appendWindow(panel, w) {
  const remain = w ? Math.max(0, Math.round(100 - (w.used_percent || 0))) + "%" : "";
  panel.appendChild(
    h("span", { class: "u-remain", title: w ? fmtCountdown(w.resets_at) : "" }, document.createTextNode(remain)),
  );
  const pace = w ? paceInfo(w.pace) : null;
  const cell = h("span", { class: "u-pace" + (pace ? " " + pace.cls : "") });
  if (pace) cell.textContent = pace.text;
  panel.appendChild(cell);
}

// render lays providers into the shared CSS grid: one row per provider as
// icon | sess% | sess-pace | / | week% | week-pace, so the columns line up
// across rows. An errored provider spans the value columns with its message.
function render(panel, snap) {
  panel.textContent = "";
  const providers = (snap && snap.providers) || [];
  if (providers.length === 0) {
    panel.hidden = true;
    return;
  }
  for (const p of providers) {
    const runtime = (p.provider || "").toLowerCase();
    const icon = RUNTIME_ICON[runtime] || runtime.slice(0, 2);
    const title = p.provider + (p.plan ? " · " + p.plan : "") + (p.account ? " · " + p.account : "");
    panel.appendChild(h("span", { class: "agent-icon u-icon runtime-" + runtime, title, text: icon }));
    if (p.error) {
      panel.appendChild(h("span", { class: "u-err", title: p.error, text: p.error }));
      continue;
    }
    const byName = {};
    for (const w of p.windows || []) byName[w.name] = w;
    appendWindow(panel, byName.session);
    panel.appendChild(h("span", { class: "u-sep", text: "/" }));
    appendWindow(panel, byName.weekly);
  }
  panel.hidden = false;
}

export function wireUsage() {
  const panel = $("usage-panel");
  if (!panel) return;
  let lastSnap = null;
  async function refresh() {
    try {
      const res = await fetch("/api/agent-usage", { cache: "no-store" });
      if (res.status === 404) {
        // Panel disabled server-side — stop polling.
        panel.hidden = true;
        clearInterval(timer);
        return;
      }
      if (!res.ok) return; // keep last render; transient unavailability
      lastSnap = await res.json();
      render(panel, lastSnap);
    } catch (e) {
      /* keep last render; next tick retries */
    }
  }
  // Re-render on a short cadence too, so reset countdown tooltips stay roughly
  // current without waiting a full poll for fresh data.
  const timer = setInterval(refresh, USAGE_POLL_MS);
  refresh();
}
