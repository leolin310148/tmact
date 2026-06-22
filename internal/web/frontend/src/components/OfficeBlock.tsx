import { useEffect, useRef, useState, type CSSProperties, type PointerEvent } from "react";
import { onPointerDownNoBlur } from "../lib/dom";
import type { PaneStatus } from "../types/server";
import smallTableSideUrl from "../assets/pixel-agents/furniture/SMALL_TABLE/SMALL_TABLE_SIDE.png";
import pcSideUrl from "../assets/pixel-agents/furniture/PC/PC_SIDE.png";
import woodenChairSideUrl from "../assets/pixel-agents/furniture/WOODEN_CHAIR/WOODEN_CHAIR_SIDE.png";
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
} from "./StatusLine";

interface OfficeBlockProps {
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

const OFFICE_COLLAPSED_KEY = "tmact.officeCollapsed";
const OFFICE_LABEL_HIDE_MS = 2000;

function runtimeClass(runtime: string): string {
  return runtime.replace(/[^a-z0-9_-]/g, "-");
}

export function OfficeBlock({ panes, selected, onSelect }: OfficeBlockProps) {
  const labelHideTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [labelsVisible, setLabelsVisible] = useState(false);
  const [collapsed, setCollapsed] = useState(() => {
    try {
      return localStorage.getItem(OFFICE_COLLAPSED_KEY) === "1";
    } catch (e) {
      return false;
    }
  });
  const items = paneListItems(panes);
  const selectedItem = items.find(({ pane }) => pane.pane_id === selected) ?? null;
  const selectedPeer = selectedItem ? panePeer(selectedItem.pane) : "";
  const selectedLabel = selectedItem
    ? (selectedPeer ? selectedPeer + " · " : "") + selectedItem.label
    : "No pane selected";
  const selectedTitle = selectedItem
    ? selectedLabel + " — " + paneStateLabel(selectedItem.pane)
    : selectedLabel;
  const officeRows = Math.ceil(items.length / 2);
  const officeStyle = { "--office-floorplan-base-h": 61 + officeRows * 50 + "px" } as CSSProperties;
  const officeClass = [
    "office-block",
    collapsed ? "office-collapsed" : "",
    labelsVisible ? "office-show-labels" : "",
  ]
    .filter(Boolean)
    .join(" ");

  useEffect(() => {
    const clearLabelTimer = () => {
      if (labelHideTimer.current) {
        clearTimeout(labelHideTimer.current);
        labelHideTimer.current = null;
      }
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.altKey || e.key === "Alt") {
        clearLabelTimer();
        setLabelsVisible(true);
      }
    };
    const onKeyUp = (e: KeyboardEvent) => {
      if (e.key === "Alt" || !e.altKey) {
        clearLabelTimer();
        setLabelsVisible(false);
      }
    };
    const onBlur = () => {
      clearLabelTimer();
      setLabelsVisible(false);
    };

    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("keyup", onKeyUp);
    window.addEventListener("blur", onBlur);
    return () => {
      clearLabelTimer();
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("keyup", onKeyUp);
      window.removeEventListener("blur", onBlur);
    };
  }, []);

  const isMobileOfficePointer = (e: PointerEvent) => {
    if (e.pointerType !== "mouse") return true;
    if (typeof window.matchMedia !== "function") return false;
    return window.matchMedia("(max-width: 760px)").matches;
  };

  const showMobileLabels = (e: PointerEvent) => {
    if (!isMobileOfficePointer(e)) return;
    if (labelHideTimer.current) {
      clearTimeout(labelHideTimer.current);
      labelHideTimer.current = null;
    }
    setLabelsVisible(true);
  };

  const scheduleMobileLabelHide = (e: PointerEvent) => {
    if (!isMobileOfficePointer(e)) return;
    if (labelHideTimer.current) clearTimeout(labelHideTimer.current);
    labelHideTimer.current = setTimeout(() => {
      labelHideTimer.current = null;
      setLabelsVisible(false);
    }, OFFICE_LABEL_HIDE_MS);
  };

  const toggleCollapsed = () => {
    setCollapsed((current) => {
      const next = !current;
      try {
        localStorage.setItem(OFFICE_COLLAPSED_KEY, next ? "1" : "0");
      } catch (e) {
        /* ignore */
      }
      return next;
    });
  };

  return (
    <aside className={officeClass} style={officeStyle} aria-label="Office block pane switcher">
      <div
        className="office-floorplan"
        onPointerDownCapture={showMobileLabels}
        onPointerUpCapture={scheduleMobileLabelHide}
        onPointerCancel={scheduleMobileLabelHide}
        onPointerLeave={scheduleMobileLabelHide}
      >
        <div className="office-floorplan-scale">
          <div className="office-rail" aria-hidden="true" />
          <div className="office-wall-glow" aria-hidden="true" />
          <div className="office-floor-base" aria-hidden="true" />
          <div className="office-shared-walls" aria-hidden="true">
            <span className="office-left-decor-floor" />
            <span className="office-right-decor-floor" />
            <span className="office-top-decor-floor" />
            <span className="office-top-wall-face" />
            <span className="office-top-wall-edge" />
          </div>
          {items.length === 0 ? (
            <div className="office-empty">No panes</div>
          ) : (
            <div className="office-seats">
              {items.map(({ pane, label }, i) => {
                const paneID = pane.pane_id ?? "";
                const runtime = paneRuntime(pane);
                const hasAgent = !!RUNTIME_ICON[runtime];
                const stateClass = paneStateClass(pane);
                const peer = panePeer(pane);
                const seatClass = [
                  "office-seat",
                  i % 2 === 0 ? "office-seat-left" : "office-seat-right",
                  hasAgent ? "occupied" : "empty-seat",
                  "state-" + stateClass,
                  selected === paneID ? "selected" : "",
                  pane.asking ? "asking" : "",
                  pane.stale ? "stale" : "",
                  runtime ? "runtime-" + runtimeClass(runtime) : "",
                ]
                  .filter(Boolean)
                  .join(" ");
                const title =
                  (peer ? peer + " — " : "") +
                  label +
                  " — " +
                  (runtime || "empty") +
                  " — " +
                  paneStateLabel(pane);

                return (
                  <button
                    key={paneID || "office-pane-" + i}
                    className={seatClass}
                    type="button"
                    title={title}
                    aria-label={"Select pane " + label + ", " + paneStateLabel(pane)}
                    aria-pressed={selected === paneID}
                    onPointerDown={onPointerDownNoBlur}
                    onClick={() => onSelect(paneID)}
                  >
                    <span className="office-scene" aria-hidden="true">
                      <span className="office-wall" />
                      <span className="office-work-area">
                        <span className="office-floor" />
                        <span className="office-partition-shadow" />
                        <span className="office-small-table-side">
                          <img src={smallTableSideUrl} alt="" draggable={false} />
                        </span>
                        <span className="office-pc-side">
                          <img src={pcSideUrl} alt="" draggable={false} />
                        </span>
                        <span className="office-wooden-chair-side">
                          <img src={woodenChairSideUrl} alt="" draggable={false} />
                        </span>
                        {hasAgent ? (
                          <span
                            className="office-seated-agent"
                            style={{
                              backgroundImage: `url(${characterUrls[i % characterUrls.length]})`,
                            }}
                          />
                        ) : null}
                      </span>
                    </span>
                    <span className="office-name-tag" aria-hidden="true">
                      {label}
                    </span>
                  </button>
                );
              })}
              {items.length % 2 === 1 ? (
                <span
                  className="office-seat office-seat-filler office-seat-right"
                  aria-hidden="true"
                >
                  <span className="office-scene">
                    <span className="office-work-area">
                      <span className="office-floor" />
                    </span>
                  </span>
                </span>
              ) : null}
            </div>
          )}
        </div>
      </div>
      <div
        className={
          "office-selected-overlay" + (selectedItem ? "" : " office-selected-overlay-empty")
        }
        aria-live="polite"
        title={selectedTitle}
      >
        {selectedLabel}
      </div>
      <button
        className="office-collapse-toggle"
        type="button"
        aria-label={collapsed ? "expand office layout" : "collapse office layout"}
        aria-expanded={!collapsed}
        onPointerDown={onPointerDownNoBlur}
        onClick={toggleCollapsed}
      >
        <span aria-hidden="true">{collapsed ? "›" : "‹"}</span>
      </button>
    </aside>
  );
}
