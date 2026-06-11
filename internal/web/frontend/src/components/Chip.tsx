// Chip — one statusline pane chip, ported 1:1 from the chip-building loop in
// app.js renderStatusline (MIGRATION_SPEC §6 items 9,10).
//
// Layout matches the original h() tree EXACTLY:
//   <div class="chip[ sel][ stale]" title="...">
//     [<span class="chip-key">KEY</span>]      // only when a hotkey is assigned
//     [<span class="peer-badge">PEER</span>]   // only when peer present
//     [indicator]                              // agent-icon OR dot OR nothing
//     <span class="chip-label">LABEL</span>
//   </div>
//
// title = (peer ? peer + " — " : "") + (cwd || session) + " — " + stateLabel
//         + (key ? " — Option+" + key : "")
//
// Indicator (paneIndicator):
//   - Known runtime → <span class="agent-icon runtime-<name>[ stale|running|asking]"
//       title="<runtime> — <stateLabel>">cc|cx|cp|g</span>
//     (stale wins: when stale, neither running nor asking class is added).
//   - Else, if asking → <span class="dot <stateClass>">?</span>.
//   - Else → null (no indicator node).
//
// Selection-into-view: app.js, after re-rendering the statusline in selectPane,
// did `$("chips").querySelector(".chip.sel").scrollIntoView({block:"nearest",
// inline:"nearest"})`. Here the selected chip scrolls itself into view via a
// layout effect when it becomes selected — same observable behavior, kept inside
// the StatusLine cluster (App no longer needs a chips ref / querySelector).

import { useLayoutEffect, useRef } from "react";
import { onPointerDownNoBlur } from "../lib/dom";
import type { PaneStatus } from "../types/server";
import {
  RUNTIME_ICON,
  paneRuntime,
  panePeer,
  paneStateClass,
  paneStateLabel,
} from "./StatusLine";

interface ChipProps {
  pane: PaneStatus;
  /** Disambiguated session label shown as the chip's main text. */
  label: string;
  /** Option+key hotkey label, or undefined past the hotkey range. */
  hotkey: string | undefined;
  selected: boolean;
  onSelect: () => void;
}

// paneIndicator — verbatim port; returns the indicator element or null.
function PaneIndicator({ pane }: { pane: PaneStatus }) {
  const runtime = paneRuntime(pane);
  const icon = RUNTIME_ICON[runtime];
  if (icon) {
    const cls = ["agent-icon", "runtime-" + runtime];
    if (pane.stale) cls.push("stale");
    else {
      if (pane.running) cls.push("running");
      if (pane.asking) cls.push("asking");
    }
    return (
      <span className={cls.join(" ")} title={runtime + " — " + paneStateLabel(pane)}>
        {icon}
      </span>
    );
  }

  if (!pane.asking) return null;
  const dotCls = paneStateClass(pane);
  return <span className={"dot " + dotCls}>?</span>;
}

export function Chip({ pane, label, hotkey, selected, onSelect }: ChipProps) {
  const ref = useRef<HTMLDivElement | null>(null);

  // Scroll the selected chip into view, mirroring app.js selectPane.
  useLayoutEffect(() => {
    if (selected && ref.current) {
      ref.current.scrollIntoView({ block: "nearest", inline: "nearest" });
    }
  }, [selected]);

  const peer = panePeer(pane);
  const className =
    "chip" + (selected ? " sel" : "") + (pane.stale ? " stale" : "");
  const title =
    (peer ? peer + " — " : "") +
    (pane.cwd || pane.session) +
    " — " +
    paneStateLabel(pane) +
    (hotkey ? " — Option+" + hotkey : "");

  return (
    <div ref={ref} className={className} title={title} onClick={onSelect}>
      {hotkey ? <span className="chip-key">{hotkey}</span> : null}
      {peer ? <span className="peer-badge">{peer}</span> : null}
      <PaneIndicator pane={pane} />
      <span className="chip-label">{label}</span>
    </div>
  );
}
