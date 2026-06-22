// OfficeDesks — redesigned office-block pane switcher (prototype).
//
// Now that the statusline only keeps the "pinned" panes visible (agents, the
// selection, panes asking for input — see StatusLine.splitPaneItems), the
// office renders the same set as a row of desks — one desk per visible pane.
// The rest collapse behind the leading floor lamp: clicking it opens a popover
// list, mirroring the statusline's "more" chip.
//
// Art (current pass): standing_desk only — characters and monitors were removed
// so we can re-stage the office from scratch.

import {
  Fragment,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent,
} from "react";
import { createPortal } from "react-dom";
import { onPointerDownNoBlur } from "../lib/dom";
import type { PaneStatus } from "../types/server";
import "./OfficeDesks.css";
import floorLampUrl from "../assets/pixel-agents/decor/floor_lamp.png";
import pendantUrl from "../assets/pixel-agents/decor/pendant_light.png";
import workDeskUrl from "../assets/pixel-agents/furniture/DESK/work_desk_thin_legs.png";
import chairBackUrl from "../assets/pixel-agents/furniture/CHAIR/aeron_chair_back.png";
import computerUrl from "../assets/pixel-agents/furniture/PC/macbook_setup.png";
import character0Url from "../assets/pixel-agents/characters/char_0.png";
import character1Url from "../assets/pixel-agents/characters/char_1.png";
import character2Url from "../assets/pixel-agents/characters/char_2.png";
import character3Url from "../assets/pixel-agents/characters/char_3.png";
import character4Url from "../assets/pixel-agents/characters/char_4.png";
import character5Url from "../assets/pixel-agents/characters/char_5.png";
import {
  RUNTIME_ICON,
  paneListItems,
  panePeer,
  paneRuntime,
  paneStateClass,
  paneStateLabel,
  splitPaneItems,
  type PaneListItem,
} from "./StatusLine";

interface OfficeDesksProps {
  panes: PaneStatus[];
  selected: string | null;
  onSelect: (paneID: string) => void;
}

// The office wall window has three time-of-day looks. The clock picks one
// (06:00–17:00 day · 17:00–18:30 sunset · otherwise night); the window easter
// egg cycles through them in this order.
type SceneMode = "day" | "sunset" | "night";
const SCENE_MODES: SceneMode[] = ["day", "sunset", "night"];

function clockSceneMode(now: Date): SceneMode {
  const mins = now.getHours() * 60 + now.getMinutes();
  if (mins >= 17 * 60 && mins < 18 * 60 + 30) return "sunset";
  if (mins >= 6 * 60 && mins < 17 * 60) return "day";
  return "night";
}

const characterUrls = [
  character0Url,
  character1Url,
  character2Url,
  character3Url,
  character4Url,
  character5Url,
];

// A stable character per pane so a given session keeps its avatar even as the
// visible set changes (don't key off the array position).
function characterFor(pane: PaneStatus): string {
  const key = (pane.pane_id ?? pane.target ?? "") + (pane.session ?? "");
  let hash = 0;
  for (let i = 0; i < key.length; i++) hash = (hash * 31 + key.charCodeAt(i)) | 0;
  return characterUrls[Math.abs(hash) % characterUrls.length]!;
}

function Desk({
  item,
  selected,
  onSelect,
}: {
  item: PaneListItem;
  selected: boolean;
  onSelect: (paneID: string) => void;
}) {
  const { pane, label } = item;
  const paneID = pane.pane_id ?? "";
  const runtime = paneRuntime(pane);
  const peer = panePeer(pane);
  const cls = [
    "desk",
    "state-" + paneStateClass(pane),
    selected ? "selected" : "",
    pane.asking ? "asking" : "",
    pane.stale ? "stale" : "",
  ]
    .filter(Boolean)
    .join(" ");
  const title =
    (peer ? peer + " — " : "") + label + " — " + (runtime || "idle") + " — " + paneStateLabel(pane);

  // Left monitor mirrors the statusline chip's runtime badge (cc/cx/cp/g) and
  // its running effect: it reuses the .agent-icon + runtime-* + running/asking
  // classes so the shared data-running-effect animations apply unchanged.
  const icon = RUNTIME_ICON[runtime];
  // The screen shows only the runtime badge + running effect. Unlike the chip,
  // the asking state is NOT styled onto the screen (no amber, no on-screen "?")
  // — it surfaces as a separate "?" over the character's head (.desk-ask) so it
  // never overflows the monitor. The chip's .agent-icon.asking is left untouched.
  const screenCls = ["desk-screen"];
  if (icon) {
    screenCls.push("agent-icon", "runtime-" + runtime);
    if (pane.stale) screenCls.push("stale");
    else if (pane.running) screenCls.push("running");
  }
  const showAsk = pane.asking && !pane.stale;

  return (
    <button
      className={cls}
      type="button"
      title={title}
      aria-label={"Select pane " + label + ", " + paneStateLabel(pane)}
      aria-pressed={selected}
      onPointerDown={onPointerDownNoBlur}
      onClick={() => onSelect(paneID)}
    >
      <span className="desk-stage" aria-hidden="true">
        <img className="desk-top" src={workDeskUrl} alt="" draggable={false} />
        <img className="desk-computer" src={computerUrl} alt="" draggable={false} />
        <span className={screenCls.join(" ")}>{icon ?? ""}</span>
        <span
          className="desk-person"
          style={{ backgroundImage: `url(${characterFor(pane)})` }}
        />
        <img className="desk-chair" src={chairBackUrl} alt="" draggable={false} />
        {showAsk ? (
          <span className="desk-ask" aria-hidden="true">
            ?
          </span>
        ) : null}
      </span>
      <span className="desk-name">{label}</span>
    </button>
  );
}

// LampMore — the leading floor lamp at the far left doubles as the overflow
// trigger. Clicking it opens the same "more" popover the old far-right "+N"
// door used to (the door art is gone, and so is the +N count badge). When there
// is no overflow the caller renders a plain decorative lamp instead.
function LampMore({
  items,
  onSelect,
}: {
  items: PaneListItem[];
  onSelect: (paneID: string) => void;
}) {
  const [open, setOpen] = useState(false);
  // The popover is portaled to <body> with fixed coords because the office bar
  // (.office-desks-floor) is an overflow scroll container that would otherwise
  // clip a popover popping up out of it. Anchored above the lamp button.
  const [pos, setPos] = useState<{ left: number; bottom: number } | null>(null);
  const btnRef = useRef<HTMLButtonElement | null>(null);
  const popRef = useRef<HTMLDivElement | null>(null);

  useLayoutEffect(() => {
    if (!open || !btnRef.current) return;
    const r = btnRef.current.getBoundingClientRect();
    // Left-anchor to the lamp (it lives at the far left, so centering would push
    // the popover off-screen), clamped to an 8px gutter.
    setPos({ left: Math.max(8, r.left), bottom: window.innerHeight - r.top + 8 });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onDocPointerDown = (e: Event) => {
      const t = e.target as Node;
      const inBtn = btnRef.current?.contains(t);
      const inPop = popRef.current?.contains(t);
      if (!inBtn && !inPop) setOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("pointerdown", onDocPointerDown, true);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onDocPointerDown, true);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  useEffect(() => {
    if (items.length === 0) setOpen(false);
  }, [items.length]);

  const moreLabel = items.length + " more pane" + (items.length === 1 ? "" : "s");
  return (
    <>
      <button
        ref={btnRef}
        type="button"
        className={"office-lamp office-lamp--more" + (open ? " open" : "")}
        title={moreLabel}
        aria-label={"Show " + moreLabel}
        aria-haspopup="menu"
        aria-expanded={open}
        onPointerDown={onPointerDownNoBlur}
        onClick={() => setOpen((v) => !v)}
      >
        <img className="office-lamp-img" src={floorLampUrl} alt="" draggable={false} />
      </button>
      {open && pos
        ? createPortal(
            <div
              ref={popRef}
              className="desk-more-pop"
              role="menu"
              style={{ position: "fixed", left: pos.left, bottom: pos.bottom, transform: "none" }}
            >
              {items.map(({ pane, label }, i) => {
                const runtime = paneRuntime(pane);
                return (
                  <button
                    key={pane.pane_id || "overflow-" + i}
                    type="button"
                    className={"desk-more-row state-" + paneStateClass(pane)}
                    role="menuitem"
                    onPointerDown={onPointerDownNoBlur}
                    onClick={() => {
                      onSelect(pane.pane_id ?? "");
                      setOpen(false);
                    }}
                  >
                    <span className="desk-more-dot" aria-hidden="true" />
                    <span className="desk-more-label">{label}</span>
                    {RUNTIME_ICON[runtime] ? (
                      <span className="desk-more-rt">{RUNTIME_ICON[runtime]}</span>
                    ) : null}
                  </button>
                );
              })}
            </div>,
            document.body,
          )
        : null}
    </>
  );
}

export function OfficeDesks({ panes, selected, onSelect }: OfficeDesksProps) {
  const labelHideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [labelsVisible, setLabelsVisible] = useState(false);
  // Easter egg: clicking the floor-to-ceiling window forces a scene mode, an
  // in-memory override of the clock (null = follow the clock). Not persisted —
  // it resets on reload.
  const [modeOverride, setModeOverride] = useState<SceneMode | null>(null);

  const items = paneListItems(panes);
  const { visible, overflow } = splitPaneItems(items, selected);
  // Chunk the visible desks into groups of three so one ceiling pendant hangs
  // centered over each group (ceil(n/3) pendants — the middle desk of each).
  const deskGroups: PaneListItem[][] = [];
  for (let i = 0; i < visible.length; i += 3) deskGroups.push(visible.slice(i, i + 3));

  // The wall window tracks the time of day (day swaps in the sunlit art and
  // drops the lamp glow; sunset/night keep the lamps lit). Recomputed each
  // render (the office re-renders on every pane poll, so it flips across the
  // boundaries on its own) unless the window easter egg has pinned a mode.
  const mode = modeOverride ?? clockSceneMode(new Date());
  const rootClass = ["office-desks", `is-${mode}`, labelsVisible ? "show-labels" : ""]
    .filter(Boolean)
    .join(" ");
  // Expose the count so CSS can keep desks centered / sized for small sets.
  const rootStyle = { "--desk-count": visible.length } as CSSProperties;

  const isMobilePointer = (e: PointerEvent) => {
    if (e.pointerType !== "mouse") return true;
    if (typeof window.matchMedia !== "function") return false;
    return window.matchMedia("(max-width: 760px)").matches;
  };
  const showMobileLabels = (e: PointerEvent) => {
    if (!isMobilePointer(e)) return;
    if (labelHideTimer.current) clearTimeout(labelHideTimer.current);
    setLabelsVisible(true);
  };
  const scheduleHide = (e: PointerEvent) => {
    if (!isMobilePointer(e)) return;
    if (labelHideTimer.current) clearTimeout(labelHideTimer.current);
    labelHideTimer.current = setTimeout(() => setLabelsVisible(false), 2000);
  };

  useEffect(() => () => {
    if (labelHideTimer.current) clearTimeout(labelHideTimer.current);
  }, []);

  return (
    <aside className={rootClass} style={rootStyle} aria-label="Office desks pane switcher">
      {/* Static wall decor (sideboard + appliances + bookcase), pinned against
          the back wall behind the desks. CSS-driven backgrounds; non-interactive. */}
      <div className="office-decor" aria-hidden="true">
        <span className="decor-sideboard" />
        <span className="decor-appliances" />
        <span className="decor-bookcase" />
      </div>
      {/* Easter egg: a transparent hotspot over the floor-to-ceiling window that
          cycles the office through day → sunset → night (sits above the wall but
          below the desks, so desk selection is unaffected). */}
      <button
        type="button"
        className="office-window-toggle"
        aria-label="Cycle office lighting (day / sunset / night)"
        title="Cycle day / sunset / night"
        onPointerDown={onPointerDownNoBlur}
        onClick={() =>
          setModeOverride(SCENE_MODES[(SCENE_MODES.indexOf(mode) + 1) % SCENE_MODES.length]!)
        }
      />
      <div
        className="office-desks-floor"
        onPointerDownCapture={showMobileLabels}
        onPointerUpCapture={scheduleHide}
        onPointerCancel={scheduleHide}
        onPointerLeave={scheduleHide}
      >
        {visible.length === 0 && overflow.length === 0 ? (
          <div className="office-desks-empty">No panes</div>
        ) : (
          <div className="desk-row">
            {/* The leading floor lamp at the far left doubles as the overflow
                trigger: when panes spill past the visible desks, clicking it
                opens the "+N" menu (replacing the old far-right door). With no
                overflow it stays plain decor. It lives outside the group loop so
                it still appears (as the lone entry point) when nothing is pinned
                and everything sits in overflow. */}
            {overflow.length > 0 ? (
              <LampMore items={overflow} onSelect={onSelect} />
            ) : (
              <span className="office-lamp" aria-hidden="true">
                <img className="office-lamp-img" src={floorLampUrl} alt="" draggable={false} />
              </span>
            )}
            {deskGroups.map((group, gi) => {
              const startIdx = gi * 3; // global index of this group's first desk
              // A floor lamp also stands every six desks (groups are size three,
              // so every second group starts on a multiple of six). The leading
              // lamp above already covers the first slot, so the loop only emits
              // the later ones — zero-width overlays (office-lamp--overlay)
              // centered on the desk boundary so they don't widen the gap between
              // the two desks they stand between.
              const lampOverlay = startIdx !== 0 && startIdx % 6 === 0;
              return (
                <Fragment key={gi}>
                  {lampOverlay ? (
                    <span className="office-lamp office-lamp--overlay" aria-hidden="true">
                      <img className="office-lamp-img" src={floorLampUrl} alt="" draggable={false} />
                    </span>
                  ) : null}
                  <div className="desk-group">
                    {/* One ceiling pendant centered over each group of three
                        desks (a partial trailing group still gets its own), its
                        top edge meeting the wall top and a warm glow below. For a
                        partial trailing group the pendant is nudged right by
                        (3 - count)/2 desk-widths so it still lands on the middle
                        slot of a notional full group (e.g. a lone 7th desk keeps
                        the pendant at the 8th-desk position) rather than drifting
                        off-center over the few desks present. */}
                    <span
                      className="office-pendant"
                      aria-hidden="true"
                      style={{ "--pendant-shift": (3 - group.length) / 2 } as CSSProperties}
                    >
                      <span className="office-pendant-glow" />
                      <img
                        className="office-pendant-img"
                        src={pendantUrl}
                        alt=""
                        draggable={false}
                      />
                    </span>
                    {group.map((item) => (
                      <Desk
                        key={item.pane.pane_id || item.pane.target}
                        item={item}
                        selected={(item.pane.pane_id ?? "") === selected}
                        onSelect={onSelect}
                      />
                    ))}
                  </div>
                </Fragment>
              );
            })}
          </div>
        )}
      </div>
    </aside>
  );
}
