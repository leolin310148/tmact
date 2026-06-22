// OfficeDesks — redesigned office-block pane switcher (prototype).
//
// Now that the statusline only keeps the "pinned" panes visible (agents, the
// selection, panes asking for input — see StatusLine.splitPaneItems), the
// office renders the same set as a row of desks — one desk per visible pane.
// The rest collapse into a "+N" door that opens a popover list, mirroring the
// statusline's "more" chip.
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

function MoreDoor({
  items,
  onSelect,
}: {
  items: PaneListItem[];
  onSelect: (paneID: string) => void;
}) {
  const [open, setOpen] = useState(false);
  // The popover is portaled to <body> with fixed coords because the office bar
  // (.office-desks-floor) is an overflow scroll container that would otherwise
  // clip a popover popping up out of it. Anchored above the door button.
  const [pos, setPos] = useState<{ left: number; bottom: number } | null>(null);
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const btnRef = useRef<HTMLButtonElement | null>(null);
  const popRef = useRef<HTMLDivElement | null>(null);

  useLayoutEffect(() => {
    if (!open || !btnRef.current) return;
    const r = btnRef.current.getBoundingClientRect();
    setPos({ left: r.left + r.width / 2, bottom: window.innerHeight - r.top + 8 });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onDocPointerDown = (e: Event) => {
      const t = e.target as Node;
      const inWrap = wrapRef.current?.contains(t);
      const inPop = popRef.current?.contains(t);
      if (!inWrap && !inPop) setOpen(false);
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

  return (
    <div className="desk-more-wrap" ref={wrapRef}>
      <button
        ref={btnRef}
        type="button"
        className={"desk-more" + (open ? " open" : "")}
        title={items.length + " more pane" + (items.length === 1 ? "" : "s")}
        aria-haspopup="menu"
        aria-expanded={open}
        onPointerDown={onPointerDownNoBlur}
        onClick={() => setOpen((v) => !v)}
      >
        <span className="desk-more-icon" aria-hidden="true" />
        <span className="desk-more-count">+{items.length}</span>
      </button>
      {open && pos
        ? createPortal(
            <div
              ref={popRef}
              className="desk-more-pop"
              role="menu"
              style={{ position: "fixed", left: pos.left, bottom: pos.bottom }}
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
    </div>
  );
}

export function OfficeDesks({ panes, selected, onSelect }: OfficeDesksProps) {
  const labelHideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [labelsVisible, setLabelsVisible] = useState(false);

  const items = paneListItems(panes);
  const { visible, overflow } = splitPaneItems(items, selected);
  // Chunk the visible desks into groups of three so one ceiling pendant hangs
  // centered over each group (ceil(n/3) pendants — the middle desk of each).
  const deskGroups: PaneListItem[][] = [];
  for (let i = 0; i < visible.length; i += 3) deskGroups.push(visible.slice(i, i + 3));

  const rootClass = ["office-desks", labelsVisible ? "show-labels" : ""].filter(Boolean).join(" ");
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
            {deskGroups.map((group, gi) => {
              const startIdx = gi * 3; // global index of this group's first desk
              // Only the very first floor lamp keeps its own slot at the far
              // left, in front of the desks. Rendered before the group so it
              // doesn't bias the pendant's centering over the group's desks.
              const firstLamp = startIdx === 0;
              return (
                <Fragment key={gi}>
                  {firstLamp ? (
                    <span className="office-lamp" aria-hidden="true">
                      <img className="office-lamp-img" src={floorLampUrl} alt="" draggable={false} />
                    </span>
                  ) : null}
                  <div className="desk-group">
                    {/* One ceiling pendant centered over each group of three
                        desks (a partial trailing group still gets its own), its
                        top edge meeting the wall top and a warm glow below. */}
                    <span className="office-pendant" aria-hidden="true">
                      <span className="office-pendant-glow" />
                      <img
                        className="office-pendant-img"
                        src={pendantUrl}
                        alt=""
                        draggable={false}
                      />
                    </span>
                    {group.map((item, j) => {
                      const i = startIdx + j;
                      // Subsequent floor lamps (every 6 desks) sit BEHIND the
                      // desks like the sideboard/bookcase — a zero-width marker
                      // that takes no row space and is covered by the desks.
                      const bgLamp = i % 6 === 0 && i !== 0;
                      return (
                        <Fragment key={item.pane.pane_id || item.pane.target}>
                          {bgLamp ? (
                            <span className="office-lamp-bg" aria-hidden="true">
                              <img
                                className="office-lamp-img"
                                src={floorLampUrl}
                                alt=""
                                draggable={false}
                              />
                            </span>
                          ) : null}
                          <Desk
                            item={item}
                            selected={(item.pane.pane_id ?? "") === selected}
                            onSelect={onSelect}
                          />
                        </Fragment>
                      );
                    })}
                  </div>
                </Fragment>
              );
            })}
          </div>
        )}
      </div>
      {/* Overflow "+N" door is parked at the far right of the wall (outside the
          scrolling desk floor) for now — to be redone with proper art later. */}
      {overflow.length > 0 ? <MoreDoor items={overflow} onSelect={onSelect} /> : null}
    </aside>
  );
}
