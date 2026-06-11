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

function runtimeClass(runtime: string): string {
  return runtime.replace(/[^a-z0-9_-]/g, "-");
}

export function OfficeBlock({ panes, selected, onSelect }: OfficeBlockProps) {
  const items = paneListItems(panes);

  return (
    <aside className="office-block" aria-label="Office block pane switcher">
      <div className="office-rail" aria-hidden="true" />
      <div className="office-wall-glow" aria-hidden="true" />
      <div className="office-shared-walls" aria-hidden="true">
        <span className="office-left-decor-floor" />
        <span className="office-top-decor-floor" />
        <span className="office-top-wall-face" />
        <span className="office-top-wall-edge" />
        <span className="office-left-wall-face" />
        <span className="office-left-wall-edge" />
        <span className="office-left-wall-cap" />
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
                        style={{ backgroundImage: `url(${characterUrls[i % characterUrls.length]})` }}
                      />
                    ) : null}
                  </span>
                </span>
              </button>
            );
          })}
        </div>
      )}
    </aside>
  );
}
